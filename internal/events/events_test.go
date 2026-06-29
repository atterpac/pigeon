package events

import (
	"sync"
	"testing"

	"github.com/atterpac/email/internal/model"
)

func evt() Event {
	return Event{Account: "a", Kind: KindUpsert, IDs: []model.MessageID{"1"}}
}

func TestPublishDelivers(t *testing.T) {
	t.Parallel()
	b := NewBus()
	ch, cancel := b.Subscribe()
	defer cancel()

	b.Publish(evt())
	got := <-ch
	if got.Account != "a" || got.Kind != KindUpsert || len(got.IDs) != 1 {
		t.Fatalf("got %+v, want one upsert for account a", got)
	}
}

func TestPublishFansOutToAllSubscribers(t *testing.T) {
	t.Parallel()
	b := NewBus()
	ch1, c1 := b.Subscribe()
	ch2, c2 := b.Subscribe()
	defer c1()
	defer c2()

	b.Publish(evt())
	if len(ch1) != 1 || len(ch2) != 1 {
		t.Fatalf("buffered = (%d, %d), want (1, 1)", len(ch1), len(ch2))
	}
}

func TestPublishIgnoresEmptyIDs(t *testing.T) {
	t.Parallel()
	b := NewBus()
	ch, cancel := b.Subscribe()
	defer cancel()

	b.Publish(Event{Account: "a", Kind: KindDelete}) // nil IDs
	b.Publish(Event{Account: "a", Kind: KindDelete, IDs: []model.MessageID{}})
	if len(ch) != 0 {
		t.Fatalf("buffered = %d, want 0 (empty-ID events ignored)", len(ch))
	}
}

func TestPublishDropsOnFullBuffer(t *testing.T) {
	t.Parallel()
	b := NewBus()
	ch, cancel := b.Subscribe()
	defer cancel()

	for range subBuffer + 10 { // overfill without draining
		b.Publish(evt())
	}
	if got := len(ch); got != subBuffer {
		t.Fatalf("buffer = %d, want %d (excess dropped)", got, subBuffer)
	}
}

func TestCancelClosesChannel(t *testing.T) {
	t.Parallel()
	b := NewBus()
	ch, cancel := b.Subscribe()

	cancel()
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed after cancel")
	}
}

func TestCancelIsIdempotent(t *testing.T) {
	t.Parallel()
	b := NewBus()
	_, cancel := b.Subscribe()

	cancel()
	cancel() // must not panic (double close)
}

func TestCancelStopsDelivery(t *testing.T) {
	t.Parallel()
	b := NewBus()
	ch, cancel := b.Subscribe()
	cancel()

	b.Publish(evt())
	if _, ok := <-ch; ok {
		t.Fatal("cancelled subscriber should receive no further events")
	}
}

func TestNilBusPublishIsNoop(t *testing.T) {
	t.Parallel()
	var b *Bus
	b.Publish(evt()) // must not panic
}

// TestPublishCancelRace exercises the close-under-lock invariant: a concurrent
// Publish must never send on a channel cancel has closed. Run with -race.
func TestPublishCancelRace(t *testing.T) {
	t.Parallel()
	b := NewBus()
	_, cancel := b.Subscribe()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 1000 {
			b.Publish(evt())
		}
	}()
	go func() {
		defer wg.Done()
		cancel()
	}()
	wg.Wait()
}
