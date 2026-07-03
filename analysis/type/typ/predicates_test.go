package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
)

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

func TestIsIntegerIndexType(t *testing.T) {
	ann := []annotation.Annotation{{Name: "min"}}
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "integer", in: Integer, want: true},
		{name: "integer literal", in: LiteralInt(2), want: true},
		{name: "number", in: Number, want: false},
		{name: "number literal", in: LiteralNumber(2), want: false},
		{name: "string literal", in: LiteralString("2"), want: false},
		{name: "alias to integer", in: NewAlias("Index", Integer), want: true},
		{name: "annotated integer", in: NewAnnotated(Integer, ann), want: true},
		{name: "optional integer", in: MaterializeOptional(Integer), want: false},
		{name: "integer literal union", in: MaterializeUnion([]Type{LiteralInt(1), LiteralInt(2)}), want: true},
		{name: "mixed union", in: MaterializeUnion([]Type{LiteralInt(1), String}), want: false},
		{name: "intersection with integer", in: MaterializeIntersection([]Type{String, Integer}), want: true},
		{name: "intersection without integer", in: MaterializeIntersection([]Type{String, Boolean}), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsIntegerIndexType(tt.in); got != tt.want {
				t.Fatalf("IsIntegerIndexType(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
