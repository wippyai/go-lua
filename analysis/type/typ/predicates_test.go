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

func TestTopLikePredicates(t *testing.T) {
	tests := []struct {
		name          string
		in            Type
		wantTop       bool
		wantAbsentTop bool
	}{
		{name: "nil", in: nil, wantTop: false, wantAbsentTop: true},
		{name: "any", in: Any, wantTop: true, wantAbsentTop: true},
		{name: "unknown", in: Unknown, wantTop: true, wantAbsentTop: true},
		{name: "nil type", in: Nil, wantTop: false, wantAbsentTop: false},
		{name: "never", in: Never, wantTop: false, wantAbsentTop: false},
		{name: "string", in: String, wantTop: false, wantAbsentTop: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTopLike(tt.in); got != tt.wantTop {
				t.Fatalf("IsTopLike(%v) = %v, want %v", tt.in, got, tt.wantTop)
			}
			if got := AbsentOrTopLike(tt.in); got != tt.wantAbsentTop {
				t.Fatalf("AbsentOrTopLike(%v) = %v, want %v", tt.in, got, tt.wantAbsentTop)
			}
		})
	}
}

func TestAdmitsFalse(t *testing.T) {
	ann := []annotation.Annotation{{Name: "source"}}
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "false literal", in: False, want: true},
		{name: "true literal", in: True, want: false},
		{name: "boolean", in: Boolean, want: true},
		{name: "nil type", in: Nil, want: false},
		{name: "string", in: String, want: false},
		{name: "any", in: Any, want: false},
		{name: "unknown", in: Unknown, want: false},
		{name: "alias to false", in: NewAlias("F", False), want: true},
		{name: "annotated false", in: NewAnnotated(False, ann), want: true},
		{name: "optional false", in: MaterializeOptional(False), want: true},
		{name: "union with false", in: MaterializeUnion([]Type{String, False}), want: true},
		{name: "union without false", in: MaterializeUnion([]Type{String, Number}), want: false},
		{name: "intersection retains false", in: MaterializeIntersection([]Type{Boolean, False}), want: true},
		{name: "intersection excludes false", in: MaterializeIntersection([]Type{Boolean, String}), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AdmitsFalse(tt.in); got != tt.want {
				t.Fatalf("AdmitsFalse(%v) = %v, want %v", tt.in, got, tt.want)
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

func TestIsBooleanType(t *testing.T) {
	ann := []annotation.Annotation{{Name: "flag"}}
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "boolean", in: Boolean, want: true},
		{name: "true literal", in: LiteralBool(true), want: true},
		{name: "false literal", in: LiteralBool(false), want: true},
		{name: "alias to boolean", in: NewAlias("Flag", Boolean), want: true},
		{name: "annotated boolean", in: NewAnnotated(Boolean, ann), want: true},
		{name: "optional boolean", in: MaterializeOptional(Boolean), want: false},
		{name: "boolean literal union", in: MaterializeUnion([]Type{LiteralBool(true), LiteralBool(false)}), want: true},
		{name: "mixed union", in: MaterializeUnion([]Type{LiteralBool(true), String}), want: false},
		{name: "intersection with boolean", in: MaterializeIntersection([]Type{String, Boolean}), want: true},
		{name: "intersection without boolean", in: MaterializeIntersection([]Type{String, Number}), want: false},
		{name: "any", in: Any, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBooleanType(tt.in); got != tt.want {
				t.Fatalf("IsBooleanType(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

const predicateTraversalLawDepth = 12_288

func TestPredicatesDeepAcyclicLaws(t *testing.T) {
	tests := []struct {
		name      string
		predicate func(Type) bool
		positive  Type
		negative  Type
	}{
		{
			name:      "admits false",
			predicate: AdmitsFalse,
			positive:  False,
			negative:  True,
		},
		{
			name:      "boolean",
			predicate: IsBooleanType,
			positive:  Boolean,
			negative:  String,
		},
		{
			name:      "integer index",
			predicate: IsIntegerIndexType,
			positive:  Integer,
			negative:  Number,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.predicate(deepPredicateAnnotations(tt.positive)) {
				t.Fatal("deep positive graph was rejected")
			}
			if tt.predicate(deepPredicateAnnotations(tt.negative)) {
				t.Fatal("deep negative graph was accepted")
			}
		})
	}
}

func TestPredicatesDeepCyclicLaws(t *testing.T) {
	admitsPositive := &Union{Members: make([]Type, 2)}
	admitsPositive.Members[0] = admitsPositive
	admitsPositive.Members[1] = False
	admitsNegative := &Intersection{Members: make([]Type, 2)}
	admitsNegative.Members[0] = admitsNegative
	admitsNegative.Members[1] = Boolean

	booleanPositive := &Intersection{Members: make([]Type, 2)}
	booleanPositive.Members[0] = booleanPositive
	booleanPositive.Members[1] = Boolean
	booleanNegative := &Union{Members: make([]Type, 2)}
	booleanNegative.Members[0] = booleanNegative
	booleanNegative.Members[1] = Boolean

	integerPositive := &Intersection{Members: make([]Type, 2)}
	integerPositive.Members[0] = integerPositive
	integerPositive.Members[1] = Integer
	integerNegative := &Union{Members: make([]Type, 2)}
	integerNegative.Members[0] = integerNegative
	integerNegative.Members[1] = Integer

	tests := []struct {
		name      string
		predicate func(Type) bool
		positive  Type
		negative  Type
	}{
		{
			name:      "admits false",
			predicate: AdmitsFalse,
			positive:  admitsPositive,
			negative:  admitsNegative,
		},
		{
			name:      "boolean",
			predicate: IsBooleanType,
			positive:  booleanPositive,
			negative:  booleanNegative,
		},
		{
			name:      "integer index",
			predicate: IsIntegerIndexType,
			positive:  integerPositive,
			negative:  integerNegative,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.predicate(deepPredicateAnnotations(tt.positive)) {
				t.Fatal("deep cyclic positive graph was rejected")
			}
			if tt.predicate(deepPredicateAnnotations(tt.negative)) {
				t.Fatal("deep cyclic negative graph was accepted")
			}
		})
	}
}

func deepPredicateAnnotations(base Type) Type {
	var current Type = base
	for range predicateTraversalLawDepth {
		current = &Annotated{Inner: current}
	}
	return current
}
