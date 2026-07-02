// Package classify assigns coarse, Gmail-like inbox categories to messages.
package classify

import (
	"net/mail"
	"net/textproto"
	"strings"

	"github.com/atterpac/pigeon/internal/model"
)

// Input is the metadata used to classify a message. Headers should use their
// normal RFC names; values are case-insensitive for all built-in rules.
type Input struct {
	Subject string
	Snippet string
	From    []model.Address
	To      []model.Address
	Cc      []model.Address
	Labels  []model.LabelID
	Headers map[string][]string
}

// Message classifies a domain message using its envelope and labels.
func Message(m model.Message) model.Category {
	return Classify(Input{
		Subject: m.Subject,
		Snippet: m.Snippet,
		From:    m.From,
		To:      m.To,
		Cc:      m.Cc,
		Labels:  m.Labels,
	})
}

// MessageWithHeaders classifies a domain message with additional RFC headers.
func MessageWithHeaders(m model.Message, headers map[string][]string) model.Category {
	return MessageWithHeadersAndBody(m, headers, "")
}

// MessageWithHeadersAndBody classifies a message after body load, when headers
// and text content provide stronger newsletter/receipt/social signals.
func MessageWithHeadersAndBody(m model.Message, headers map[string][]string, body string) model.Category {
	in := Input{
		Subject: m.Subject,
		Snippet: strings.TrimSpace(m.Snippet + " " + body),
		From:    m.From,
		To:      m.To,
		Cc:      m.Cc,
		Labels:  m.Labels,
		Headers: headers,
	}
	return Classify(in)
}

// Classify returns a conservative category. Primary is the default unless a
// stronger bulk, notification, social, forum, or promotion signal is present.
func Classify(in Input) model.Category {
	if cat := categoryFromProviderLabels(in.Labels); cat != "" {
		return cat
	}

	from := firstAddress(in.From)
	fromAddr := strings.ToLower(from.Addr)
	fromName := strings.ToLower(from.Name)
	subject := strings.ToLower(in.Subject)
	snippet := strings.ToLower(in.Snippet)
	// Pad the haystack (not the needles) so boundary-spaced phrases like
	// " digest " still match a term at the start or end of the text.
	text := " " + subject + " " + snippet + " "
	domain := domainOf(fromAddr)

	if isForum(in, fromAddr, domain, text) {
		return model.CategoryForums
	}
	if isSocial(fromAddr, domain, text) {
		return model.CategorySocial
	}
	if isUpdate(in, fromAddr, fromName, domain, text) {
		return model.CategoryUpdates
	}
	if isPromotion(in, fromAddr, fromName, domain, text) {
		return model.CategoryPromotions
	}
	return model.CategoryPrimary
}

// HeaderMap extracts a normalized multi-value header map from a parsed message.
func HeaderMap(h mail.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, values := range h {
		out[textproto.CanonicalMIMEHeaderKey(k)] = append([]string(nil), values...)
	}
	return out
}

func categoryFromProviderLabels(labels []model.LabelID) model.Category {
	for _, label := range labels {
		switch strings.ToLower(string(label)) {
		case "category_promotions", "categories/promotions", "[gmail]/categories/promotions":
			return model.CategoryPromotions
		case "category_updates", "categories/updates", "[gmail]/categories/updates":
			return model.CategoryUpdates
		case "category_social", "categories/social", "[gmail]/categories/social":
			return model.CategorySocial
		case "category_forums", "categories/forums", "[gmail]/categories/forums":
			return model.CategoryForums
		case "category_personal", "category_primary", "categories/primary", "[gmail]/categories/primary":
			return model.CategoryPrimary
		}
	}
	return ""
}

func isForum(in Input, fromAddr, domain, text string) bool {
	return hasHeader(in.Headers, "List-Id") ||
		containsAny(domain, "groups.google.com", "googlegroups.com", "groups.io", "discourse", "forum") ||
		containsAny(fromAddr, "groups@", "forum@", "mailing-list@") ||
		containsAny(text, " mailing list ", " digest ", " forum ")
}

func isSocial(fromAddr, domain, text string) bool {
	if containsAny(domain,
		"facebookmail.com", "instagram.com", "linkedin.com", "twitter.com", "x.com",
		"tiktok.com", "redditmail.com", "pinterest.com", "snapchat.com", "discord.com", "bsky.app", "threads.net", "quora.com",
	) {
		return true
	}
	if containsAny(fromAddr, "notification@", "notifications@", "notify@") &&
		containsAny(domain, "facebook", "instagram", "linkedin", "twitter", "reddit", "discord") {
		return true
	}
	return containsAny(text, " mentioned you ", " tagged you ", " new follower ", " connection request ", "friend request", "trending stories")
}

func isUpdate(in Input, fromAddr, fromName, domain, text string) bool {
	if headerHasAny(in.Headers, "Auto-Submitted", "auto-generated", "auto-replied") {
		return true
	}
	if containsAny(fromAddr, "security@", "account@", "billing@", "invoice@", "receipts@", "receipt@", "support@", "alerts@", "alert@") {
		return true
	}
	if containsAny(fromName, "security", "billing", "support") {
		return true
	}
	if containsAny(domain, "amazon.", "paypal.", "stripe.", "github.com", "gitlab.com", "vercel.com", "netlify.com", "cloudflare.com") &&
		containsAny(text, "receipt", "invoice", "order", "shipped", "delivery", "security", "password", "sign-in", "login", "verification", "code", "alert", "billing") {
		return true
	}
	return containsAny(text,
		"receipt", "invoice", "order confirmation", "shipped", "delivered", "delivery",
		"payment", "billing", "statement", "security alert", "password", "verification code",
		"confirm your email", "sign-in", "login", "two-factor", "2fa", "app password", "created to sign in",
	)
}

// Brand-specific promotion signals live as data so new senders can be added
// without touching classification logic. Entries must be lowercase.
var (
	// promoDomains are sender domains whose mail is promotional when it also
	// carries promotional text.
	promoDomains = []string{
		"e.newyorktimes.com", "newyorktimes.com", "nytimes.com", "mail.hellobrigit.com",
		"nintendo.net", "xtime.com", "dollarshaveclub.com", "updates.corsair.com",
		"selectrewards.com", "devices.life360.com", "e.lowes.com", "lowes.com",
		"e.amazongames.com", "mail.nerdwallet.com", "emails.macys.com", "macys.com",
		"members.wayfair.com", "wayfair.com", "substack.com", "mailchimp.com",
		"campaignmonitor.com", "sendgrid.net", "sendgrid.com", "klaviyomail.com",
	}
	// promoSenders are local-parts that mark a sender as promotional on their own.
	promoSenders = []string{
		"newsletter@", "newsletters@", "deals@", "offers@", "marketing@", "promo@",
		"promotions@", "sale@", "news@", "shop@", "editor@", "members@", "hello@mail.",
		"email@", "lowes@", "corsair@", "nytimes@",
	}
	// promoNames are display-name fragments that mark a sender as promotional
	// when paired with promotional text.
	promoNames = []string{
		"newsletter", "deals", "offers", "rewards", "macy", "lowe", "wayfair",
		"dollar shave", "new york times", "brigit", "nintendo", "corsair",
	}
)

func isPromotion(in Input, fromAddr, fromName, domain, text string) bool {
	bulk := headerHasAny(in.Headers, "Precedence", "bulk", "list") ||
		hasHeader(in.Headers, "List-Unsubscribe") ||
		hasHeader(in.Headers, "List-Unsubscribe-Post")
	promoText := containsAny(text,
		"sale", "deal", "deals", "offer", "offers", "discount", "save", "savings", "coupon",
		"promo", "upgrade", "subscribe", "unsubscribe", "limited time", "ends tonight",
		"ends tomorrow", "% off", "promo code", "upgrade now", "shop now", "gift card",
		"reward", "rewards", "member exclusive", "exclusive is here", "flash sale",
		"prime day", "service savings", "request a loan", "pre-qualify",
	)
	if containsAny(fromAddr, promoSenders...) {
		return true
	}
	if containsAny(fromName, promoNames...) && promoText {
		return true
	}
	if containsAny(domain, promoDomains...) && promoText {
		return true
	}
	if bulk && promoText {
		return true
	}
	if bulk && containsAny(domain, "mailchimp", "sendgrid", "campaign", "klaviyo", "constantcontact", "substack") {
		return true
	}
	// no-reply / hello senders only count with promo text; support@ and email@
	// are handled earlier (isUpdate and promoSenders respectively).
	return promoText && containsAny(fromAddr, "no-reply@", "noreply@", "hello@")
}

func firstAddress(addrs []model.Address) model.Address {
	if len(addrs) == 0 {
		return model.Address{}
	}
	return addrs[0]
}

func domainOf(addr string) string {
	at := strings.LastIndexByte(addr, '@')
	if at < 0 || at == len(addr)-1 {
		return ""
	}
	return addr[at+1:]
}

func hasHeader(headers map[string][]string, name string) bool {
	if len(headers) == 0 {
		return false
	}
	_, ok := headers[textproto.CanonicalMIMEHeaderKey(name)]
	return ok
}

func headerHasAny(headers map[string][]string, name string, needles ...string) bool {
	values := headers[textproto.CanonicalMIMEHeaderKey(name)]
	for _, value := range values {
		if containsAny(strings.ToLower(value), needles...) {
			return true
		}
	}
	return false
}

// containsAny reports whether s contains any of needles. s is matched as-is;
// needles must already be lowercase (every caller passes lowercase literals).
func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
