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

func TestIsAny(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "any", in: Any, want: true},
		{name: "unknown", in: Unknown, want: false},
		{name: "number", in: Number, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAny(tt.in); got != tt.want {
				t.Fatalf("IsAny(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsNever(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "never", in: Never, want: true},
		{name: "unknown", in: Unknown, want: false},
		{name: "string", in: String, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNever(tt.in); got != tt.want {
				t.Fatalf("IsNever(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestAbsentOrUnknown(t *testing.T) {
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
			if got := AbsentOrUnknown(tt.in); got != tt.want {
				t.Fatalf("AbsentOrUnknown(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestUnknownOrNil(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: true},
		{name: "unknown", in: Unknown, want: true},
		{name: "nil type", in: Nil, want: true},
		{name: "any", in: Any, want: false},
		{name: "number", in: Number, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UnknownOrNil(tt.in); got != tt.want {
				t.Fatalf("UnknownOrNil(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestHasKnown(t *testing.T) {
	tests := []struct {
		name string
		in   []Type
		want bool
	}{
		{name: "nil slice", in: nil, want: false},
		{name: "unknown only", in: []Type{Unknown, nil}, want: false},
		{name: "includes nil type", in: []Type{Unknown, Nil}, want: true},
		{name: "includes concrete", in: []Type{Unknown, String}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasKnown(tt.in); got != tt.want {
				t.Fatalf("HasKnown(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestUnknownOnlyOrEmpty(t *testing.T) {
	tests := []struct {
		name string
		in   []Type
		want bool
	}{
		{name: "nil slice", in: nil, want: true},
		{name: "unknown only", in: []Type{Unknown, nil}, want: true},
		{name: "has concrete", in: []Type{Unknown, Number}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UnknownOnlyOrEmpty(tt.in); got != tt.want {
				t.Fatalf("UnknownOnlyOrEmpty(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
