package mime

import (
	"strings"
	"testing"
)

func TestStripTagsDropsScriptStyleAndEntities(t *testing.T) {
	in := `<style>p{color:red}</style><script>alert(1)</script><p>Hello &amp; bye</p>`
	if got := stripTags(in); got != "Hello & bye" {
		t.Fatalf("stripTags = %q, want %q", got, "Hello & bye")
	}
}

func TestParseHTMLFallbackStripsTags(t *testing.T) {
	raw := []byte("MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<style>x{}</style><p>Body &amp; more</p>\r\n")
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "Body & more" {
		t.Fatalf("fallback text = %q, want %q", parsed.Text, "Body & more")
	}
}

func TestParseRejectsOversizePart(t *testing.T) {
	orig := MaxPartBytes
	MaxPartBytes = 8
	defer func() { MaxPartBytes = orig }()

	raw := []byte("MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"this body is definitely longer than eight bytes\r\n")
	if _, err := Parse(raw); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected size-limit error, got %v", err)
	}
}

func TestParseWindows1252Body(t *testing.T) {
	raw := []byte("From: Promo <promo@example.com>\r\n" +
		"To: You <you@example.com>\r\n" +
		"Subject: Windows charset\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=windows-1252\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"Today=92s deal saves 20=25.\r\n")

	parsed, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "Today\u2019s deal saves 20%.\r\n" {
		t.Fatalf("unexpected decoded text: %q", parsed.Text)
	}
}
