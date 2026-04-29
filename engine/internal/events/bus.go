// Package events houses the process-local event bus that fans domain
// events out to in-process subscribers. Today the only consumer is the
// /v1/stream SSE handler; a future distributed-broker variant could
// publish beside it (no specific theme yet — process-local fan-out
// is sufficient for the single-node deployment shape).
//
// Design notes:
//   - Each Subscribe returns a fresh bounded channel + a cancel fn.
//   - Publish never blocks the publisher. If a subscriber's buffer is
//     full, that subscriber is dropped (channel closed) and the publish
//     moves on to the next subscriber — backpressure policy is "drop
//     slow consumers", not "block the bus".
//   - ctx cancellation on Subscribe auto-unsubscribes; explicit cancel
//     fn is idempotent.
package events

import (
	"context"
	"sync"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// Bus is a fan-out broadcaster. Safe for concurrent Publish/Subscribe.
type Bus struct {
	mu         sync.Mutex
	subs       map[int]chan domain.DomainEvent
	nextID     int
	bufferSize int
}

// NewBus constructs a Bus. bufferSize sets the per-subscriber channel
// capacity; values < 1 coerce to 256 (the config default).
func NewBus(bufferSize int) *Bus {
	if bufferSize < 1 {
		bufferSize = 256
	}
	return &Bus{
		subs:       map[int]chan domain.DomainEvent{},
		bufferSize: bufferSize,
	}
}

// Subscribe returns a receive-only channel of events plus a cancel fn.
// Cancel closes the channel and removes the subscription.
// When ctx is Done, the Bus auto-cancels this subscription.
//
// No error return: the current implementation is process-local and
// cannot fail. If a distributed broker variant ever lands, a new
// Subscribe shape will carry the error signal; existing callers keep
// working.
func (b *Bus) Subscribe(ctx context.Context) (<-chan domain.DomainEvent, func()) {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	ch := make(chan domain.DomainEvent, b.bufferSize)
	b.subs[id] = ch
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if existing, ok := b.subs[id]; ok {
				delete(b.subs, id)
				close(existing)
			}
			b.mu.Unlock()
		})
	}

	go func() {
		<-ctx.Done()
		cancel()
	}()

	return ch, cancel
}

// Publish delivers e to every active subscriber non-blockingly. Subscribers
// whose channel is full are dropped (removed + closed); Publish returns
// after attempting each delivery.
func (b *Bus) Publish(e domain.DomainEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, ch := range b.subs {
		select {
		case ch <- e:
			// delivered
		default:
			// subscriber too slow; drop it
			delete(b.subs, id)
			close(ch)
		}
	}
}

// SubscriberCount returns the number of live subscribers. Primarily useful
// for tests and /readyz diagnostics.
func (b *Bus) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
