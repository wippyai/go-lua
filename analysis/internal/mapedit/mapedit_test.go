package mapedit

import "testing"

func TestCloneNilInputReturnsNil(t *testing.T) {
	if got := Clone(map[string]int(nil)); got != nil {
		t.Fatalf("Clone(nil) = %#v, want nil", got)
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
