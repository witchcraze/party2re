package event_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/event"
)

type dummyEvent struct {
	name    string
	payload string
}

func (e dummyEvent) EventName() string {
	return e.name
}

func TestDispatcher_PublishSync(t *testing.T) {
	d := event.NewDispatcher()
	var executedCount int

	d.SubscribeSync("test.event", func(ctx context.Context, evt event.Event) error {
		executedCount++
		return nil
	})

	err := d.PublishSync(context.Background(), dummyEvent{name: "test.event", payload: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if executedCount != 1 {
		t.Errorf("expected executedCount 1, got %d", executedCount)
	}

	// Unsubscribed event
	err = d.PublishSync(context.Background(), dummyEvent{name: "other.event"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if executedCount != 1 {
		t.Errorf("expected executedCount 1, got %d", executedCount)
	}

	// Nil event
	if err := d.PublishSync(context.Background(), nil); !errors.Is(err, event.ErrNilEvent) {
		t.Errorf("expected ErrNilEvent, got %v", err)
	}
}

func TestDispatcher_PublishSync_ErrorRollback(t *testing.T) {
	d := event.NewDispatcher()
	errCustom := errors.New("custom business failure")

	d.SubscribeSync("test.fail", func(ctx context.Context, evt event.Event) error {
		return errCustom
	})

	err := d.PublishSync(context.Background(), dummyEvent{name: "test.fail"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errCustom) {
		t.Errorf("expected error wrapping %v, got %v", errCustom, err)
	}
}

func TestDispatcher_PublishAsync(t *testing.T) {
	d := event.NewDispatcher()
	var executedCount int64

	d.SubscribeAsync("async.event", func(ctx context.Context, evt event.Event) error {
		atomic.AddInt64(&executedCount, 1)
		return nil
	})

	d.PublishAsync(context.Background(), dummyEvent{name: "async.event"})
	d.WaitAsync()

	if val := atomic.LoadInt64(&executedCount); val != 1 {
		t.Errorf("expected executedCount 1, got %d", val)
	}

	// PublishAsync with nil does nothing
	d.PublishAsync(context.Background(), nil)
	d.WaitAsync()
}

func TestDispatcher_NilHandlerIgnored(t *testing.T) {
	d := event.NewDispatcher()
	d.SubscribeSync("", nil)
	d.SubscribeSync("foo", nil)
	d.SubscribeAsync("", nil)
	d.SubscribeAsync("bar", nil)

	if err := d.PublishSync(context.Background(), dummyEvent{name: "foo"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDispatcher_ConcurrentAccess(t *testing.T) {
	d := event.NewDispatcher()
	var count int64
	d.SubscribeSync("concurrent", func(ctx context.Context, evt event.Event) error {
		atomic.AddInt64(&count, 1)
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.PublishSync(context.Background(), dummyEvent{name: "concurrent"})
		}()
	}
	wg.Wait()

	if val := atomic.LoadInt64(&count); val != 50 {
		t.Errorf("expected 50 sync calls, got %d", val)
	}
}

func TestDispatcher_AsyncResilientExecution(t *testing.T) {
	d := event.NewDispatcher()
	var completed int64

	d.SubscribeAsync("resilient", func(ctx context.Context, evt event.Event) error {
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&completed, 1)
		return errors.New("non-fatal async error")
	})

	d.PublishAsync(context.Background(), dummyEvent{name: "resilient"})
	d.WaitAsync()

	if val := atomic.LoadInt64(&completed); val != 1 {
		t.Errorf("expected 1 completed async execution, got %d", val)
	}
}
