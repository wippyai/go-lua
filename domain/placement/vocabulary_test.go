package placement

import "testing"

func TestNameFrozenVocabulary(t *testing.T) {
	escapes := []struct {
		value Escape
		name  string
	}{
		{None, "none"},
		{Borrow, "borrow"},
		{Retain, "retain"},
		{Store, "store"},
		{Send, "send"},
		{Export, "export"},
		{Opaque, "opaque"},
		{Return, "return"},
	}
	for _, item := range escapes {
		if got := item.value.Name(); got != item.name {
			t.Fatalf("escape name %d = %q, want %q", item.value, got, item.name)
		}
		if got := item.value.String(); got != item.name {
			t.Fatalf("escape string %d = %q, want %q", item.value, got, item.name)
		}
	}

	placements := []struct {
		value Placement
		name  string
	}{
		{Bottom, "bottom"},
		{Stack, "stack"},
		{OwnedHeap, "owned-heap"},
		{SharedHeap, "shared-heap"},
		{Unknown, "unknown"},
		{Interpreter, "interpreter"},
		{Register, "register"},
	}
	for _, item := range placements {
		if got := item.value.String(); got != item.name {
			t.Fatalf("placement name %d = %q, want %q", item.value, got, item.name)
		}
	}
}

func TestEscapePlacementMappingIsTotal(t *testing.T) {
	for _, item := range []struct {
		escape    Escape
		placement Placement
		applies   bool
	}{
		{None, Bottom, false},
		{Borrow, Bottom, false},
		{Retain, OwnedHeap, true},
		{Store, OwnedHeap, true},
		{Send, SharedHeap, true},
		{Export, SharedHeap, true},
		{Opaque, SharedHeap, true},
		{Return, OwnedHeap, true},
	} {
		got, applies := item.escape.Placement()
		if got != item.placement || applies != item.applies {
			t.Fatalf("%s placement = %s/%t, want %s/%t", item.escape.Name(), got, applies, item.placement, item.applies)
		}
	}
}
