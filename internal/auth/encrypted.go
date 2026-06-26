package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Encrypted is a CredentialStore backed by a single file encrypted at rest with
// AES-256-GCM, the key derived from a passphrase via argon2id. It needs no
// daemon and works identically on desktops, servers, and CI — the portable
// option. The passphrase is supplied lazily so callers can prompt or read it
// from the environment only when access is actually required.
type Encrypted struct {
	path string
	pass func() ([]byte, error)
	mu   sync.Mutex
}

// argon2id parameters. Tuned for an interactive secret unlocked occasionally;
// stored in the file envelope so they can change without breaking old files.
type kdfParams struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"` // KiB
	Threads uint8  `json:"threads"`
}

var defaultKDF = kdfParams{Time: 3, Memory: 64 * 1024, Threads: 4}

const (
	saltLen = 16
	keyLen  = 32 // AES-256
)

// envelope is the on-disk JSON wrapper around the encrypted credential map.
type envelope struct {
	Version int       `json:"v"`
	KDF     string    `json:"kdf"`
	Params  kdfParams `json:"params"`
	Salt    []byte    `json:"salt"`
	Nonce   []byte    `json:"nonce"`
	Cipher  []byte    `json:"ct"`
}

// NewEncrypted returns an encrypted store at path. pass is called when the file
// must be read or written; it should return the passphrase bytes.
func NewEncrypted(path string, pass func() ([]byte, error)) *Encrypted {
	return &Encrypted{path: path, pass: pass}
}

// PassphraseFromEnv reads the passphrase from EMAIL_CRED_PASSPHRASE, falling
// back to the contents of the file named by EMAIL_CRED_KEYFILE. It errors if
// neither is set, so secrets are never written under an empty passphrase.
func PassphraseFromEnv() func() ([]byte, error) {
	return func() ([]byte, error) {
		if p := os.Getenv("EMAIL_CRED_PASSPHRASE"); p != "" {
			return []byte(p), nil
		}
		if kf := os.Getenv("EMAIL_CRED_KEYFILE"); kf != "" {
			b, err := os.ReadFile(kf)
			if err != nil {
				return nil, fmt.Errorf("read keyfile: %w", err)
			}
			return b, nil
		}
		return nil, errors.New("no passphrase: set EMAIL_CRED_PASSPHRASE or EMAIL_CRED_KEYFILE")
	}
}

func deriveKey(pass, salt []byte, p kdfParams) []byte {
	return argon2.IDKey(pass, salt, p.Time, p.Memory, p.Threads, keyLen)
}

func (e *Encrypted) load() (map[string]Credential, error) {
	b, err := os.ReadFile(e.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Credential{}, nil
	}
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("parse credential file: %w", err)
	}
	if env.Version != 1 || env.KDF != "argon2id" {
		return nil, fmt.Errorf("unsupported credential file (v=%d kdf=%s)", env.Version, env.KDF)
	}

	pass, err := e.pass()
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(deriveKey(pass, env.Salt, env.Params))
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, env.Nonce, env.Cipher, nil)
	if err != nil {
		return nil, errors.New("decrypt failed: wrong passphrase or corrupt file")
	}
	m := map[string]Credential{}
	if err := json.Unmarshal(plain, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (e *Encrypted) save(m map[string]Credential) error {
	pass, err := e.pass()
	if err != nil {
		return err
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	gcm, err := newGCM(deriveKey(pass, salt, defaultKDF))
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	plain, err := json.Marshal(m)
	if err != nil {
		return err
	}
	env := envelope{
		Version: 1,
		KDF:     "argon2id",
		Params:  defaultKDF,
		Salt:    salt,
		Nonce:   nonce,
		Cipher:  gcm.Seal(nil, nonce, plain, nil),
	}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(e.path), 0o700); err != nil {
		return err
	}
	tmp := e.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, e.path)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (e *Encrypted) Get(_ context.Context, account string) (Credential, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.load()
	if err != nil {
		return Credential{}, err
	}
	c, ok := m[account]
	if !ok {
		return Credential{}, ErrNotFound
	}
	return c, nil
}

func (e *Encrypted) Set(_ context.Context, account string, c Credential) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.load()
	if err != nil {
		return err
	}
	m[account] = c
	return e.save(m)
}

func (e *Encrypted) Delete(_ context.Context, account string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, err := e.load()
	if err != nil {
		return err
	}
	if _, ok := m[account]; !ok {
		return ErrNotFound
	}
	delete(m, account)
	return e.save(m)
}
