package recursion

import "testing"

func TestRecursionGuardEnterLimit(t *testing.T) {
	t.Parallel()

	guard := NewGuard(1)

	next, ok := guard.Enter()
	if !ok {
		t.Fatalf("expected first enter to succeed")
	}

	next, ok = next.Enter()
	if !ok {
		t.Fatalf("expected second enter to succeed")
	}

	if _, ok = next.Enter(); ok {
		t.Fatalf("expected third enter to fail at max depth")
	}
}
