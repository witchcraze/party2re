package valkeytest

import (
	"context"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
)

var (
	_ valkey.Client          = (*MockClient)(nil)
	_ valkey.DedicatedClient = (*MockDedicatedClient)(nil)
)

// MockOption configures a MockClient.
type MockOption func(*MockClient)

// WithDoHandler configures the Do handler on MockClient.
func WithDoHandler(fn func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult) MockOption {
	return func(m *MockClient) {
		m.DoFn = fn
	}
}

// WithDoMultiHandler configures the DoMulti handler on MockClient.
func WithDoMultiHandler(fn func(ctx context.Context, multi ...valkey.Completed) []valkey.ValkeyResult) MockOption {
	return func(m *MockClient) {
		m.DoMultiFn = fn
	}
}

// MockClient implements valkey.Client for offline deterministic testing without an external Valkey process.
type MockClient struct {
	DoFn        func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult
	DoMultiFn   func(ctx context.Context, multi ...valkey.Completed) []valkey.ValkeyResult
	DoCacheFn   func(ctx context.Context, cmd valkey.Cacheable, ttl time.Duration) valkey.ValkeyResult
	ReceiveFn   func(ctx context.Context, subscribe valkey.Completed, fn func(msg valkey.PubSubMessage)) error
	CloseFn     func()
	DedicatedFn func(fn func(valkey.DedicatedClient) error) error
	DedicateFn  func() (valkey.DedicatedClient, func())

	mu           sync.Mutex
	recordedCmds []valkey.Completed
}

// NewMockClient creates a new MockClient initialized with optional configuration.
func NewMockClient(opts ...MockOption) *MockClient {
	m := &MockClient{}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// B returns a safe command builder.
func (m *MockClient) B() valkey.Builder {
	return NewBuilder()
}

// Do executes a single command, recording it and dispatching to DoFn if configured.
func (m *MockClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	m.recordCommand(cmd)
	if m.DoFn != nil {
		return m.DoFn(ctx, cmd)
	}
	return valkey.ValkeyResult{}
}

// DoMulti executes multiple commands, recording them and dispatching to DoMultiFn if configured.
// If DoMultiFn is nil, it executes each command sequentially via Do().
func (m *MockClient) DoMulti(ctx context.Context, multi ...valkey.Completed) []valkey.ValkeyResult {
	for _, cmd := range multi {
		m.recordCommand(cmd)
	}
	if m.DoMultiFn != nil {
		return m.DoMultiFn(ctx, multi...)
	}
	results := make([]valkey.ValkeyResult, len(multi))
	for i, cmd := range multi {
		if m.DoFn != nil {
			results[i] = m.DoFn(ctx, cmd)
		}
	}
	return results
}

// DoCache executes a cacheable command, dispatching to DoCacheFn if configured.
func (m *MockClient) DoCache(ctx context.Context, cmd valkey.Cacheable, ttl time.Duration) valkey.ValkeyResult {
	if m.DoCacheFn != nil {
		return m.DoCacheFn(ctx, cmd, ttl)
	}
	return valkey.ValkeyResult{}
}

// DoMultiCache executes multiple cacheable commands.
func (m *MockClient) DoMultiCache(ctx context.Context, multi ...valkey.CacheableTTL) []valkey.ValkeyResult {
	return make([]valkey.ValkeyResult, len(multi))
}

// DoStream returns an empty ValkeyResultStream.
func (m *MockClient) DoStream(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResultStream {
	m.recordCommand(cmd)
	return valkey.ValkeyResultStream{}
}

// DoMultiStream returns an empty MultiValkeyResultStream.
func (m *MockClient) DoMultiStream(ctx context.Context, multi ...valkey.Completed) valkey.MultiValkeyResultStream {
	for _, cmd := range multi {
		m.recordCommand(cmd)
	}
	return valkey.MultiValkeyResultStream{}
}

// Dedicated executes a function within a dedicated client context.
func (m *MockClient) Dedicated(fn func(valkey.DedicatedClient) error) error {
	if m.DedicatedFn != nil {
		return m.DedicatedFn(fn)
	}
	d := &MockDedicatedClient{client: m}
	return fn(d)
}

// Dedicate returns a DedicatedClient and a release callback.
func (m *MockClient) Dedicate() (valkey.DedicatedClient, func()) {
	if m.DedicateFn != nil {
		return m.DedicateFn()
	}
	d := &MockDedicatedClient{client: m}
	return d, func() {}
}

// Receive receives pubsub messages, dispatching to ReceiveFn if configured.
func (m *MockClient) Receive(ctx context.Context, subscribe valkey.Completed, fn func(msg valkey.PubSubMessage)) error {
	m.recordCommand(subscribe)
	if m.ReceiveFn != nil {
		return m.ReceiveFn(ctx, subscribe, fn)
	}
	return nil
}

// Close closes the mock client, invoking CloseFn if configured.
func (m *MockClient) Close() {
	if m.CloseFn != nil {
		m.CloseFn()
	}
}

// Nodes returns a single-entry map with the mock client.
func (m *MockClient) Nodes() map[string]valkey.Client {
	return map[string]valkey.Client{"default": m}
}

// Mode returns ClientModeStandalone.
func (m *MockClient) Mode() valkey.ClientMode {
	return valkey.ClientModeStandalone
}

// RecordedCommands returns a copy of all completed commands executed on this mock.
func (m *MockClient) RecordedCommands() []valkey.Completed {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]valkey.Completed, len(m.recordedCmds))
	copy(copied, m.recordedCmds)
	return copied
}

// RecordedCommandStrings returns all executed commands as slices of command argument strings.
func (m *MockClient) RecordedCommandStrings() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([][]string, len(m.recordedCmds))
	for i, cmd := range m.recordedCmds {
		res[i] = cmd.Commands()
	}
	return res
}

// LastCommandStrings returns the argument strings of the most recently executed command, or nil if none.
func (m *MockClient) LastCommandStrings() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.recordedCmds) == 0 {
		return nil
	}
	return m.recordedCmds[len(m.recordedCmds)-1].Commands()
}

// ResetRecordedCommands clears all recorded command history.
func (m *MockClient) ResetRecordedCommands() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordedCmds = nil
}

func (m *MockClient) recordCommand(cmd valkey.Completed) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordedCmds = append(m.recordedCmds, cmd)
}

// MockDedicatedClient implements valkey.DedicatedClient for offline testing.
type MockDedicatedClient struct {
	client *MockClient
}

// B returns the builder from the parent MockClient.
func (d *MockDedicatedClient) B() valkey.Builder {
	return d.client.B()
}

// Do executes a single command on the parent MockClient.
func (d *MockDedicatedClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	return d.client.Do(ctx, cmd)
}

// DoMulti executes multiple commands on the parent MockClient.
func (d *MockDedicatedClient) DoMulti(ctx context.Context, multi ...valkey.Completed) []valkey.ValkeyResult {
	return d.client.DoMulti(ctx, multi...)
}

// Receive executes pubsub receive on the parent MockClient.
func (d *MockDedicatedClient) Receive(ctx context.Context, subscribe valkey.Completed, fn func(msg valkey.PubSubMessage)) error {
	return d.client.Receive(ctx, subscribe, fn)
}

// Close invokes Close on the parent MockClient.
func (d *MockDedicatedClient) Close() {
	d.client.Close()
}

// SetPubSubHooks stubs pubsub hook registration.
func (d *MockDedicatedClient) SetPubSubHooks(hooks valkey.PubSubHooks) <-chan error {
	return nil
}

// SetOnInvalidations stubs cache invalidation callback registration.
func (d *MockDedicatedClient) SetOnInvalidations(fn func([]valkey.ValkeyMessage)) <-chan error {
	return nil
}
