package mutation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
)

func TestMutate_String(t *testing.T) {
	m := Mutate{
		Target:    effect.ParamRef{Index: 0},
		Transform: Unchanged{},
	}

	got := m.String()
	if got != "mutate(param[0], unchanged)" {
		t.Errorf("Mutate.String() = %q", got)
	}

	mWithDelta := Mutate{
		Target:      effect.ParamRef{Index: 0},
		Transform:   ElementUnion{Source: effect.ParamRef{Index: 1}},
		LengthDelta: expr.C(1),
	}

	got = mWithDelta.String()
	if got != "mutate(param[0], union_elem(param[1]), delta=1)" {
		t.Errorf("Mutate with delta.String() = %q", got)
	}
}

func TestMutate_Equals(t *testing.T) {
	m1 := Mutate{Target: effect.ParamRef{Index: 0}, Transform: Unchanged{}}
	m2 := Mutate{Target: effect.ParamRef{Index: 0}, Transform: Unchanged{}}
	m3 := Mutate{Target: effect.ParamRef{Index: 1}, Transform: Unchanged{}}

	if !m1.Equals(m2) {
		t.Error("same Mutates should be equal")
	}

	if m1.Equals(m3) {
		t.Error("different target Mutates should not be equal")
	}

	if m1.Equals(returns.Return{}) {
		t.Error("Mutate should not equal Return")
	}

	m4 := Mutate{Target: effect.ParamRef{Index: 0}, Transform: ElementUnion{Source: effect.ParamRef{Index: 1}}}
	if m1.Equals(m4) {
		t.Error("different transform Mutates should not be equal")
	}

	m5 := Mutate{Target: effect.ParamRef{Index: 0}, Transform: Unchanged{}, LengthDelta: expr.C(1)}
	m6 := Mutate{Target: effect.ParamRef{Index: 0}, Transform: Unchanged{}, LengthDelta: expr.C(2)}

	if m5.Equals(m6) {
		t.Error("different LengthDelta Mutates should not be equal")
	}

	m7 := Mutate{Target: effect.ParamRef{Index: 0}, Transform: Unchanged{}, LengthDelta: expr.C(1)}
	if !m5.Equals(m7) {
		t.Error("same LengthDelta Mutates should be equal")
	}
}

func TestTransformEqualsPointerLeftOnly(t *testing.T) {
	tr := ToArray{Element: effect.ParamRef{Index: 0}}

	if !transformEquals(&tr, tr) {
		t.Error("pointer transform on left should equal value transform")
	}
	if transformEquals(tr, &tr) {
		t.Error("value transform on left should not equal pointer transform")
	}
	if transformEquals(&tr, &tr) {
		t.Error("pointer transforms on both sides should not be equal")
	}
}

func TestElementUnion_String(t *testing.T) {
	e := ElementUnion{Source: effect.ParamRef{Index: 1}}
	if got := e.String(); got != "union_elem(param[1])" {
		t.Errorf("ElementUnion.String() = %q", got)
	}
}

func TestContainerElementUnion_String(t *testing.T) {
	c := ContainerElementUnion{Container: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: 1}}
	if got := c.String(); got != "union_elem(param[0], param[1])" {
		t.Errorf("ContainerElementUnion.String() = %q", got)
	}
}

func TestToArray_String(t *testing.T) {
	ta := ToArray{Element: effect.ParamRef{Index: 0}}
	if got := ta.String(); got != "to_array(param[0])" {
		t.Errorf("ToArray.String() = %q", got)
	}
}

func TestUnchanged_String(t *testing.T) {
	u := Unchanged{}
	if got := u.String(); got != "unchanged" {
		t.Errorf("Unchanged.String() = %q", got)
	}
}

func TestLengthChange_String(t *testing.T) {
	lc := LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1}
	if got := lc.String(); got != "len(param[0]) += 1" {
		t.Errorf("LengthChange positive.String() = %q", got)
	}

	lcNeg := LengthChange{Target: effect.ParamRef{Index: 0}, Delta: -1}
	if got := lcNeg.String(); got != "len(param[0]) -= 1" {
		t.Errorf("LengthChange negative.String() = %q", got)
	}
}

func TestLengthChange_Equals(t *testing.T) {
	lc1 := LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1}
	lc2 := LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1}
	lc3 := LengthChange{Target: effect.ParamRef{Index: 1}, Delta: 1}

	if !lc1.Equals(lc2) {
		t.Error("same LengthChanges should be equal")
	}

	if lc1.Equals(lc3) {
		t.Error("different target LengthChanges should not be equal")
	}

	lc4 := LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 2}
	if lc1.Equals(lc4) {
		t.Error("different Delta LengthChanges should not be equal")
	}

	lc5 := LengthChange{Target: effect.ParamRef{Index: 0}, Delta: -1}
	if lc1.Equals(lc5) {
		t.Error("positive and negative Delta should not be equal")
	}
}

func TestTableMutator(t *testing.T) {
	tm := TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: 1}}
	if got := tm.String(); got != "table_mutator(param[0], param[1])" {
		t.Errorf("TableMutator.String() = %q", got)
	}

	if !tm.Equals(TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: 1}}) {
		t.Error("same TableMutator should be equal")
	}

	if tm.Equals(TableMutator{Target: effect.ParamRef{Index: 1}, Value: effect.ParamRef{Index: 1}}) {
		t.Error("different target should not be equal")
	}

	if tm.Equals(TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: 2}}) {
		t.Error("different value should not be equal")
	}

	if tm.Equals(returns.Return{}) {
		t.Error("TableMutator should not equal Return")
	}
}

func TestLabelInterface(t *testing.T) {
	labels := []effect.Label{
		Mutate{},
		LengthChange{},
		TableMutator{},
	}

	for _, l := range labels {
		_ = l.String()
		_ = l.Equals(l)
	}
}

func TestTransformInterface(t *testing.T) {
	transforms := []TypeTransform{
		ElementUnion{},
		ContainerElementUnion{},
		ToArray{},
		Unchanged{},
	}

	for _, tr := range transforms {
		_ = tr.String()
	}
}

func TestMarkerMethods(t *testing.T) {
	Mutate{}.EffectLabel()
	LengthChange{}.EffectLabel()
	TableMutator{}.EffectLabel()

	ElementUnion{}.transform()
	ContainerElementUnion{}.transform()
	ToArray{}.transform()
	Unchanged{}.transform()
}

func TestMutateEffect(t *testing.T) {
	m := Mutates(0, ElementUnion{Source: effect.ParamRef{Index: 1}})

	if !HasMutate(m) {
		t.Error("Should have mutation")
	}

	got := GetMutate(m, 0)
	if got == nil {
		t.Error("Should find mutation for param 0")
	}

	if GetMutate(m, 1) != nil {
		t.Error("Should not find mutation for param 1")
	}
}

func TestTableMutatorEffects(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{TableMutator{
		Target: effect.ParamRef{Index: 0},
		Value:  effect.ParamRef{Index: 1},
	}}}

	if !HasTableMutator(r) {
		t.Error("Should have table mutator")
	}

	mut := GetTableMutator(r)
	if mut == nil {
		t.Error("Should find table mutator")
	}

	if GetTableMutator(effect.Empty) != nil {
		t.Error("Empty row should not have table mutator")
	}
}
