package symbol

import "testing"

func TestIDZero(t *testing.T) {
	var id ID
	if id != 0 {
		t.Fatalf("zero ID = %d, want 0", id)
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
		{Function, 5},
	}
	for _, tt := range tests {
		if int(tt.kind) != tt.want {
			t.Errorf("Kind %v = %d, want %d", tt.kind, tt.kind, tt.want)
		}
	}
}
