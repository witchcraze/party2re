package event

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrNilEvent   = errors.New("event is nil")
	ErrNilHandler = errors.New("handler is nil")
)

// Event is the interface that all domain events implement.
type Event interface {
	EventName() string
}

// Handler is a function that processes a specific domain event.
type Handler func(ctx context.Context, evt Event) error

// Dispatcher coordinates in-process event subscriptions and two-phase publishing.
type Dispatcher struct {
	mu            sync.RWMutex
	syncHandlers  map[string][]Handler
	asyncHandlers map[string][]Handler
	wg            sync.WaitGroup
}

// NewDispatcher creates an initialized Dispatcher instance.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		syncHandlers:  make(map[string][]Handler),
		asyncHandlers: make(map[string][]Handler),
	}
}

// SubscribeSync registers a synchronous handler executed within the publisher's transaction context.
// If any sync handler returns an error, transaction execution aborts and rolls back.
func (d *Dispatcher) SubscribeSync(eventName string, handler Handler) {
	if handler == nil || eventName == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.syncHandlers[eventName] = append(d.syncHandlers[eventName], handler)
}

// SubscribeAsync registers an asynchronous handler executed post-commit in a separate goroutine.
func (d *Dispatcher) SubscribeAsync(eventName string, handler Handler) {
	if handler == nil || eventName == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.asyncHandlers[eventName] = append(d.asyncHandlers[eventName], handler)
}

// PublishSync dispatches an event synchronously to all registered sync handlers.
func (d *Dispatcher) PublishSync(ctx context.Context, evt Event) error {
	if evt == nil {
		return ErrNilEvent
	}

	d.mu.RLock()
	handlers := append([]Handler(nil), d.syncHandlers[evt.EventName()]...)
	d.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, evt); err != nil {
			return fmt.Errorf("sync handler failed for event %q: %w", evt.EventName(), err)
		}
	}
	return nil
}

// PublishAsync dispatches an event asynchronously to all registered async handlers.
func (d *Dispatcher) PublishAsync(ctx context.Context, evt Event) {
	if evt == nil {
		return
	}

	d.mu.RLock()
	handlers := append([]Handler(nil), d.asyncHandlers[evt.EventName()]...)
	d.mu.RUnlock()

	for _, h := range handlers {
		d.wg.Add(1)
		go func(fn Handler) {
			defer d.wg.Done()
			_ = fn(ctx, evt)
		}(h)
	}
}

// WaitAsync blocks until all currently executing asynchronous handlers complete.
// Intended primarily for testing and graceful shutdown.
func (d *Dispatcher) WaitAsync() {
	d.wg.Wait()
}
