package lua

import (
	"fmt"
	"testing"
)

func TestUtf8EscapesMatchChar(t *testing.T) {
	L := NewState()
	defer L.Close()
	for _, codepoint := range []uint32{
		0, 0x7F, 0x80, 0x7FF, 0x800, 0xD800, 0xFFFF, 0x10000,
		0x10FFFF, 0x110000, 0x1FFFFF, 0x200000, 0x3FFFFFF, 0x4000000, 0x7FFFFFFF,
	} {
		if err := L.DoString(fmt.Sprintf(`assert("\u{%X}" == utf8.char(0x%X))`, codepoint, codepoint)); err != nil {
			t.Fatalf("codepoint %X: %v", codepoint, err)
		}
	}
}
