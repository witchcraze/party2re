package valkeytest_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestNewBuilder_CommandConstruction(t *testing.T) {
	b := valkeytest.NewBuilder()

	// 1. Simple GET
	getCmd := b.Get().Key("test:key").Build()
	if !reflect.DeepEqual(getCmd.Commands(), []string{"GET", "test:key"}) {
		t.Fatalf("unexpected GET commands: %v", getCmd.Commands())
	}

	// 2. SET with TTL
	setCmd := b.Set().Key("test:key").Value("test-val").Ex(10 * time.Second).Build()
	if len(setCmd.Commands()) < 4 || setCmd.Commands()[0] != "SET" {
		t.Fatalf("unexpected SET command: %v", setCmd.Commands())
	}

	// 3. ZADD
	zaddCmd := b.Zadd().Key("test:zset").ScoreMember().ScoreMember(100, "member-1").Build()
	if len(zaddCmd.Commands()) < 4 || zaddCmd.Commands()[0] != "ZADD" {
		t.Fatalf("unexpected ZADD command: %v", zaddCmd.Commands())
	}

	// 4. Eval / Evalsha
	evalCmd := b.Evalsha().Sha1("fake-sha").Numkeys(1).Key("test:key").Arg("arg1").Build()
	if len(evalCmd.Commands()) < 5 || evalCmd.Commands()[0] != "EVALSHA" {
		t.Fatalf("unexpected EVALSHA command: %v", evalCmd.Commands())
	}
}

func TestMockClient_CommandRecordingAndDo(t *testing.T) {
	ctx := context.Background()

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "GET" {
			return valkeytest.MakeStringResult("mock-value")
		}
		return valkeytest.MakeOKResult()
	}))

	b := client.B()

	// Initial state
	if len(client.RecordedCommands()) != 0 {
		t.Fatalf("expected 0 recorded commands, got %d", len(client.RecordedCommands()))
	}
	if client.LastCommandStrings() != nil {
		t.Fatalf("expected nil last command strings, got %v", client.LastCommandStrings())
	}

	// Execute GET
	res1 := client.Do(ctx, b.Get().Key("key:1").Build())
	val1, err := res1.ToString()
	if err != nil || val1 != "mock-value" {
		t.Fatalf("expected 'mock-value', got val=%q, err=%v", val1, err)
	}

	// Check recording
	if len(client.RecordedCommands()) != 1 {
		t.Fatalf("expected 1 recorded command, got %d", len(client.RecordedCommands()))
	}
	if !reflect.DeepEqual(client.LastCommandStrings(), []string{"GET", "key:1"}) {
		t.Fatalf("unexpected last command strings: %v", client.LastCommandStrings())
	}

	// Execute SET
	res2 := client.Do(ctx, b.Set().Key("key:2").Value("val:2").Build())
	val2, err := res2.ToString()
	if err != nil || val2 != "OK" {
		t.Fatalf("expected 'OK', got val=%q, err=%v", val2, err)
	}

	// Check all recorded command strings
	allCmds := client.RecordedCommandStrings()
	expected := [][]string{
		{"GET", "key:1"},
		{"SET", "key:2", "val:2"},
	}
	if !reflect.DeepEqual(allCmds, expected) {
		t.Fatalf("expected %v, got %v", expected, allCmds)
	}

	// Reset recorded commands
	client.ResetRecordedCommands()
	if len(client.RecordedCommands()) != 0 {
		t.Fatalf("expected 0 recorded commands after reset, got %d", len(client.RecordedCommands()))
	}
}

func TestMockClient_DoMulti(t *testing.T) {
	ctx := context.Background()

	// 1. Default fallback: executes via DoFn sequentially
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult(cmd.Commands()[1])
	}))

	b := client.B()
	cmd1 := b.Get().Key("res-1").Build()
	cmd2 := b.Get().Key("res-2").Build()

	resList := client.DoMulti(ctx, cmd1, cmd2)
	if len(resList) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resList))
	}
	s1, _ := resList[0].ToString()
	s2, _ := resList[1].ToString()
	if s1 != "res-1" || s2 != "res-2" {
		t.Fatalf("unexpected results: %q, %q", s1, s2)
	}

	// 2. Custom DoMultiFn
	customClient := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkey.Completed) []valkey.ValkeyResult {
		return []valkey.ValkeyResult{
			valkeytest.MakeIntResult(42),
			valkeytest.MakeIntResult(84),
		}
	}))

	resList2 := customClient.DoMulti(ctx, cmd1, cmd2)
	if len(resList2) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resList2))
	}
	i1, _ := resList2[0].ToInt64()
	i2, _ := resList2[1].ToInt64()
	if i1 != 42 || i2 != 84 {
		t.Fatalf("unexpected custom results: %d, %d", i1, i2)
	}
}

func TestMockClient_LifecycleAndDedicated(t *testing.T) {
	ctx := context.Background()
	closed := false

	client := valkeytest.NewMockClient()
	client.CloseFn = func() { closed = true }

	// Mode and Nodes
	if client.Mode() != valkey.ClientModeStandalone {
		t.Fatalf("expected ClientModeStandalone, got %v", client.Mode())
	}
	nodes := client.Nodes()
	if len(nodes) != 1 || nodes["default"] != client {
		t.Fatalf("unexpected nodes map: %v", nodes)
	}

	// Dedicated callback
	dedicatedRan := false
	err := client.Dedicated(func(dc valkey.DedicatedClient) error {
		dedicatedRan = true
		dc.Close()
		_ = dc.SetPubSubHooks(valkey.PubSubHooks{})
		_ = dc.SetOnInvalidations(func([]valkey.ValkeyMessage) {})
		_ = dc.Do(ctx, dc.B().Get().Key("ded-key").Build())
		return nil
	})
	if err != nil || !dedicatedRan {
		t.Fatalf("expected dedicated to run successfully, err=%v", err)
	}
	if !reflect.DeepEqual(client.LastCommandStrings(), []string{"GET", "ded-key"}) {
		t.Fatalf("unexpected last command on dedicated: %v", client.LastCommandStrings())
	}

	// Dedicate manual
	dc, cancel := client.Dedicate()
	if dc == nil || cancel == nil {
		t.Fatal("expected non-nil dedicated client and cancel func")
	}
	cancel()

	// Receive
	var receivedMsg string
	_ = client.Receive(ctx, client.B().Subscribe().Channel("chan-1").Build(), func(msg valkey.PubSubMessage) {
		receivedMsg = msg.Message
	})
	_ = receivedMsg
	if len(client.RecordedCommands()) < 2 {
		t.Fatalf("expected recorded subscribe command")
	}

	// DoStream and DoMultiStream
	_ = client.DoStream(ctx, client.B().Get().Key("stream-key").Build())
	_ = client.DoMultiStream(ctx, client.B().Get().Key("stream-m1").Build(), client.B().Get().Key("stream-m2").Build())

	// DoCache and DoMultiCache
	client.DoCacheFn = func(ctx context.Context, cmd valkey.Cacheable, ttl time.Duration) valkey.ValkeyResult {
		return valkeytest.MakeStringResult("cached-val")
	}
	resCache := client.DoCache(ctx, client.B().Get().Key("cache-k").Cache(), time.Minute)
	if s, err := resCache.ToString(); err != nil || s != "cached-val" {
		t.Fatalf("expected cached-val, got %q, err=%v", s, err)
	}
	multiCacheRes := client.DoMultiCache(ctx)
	if len(multiCacheRes) != 0 {
		t.Fatalf("expected empty multiCacheRes, got %d", len(multiCacheRes))
	}

	// Custom DedicatedFn & DedicateFn
	client.DedicatedFn = func(fn func(valkey.DedicatedClient) error) error {
		return errors.New("custom-ded-err")
	}
	if err := client.Dedicated(func(dc valkey.DedicatedClient) error { return nil }); err == nil || err.Error() != "custom-ded-err" {
		t.Fatalf("expected custom-ded-err, got %v", err)
	}

	client.DedicateFn = func() (valkey.DedicatedClient, func()) {
		return nil, nil
	}
	if c, _ := client.Dedicate(); c != nil {
		t.Fatal("expected nil client from custom DedicateFn")
	}

	// DedicatedClient DoMulti and Receive
	dedicatedRan2 := false
	client.DedicatedFn = nil
	_ = client.Dedicated(func(dc valkey.DedicatedClient) error {
		dedicatedRan2 = true
		_ = dc.DoMulti(ctx, dc.B().Get().Key("k1").Build())
		_ = dc.Receive(ctx, dc.B().Subscribe().Channel("c1").Build(), func(msg valkey.PubSubMessage) {})
		return nil
	})
	if !dedicatedRan2 {
		t.Fatal("expected dedicated2 to run")
	}

	// Receive with ReceiveFn
	receiveCalled := false
	client.ReceiveFn = func(ctx context.Context, subscribe valkey.Completed, fn func(msg valkey.PubSubMessage)) error {
		receiveCalled = true
		fn(valkey.PubSubMessage{Channel: "c1", Message: "m1"})
		return nil
	}
	var gotPubSub string
	_ = client.Receive(ctx, client.B().Subscribe().Channel("c1").Build(), func(msg valkey.PubSubMessage) {
		gotPubSub = msg.Message
	})
	if !receiveCalled || gotPubSub != "m1" {
		t.Fatalf("expected receiveCalled=true and gotPubSub='m1', got %v, %q", receiveCalled, gotPubSub)
	}

	// Close
	client.Close()
	if !closed {
		t.Fatal("expected CloseFn to be called")
	}
}

func TestRESPBuilders(t *testing.T) {
	// 1. MakeIntSliceResult
	intSliceRes := valkeytest.MakeIntSliceResult([]int64{10, -20, 300})
	intVals, err := intSliceRes.AsIntSlice()
	if err != nil {
		t.Fatalf("AsIntSlice error: %v", err)
	}
	if !reflect.DeepEqual(intVals, []int64{10, -20, 300}) {
		t.Fatalf("unexpected int slice: %v", intVals)
	}

	// Empty int slice
	emptyIntRes := valkeytest.MakeIntSliceResult([]int64{})
	emptyVals, err := emptyIntRes.AsIntSlice()
	if err != nil {
		t.Fatalf("empty AsIntSlice error: %v", err)
	}
	if len(emptyVals) != 0 {
		t.Fatalf("expected empty int slice, got %v", emptyVals)
	}

	// 2. MakeIntResult
	intRes := valkeytest.MakeIntResult(12345)
	intVal, err := intRes.ToInt64()
	if err != nil || intVal != 12345 {
		t.Fatalf("expected 12345, got %d, err=%v", intVal, err)
	}

	negIntRes := valkeytest.MakeIntResult(-987)
	negVal, err := negIntRes.ToInt64()
	if err != nil || negVal != -987 {
		t.Fatalf("expected -987, got %d, err=%v", negVal, err)
	}

	// 3. MakeStringResult
	strRes := valkeytest.MakeStringResult("hello-party2")
	strVal, err := strRes.ToString()
	if err != nil || strVal != "hello-party2" {
		t.Fatalf("expected 'hello-party2', got %q, err=%v", strVal, err)
	}

	// 4. MakeStringSliceResult
	strSliceRes := valkeytest.MakeStringSliceResult([]string{"alpha", "beta", "gamma"})
	strVals, err := strSliceRes.AsStrSlice()
	if err != nil {
		t.Fatalf("AsStrSlice error: %v", err)
	}
	if !reflect.DeepEqual(strVals, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("unexpected string slice: %v", strVals)
	}

	// 5. MakeBoolResult
	boolTrueRes := valkeytest.MakeBoolResult(true)
	bVal, err := boolTrueRes.AsBool()
	if err != nil || !bVal {
		t.Fatalf("expected true, got %v, err=%v", bVal, err)
	}

	boolFalseRes := valkeytest.MakeBoolResult(false)
	bValFalse, err := boolFalseRes.AsBool()
	if err != nil || bValFalse {
		t.Fatalf("expected false, got %v, err=%v", bValFalse, err)
	}

	// 6. MakeNilResult
	nilRes := valkeytest.MakeNilResult()
	if !valkey.IsValkeyNil(nilRes.Error()) {
		t.Fatalf("expected valkey.IsValkeyNil(err)=true, got %v", nilRes.Error())
	}
	msg, _ := nilRes.ToMessage()
	if !msg.IsNil() {
		t.Fatal("expected msg.IsNil()=true for MakeNilResult")
	}

	// 7. MakeOKResult
	okRes := valkeytest.MakeOKResult()
	okStr, err := okRes.ToString()
	if err != nil || okStr != "OK" {
		t.Fatalf("expected 'OK', got %q, err=%v", okStr, err)
	}
	okBool, err := okRes.AsBool()
	if err != nil || !okBool {
		t.Fatalf("expected AsBool=true for OK result, got %v, err=%v", okBool, err)
	}

	// 8. MakeErrorResult
	customErr := errors.New("valkey io timeout")
	errRes := valkeytest.MakeErrorResult(customErr)
	if !errors.Is(errRes.Error(), customErr) {
		t.Fatalf("expected customErr, got %v", errRes.Error())
	}
}
