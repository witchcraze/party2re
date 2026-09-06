package valkeytest

import (
	"unsafe"

	"github.com/valkey-io/valkey-go"
)

// NewBuilder returns a valkey.Builder initialized for standalone command construction
// without requiring an active network connection or triggering unexported slot verification assertions.
func NewBuilder() valkey.Builder {
	dummy := struct{ ks uint16 }{ks: 1 << 15}
	return *(*valkey.Builder)(unsafe.Pointer(&dummy))
}
