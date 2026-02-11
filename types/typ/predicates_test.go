package typ

import "testing"

func TestIsUnknown(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "unknown", in: Unknown, want: true},
		{name: "any", in: Any, want: false},
		{name: "nil type", in: Nil, want: false},
		{name: "string", in: String, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnknown(tt.in); got != tt.want {
				t.Fatalf("IsUnknown(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsAbsentOrUnknown(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: true},
		{name: "unknown", in: Unknown, want: true},
		{name: "nil type", in: Nil, want: false},
		{name: "any", in: Any, want: false},
		{name: "number", in: Number, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAbsentOrUnknown(tt.in); got != tt.want {
				t.Fatalf("IsAbsentOrUnknown(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
