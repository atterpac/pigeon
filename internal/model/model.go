// Package model holds the core domain types shared across the SDK. These are
// provider-agnostic so additional backends can map onto them; today the only
// backend is IMAP/SMTP.
package model

import "time"

// AccountID identifies a configured account in the local store.
type AccountID string

// MessageID is the SDK-internal stable id for a message (typically the
// RFC 5322 Message-ID, falling back to a synthesized id).
type MessageID string

// ThreadID groups messages into a conversation.
type ThreadID string

// LabelID identifies a mailbox/folder mapped to a label.
type LabelID string

// Kind distinguishes the backend used by an account. IMAP/SMTP is the only
// backend; the type is retained for the stored account schema and future use.
type Kind int

const (
	KindIMAP Kind = iota
)

func (k Kind) String() string {
	switch k {
	case KindIMAP:
		return "imap"
	default:
		return "unknown"
	}
}

// Account is a configured mailbox login.
type Account struct {
	ID    AccountID
	Kind  Kind
	Email string
	Name  string
	// Connection details for custom IMAP/SMTP accounts. Empty for well-known
	// domains (e.g. gmail.com), which are resolved from a built-in endpoint map.
	// Credentials live in auth.CredentialStore, never here.
	IMAPHost string
	IMAPPort int
	SMTPHost string
	SMTPPort int
}

// Mailbox is a normalized IMAP folder (Gmail labels appear as folders over IMAP).
type Mailbox struct {
	ID      LabelID
	Account AccountID
	Name    string // display name
	Path    string // IMAP folder path
	Role    Role   // semantic role if known (Inbox, Sent, ...)
	Unread  int
	Total   int
	// User-assigned folder icon (local presentation metadata; not from the
	// provider). Icon is a registry id, Weight a Phosphor weight, Color a theme
	// token. Empty means "use the default icon for this folder".
	Icon       string
	IconWeight string
	IconColor  string
}

// Role tags well-known mailboxes so the UI can find them across providers.
type Role int

const (
	RoleNone Role = iota
	RoleInbox
	RoleSent
	RoleDrafts
	RoleTrash
	RoleSpam
	RoleArchive
)

func (r Role) String() string {
	switch r {
	case RoleInbox:
		return "inbox"
	case RoleSent:
		return "sent"
	case RoleDrafts:
		return "drafts"
	case RoleTrash:
		return "trash"
	case RoleSpam:
		return "spam"
	case RoleArchive:
		return "archive"
	default:
		return "none"
	}
}

// Address is a parsed RFC 5322 address.
type Address struct {
	Name string
	Addr string
}

// Contact is an address-book entry harvested from message envelopes: an address
// plus the most-recent display name seen for it, with frequency/recency used to
// rank recipient autocomplete.
type Contact struct {
	Name     string
	Addr     string
	LastSeen time.Time
	Freq     int
}

// Flag is a per-message state bit (seen, flagged, answered, draft, ...).
type Flag string

const (
	FlagSeen     Flag = "\\Seen"
	FlagFlagged  Flag = "\\Flagged"
	FlagAnswered Flag = "\\Answered"
	FlagDraft    Flag = "\\Draft"
	FlagDeleted  Flag = "\\Deleted"
)

// Category is the app-level inbox grouping used for Gmail-like sections.
type Category string

const (
	CategoryPrimary    Category = "primary"
	CategoryPromotions Category = "promotions"
	CategoryUpdates    Category = "updates"
	CategorySocial     Category = "social"
	CategoryForums     Category = "forums"
)

// Message is the envelope + metadata. Body parts are loaded lazily.
type Message struct {
	ID             MessageID
	Thread         ThreadID
	Account        AccountID
	Subject        string
	From           []Address
	To             []Address
	Cc             []Address
	Bcc            []Address
	Date           time.Time
	Snippet        string
	Category       Category
	Flags          []Flag
	Labels         []LabelID
	HasAttachments bool
	// RFCMessageID is the RFC 5322 Message-ID header; References is the chain.
	// Used to construct correct reply/forward threading.
	RFCMessageID string
	References   []string
	// BodyLoaded reports whether Parts are populated in the store.
	BodyLoaded   bool
	BodyCachedAt time.Time
	LastOpenedAt time.Time
	Parts        []Part
}

// Part is a single MIME part of a message body.
type Part struct {
	ContentType string
	Charset     string
	Disposition string // inline | attachment
	Filename    string
	// ContentID is the bare Content-ID (no angle brackets) for inline parts
	// referenced from HTML via cid: URLs; empty for ordinary parts.
	ContentID string
	Size      int64
	// Content is populated only when loaded; large attachments are spooled to
	// the blob store and referenced by BlobRef instead.
	Content []byte
	BlobRef string
}

// Attachment is part metadata surfaced for listing without loading bytes.
type Attachment struct {
	Filename    string
	ContentType string
	Size        int64
	BlobRef     string
}

// RawMessage is an RFC 5322 byte blob (incoming fetch or outgoing send).
type RawMessage struct {
	ID    MessageID
	Bytes []byte
}

// ThreadListItem is a denormalized conversation-list row, purpose-built for a
// UI inbox view: everything needed to render one row without loading messages.
type ThreadListItem struct {
	ID             ThreadID
	Account        AccountID
	Subject        string
	Last           time.Time
	Unread         bool
	Count          int       // messages in the thread
	LatestSender   Address   // From of the newest message
	Participants   []Address // distinct senders across the thread, oldest first
	Snippet        string    // newest message's snippet
	Category       Category  // newest message category
	HasAttachments bool
	Labels         []LabelID // union of labels across the thread
}

// Outgoing is a message to be composed and sent.
type Outgoing struct {
	From    Address
	To      []Address
	Cc      []Address
	Bcc     []Address
	Subject string
	Text    string // text/plain body
	HTML    string // optional text/html alternative

	// Threading: when replying, set InReplyTo to the parent's Message-ID and
	// References to the full chain so clients thread the reply correctly.
	InReplyTo  string
	References []string
	// Thread, if set, attaches the message to an existing provider thread.
	Thread ThreadID

	Attachments []Outfile
}

// Draft is a locally-saved, autosaved compose draft.
type Draft struct {
	ID      string
	Account AccountID
	Message Outgoing
	Updated time.Time
}

// Snoozed records a message hidden from the inbox until Until.
type Snoozed struct {
	MessageID MessageID
	Until     time.Time
}

// Outfile is an attachment to include in an Outgoing message.
type Outfile struct {
	Filename    string
	ContentType string
	Content     []byte
	// ContentID, when set, marks this as an inline part (Content-Disposition:
	// inline) embeddable from the HTML body via cid:<ContentID>.
	ContentID string
}

// Thread is a conversation grouping.
type Thread struct {
	ID       ThreadID
	Account  AccountID
	Subject  string
	Messages []MessageID
	Last     time.Time
	Unread   bool
}
