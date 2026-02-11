package siblings

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeSiblingType_NilPrev(t *testing.T) {
	next := typ.Number
	result := MergeSiblingType(nil, next)
	if result != next {
		t.Errorf("expected next when prev is nil, got %v", result)
	}
}

func TestMergeSiblingType_NilNext(t *testing.T) {
	prev := typ.String
	result := MergeSiblingType(prev, nil)
	if result != prev {
		t.Errorf("expected prev when next is nil, got %v", result)
	}
}

func TestMergeSiblingType_BothNil(t *testing.T) {
	result := MergeSiblingType(nil, nil)
	if result != nil {
		t.Errorf("expected nil when both are nil, got %v", result)
	}
}

func TestMergeSiblingType_PrefersFunctionWithReturns(t *testing.T) {
	prev := typ.Func().Build()
	next := typ.Func().Returns(typ.String).Build()
	result := MergeSiblingType(prev, next)
	fn, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("expected function result, got %T", result)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("expected function with string return, got %v", result)
	}
}

func TestMergeSiblingType_PrefersAliasFunctionWithReturns(t *testing.T) {
	prev := typ.NewAlias("FnPrev", typ.Func().Build())
	next := typ.NewAlias("FnNext", typ.Func().Returns(typ.String).Build())
	result := MergeSiblingType(prev, next)
	if !typ.TypeEquals(result, next) {
		t.Fatalf("expected alias-backed function with returns to be preferred, got %v", result)
	}
}
