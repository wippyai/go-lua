package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
)

func TestNewEffect(t *testing.T) {
	onReturn := []Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}}
	onTrue := []Constraint{Truthy{Path: Path{Root: "$0"}}}
	onFalse := []Constraint{Falsy{Path: Path{Root: "$0"}}}

	e := NewEffect(onReturn, onTrue, onFalse)
	if e == nil {
		t.Fatal("NewEffect returned nil")
	}

	if len(e.OnReturn.MustConstraints()) != 1 {
		t.Errorf("expected OnReturn len 1, got %d", len(e.OnReturn.MustConstraints()))
	}

	if len(e.OnTrue.MustConstraints()) != 1 {
		t.Errorf("expected OnTrue len 1, got %d", len(e.OnTrue.MustConstraints()))
	}

	if len(e.OnFalse.MustConstraints()) != 1 {
		t.Errorf("expected OnFalse len 1, got %d", len(e.OnFalse.MustConstraints()))
	}
}

func TestFunctionEffect_IsEmpty(t *testing.T) {
	var nilEffect *FunctionEffect
	if !nilEffect.IsEmpty() {
		t.Error("nil effect should be empty")
	}

	empty := &FunctionEffect{}
	if !empty.IsEmpty() {
		t.Error("empty effect should be empty")
	}

	nonEmpty := NewEffect(
		[]Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
		nil, nil,
	)
	if nonEmpty.IsEmpty() {
		t.Error("effect with OnReturn should not be empty")
	}
}

func TestFunctionEffect_HasAssertSemantics(t *testing.T) {
	var nilEffect *FunctionEffect
	if nilEffect.HasAssertSemantics() {
		t.Error("nil effect should not have assert semantics")
	}

	assert := NewEffect(
		[]Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
		nil, nil,
	)
	if !assert.HasAssertSemantics() {
		t.Error("effect with OnReturn should have assert semantics")
	}

	predicate := NewEffect(nil,
		[]Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
		nil,
	)
	if predicate.HasAssertSemantics() {
		t.Error("effect with only OnTrue should not have assert semantics")
	}
}

func TestFunctionEffect_HasPredicateSemantics(t *testing.T) {
	var nilEffect *FunctionEffect
	if nilEffect.HasPredicateSemantics() {
		t.Error("nil effect should not have predicate semantics")
	}

	withOnTrue := NewEffect(nil,
		[]Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
		nil,
	)
	if !withOnTrue.HasPredicateSemantics() {
		t.Error("effect with OnTrue should have predicate semantics")
	}

	withOnFalse := NewEffect(nil, nil,
		[]Constraint{NotHasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
	)
	if !withOnFalse.HasPredicateSemantics() {
		t.Error("effect with OnFalse should have predicate semantics")
	}

	assertOnly := NewEffect(
		[]Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
		nil, nil,
	)
	if assertOnly.HasPredicateSemantics() {
		t.Error("effect with only OnReturn should not have predicate semantics")
	}
}

func TestFunctionEffect_Equals(t *testing.T) {
	e1 := NewEffect(
		[]Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
		[]Constraint{Truthy{Path: Path{Root: "$0"}}},
		nil,
	)
	e2 := NewEffect(
		[]Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
		[]Constraint{Truthy{Path: Path{Root: "$0"}}},
		nil,
	)

	if !e1.Equals(e2) {
		t.Error("identical effects should be equal")
	}

	e3 := NewEffect(
		[]Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("number")}},
		nil, nil,
	)
	if e1.Equals(e3) {
		t.Error("different effects should not be equal")
	}

	var nilEffect *FunctionEffect
	if !nilEffect.Equals(nil) {
		t.Error("nil effect should equal nil")
	}

	if nilEffect.Equals(e1) {
		t.Error("nil effect should not equal non-nil")
	}

	if e1.Equals(nilEffect) {
		t.Error("non-nil effect should not equal nil")
	}

	if e1.Equals("not an effect") {
		t.Error("effect should not equal non-effect type")
	}
}

func TestFunctionEffect_Substitute(t *testing.T) {
	e := NewEffect(
		[]Constraint{HasType{Path: Path{Root: "$0"}, Type: narrow.BuiltinTypeKey("string")}},
		[]Constraint{Truthy{Path: Path{Root: "$1"}}},
		nil,
	)

	args := []Path{{Root: "x"}, {Root: "y"}}
	subst := e.Substitute(args)

	if subst == nil {
		t.Fatal("Substitute returned nil")
	}

	// Check OnReturn was substituted
	onReturnList := subst.OnReturn.MustConstraints()
	if len(onReturnList) != 1 {
		t.Fatalf("expected 1 OnReturn constraint, got %d", len(onReturnList))
	}

	if ht, ok := onReturnList[0].(HasType); ok {
		if ht.Path.Root != "x" {
			t.Errorf("expected path root 'x', got %q", ht.Path.Root)
		}
	} else {
		t.Error("expected HasType constraint")
	}

	// Check OnTrue was substituted
	onTrueList := subst.OnTrue.MustConstraints()
	if len(onTrueList) != 1 {
		t.Fatalf("expected 1 OnTrue constraint, got %d", len(onTrueList))
	}

	if tr, ok := onTrueList[0].(Truthy); ok {
		if tr.Path.Root != "y" {
			t.Errorf("expected path root 'y', got %q", tr.Path.Root)
		}
	} else {
		t.Error("expected Truthy constraint")
	}
}

func TestFunctionEffect_SubstituteNil(t *testing.T) {
	var nilEffect *FunctionEffect
	if nilEffect.Substitute([]Path{{Root: "x"}}) != nil {
		t.Error("substituting nil effect should return nil")
	}

	empty := &FunctionEffect{}
	if empty.Substitute([]Path{{Root: "x"}}) != nil {
		t.Error("substituting empty effect should return nil")
	}
}

func TestFunctionEffect_IsRefinementInfo(t *testing.T) {
	e := &FunctionEffect{}
	e.IsRefinementInfo() // Should not panic
}

func TestFunctionEffect_KeysCollectorInfo(t *testing.T) {
	eff := &FunctionEffect{
		OnReturn: FromConstraints(KeyOf{
			Table: ParamPath(0),
			Key:   RetPath(1),
		}),
	}

	paramIdx, returnIdx, ok := eff.KeysCollectorInfo()
	if !ok {
		t.Fatal("expected keys-collector info")
	}
	if paramIdx != 0 {
		t.Fatalf("param index = %d, want 0", paramIdx)
	}
	if returnIdx != 1 {
		t.Fatalf("return index = %d, want 1", returnIdx)
	}
	if got := eff.KeysCollectorParamIndex(); got != 0 {
		t.Fatalf("KeysCollectorParamIndex = %d, want 0", got)
	}
}

func TestFunctionEffect_KeysCollectorInfo_InvalidKeyPath(t *testing.T) {
	eff := &FunctionEffect{
		OnReturn: FromConstraints(KeyOf{
			Table: ParamPath(0),
			Key:   Path{Root: "ret[abc]"},
		}),
	}

	if _, _, ok := eff.KeysCollectorInfo(); ok {
		t.Fatal("expected no keys-collector info for invalid return key path")
	}
	if got := eff.KeysCollectorParamIndex(); got != -1 {
		t.Fatalf("KeysCollectorParamIndex = %d, want -1", got)
	}
}
