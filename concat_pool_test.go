package lua

import (
	"strings"
	"testing"
)

func TestConcatPoolsRetainNoStrings(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`local a, b, c = "x", "y", "z" for i = 1, 100 do local s = a .. b .. c .. i end`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		p := stringPartsPool.Get().(*[]string)
		for _, s := range (*p)[:cap(*p)] {
			if s != "" {
				t.Fatalf("pooled parts slice retains %q", s)
			}
		}
		b := stringBuilderPool.Get().(*strings.Builder)
		if b.Cap() != 0 {
			t.Fatalf("pooled builder retains a %d byte buffer", b.Cap())
		}
		_ = p
	}
}
