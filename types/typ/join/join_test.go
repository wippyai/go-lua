package join

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestTwo(t *testing.T) {
	t.Run("nil left", func(t *testing.T) {
		if Two(nil, typ.String) != typ.String {
			t.Error("nil left should return right")
		}
	})

	t.Run("nil right", func(t *testing.T) {
		if Two(typ.String, nil) != typ.String {
			t.Error("nil right should return left")
		}
	})

	t.Run("unknown left", func(t *testing.T) {
		if Two(typ.Unknown, typ.String) != typ.String {
			t.Error("unknown left should return right")
		}
	})

	t.Run("unknown right", func(t *testing.T) {
		if Two(typ.String, typ.Unknown) != typ.String {
			t.Error("unknown right should return left")
		}
	})

	t.Run("equal types", func(t *testing.T) {
		if Two(typ.String, typ.String) != typ.String {
			t.Error("equal types should return same")
		}
	})

	t.Run("different types", func(t *testing.T) {
		result := Two(typ.String, typ.Number)
		union, ok := result.(*typ.Union)
		if !ok {
			t.Fatal("different types should create union")
		}
		if len(union.Members) != 2 {
			t.Errorf("expected 2 members, got %d", len(union.Members))
		}
	})
}

func TestReturnVectors(t *testing.T) {
	t.Run("empty left", func(t *testing.T) {
		right := []typ.Type{typ.String}
		result := ReturnVectors(nil, right)
		if len(result) != 1 || result[0] != typ.String {
			t.Error("empty left should return right")
		}
	})

	t.Run("empty right", func(t *testing.T) {
		left := []typ.Type{typ.String}
		result := ReturnVectors(left, nil)
		if len(result) != 1 || result[0] != typ.String {
			t.Error("empty right should return left")
		}
	})

	t.Run("same length", func(t *testing.T) {
		left := []typ.Type{typ.String}
		right := []typ.Type{typ.Number}
		result := ReturnVectors(left, right)
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
		if _, ok := result[0].(*typ.Union); !ok {
			t.Error("should create union")
		}
	})

	t.Run("different lengths", func(t *testing.T) {
		left := []typ.Type{typ.String, typ.Number}
		right := []typ.Type{typ.Boolean}
		result := ReturnVectors(left, right)
		if len(result) != 2 {
			t.Errorf("expected 2, got %d", len(result))
		}
	})
}

func TestIsUnknownOrNil(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil", nil, true},
		{"Unknown", typ.Unknown, true},
		{"Nil type", typ.Nil, true},
		{"String", typ.String, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnknownOrNil(tt.t); got != tt.want {
				t.Errorf("IsUnknownOrNil() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithReturns(t *testing.T) {
	t.Run("nil sig", func(t *testing.T) {
		if WithReturns(nil, []typ.Type{typ.String}) != nil {
			t.Error("nil sig should return nil")
		}
	})

	t.Run("graft returns", func(t *testing.T) {
		sig := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
		result := WithReturns(sig, []typ.Type{typ.Boolean})
		if len(result.Returns) != 1 || result.Returns[0] != typ.Boolean {
			t.Error("should graft new returns")
		}
		if len(result.Params) != 1 || result.Params[0].Type != typ.String {
			t.Error("should preserve params")
		}
	})

	t.Run("normalize nil returns", func(t *testing.T) {
		sig := typ.Func().Build()
		result := WithReturns(sig, []typ.Type{nil, typ.String})
		if result.Returns[0] != typ.Unknown {
			t.Error("nil should be normalized to Unknown")
		}
	})
}
