package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestEntrySeedPlanIsDetachedAndMissingOnly(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	firstSlot := key.SymbolValue(symbol.ID(1))
	secondSlot := key.SymbolValue(symbol.ID(2))
	firstSeed := presentValue(reg)
	secondSeed := absentValue(reg)
	actual := product.Top()

	source := []ValueSeed{{Slot: firstSlot, Value: firstSeed}}
	plan := NewEntrySeedPlan(source)
	source[0] = ValueSeed{Slot: secondSlot, Value: secondSeed}
	clone := plan.Clone()

	entry := State{}.WriteValue(reg, secondSlot, actual)
	for name, candidate := range map[string]EntrySeedPlan{"plan": plan, "clone": clone} {
		if !candidate.Valid() || candidate.Len() != 1 || candidate.Empty() {
			t.Fatalf("%s size = %d/%t, want 1/non-empty", name, candidate.Len(), candidate.Empty())
		}
		seeded, err := candidate.Apply(reg, entry)
		if err != nil {
			t.Fatal(err)
		}
		if got := seeded.ReadValue(reg, firstSlot); !valueDomain.Equal(got, firstSeed) {
			t.Fatalf("%s detached seed = %s, want original seed", name, formatValue(reg, got))
		}
		if got := seeded.ReadValue(reg, secondSlot); !valueDomain.Equal(got, actual) {
			t.Fatalf("%s replaced route value = %s, want preserved actual", name, formatValue(reg, got))
		}
	}
}

func TestEmptyEntrySeedPlanIsIdentity(t *testing.T) {
	reg := standard.Registry()
	entry := State{}.WriteValue(reg, key.SymbolValue(symbol.ID(1)), presentValue(reg))
	plan := NewEntrySeedPlan(nil)
	if !plan.Valid() || !plan.Empty() || plan.Len() != 0 {
		t.Fatalf("empty plan = %d/%t", plan.Len(), plan.Empty())
	}
	if got, err := plan.Apply(reg, entry); err != nil || !Domain(reg).Equal(got, entry) {
		t.Fatal("empty entry-seed plan changed State")
	}
	if (EntrySeedPlan{}).Valid() {
		t.Fatal("zero entry-seed plan unexpectedly has prepared authority")
	}
}

func TestEntrySeedPlanSlotsAreDetachedSortedAndDeduplicated(t *testing.T) {
	first := key.SymbolValue(symbol.ID(1))
	second := key.ReturnSlot(0)
	source := []ValueSeed{
		{Slot: second},
		{Slot: 0},
		{Slot: first},
		{Slot: second},
	}
	plan := NewEntrySeedPlan(source)
	source[0].Slot = key.SymbolValue(symbol.ID(99))

	got := plan.Slots()
	want := []key.Value{first, second}
	if len(got) != len(want) {
		t.Fatalf("seed slots = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("seed slots[%d] = %d, want %d", index, got[index], want[index])
		}
	}

	got[0] = key.SymbolValue(symbol.ID(100))
	cloneSlots := plan.Clone().Slots()
	if len(cloneSlots) != len(want) || cloneSlots[0] != want[0] || cloneSlots[1] != want[1] {
		t.Fatalf("mutating returned inventory changed plan: %v", cloneSlots)
	}
	if slots := (EntrySeedPlan{}).Slots(); slots != nil {
		t.Fatalf("invalid entry-seed plan slots = %v, want nil", slots)
	}
}

func TestEntrySeedPlanValuesForSlotsUsesRequestedTupleOrder(t *testing.T) {
	reg := standard.Registry()
	first := key.SymbolValue(symbol.ID(1))
	second := key.SymbolValue(symbol.ID(2))
	firstValue := presentValue(reg)
	secondValue := absentValue(reg)
	plan := NewEntrySeedPlan([]ValueSeed{{Slot: first, Value: firstValue}, {Slot: second, Value: secondValue}})

	got, ok := plan.ValuesForSlots([]key.Value{second, first})
	if !ok || len(got) != 2 || !product.Equal(reg, got[0], secondValue) || !product.Equal(reg, got[1], firstValue) {
		t.Fatalf("ordered values = %#v/%v", got, ok)
	}
	if got, ok := plan.ValuesForSlots([]key.Value{first, key.SymbolValue(symbol.ID(3))}); ok || got != nil {
		t.Fatalf("missing-slot values = %#v/%v, want fail closed", got, ok)
	}
	if got, ok := (EntrySeedPlan{}).ValuesForSlots(nil); ok || got != nil {
		t.Fatalf("invalid-plan values = %#v/%v, want fail closed", got, ok)
	}
}

func TestEntrySeedFactorPlanIsTheConcreteMissingOnlyLaw(t *testing.T) {
	reg := standard.Registry()
	first := key.SymbolValue(symbol.ID(1))
	second := key.SymbolValue(symbol.ID(2))
	third := key.ReturnSlot(0)
	seedFirst := presentValue(reg)
	seedSecond := absentValue(reg)
	actual := product.Top()
	plan := NewEntrySeedPlan([]ValueSeed{
		{Slot: first, Value: seedFirst},
		{Slot: second, Value: seedSecond},
		{Slot: third, Value: seedFirst},
	})

	type formalSlot uint8
	addresses := map[key.Value]formalSlot{first: 7, second: 3, third: 9}
	factorPlan, err := BindEntrySeedFactorPlan(reg, plan, func(slot key.Value) (formalSlot, bool) {
		value, ok := addresses[slot]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	input := ValueFactor[formalSlot]{Values: map[formalSlot]product.Value{addresses[second]: actual}}
	got, err := factorPlan.Apply(reg, input)
	if err != nil {
		t.Fatal(err)
	}
	if value := got.Values[addresses[first]]; !product.Equal(reg, value, seedFirst) {
		t.Fatalf("first seed = %#v", value)
	}
	if value := got.Values[addresses[second]]; !product.Equal(reg, value, actual) {
		t.Fatalf("route actual was replaced = %#v", value)
	}
	if value := got.Values[addresses[third]]; !product.Equal(reg, value, seedFirst) {
		t.Fatalf("return seed = %#v", value)
	}

	concrete := State{}.WriteValue(reg, second, actual)
	concrete, err = plan.Apply(reg, concrete)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		slot key.Value
		want product.Value
	}{{first, seedFirst}, {second, actual}, {third, seedFirst}} {
		if value := concrete.ReadValue(reg, item.slot); !product.Equal(reg, value, item.want) {
			t.Fatalf("concrete slot %d = %#v, want %#v", item.slot, value, item.want)
		}
	}
}

func TestEntrySeedFactorPlanRejectsIncompleteAndAliasedVocabulary(t *testing.T) {
	reg := standard.Registry()
	first := key.SymbolValue(symbol.ID(1))
	second := key.SymbolValue(symbol.ID(2))
	plan := NewEntrySeedPlan([]ValueSeed{{Slot: first, Value: presentValue(reg)}, {Slot: second, Value: absentValue(reg)}})
	if _, err := BindEntrySeedFactorPlan(reg, plan, func(slot key.Value) (uint8, bool) {
		return uint8(slot), slot == first
	}); err == nil {
		t.Fatal("incomplete factor vocabulary was accepted")
	}
	if _, err := BindEntrySeedFactorPlan(reg, plan, func(key.Value) (uint8, bool) { return 1, true }); err == nil {
		t.Fatal("aliased factor vocabulary was accepted")
	}
	if (EntrySeedFactorPlan[uint8]{}).Valid() {
		t.Fatal("zero factor plan is valid")
	}
}

func TestEntrySeedFactorPlanMatchesConcreteEdgeMatrix(t *testing.T) {
	reg := standard.Registry()
	slot := key.SymbolValue(symbol.ID(11))
	bottom := product.Bottom(reg)
	first := presentValue(reg)
	second := absentValue(reg)
	for _, test := range []struct {
		name  string
		seeds []ValueSeed
		input ValueLaneFactor
	}{
		{name: "duplicate bottom then value", seeds: []ValueSeed{{Slot: slot, Value: bottom}, {Slot: slot, Value: first}}},
		{name: "duplicate first wins", seeds: []ValueSeed{{Slot: slot, Value: first}, {Slot: slot, Value: second}}},
		{name: "bottom seed canonicalizes bottom", seeds: []ValueSeed{{Slot: slot, Value: bottom}}, input: ValueLaneFactor{Values: map[key.Value]product.Value{slot: bottom}}},
		{name: "explicit bottom canonicalized", seeds: []ValueSeed{{Slot: slot, Value: first}}, input: ValueLaneFactor{Values: map[key.Value]product.Value{slot: bottom}}},
		{name: "actual preserved", seeds: []ValueSeed{{Slot: slot, Value: first}}, input: ValueLaneFactor{Values: map[key.Value]product.Value{slot: second}}},
		{name: "top fixed point", seeds: []ValueSeed{{Slot: slot, Value: first}}, input: ValueLaneFactor{Top: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := NewEntrySeedPlan(test.seeds)
			factorPlan, err := BindEntrySeedFactorPlan(reg, plan, func(value key.Value) (key.Value, bool) { return value, value != 0 })
			if err != nil {
				t.Fatal(err)
			}
			gotFactor, err := factorPlan.Apply(reg, test.input)
			if err != nil {
				t.Fatal(err)
			}
			entry := RecomposeValueLane(reg, Domain(reg), Domain(reg).Bottom(), test.input)
			gotState, err := plan.Apply(reg, entry)
			if err != nil {
				t.Fatal(err)
			}
			_, gotConcrete := DecomposeValueLane(Domain(reg), gotState)
			if !ValueFactorLattice[key.Value](reg).Equal(gotFactor, gotConcrete) {
				t.Fatalf("factor/concrete = %#v/%#v", gotFactor, gotConcrete)
			}
			current := bottom
			if test.input.Top {
				current = product.Top()
			} else if value, present := test.input.Values[slot]; present {
				current = value
			}
			gotCoordinate, err := factorPlan.ApplyValue(reg, slot, current)
			if err != nil {
				t.Fatal(err)
			}
			wantCoordinate := product.Top()
			if !gotFactor.Top {
				wantCoordinate = bottom
				if value, present := gotFactor.Values[slot]; present {
					wantCoordinate = value
				}
			}
			if !product.Equal(reg, gotCoordinate, wantCoordinate) {
				t.Fatalf("sparse/full coordinate = %#v/%#v", gotCoordinate, wantCoordinate)
			}
		})
	}

	foreign, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	foreignValue := presentValue(foreign)
	if _, err := BindEntrySeedFactorPlan(reg,
		NewEntrySeedPlan([]ValueSeed{{Slot: slot, Value: foreignValue}}),
		func(value key.Value) (key.Value, bool) { return value, true },
	); err == nil {
		t.Fatal("foreign seed registry was accepted")
	}
	plan, err := BindEntrySeedFactorPlan(reg,
		NewEntrySeedPlan([]ValueSeed{{Slot: slot, Value: first}}),
		func(value key.Value) (key.Value, bool) { return value, true },
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(reg, ValueLaneFactor{Values: map[key.Value]product.Value{slot: foreignValue}}); err == nil {
		t.Fatal("foreign input registry was accepted")
	}
}
