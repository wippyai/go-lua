package formalfreeze

import "testing"

func TestFormalFreezeSelectionTagsRecoverLogicalActualOrdinal(t *testing.T) {
	for _, item := range []struct {
		tag   actualTag
		count int
		want  int
	}{
		{tag: 3, count: 3, want: 2},
		{tag: 1, count: 3, want: 0},
		{tag: 2, count: 3, want: 1},
	} {
		if got, ok := actualOrdinal(item.tag, item.count); !ok || got != item.want {
			t.Fatalf("tag %d/count %d = %d/%t, want %d/true", item.tag, item.count, got, ok, item.want)
		}
	}
	for _, item := range []struct {
		tag   actualTag
		count int
	}{
		{tag: 0, count: 3},
		{tag: 4, count: 3},
		{tag: ^actualTag(0), count: 3},
		{tag: 1, count: 0},
		{tag: 1, count: -1},
	} {
		if _, ok := actualOrdinal(item.tag, item.count); ok {
			t.Fatalf("malformed actual tag %d/count %d was admitted", item.tag, item.count)
		}
	}
}
