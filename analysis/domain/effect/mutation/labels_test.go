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

func TestTransformEqualsNormalizesPointers(t *testing.T) {
	tr := ToArray{Element: effect.ParamRef{Index: 0}}

	if !transformEquals(&tr, tr) {
		t.Error("pointer transform on left should equal value transform")
	}
	if !transformEquals(tr, &tr) {
		t.Error("value transform on left should equal pointer transform")
	}
	if !transformEquals(&tr, &tr) {
		t.Error("pointer transforms on both sides should be equal")
	}
}

func TestTransformEqualsHandlesTypedNilPointers(t *testing.T) {
	var nilElement *ElementUnion
	var nilContainer *ContainerElementUnion
	var nilArray *ToArray
	var nilUnchanged *Unchanged

	if !isNilTypeTransform(nilElement) {
		t.Fatal("typed nil ElementUnion should be nil-like")
	}
	if !isNilTypeTransform(nilContainer) {
		t.Fatal("typed nil ContainerElementUnion should be nil-like")
	}
	if !isNilTypeTransform(nilArray) {
		t.Fatal("typed nil ToArray should be nil-like")
	}
	if !isNilTypeTransform(nilUnchanged) {
		t.Fatal("typed nil Unchanged should be nil-like")
	}
	if !transformEquals(nilElement, nilContainer) {
		t.Fatal("nil-like transforms should compare equal")
	}
	if transformEquals(nilElement, ElementUnion{}) {
		t.Fatal("typed nil ElementUnion should not equal concrete ElementUnion")
	}
	if transformEquals(Unchanged{}, nilUnchanged) {
		t.Fatal("concrete Unchanged should not equal typed nil Unchanged")
	}
	if transformEquals(ToArray{}, nilArray) {
		t.Fatal("concrete ToArray should not equal typed nil ToArray")
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
	r := effect.Row{Labels: []effect.Label{Mutate{
		Target:    effect.ParamRef{Index: 0},
		Transform: ElementUnion{Source: effect.ParamRef{Index: 1}},
	}}}

	if !r.Has(func(l effect.Label) bool {
		_, ok := l.(Mutate)
		return ok
	}) {
		t.Error("Should have mutation")
	}

	got, ok := effect.NormalizeLabel(r.Labels[0]).(Mutate)
	if !ok {
		t.Fatal("Should normalize mutation label")
	}
	if got.Target.Index != 0 {
		t.Errorf("Mutation target index = %d, want 0", got.Target.Index)
	}
}

func TestTableMutatorEffects(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{TableMutator{
		Target: effect.ParamRef{Index: 0},
		Value:  effect.ParamRef{Index: 1},
	}}}

	if !r.Has(func(l effect.Label) bool {
		_, ok := l.(TableMutator)
		return ok
	}) {
		t.Error("Should have table mutator")
	}

	mut, ok := effect.NormalizeLabel(r.Labels[0]).(TableMutator)
	if !ok {
		t.Fatal("Should normalize table mutator label")
	}
	if mut.Target.Index != 0 || mut.Value.Index != 1 {
		t.Error("Should find normalized table mutator")
	}

	if effect.Empty.Has(func(l effect.Label) bool {
		_, ok := l.(TableMutator)
		return ok
	}) {
		t.Error("Empty row should not have table mutator")
	}
}

func TestPathInvalidationTargetProjectsOnlyActiveMutationAuthority(t *testing.T) {
	tests := []struct {
		name  string
		label effect.Label
		want  int
	}{
		{
			name: "mutate ignores transform payload",
			label: Mutate{
				Target: effect.ParamRef{Index: 0},
				Transform: ContainerElementUnion{
					Container: effect.ParamRef{Index: 1},
					Value:     effect.ParamRef{Index: 2},
				},
				LengthDelta: expr.C(3),
			},
			want: 0,
		},
		{
			name:  "table mutator ignores value payload",
			label: TableMutator{Target: effect.ParamRef{Index: 1}, Value: effect.ParamRef{Index: 2}},
			want:  1,
		},
		{
			name:  "length change target invalidates regardless of delta sign",
			label: LengthChange{Target: effect.ParamRef{Index: 2}, Delta: -1},
			want:  2,
		},
		{
			name:  "pointer label normalizes",
			label: &TableMutator{Target: effect.ParamRef{Index: 3}, Value: effect.ParamRef{Index: 4}},
			want:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PathInvalidationTarget(tt.label)
			if !ok {
				t.Fatalf("PathInvalidationTarget(%T) ok = false, want true", tt.label)
			}
			if got.Index != tt.want {
				t.Fatalf("PathInvalidationTarget(%T) = %v, want param[%d]", tt.label, got, tt.want)
			}
		})
	}

	if got, ok := PathInvalidationTarget(returns.Return{}); ok || got.Index != 0 {
		t.Fatalf("PathInvalidationTarget(non-mutation) = %v/%v, want zero false", got, ok)
	}
}

func TestPositiveLengthFloorProjectsOnlyPositiveLengthChange(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	tests := []struct {
		name      string
		label     effect.Label
		wantFloor int
		wantOK    bool
	}{
		{
			name:      "positive length change",
			label:     LengthChange{Target: p0, Delta: 2},
			wantFloor: 2,
			wantOK:    true,
		},
		{
			name:      "pointer length change",
			label:     &LengthChange{Target: p0, Delta: 3},
			wantFloor: 3,
			wantOK:    true,
		},
		{
			name:   "zero length change is metadata",
			label:  LengthChange{Target: p0, Delta: 0},
			wantOK: false,
		},
		{
			name:   "negative length change is metadata",
			label:  LengthChange{Target: p0, Delta: -1},
			wantOK: false,
		},
		{
			name: "mutate length delta is metadata",
			label: Mutate{
				Target:      p0,
				Transform:   Unchanged{},
				LengthDelta: expr.C(4),
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, floor, ok := PositiveLengthFloor(tt.label)
			if ok != tt.wantOK {
				t.Fatalf("PositiveLengthFloor(%T) ok = %v, want %v", tt.label, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if target.Index != p0.Index || floor != tt.wantFloor {
				t.Fatalf("PositiveLengthFloor(%T) = %v/%d, want %v/%d", tt.label, target, floor, p0, tt.wantFloor)
			}
		})
	}
}
