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
