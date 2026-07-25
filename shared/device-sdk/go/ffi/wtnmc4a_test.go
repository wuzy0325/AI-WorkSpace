//go:build windows

package ffi

import (
	"testing"
	"unsafe"
)

func TestWTNMC4ARR1RawMatchesSDKLayout(t *testing.T) {
	if got := unsafe.Sizeof(wtnmc4aRR1Raw{}); got != 64 {
		t.Fatalf("WTNMC4A_PARA_RR1 size = %d, want 64", got)
	}
}
