package mapedit

import "testing"

func TestCloneNilInputReturnsNil(t *testing.T) {
	if got := Clone(map[string]int(nil)); got != nil {
		t.Fatalf("Clone(nil) = %#v, want nil", got)
	}
	if got := Clone(map[string]int{}); got != nil {
		t.Fatalf("Clone(empty) = %#v, want nil", got)
	}
}

func TestCloneCreatesIndependentCopy(t *testing.T) {
	original := map[string]int{"a": 1}
	clone := Clone(original)
	if len(clone) != 1 || clone["a"] != 1 {
		t.Fatalf("Clone(%#v) = %#v, want copied entry", original, clone)
	}
	clone["a"] = 99
	if got := original["a"]; got != 1 {
		t.Fatalf("Clone should not mutate original map, got original[a]=%d", got)
	}
}

func TestWithCreatesIndependentCopyFromNilAndExistingMap(t *testing.T) {
	if got := With(map[string]int(nil), "a", 1); len(got) != 1 || got["a"] != 1 {
		t.Fatalf("With(nil, ...) = %#v, want one entry", got)
	}

	original := map[string]int{"a": 1}
	clone := With(original, "b", 2)
	clone["a"] = 99
	if got := original["a"]; got != 1 {
		t.Fatalf("With should not mutate original map, got original[a]=%d", got)
	}
}

func TestWithoutCreatesIndependentCopy(t *testing.T) {
	original := map[string]int{"a": 1, "b": 2}
	clone, removed := Without(original, "a")
	if !removed {
		t.Fatalf("Without should report removal")
	}
	clone["b"] = 99
	if got := original["b"]; got != 2 {
		t.Fatalf("Without should not mutate original map, got original[b]=%d", got)
	}
}

func TestWithoutAbsentKeyReturnsOriginalUnchanged(t *testing.T) {
	original := map[string]int{"a": 1}
	next, removed := Without(original, "missing")
	if removed {
		t.Fatalf("Without should report no removal for absent key")
	}
	next["a"] = 2
	if got := original["a"]; got != 2 {
		t.Fatalf("Without absent key should leave map unchanged and shared, got original[a]=%d", got)
	}

	empty := map[string]int{}
	next, removed = Without(empty, "missing")
	if removed {
		t.Fatalf("Without should report no removal for absent key in empty map")
	}
	if next != nil {
		t.Fatalf("Without absent key in empty map = %#v, want normalized nil", next)
	}

	next, removed = Without(map[string]int(nil), "missing")
	if removed || next != nil {
		t.Fatalf("Without nil missing = %#v/%v, want nil/false", next, removed)
	}
}

func TestWithoutLastKeyReturnsNil(t *testing.T) {
	next, removed := Without(map[string]int{"a": 1}, "a")
	if !removed {
		t.Fatalf("Without should report removal")
	}
	if next != nil {
		t.Fatalf("Without last key = %#v, want nil", next)
	}
}

func TestDeleteMatchingCreatesIndependentCopy(t *testing.T) {
	original := map[string]int{"a": 1, "b": 2, "c": 3}
	next, removed := DeleteMatching(original, func(_ string, value int) bool {
		return value%2 == 0
	})
	if !removed {
		t.Fatalf("DeleteMatching should report removal")
	}
	if len(next) != 2 || next["a"] != 1 || next["c"] != 3 {
		t.Fatalf("DeleteMatching = %#v, want only odd values", next)
	}
	next["a"] = 99
	if got := original["a"]; got != 1 {
		t.Fatalf("DeleteMatching should not mutate original map, got original[a]=%d", got)
	}
}

func TestDeleteMatchingNoMatchReturnsOriginalUnchanged(t *testing.T) {
	original := map[string]int{"a": 1}
	next, removed := DeleteMatching(original, func(_ string, value int) bool {
		return value == 2
	})
	if removed {
		t.Fatalf("DeleteMatching should report no removal")
	}
	next["a"] = 2
	if got := original["a"]; got != 2 {
		t.Fatalf("DeleteMatching no-op should leave map unchanged and shared, got original[a]=%d", got)
	}
}

func TestDeleteMatchingAllMatchesReturnsNil(t *testing.T) {
	next, removed := DeleteMatching(map[string]int{"a": 1}, func(string, int) bool {
		return true
	})
	if !removed {
		t.Fatalf("DeleteMatching should report removal")
	}
	if next != nil {
		t.Fatalf("DeleteMatching last entry = %#v, want nil", next)
	}
}
