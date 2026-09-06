package valkeytest

import (
	"encoding/binary"

	"github.com/valkey-io/valkey-go"
)

// MakeIntSliceResult builds a ValkeyResult containing an array of RESP integer messages.
func MakeIntSliceResult(values []int64) valkey.ValkeyResult {
	buf := make([]byte, 7) // 7 bytes TTL prefix required by CacheUnmarshalView
	buf = append(buf, 42)  // typeArray ('*')
	lenBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBuf, uint64(len(values)))
	buf = append(buf, lenBuf...)
	for _, v := range values {
		buf = append(buf, 58) // typeInteger (':')
		intBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(intBuf, uint64(v))
		buf = append(buf, intBuf...)
	}
	var msg valkey.ValkeyMessage
	_ = msg.CacheUnmarshalView(buf)
	return valkey.NewResult(msg, nil)
}

// MakeIntResult builds a ValkeyResult containing a single RESP integer message.
func MakeIntResult(value int64) valkey.ValkeyResult {
	buf := make([]byte, 7)
	buf = append(buf, 58) // typeInteger (':')
	intBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(intBuf, uint64(value))
	buf = append(buf, intBuf...)
	var msg valkey.ValkeyMessage
	_ = msg.CacheUnmarshalView(buf)
	return valkey.NewResult(msg, nil)
}

// MakeStringResult builds a ValkeyResult containing a RESP blob string message.
func MakeStringResult(value string) valkey.ValkeyResult {
	buf := make([]byte, 7)
	buf = append(buf, 36) // typeBlobString ('$')
	lenBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBuf, uint64(len(value)))
	buf = append(buf, lenBuf...)
	buf = append(buf, []byte(value)...)
	var msg valkey.ValkeyMessage
	_ = msg.CacheUnmarshalView(buf)
	return valkey.NewResult(msg, nil)
}

// MakeStringSliceResult builds a ValkeyResult containing an array of RESP blob string messages.
func MakeStringSliceResult(values []string) valkey.ValkeyResult {
	buf := make([]byte, 7)
	buf = append(buf, 42) // typeArray ('*')
	lenBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBuf, uint64(len(values)))
	buf = append(buf, lenBuf...)
	for _, v := range values {
		buf = append(buf, 36) // typeBlobString ('$')
		strLenBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(strLenBuf, uint64(len(v)))
		buf = append(buf, strLenBuf...)
		buf = append(buf, []byte(v)...)
	}
	var msg valkey.ValkeyMessage
	_ = msg.CacheUnmarshalView(buf)
	return valkey.NewResult(msg, nil)
}

// MakeBoolResult builds a ValkeyResult containing a RESP boolean message.
func MakeBoolResult(value bool) valkey.ValkeyResult {
	buf := make([]byte, 7)
	buf = append(buf, 35) // typeBool ('#')
	boolBuf := make([]byte, 8)
	if value {
		binary.BigEndian.PutUint64(boolBuf, 1)
	}
	buf = append(buf, boolBuf...)
	var msg valkey.ValkeyMessage
	_ = msg.CacheUnmarshalView(buf)
	return valkey.NewResult(msg, nil)
}

// MakeNilResult builds a ValkeyResult representing a Valkey Nil response.
func MakeNilResult() valkey.ValkeyResult {
	buf := make([]byte, 7)
	buf = append(buf, 95) // typeNull ('_')
	buf = append(buf, make([]byte, 8)...)
	var msg valkey.ValkeyMessage
	_ = msg.CacheUnmarshalView(buf)
	return valkey.NewResult(msg, valkey.Nil)
}

// MakeOKResult builds a ValkeyResult representing an "OK" string response.
func MakeOKResult() valkey.ValkeyResult {
	return MakeStringResult("OK")
}

// MakeErrorResult builds a ValkeyResult containing an execution or network error.
func MakeErrorResult(err error) valkey.ValkeyResult {
	return valkey.NewErrorResult(err)
}
