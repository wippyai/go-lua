package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestIndexLiteralEqualityConstraints_NilValue(t *testing.T) {
	result := IndexLiteralEqualityConstraints(Path{Root: "x"}, typ.String, nil)
	if len(result) != 1 {
		t.Error("should return single constraint even with nil value")
	}
}

func TestIndexLiteralEqualityConstraints_LiteralKey(t *testing.T) {
	key := typ.LiteralString("key")
	value := typ.LiteralString("value")
	result := IndexLiteralEqualityConstraints(Path{Root: "x"}, key, value)
	if len(result) != 1 {
		t.Errorf("literal key should produce 1 constraint, got %d", len(result))
	}
}

func TestIndexLiteralEqualityConstraints_UnionKey(t *testing.T) {
	key := typ.NewUnion(typ.LiteralString("a"), typ.LiteralString("b"))
	value := typ.LiteralString("value")
	result := IndexLiteralEqualityConstraints(Path{Root: "x"}, key, value)
	if len(result) != 2 {
		t.Errorf("union key with 2 literals should produce 2 constraints, got %d", len(result))
	}
}

func TestIndexPathEqualityConstraints_LiteralKeyEquals(t *testing.T) {
	key := typ.LiteralString("key")
	target := Path{Root: "x"}
	value := Path{Root: "y"}
	result := IndexPathEqualityConstraints(target, key, value, true)
	if len(result) != 1 {
		t.Errorf("literal key equals should produce 1 constraint, got %d", len(result))
	}
	if _, ok := result[0].(IndexEqualsPath); !ok {
		t.Fatalf("constraint = %T, want IndexEqualsPath", result[0])
	}
}

func TestIndexPathEqualityConstraints_LiteralKeyNotEquals(t *testing.T) {
	key := typ.LiteralString("key")
	target := Path{Root: "x"}
	value := Path{Root: "y"}
	result := IndexPathEqualityConstraints(target, key, value, false)
	if len(result) != 1 {
		t.Errorf("literal key not equals should produce 1 constraint, got %d", len(result))
	}
	if _, ok := result[0].(IndexNotEqualsPath); !ok {
		t.Fatalf("constraint = %T, want IndexNotEqualsPath", result[0])
	}
}
