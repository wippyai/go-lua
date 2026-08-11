package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestStaticTypeValueSeedsPreserveDuplicateContentMounts(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "duplicate_typevalue.lua", Text: []byte(`
local subject = 1
type Dynamic = typeof(subject)
string(subject)
Dynamic(subject)
`)})
	if err != nil {
		t.Fatal(err)
	}
	q, err := programlower.Lower(programlower.Source{Name: "duplicate_typevalue.lua", Text: []byte(`
local subject = 1
type Dynamic = typeof(subject)
string(subject)
Dynamic(subject)
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	left := sealStaticTypeValueFixture(t, contract, []linkproject.Module{{Name: "left", Program: p}, {Name: "right", Program: q}})
	right := sealStaticTypeValueFixture(t, contract, []linkproject.Module{{Name: "right", Program: q}, {Name: "left", Program: p}})
	if left.TypeValueSeeds().Count() != 4 || right.TypeValueSeeds().Count() != 4 {
		t.Fatal("Static did not retain four mounted occurrences")
	}
	for index := 0; index < 4; index++ {
		l, _ := left.TypeValueSeeds().At(index)
		r, _ := right.TypeValueSeeds().At(index)
		if _, ok := left.TypeValueSeeds().Source(l); !ok {
			t.Fatal("left seed source")
		}
		if _, ok := right.TypeValueSeeds().Source(r); !ok {
			t.Fatal("right seed source")
		}
		if _, ok := left.TypeValueSeeds().RootIdentity(l); !ok {
			t.Fatal("left seed root")
		}
		if _, ok := right.TypeValueSeeds().RootIdentity(r); !ok {
			t.Fatal("right seed root")
		}
	}
}

func sealStaticTypeValueFixture(t testing.TB, contract *target.Contract, modules []linkproject.Module) *staticdomain.Authority {
	t.Helper()
	source, err := link.Seal(&link.Spec{Target: contract, Modules: modules})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(source)
	if !ok {
		t.Fatal("typeauthority seal")
	}
	statics, _, err := staticdomain.Seal(source, types)
	if err != nil {
		t.Fatal(err)
	}
	heaps, ok := heap.Seal(source)
	if !ok {
		t.Fatal("heap seal")
	}
	if _, ok := New(statics, heaps); !ok {
		t.Fatal("typevalue seal")
	}
	return statics
}
