// Package events is an in-process changefeed. Store mutations publish Events and
// a Bus fans them out to subscribers (re-exposed via the email.Client facade) so
// a UI can react instead of polling. Events are best-effort hints: a slow
// subscriber may miss individual events (its buffer drops), so treat an event as
// "something changed, refetch" rather than an authoritative log.
package events

import (
	"sync"

	"github.com/atterpac/email/internal/model"
)

// Kind classifies a change.
type Kind string

const (
	KindUpsert Kind = "upsert" // new or updated message envelopes
	KindFlag   Kind = "flag"   // flag/label changes on existing messages
	KindDelete Kind = "delete" // messages removed
)

// Event describes a store change.
type Event struct {
	Account model.AccountID
	Kind    Kind
	IDs     []model.MessageID
}

// subBuffer is the per-subscriber channel capacity before events drop.
const subBuffer = 256

// Bus is a concurrency-safe in-process publish/subscribe hub.
type Bus struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]chan Event
}

// NewBus returns an empty bus.
func NewBus() *Bus { return &Bus{subs: map[int]chan Event{}} }

// Subscribe returns a channel of events and a cancel func. The channel is
// closed when cancel is called. Cancel is idempotent.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	ch := make(chan Event, subBuffer)
	b.subs[id] = ch

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if c, ok := b.subs[id]; ok {
				delete(b.subs, id)
				close(c)
			}
		})
	}
	return ch, cancel
}

// Publish delivers e to all subscribers. Events with no IDs are ignored.
// Non-blocking: if a subscriber's buffer is full, the event is dropped for that
// subscriber.
func (b *Bus) Publish(e Event) {
	if b == nil || len(e.IDs) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default: // slow subscriber; drop (it should refetch on any event)
		}
	}
}
