package events_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
)

func TestBus_PublishDeliversToSubscriber(t *testing.T) {
	bus := events.NewBus(16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, _ := bus.Subscribe(ctx)
	now := time.Now().UTC()
	go bus.Publish(domain.IncidentOpenedEvent{
		IncidentID: "INC1", IoCID: "IOC1", At: now,
	})

	select {
	case e := <-ch:
		require.Equal(t, "incident.opened", e.EventType())
		require.Equal(t, "INC1", e.AggregateID())
		require.WithinDuration(t, now, e.OccurredAt(), 0)
	case <-time.After(time.Second):
		t.Fatal("event not received")
	}
}

func TestBus_FanOutToMultipleSubscribers(t *testing.T) {
	bus := events.NewBus(16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a, _ := bus.Subscribe(ctx)
	b, _ := bus.Subscribe(ctx)
	require.Equal(t, 2, bus.SubscriberCount())

	go bus.Publish(domain.SBOMIngestedEvent{SBOMID: "SBOM1", ComponentRef: "kind:Component/default/web", At: time.Now()})

	for name, ch := range map[string]<-chan domain.DomainEvent{"a": a, "b": b} {
		select {
		case e := <-ch:
			require.Equal(t, "sbom.ingested", e.EventType(), "subscriber %s", name)
		case <-time.After(time.Second):
			t.Fatalf("subscriber %s did not receive", name)
		}
	}
}

func TestBus_CancelClosesChannelAndRemovesSubscriber(t *testing.T) {
	bus := events.NewBus(4)
	ch, cancel := bus.Subscribe(context.Background())
	require.Equal(t, 1, bus.SubscriberCount())
	cancel()

	select {
	case _, ok := <-ch:
		require.False(t, ok, "channel must be closed after cancel")
	case <-time.After(time.Second):
		t.Fatal("channel not closed within 1s")
	}
	require.Equal(t, 0, bus.SubscriberCount())
}

func TestBus_CancelIsIdempotent(_ *testing.T) {
	bus := events.NewBus(4)
	_, cancel := bus.Subscribe(context.Background())
	cancel()
	cancel() // must not panic on double-close
}

func TestBus_ContextDoneUnsubscribes(t *testing.T) {
	bus := events.NewBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := bus.Subscribe(ctx)
	cancel()

	require.Eventually(t, func() bool {
		select {
		case _, ok := <-ch:
			return !ok
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "ctx cancel must auto-unsubscribe")
	require.Equal(t, 0, bus.SubscriberCount())
}

// TestBus_PublishDoesNotBlockOnSlowConsumer is the publisher-side half
// of the drop-on-full contract. A subscriber that never drains must not
// hold up a flood of publishes.
func TestBus_PublishDoesNotBlockOnSlowConsumer(t *testing.T) {
	bus := events.NewBus(4)
	_, _ = bus.Subscribe(context.Background())

	done := make(chan struct{})
	start := time.Now()
	go func() {
		for i := 0; i < 256; i++ {
			bus.Publish(domain.SBOMIngestedEvent{SBOMID: fmt.Sprintf("s-%d", i), ComponentRef: "x", At: time.Now()})
		}
		close(done)
	}()

	select {
	case <-done:
		elapsed := time.Since(start)
		require.Less(t, elapsed, time.Second, "256 publishes must not take ~seconds with a slow consumer")
	case <-time.After(2 * time.Second):
		t.Fatal("publisher blocked by slow consumer")
	}
}

// TestBus_SlowConsumerDropped is the subscriber-side half. After the
// buffer fills, the subscription is revoked (channel closed). The number
// of events actually received is at most bufferSize.
func TestBus_SlowConsumerDropped(t *testing.T) {
	bus := events.NewBus(4)
	ch, _ := bus.Subscribe(context.Background())

	for i := 0; i < 256; i++ {
		bus.Publish(domain.SBOMIngestedEvent{SBOMID: "x", ComponentRef: "y", At: time.Now()})
	}

	received := 0
	for range ch { // exits when channel is closed
		received++
	}
	require.LessOrEqual(t, received, 4, "slow consumer must not receive more than bufferSize events")
	require.Equal(t, 0, bus.SubscriberCount(), "bus must drop the slow subscriber")
}
