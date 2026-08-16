package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestStaticTypeValueSeedsPreserveDuplicateContentMounts(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "duplicate_typevalue.lua", Text: []byte(`
local subject = 1
string(subject)
string(subject)
`)})
	if err != nil {
		t.Fatal(err)
	}
	q, err := programlower.Lower(programlower.Source{Name: "duplicate_typevalue.lua", Text: []byte(`
local subject = 1
string(subject)
string(subject)
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	leftSource := sealTypeValueLink(t, contract, []linkproject.Module{{Name: "left", Program: p}, {Name: "right", Program: q}})
	rightSource := sealTypeValueLink(t, contract, []linkproject.Module{{Name: "right", Program: q}, {Name: "left", Program: p}})
	leftStatics, leftHeaps := sealTypeValueAuthorities(t, leftSource, contract)
	rightStatics, rightHeaps := sealTypeValueAuthorities(t, rightSource, contract)
	left, leftOK := New(leftStatics, leftHeaps)
	right, rightOK := New(rightStatics, rightHeaps)
	if !leftOK || !rightOK || left == nil || right == nil {
		t.Fatal("typevalue seal")
	}
	if left.SeedCount() != 4 || right.SeedCount() != 4 {
		t.Fatal("Static did not retain four mounted occurrences")
	}
	for index := 0; index < 4; index++ {
		l, lOK := left.SeedAt(index)
		r, rOK := right.SeedAt(index)
		if !lOK || !rOK {
			t.Fatal("mounted seed")
		}
		if _, ok := left.SeedID(l); !ok {
			t.Fatal("left seed identity")
		}
		if _, ok := right.SeedID(r); !ok {
			t.Fatal("right seed identity")
		}
		if _, ok := left.SeedRoot(l); !ok {
			t.Fatal("left seed root")
		}
		if _, ok := right.SeedRoot(r); !ok {
			t.Fatal("right seed root")
		}
	}
}

func sealTypeValueLink(t testing.TB, contract *target.Contract, modules []linkproject.Module) *link.Link {
	t.Helper()
	source, err := link.Seal(&link.Spec{Target: contract, Modules: modules})
	if err != nil {
		t.Fatal(err)
	}
	return source
}
