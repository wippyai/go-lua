package stringlib

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestCaptureTypes(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    []typ.Type
	}{
		{name: "none", pattern: "%d+", want: nil},
		{name: "string captures", pattern: "(%w+)=(%d+)", want: []typ.Type{typ.String, typ.String}},
		{name: "position capture", pattern: "()id=(%d+)", want: []typ.Type{typ.Integer, typ.String}},
		{name: "escaped paren", pattern: "%(%w+%)", want: nil},
		{name: "class paren", pattern: "[()](%w+)", want: []typ.Type{typ.String}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CaptureTypes(tt.pattern)
			if len(got) != len(tt.want) {
				t.Fatalf("capture count = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if !typ.TypeEquals(got[i], tt.want[i]) {
					t.Fatalf("capture %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMatchReturnTypes(t *testing.T) {
	for _, test := range []struct {
		pattern string
		want    []typ.Type
	}{
		{pattern: "^__", want: []typ.Type{normalize.Optional(typ.String)}},
		{pattern: "()(%w+)", want: []typ.Type{normalize.Optional(typ.Integer), normalize.Optional(typ.String)}},
	} {
		if got := MatchReturnTypes(test.pattern); !sameTypes(got, test.want) {
			t.Fatalf("MatchReturnTypes(%q) = %v, want %v", test.pattern, got, test.want)
		}
	}
}

func TestStringLibraryReturnShapes(t *testing.T) {
	captureValue := typeexpr.Union(typ.String, typ.Integer)

	byteFn, ok := Method("byte")
	if !ok {
		t.Fatal("byte signature missing")
	}
	if got, want := byteFn.Returns, []typ.Type{normalize.Optional(typ.Integer)}; !sameTypes(got, want) {
		t.Fatalf("byte returns = %v, want %v", got, want)
	}

	matchFn, ok := Method("match")
	if !ok {
		t.Fatal("match signature missing")
	}
	if got, want := matchFn.Returns, []typ.Type{normalize.Optional(captureValue)}; !sameTypes(got, want) {
		t.Fatalf("match returns = %v, want %v", got, want)
	}

	gsubFn, ok := Method("gsub")
	if !ok {
		t.Fatal("gsub signature missing")
	}
	if got, want := gsubFn.Returns, []typ.Type{typ.String, typ.Integer}; !sameTypes(got, want) {
		t.Fatalf("gsub returns = %v, want %v", got, want)
	}

	dumpFn, ok := Method("dump")
	if !ok {
		t.Fatal("dump signature missing")
	}
	if got, want := dumpFn.Returns, []typ.Type{typ.Never}; !sameTypes(got, want) {
		t.Fatalf("dump returns = %v, want %v", got, want)
	}
}

func TestGMatchIteratorTypes(t *testing.T) {
	got := GMatchIterator([]typ.Type{typ.Integer, typ.String})
	want := typ.Func().
		Returns(normalize.Optional(typ.Integer), normalize.Optional(typ.String)).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("literal iterator = %v, want %v", got, want)
	}

	noCapture := GMatchIterator(nil)
	wantNoCapture := typ.Func().
		Returns(normalize.Optional(typ.String)).
		Build()
	if !typ.TypeEquals(noCapture, wantNoCapture) {
		t.Fatalf("no-capture iterator = %v, want %v", noCapture, wantNoCapture)
	}

	general := GeneralGMatchIterator()
	if len(general.Returns) != generalCaptureReturnSlots {
		t.Fatalf("general iterator return count = %d, want %d", len(general.Returns), generalCaptureReturnSlots)
	}
	wantGeneralSlot := normalize.Optional(typeexpr.Union(typ.String, typ.Integer))
	for i, got := range general.Returns {
		if !typ.TypeEquals(got, wantGeneralSlot) {
			t.Fatalf("general iterator slot %d = %v, want %v", i, got, wantGeneralSlot)
		}
	}
}

func sameTypes(got, want []typ.Type) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !typ.TypeEquals(got[i], want[i]) {
			return false
		}
	}
	return true
}
