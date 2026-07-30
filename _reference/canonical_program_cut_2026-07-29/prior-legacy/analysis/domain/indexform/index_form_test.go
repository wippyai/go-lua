package indexform

import (
	"math"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestIndexFormPreservesNestedArrayPath(t *testing.T) {
	array := pathdom.NewPath(symbol.ID(7), "actor").Field("mailbox").Field("queue")
	form, ok := NewModuloLengthIndex(array)
	if !ok || !form.Valid() || form.Kind() != IndexFormModuloLength {
		t.Fatalf("NewModuloLengthIndex = %#v/%v", form, ok)
	}
	got, ok := form.ArrayPath()
	if !ok || !got.Equal(array) {
		t.Fatalf("ArrayPath = %v/%v, want %s", got, ok, array.String())
	}
	// Returned paths are reconstructed from the comparable stable key and are
	// therefore isolated from the sealed descriptor.
	got.Segments[0] = segment.Segment{Kind: segment.SegmentField, Name: "changed"}
	again, _ := form.ArrayPath()
	if !again.Equal(array) {
		t.Fatalf("caller mutation changed sealed path: %s", again.String())
	}
	if _, exists := map[IndexForm]struct{}{form: {}}[form]; !exists {
		t.Fatal("sealed form is not usable as a comparable query identity")
	}
}

func TestIndexFormRejectsInvalidAffineTerms(t *testing.T) {
	path := pathdom.NewPath(symbol.ID(8), "i")
	for _, coeff := range []int64{0, -1, math.MinInt64} {
		if form, ok := NewAffineIndex(path, coeff, 0); ok || form.Valid() {
			t.Fatalf("coefficient %d produced valid form %#v", coeff, form)
		}
	}
	if form, ok := NewAffineIndex(pathdom.Path{}, 1, 0); ok || form.Valid() {
		t.Fatalf("empty path produced valid form %#v", form)
	}
}

func TestCheckedIndexArithmeticRejectsOverflow(t *testing.T) {
	for _, test := range []struct {
		name        string
		left, right int64
		multiply    bool
	}{
		{name: "add positive", left: math.MaxInt64, right: 1},
		{name: "add negative", left: math.MinInt64, right: -1},
		{name: "multiply positive", left: math.MaxInt64, right: 2, multiply: true},
		{name: "multiply minimum", left: math.MinInt64, right: -1, multiply: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var ok bool
			if test.multiply {
				_, ok = CheckedMulInt64(test.left, test.right)
			} else {
				_, ok = CheckedAddInt64(test.left, test.right)
			}
			if ok {
				t.Fatal("overflowing operation accepted")
			}
		})
	}
}
