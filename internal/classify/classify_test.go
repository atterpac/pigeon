package classify

import (
	"testing"

	"github.com/atterpac/pigeon/internal/model"
)

func TestProviderCategoryLabelsWin(t *testing.T) {
	got := Classify(Input{Labels: []model.LabelID{"CATEGORY_PROMOTIONS"}, Subject: "Security alert"})
	if got != model.CategoryPromotions {
		t.Fatalf("expected promotions label to win, got %q", got)
	}
}

func TestHeaderBasedPromotions(t *testing.T) {
	got := Classify(Input{
		Subject: "Sale ends tomorrow",
		From:    []model.Address{{Name: "The Shop", Addr: "deals@example.com"}},
		Headers: map[string][]string{
			"List-Unsubscribe": {"<mailto:unsubscribe@example.com>"},
			"Precedence":       {"bulk"},
		},
	})
	if got != model.CategoryPromotions {
		t.Fatalf("expected promotions, got %q", got)
	}
}

func TestAccountUpdates(t *testing.T) {
	got := Classify(Input{
		Subject: "Your verification code",
		From:    []model.Address{{Addr: "security@github.com"}},
	})
	if got != model.CategoryUpdates {
		t.Fatalf("expected updates, got %q", got)
	}
}

func TestDefaultPrimary(t *testing.T) {
	got := Classify(Input{
		Subject: "Lunch tomorrow?",
		From:    []model.Address{{Name: "Jane", Addr: "jane@example.com"}},
	})
	if got != model.CategoryPrimary {
		t.Fatalf("expected primary, got %q", got)
	}
}

func TestSocialClassification(t *testing.T) {
	cases := []struct {
		name string
		in   Input
	}{
		{"known domain", Input{From: []model.Address{{Addr: "info@linkedin.com"}}}},
		{"notification sender + social domain", Input{
			From: []model.Address{{Addr: "notification@email.facebook.com"}},
		}},
		{"social text phrase", Input{
			Subject: "Jane tagged you in a photo",
			From:    []model.Address{{Name: "Jane", Addr: "jane@example.com"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.in); got != model.CategorySocial {
				t.Fatalf("expected social, got %q", got)
			}
		})
	}
}

func TestForumClassification(t *testing.T) {
	cases := []struct {
		name string
		in   Input
	}{
		{"list-id header", Input{
			From:    []model.Address{{Addr: "owner@example.com"}},
			Headers: map[string][]string{"List-Id": {"<dev.example.com>"}},
		}},
		{"groups domain", Input{From: []model.Address{{Addr: "team@groups.google.com"}}}},
		{"digest phrase at end of subject", Input{
			Subject: "Weekly digest",
			From:    []model.Address{{Addr: "bot@example.com"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.in); got != model.CategoryForums {
				t.Fatalf("expected forums, got %q", got)
			}
		})
	}
}

func TestAutoSubmittedUpdates(t *testing.T) {
	got := Classify(Input{
		Subject: "Out of office",
		From:    []model.Address{{Addr: "person@example.com"}},
		Headers: map[string][]string{"Auto-Submitted": {"auto-replied"}},
	})
	if got != model.CategoryUpdates {
		t.Fatalf("expected updates, got %q", got)
	}
}

func TestNewsletterBodyTextPromotions(t *testing.T) {
	msg := model.Message{
		Subject: "Discover all The Times offers",
		From:    []model.Address{{Name: "The New York Times", Addr: "nytimes@e.newyorktimes.com"}},
	}
	got := MessageWithHeadersAndBody(msg, nil, "View in browser. Upgrade now. Sale ends tomorrow. To stop receiving offers, unsubscribe.")
	if got != model.CategoryPromotions {
		t.Fatalf("expected promotions, got %q", got)
	}
}

func TestRecentInboxPromotionSnippets(t *testing.T) {
	cases := []model.Message{
		{
			Subject: "Your no-cost upgrade expires tomorrow.",
			Snippet: "Discover all The Times offers at no extra cost for the first year.",
			From:    []model.Address{{Name: "The New York Times", Addr: "nytimes@e.newyorktimes.com"}},
		},
		{
			Subject: "$500-$40,000 debt consolidation loans",
			Snippet: "Request a loan now, regardless of your score.",
			From:    []model.Address{{Name: "Brigit", Addr: "hello@mail.hellobrigit.com"}},
		},
		{
			Subject: "Shop early: up to 65% off big home upgrades",
			Snippet: "Macy's sale",
			From:    []model.Address{{Name: "Macy's", Addr: "shop@emails.macys.com"}},
		},
		{
			Subject: "END TABLES FLASH SALE Beat the clock!",
			Snippet: "Up to 70% OFF + FAST shipping",
			From:    []model.Address{{Name: "Wayfair", Addr: "editor@members.wayfair.com"}},
		},
	}
	for _, msg := range cases {
		if got := Message(msg); got != model.CategoryPromotions {
			t.Fatalf("%q: expected promotions, got %q", msg.Subject, got)
		}
	}
}
