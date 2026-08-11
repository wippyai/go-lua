package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestSealPublishesTotalStaticTypeValueDispositions(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "class_runtime_plan.lua", Text: []byte(`
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
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "class_runtime_plan", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(source)
	if !ok {
		t.Fatal("type authority")
	}
	statics, runtime, err := Seal(source, types)
	if err != nil {
		t.Fatal(err)
	}
	if runtime == nil || statics.TypeValueSeeds().Count() != 2 {
		t.Fatal("Static TypeValue totality")
	}
	exact, other := 0, 0
	seeds := statics.TypeValueSeeds()
	for index := 0; index < seeds.Count(); index++ {
		seed, ok := seeds.At(index)
		if !ok {
			t.Fatal("seed")
		}
		if _, ok := seeds.Source(seed); !ok {
			t.Fatal("source")
		}
		if _, ok := seeds.Identity(seed); !ok {
			t.Fatal("identity")
		}
		if inner, ok := seeds.ExactInner(seed); ok {
			exact++
			if !runtime.Equal(inner, inner) {
				t.Fatal("foreign exact inner")
			}
		} else {
			other++
		}
	}
	if exact != 1 || other != 1 {
		t.Fatalf("Static TypeValue dispositions = %d/%d, want 1/1", exact, other)
	}
}

func TestTypeValueInvalidAndBottomResultsAreExplicitOther(t *testing.T) {
	authority := &Authority{results: []resultRow{{kind: KindBottom}, {kind: KindTop}, {kind: KindInvalid, fault: FaultUnknown}}}
	set := &ClassSet{authority: authority}
	// Neither nonconcrete branch consults Runtime after admission.
	runtime := &typeauthority.Runtime{}
	for _, index := range []uint32{0, 2} {
		if inner, exact, err := set.typeValueExactInner(Value{owner: authority, index: index}, runtime); err != nil || exact || inner != (typeauthority.RuntimeInner{}) {
			t.Fatalf("nonconcrete %d = %v/%v/%v", index, inner, exact, err)
		}
	}
}
