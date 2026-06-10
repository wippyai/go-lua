package symbol

import "testing"

func TestIDZero(t *testing.T) {
	var id ID
	if id != 0 {
		t.Fatalf("zero ID = %d, want 0", id)
	}
}

func TestNextReturnsDistinctIDs(t *testing.T) {
	a := Next()
	b := Next()
	if a == 0 || b == 0 {
		t.Fatalf("Next returned zero IDs: %d %d", a, b)
	}
	if a == b {
		t.Fatalf("Next returned duplicate IDs: %d", a)
	}
}

func TestReserve(t *testing.T) {
	if got := Reserve(0); got != 0 {
		t.Fatalf("Reserve(0) = %d, want 0", got)
	}
	start := Reserve(3)
	if start == 0 {
		t.Fatal("Reserve(3) returned zero")
	}
	if ID(uint64(start)+2) == 0 {
		t.Fatal("reserved range overflowed to zero")
	}
}

func TestKindValues(t *testing.T) {
	tests := []struct {
		kind Kind
		want int
	}{
		{Unknown, 0},
		{Param, 1},
		{Local, 2},
		{Global, 3},
		{Upvalue, 4},
	}
	for _, tt := range tests {
		if int(tt.kind) != tt.want {
			t.Errorf("Kind %v = %d, want %d", tt.kind, tt.kind, tt.want)
		}
	}
}
