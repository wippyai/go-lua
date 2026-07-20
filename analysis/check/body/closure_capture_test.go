package body

import (
	"testing"
)

func requireClosureCaptureFact(t *testing.T, result *Result, name string) ClosureCaptureFact {
	t.Helper()
	var found ClosureCaptureFact
	var ok bool
	result.ForEachClosureCaptureFact(func(fact ClosureCaptureFact) bool {
		if fact.Name == name {
			found = fact
			ok = true
			return false
		}
		return true
	})
	if !ok {
		t.Fatalf("closure capture %q not found", name)
	}
	return found
}
