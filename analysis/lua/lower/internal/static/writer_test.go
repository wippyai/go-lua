package static

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestStaticWriterTracksIndependentChildRanges(t *testing.T) {
	w := New(nil, nil, nil, nil, nil, nil, nil, "static.lua")
	if w.Mark() != 0 || w.FieldMark() != 0 || w.InterfaceMemberMark() != 0 || !w.Clean() {
		t.Fatal("new static writer did not start with empty ranges")
	}
	if err := w.Append(17); err != nil {
		t.Fatal(err)
	}
	mark := 0
	term, err := w.Take(mark)
	if err != nil || term != 17 {
		t.Fatalf("Take(%d) = %d, %v; want 17", mark, term, err)
	}
	if !w.Clean() {
		t.Fatal("completed child range remained in static scratch")
	}
}

func TestStaticWriterRejectsZeroChildren(t *testing.T) {
	var w Writer
	if err := w.Append(0); err == nil {
		t.Fatal("Append accepted a zero static child")
	}
	if _, err := w.Take(0); err == nil {
		t.Fatal("Take accepted an empty child range")
	}
	if err := w.AppendInterfaceMethod("", ast.Position{}, 1); err == nil {
		t.Fatal("AppendInterfaceMethod accepted an unnamed member")
	}
}
