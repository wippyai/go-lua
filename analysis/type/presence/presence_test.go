package presence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestAbsentOrUnknown(t *testing.T) {
	tests := []struct {
		name string
		in   typ.Type
		want bool
	}{
		{name: "nil", in: nil, want: true},
		{name: "unknown", in: typ.Unknown, want: true},
		{name: "nil type", in: typ.Nil, want: false},
		{name: "any", in: typ.Any, want: false},
		{name: "number", in: typ.Number, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AbsentOrUnknown(tt.in); got != tt.want {
				t.Fatalf("AbsentOrUnknown(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestHasKnown(t *testing.T) {
	tests := []struct {
		name string
		in   []typ.Type
		want bool
	}{
		{name: "nil slice", in: nil, want: false},
		{name: "unknown only", in: []typ.Type{typ.Unknown, nil}, want: false},
		{name: "includes nil type", in: []typ.Type{typ.Unknown, typ.Nil}, want: true},
		{name: "includes concrete", in: []typ.Type{typ.Unknown, typ.String}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasKnown(tt.in); got != tt.want {
				t.Fatalf("HasKnown(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestUnknownOrNil(t *testing.T) {
	tests := []struct {
		name string
		in   typ.Type
		want bool
	}{
		{name: "nil", in: nil, want: true},
		{name: "unknown", in: typ.Unknown, want: true},
		{name: "nil type", in: typ.Nil, want: true},
		{name: "any", in: typ.Any, want: false},
		{name: "number", in: typ.Number, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UnknownOrNil(tt.in); got != tt.want {
				t.Fatalf("UnknownOrNil(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestUnknownOnlyOrEmpty(t *testing.T) {
	tests := []struct {
		name string
		in   []typ.Type
		want bool
	}{
		{name: "nil slice", in: nil, want: true},
		{name: "unknown only", in: []typ.Type{typ.Unknown, nil}, want: true},
		{name: "has concrete", in: []typ.Type{typ.Unknown, typ.Number}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UnknownOnlyOrEmpty(tt.in); got != tt.want {
				t.Fatalf("UnknownOnlyOrEmpty(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
