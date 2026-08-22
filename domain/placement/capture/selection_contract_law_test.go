package capture

import "testing"

func TestCaptureSelectionTagsRecoverLogicalSourceOrdinal(t *testing.T) {
	candidate := operand{sources: []source{{tag: 1}, {tag: 2}, {tag: 3}}}
	for _, item := range []struct {
		tag  routeTag
		want int
	}{
		{tag: 3, want: 2},
		{tag: 1, want: 0},
		{tag: 2, want: 1},
	} {
		if got, ok := sourceOrdinal(candidate, item.tag); !ok || got != item.want {
			t.Fatalf("tag %d = %d/%t, want %d/true", item.tag, got, ok, item.want)
		}
	}
	for _, tag := range []routeTag{0, 4, ^routeTag(0)} {
		if _, ok := sourceOrdinal(candidate, tag); ok {
			t.Fatalf("malformed source tag %d was admitted", tag)
		}
	}
}
