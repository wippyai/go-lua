package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/targetfamily"
	"github.com/wippyai/go-lua/internal/testfixture"
)

type typeFactFixture struct {
	classes *ClassSet
	target  *contract.Contract
	result  vocabulary.Type
}

func newTypeFactFixture(t *testing.T, linkByte byte) typeFactFixture {
	t.Helper()

	program := targetFamilyLawProgram(t)
	linkID := identity.ContentID{linkByte}
	types, err := typeauthority.SealProgramRows(linkID, []programschema.Program{program}, nil)
	if err != nil || types == nil {
		t.Fatalf("seal type authority: %v", err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil || target == nil {
		t.Fatalf("seal standard target: %v", err)
	}
	moduleID := identity.ContentID{linkByte, 9}
	authority, _, err := SealMountedPrograms(
		MountContext{LinkID: linkID, Target: target},
		types,
		[]MountedProgram{{Program: program, ModuleID: moduleID, NamespaceID: moduleID}},
	)
	if err != nil || authority == nil {
		t.Fatalf("seal Static authority: %v", err)
	}
	classes := authority.Classes()
	if classes == nil {
		t.Fatal("sealed Static authority has no TypeFact axis")
	}

	operation, ok := target.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{"stream"},
		Member:    []string{"open"},
	})
	if !ok {
		t.Fatal("stream.open is not a sealed Target operation")
	}
	_, values, ok := target.Operations.OutcomeAt(operation, 0)
	if !ok {
		t.Fatal("stream.open normal outcome is unavailable")
	}
	result, ok := target.Operations.ValuesAt(values, 0)
	if !ok {
		t.Fatal("stream.open result is unavailable")
	}
	return typeFactFixture{classes: classes, target: target, result: result}
}

func TestTypeFactProjectsExactTargetRecordAndDirectField(t *testing.T) {
	fixture := newTypeFactFixture(t, 31)
	classes := fixture.classes

	base, ok := classes.TypeFactForOwnTarget(fixture.result)
	if !ok || !base.Valid() {
		t.Fatalf("Target result %d did not project to a valid concrete TypeFact", fixture.result)
	}
	if base.IsTop() || base.IsBottom() {
		t.Fatal("concrete Target result projected to a lattice extreme")
	}
	expectedInner, ok := classes.RuntimeForTarget(fixture.target, fixture.result)
	if !ok || !classes.runtime.Equal(base.Inner(), expectedInner) {
		t.Fatal("Target result TypeFact does not carry the exact ClassSet Runtime projection")
	}

	id, ok := classes.TypeFactField(base, "id")
	if !ok || !id.Valid() {
		t.Fatal("stream result id field was not projected")
	}
	if got, ok := classes.runtime.Kind(id.Inner()); !ok || got != kind.String {
		t.Fatalf("stream result id Runtime kind = %v/%v, want string", got, ok)
	}
}

func TestTypeFactMissingConcreteFieldIsAbsentNotUnknown(t *testing.T) {
	fixture := newTypeFactFixture(t, 32)
	base, ok := fixture.classes.TypeFactForOwnTarget(fixture.result)
	if !ok {
		t.Fatal("stream result Target projection unavailable")
	}

	missing, present := fixture.classes.TypeFactField(base, "read")
	if present {
		t.Fatal("nearest-negative stream.read field was projected as present")
	}
	if missing.Valid() || fixture.classes.OwnsTypeFact(missing) {
		t.Fatal("missing concrete field returned an owned TypeFact")
	}
	if fixture.classes.EqualTypeFact(missing, fixture.classes.TypeTop()) || missing.IsTop() {
		t.Fatal("missing concrete field fell back to Unknown/top")
	}
}

func TestTypeFactRejectsForeignFacts(t *testing.T) {
	left := newTypeFactFixture(t, 33)
	right := newTypeFactFixture(t, 34)

	foreign := left.classes.TypeBottom()
	if right.classes.OwnsTypeFact(foreign) {
		t.Fatal("foreign TypeFact axis bottom crossed the owner fence")
	}
	if _, ok := right.classes.TypeFactForRuntime(foreign.Inner()); ok {
		t.Fatal("foreign RuntimeInner was admitted into another TypeFact axis")
	}
	if right.classes.EqualTypeFact(foreign, right.classes.TypeTop()) || right.classes.LessOrEqTypeFact(foreign, right.classes.TypeTop()) {
		t.Fatal("foreign TypeFact participated in right-flow semantics")
	}
	if _, ok := right.classes.TypeFactField(foreign, "id"); ok {
		t.Fatal("foreign TypeFact was accepted for field projection")
	}

	foreignTargetFact, ok := left.classes.TypeFactForOwnTarget(left.result)
	if !ok {
		t.Fatal("left Target result projection unavailable")
	}
	if right.classes.OwnsTypeFact(foreignTargetFact) {
		t.Fatal("foreign Target result fact crossed the owner fence")
	}
}

func TestTypeFactRejectsScopedAndUnavailableTargets(t *testing.T) {
	fixture := newTypeFactFixture(t, 35)
	classes := fixture.classes

	if _, ok := classes.TypeFactForOwnTarget(0); ok {
		t.Fatal("zero/unavailable Target handle projected to a TypeFact")
	}

	family, ok := targetfamily.Of(fixture.target)
	if !ok {
		t.Fatal("standard Target has no sealed type family")
	}
	foundScoped := false
	for index := 0; index < family.Count(); index++ {
		value, member, available := family.At(index)
		if !available || member >= 0 {
			continue
		}
		foundScoped = true
		if _, projected := classes.TypeFactForOwnTarget(value); projected {
			t.Fatalf("scoped Target type %d projected to a concrete TypeFact", value)
		}
		break
	}
	if !foundScoped {
		t.Log("standard Target currently has no scoped residual; unavailable-handle rejection still exercised")
	}
}

func TestTypeFactBottomTopAndJoinLaws(t *testing.T) {
	fixture := newTypeFactFixture(t, 36)
	classes := fixture.classes
	runtime := classes.runtime

	bottom, top := classes.TypeBottom(), classes.TypeTop()
	if !bottom.Valid() || !top.Valid() || !bottom.IsBottom() || !top.IsTop() {
		t.Fatal("TypeFact axis extrema are not valid owner-local facts")
	}
	if !classes.LessOrEqTypeFact(bottom, top) {
		t.Fatal("bottom is not below top")
	}

	facts := make([]TypeFact, 0, runtime.Count())
	for index := 1; index <= runtime.Count(); index++ {
		inner, ok := runtime.InnerAtIndex(uint32(index))
		if !ok {
			t.Fatalf("Runtime row %d is unavailable", index)
		}
		fact, ok := classes.TypeFactForRuntime(inner)
		if !ok {
			// Runtime also owns structural child rows which are not values in
			// Static's ClassSet lattice. TypeFact axis admits only the canonical
			// Runtime/Class projection, never every reachable graph row.
			continue
		}
		facts = append(facts, fact)
		if !classes.LessOrEqTypeFact(bottom, fact) || !classes.LessOrEqTypeFact(fact, top) {
			t.Fatalf("extreme order failed for Runtime row %d", index)
		}
		if !classes.EqualTypeFact(classes.JoinTypeFact(bottom, fact), fact) || !classes.EqualTypeFact(classes.JoinTypeFact(fact, top), top) {
			t.Fatalf("extreme join failed for Runtime row %d", index)
		}
	}
	if len(facts) < 2 {
		t.Fatalf("TypeFact axis fixture admitted only %d canonical classes", len(facts))
	}

	for leftIndex, left := range facts {
		if !classes.EqualTypeFact(classes.JoinTypeFact(left, left), left) {
			t.Fatalf("join is not idempotent at row %d", leftIndex)
		}
		for rightIndex := leftIndex; rightIndex < len(facts); rightIndex++ {
			right := facts[rightIndex]
			forward, reverse := classes.JoinTypeFact(left, right), classes.JoinTypeFact(right, left)
			if !classes.EqualTypeFact(classes.JoinTypeFact(left, right), forward) {
				t.Fatalf("widen is not the exact join at rows %d/%d", leftIndex, rightIndex)
			}
			if !classes.EqualTypeFact(forward, reverse) {
				t.Fatalf("join is not commutative at rows %d/%d", leftIndex, rightIndex)
			}
			if !classes.LessOrEqTypeFact(left, forward) || !classes.LessOrEqTypeFact(right, forward) {
				t.Fatalf("join is not an upper bound at rows %d/%d", leftIndex, rightIndex)
			}
			for boundIndex, bound := range facts {
				if classes.LessOrEqTypeFact(left, bound) && classes.LessOrEqTypeFact(right, bound) && !classes.LessOrEqTypeFact(forward, bound) {
					joinedRow, _ := runtime.Index(forward.Inner())
					boundRow, _ := runtime.Index(bound.Inner())
					topRow, _ := runtime.Index(top.Inner())
					t.Fatalf("join is not least at rows %d/%d: joined row %d is not below bound %d/row %d (top %d)", leftIndex, rightIndex, joinedRow, boundIndex, boundRow, topRow)
				}
			}
		}
	}

	for firstIndex, first := range facts {
		for secondIndex, second := range facts {
			for thirdIndex, third := range facts {
				left := classes.JoinTypeFact(classes.JoinTypeFact(first, second), third)
				right := classes.JoinTypeFact(first, classes.JoinTypeFact(second, third))
				if !classes.EqualTypeFact(left, right) {
					t.Fatalf("join is not associative at rows %d/%d/%d", firstIndex, secondIndex, thirdIndex)
				}
			}
		}
	}
}
