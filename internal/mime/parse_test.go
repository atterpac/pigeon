package mime

import "testing"

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
