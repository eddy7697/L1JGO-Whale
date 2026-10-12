package handler

import (
	"bytes"
	"testing"
)

func TestBuildLiquorMatchesJavaObjectIDAndType(t *testing.T) {
	for _, liquorType := range []byte{0, 8} {
		// Java ServerBasePacket 與 Writer.Bytes 都會補齊至 8 位元組。
		want := []byte{103, 0x78, 0x56, 0x34, 0x12, liquorType, 0, 0}
		if got := BuildLiquor(0x12345678, liquorType); !bytes.Equal(got, want) {
			t.Fatalf("liquor type %d: got %v, want %v", liquorType, got, want)
		}
	}
}
