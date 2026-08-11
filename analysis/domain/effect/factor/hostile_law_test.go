package factor

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/call"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type effectHostileFixture struct {
	contract    *target.Contract
	linked      *link.Link
	packs       *pack.Schema
	calls       *call.Algebra
	factor      *Algebra
	owner       target.Operation
	callback    target.CallbackID
	application linkproject.Application
	root        Root
}

func newEffectHostileFixture(t *testing.T, spec target.Spec, source string) effectHostileFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "effect_factor_hostile.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "effect_factor_hostile", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("seal type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	packs, ok := pack.Seal(linked, statics)
	if !ok {
		t.Fatal("seal Pack")
	}
	factor, ok := New(linked, packs, contract)
	if !ok {
		t.Fatal("seal Effect factor")
	}
	calls, ok := call.New(linked)
	if !ok {
		t.Fatal("seal Call factor")
	}
	owner, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"sink"}})
	if !ok {
		t.Fatal("sink operation")
	}
	callback, _ := contract.CallbackAt(owner, 0)
	application, root := effectApplicationRoot(t, factor, linked, 0)
	return effectHostileFixture{contract: contract, linked: linked, packs: packs, calls: calls, factor: factor, owner: owner, callback: callback, application: application, root: root}
}

func effectApplicationRoot(t *testing.T, algebra *Algebra, linked *link.Link, callIndex int) (linkproject.Application, Root) {
	t.Helper()
	applications := linked.Project().Applications()
	calls := applications.Calls()
	if callIndex < 0 || callIndex >= calls.Count() {
		t.Fatalf("call %d unavailable", callIndex)
	}
	application, ok := calls.At(callIndex)
	if !ok {
		t.Fatalf("Call application %d", callIndex)
	}
	shard, callTerm, ok := applications.Call(application)
	if !ok {
		t.Fatal("call occurrence")
	}
	mounted, ok := linked.Project().Mounts().Program(shard)
	if !ok || mounted == nil {
		t.Fatal("mounted Program")
	}
	body, _, _, ok := mounted.Source().Index().Position(callTerm)
	if !ok {
		t.Fatal("call body position")
	}
	root, ok := algebra.RootForBody(shard, body)
	if !ok {
		t.Fatal("Effect body root")
	}
	return application, root
}

func effectHostileSpec(duplicates bool, rowTail target.RowTail, rowArgs, typeArgs bool, callback bool, typeConstraint typ.Type) target.Spec {
	var formals []*typ.TypeParam
	var targetFormals []*typ.TypeParam
	if typeArgs {
		formals = []*typ.TypeParam{typ.NewTypeParam("T", typeConstraint)}
		targetFormals = []*typ.TypeParam{typ.NewTypeParam("U", nil)}
	}
	ownerRowFormalCount := 0
	targetRowFormalCount := 0
	var rowVariable target.RowVar
	var ownerRowTail target.RowTail = target.RowClosed
	if rowArgs || rowTail == target.RowVariable {
		ownerRowFormalCount = 1
		rowVariable = 0
	}
	if rowArgs {
		targetRowFormalCount = 1
	}
	if rowTail != 0 {
		ownerRowTail = rowTail
	}
	args := target.EffectSpec{Target: 2, ValueArgs: []target.ValueFormal{0}}
	if typeArgs {
		args.TypeArgs = []target.TypeFormal{0}
	}
	if rowArgs {
		args.RowArgs = []target.RowVar{0}
	}
	occurrences := []target.EffectSpec{args}
	if duplicates {
		occurrences = append(occurrences, args)
	}
	owner := target.OperationSpec{
		Bindings:    []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"sink"}}},
		TypeFormals: formals,
		RowFormals:  uint32(ownerRowFormalCount),
		Input:       target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
		Outcomes:    []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:     target.RowSpec{Occurrences: occurrences, Tail: ownerRowTail, Var: rowVariable},
	}
	if callback {
		empty := target.ValuesSpec{Tail: target.ValuesClosed}
		terminals := []target.TerminalSpec{
			{Kind: flowkind.OutcomeNormal, Values: empty}, {Kind: flowkind.OutcomeReturn, Values: empty},
			{Kind: flowkind.OutcomeThrow, Values: empty}, {Kind: flowkind.OutcomeYield, Values: empty},
			{Kind: flowkind.OutcomeCancel, Values: empty},
		}
		callbackRow := target.RowSpec{Occurrences: []target.EffectSpec{args}, Tail: target.RowClosed}
		owner.Callbacks = []target.CallbackSpec{{
			Function: target.InputSource{Kind: target.InputSourceValueFormal}, Admission: target.OrdinaryCallable,
			Arguments: empty, Outcomes: terminals, Lifecycle: target.CallbackRetainedOptionalOnce, Effects: callbackRow,
		}}
	}
	targetOperation := target.OperationSpec{
		Bindings:    []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"effect-target"}}},
		TypeFormals: targetFormals,
		RowFormals:  uint32(targetRowFormalCount),
		Input:       target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
		Outcomes:    []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:     target.RowSpec{Tail: target.RowClosed},
	}
	return target.Spec{Operations: []target.OperationSpec{owner, targetOperation}}
}

func effectKnownAtom(t *testing.T, fixture effectHostileFixture, effect int) Atom {
	t.Helper()
	atom, ok := fixture.factor.CallEffectAtom(fixture.root, fixture.application, fixture.owner, effect)
	if !ok {
		t.Fatalf("known ordinary effect %d unavailable", effect)
	}
	return atom
}

func effectAtomID(t *testing.T, algebra *Algebra, atom Atom) keyspace.ContentID {
	t.Helper()
	id, ok := algebra.AtomID(atom)
	if !ok || !id.Available() {
		t.Fatal("atom identity unavailable")
	}
	return id
}

func TestEffectFactorKnownOrdinaryCallbackAndDuplicateQuotient(t *testing.T) {
	fixture := newEffectHostileFixture(t, effectHostileSpec(true, target.RowClosed, false, false, true, nil), `local function sink(value) return value end
sink(1)`)
	first := effectKnownAtom(t, fixture, 0)
	second := effectKnownAtom(t, fixture, 1)
	firstID := effectAtomID(t, fixture.factor, first)
	secondID := effectAtomID(t, fixture.factor, second)
	if firstID != secondID {
		t.Fatal("duplicate effect descriptors did not quotient to one atom")
	}
	if fixture.callback == 0 {
		t.Fatal("callback fixture missing callback")
	}
	callbackAtom, ok := fixture.factor.CallbackEffectAtom(fixture.root, fixture.application, fixture.owner, fixture.callback, 0)
	if !ok {
		t.Fatal("known callback effect unavailable")
	}
	callbackID := effectAtomID(t, fixture.factor, callbackAtom)
	if callbackID != firstID {
		t.Fatal("ordinary and callback descriptors with equal substitution diverged")
	}
}

func TestEffectFactorOpenRowKnownAndUnknownWitness(t *testing.T) {
	fixture := newEffectHostileFixture(t, effectHostileSpec(false, target.RowClosed, false, false, false, nil), `local function sink(value) return value end
sink(1)`)
	known := effectKnownAtom(t, fixture, 0)
	opaque, ok := fixture.contract.Opaque()
	if !ok {
		t.Fatal("opaque operation unavailable")
	}
	unknown, ok := fixture.factor.OpenOperationUnknown(fixture.root, fixture.application, opaque)
	if !ok {
		t.Fatal("opaque open-row witness unavailable")
	}
	callKey, ok := fixture.calls.KeyForApplication(fixture.application)
	if !ok {
		t.Fatal("Call application key unavailable")
	}
	callValue, ok := fixture.calls.DispatchValue(callKey, nil, true)
	if !ok || !callValue.HasOpaqueAlternative() {
		t.Fatal("Call did not retain its opaque alternative")
	}
	callUnknown, ok := fixture.factor.OpaqueCallUnknown(fixture.root, fixture.calls, fixture.application, callValue)
	if !ok {
		t.Fatal("opaque Call evidence did not issue UnknownExternal")
	}
	knownID := effectAtomID(t, fixture.factor, known)
	unknownID := effectAtomID(t, fixture.factor, unknown)
	if knownID == unknownID {
		t.Fatal("known and UnknownExternal atoms collided")
	}
	if callUnknownID := effectAtomID(t, fixture.factor, callUnknown); callUnknownID != unknownID {
		t.Fatal("opaque Call and open-row evidence did not share UnknownExternal")
	}
	knownValue, ok := fixture.factor.Singleton(known)
	if !ok {
		t.Fatal("known singleton")
	}
	unknownValue, ok := fixture.factor.Singleton(unknown)
	if !ok {
		t.Fatal("unknown singleton")
	}
	joined, ok := fixture.factor.Join(knownValue, unknownValue)
	if !ok || !fixture.factor.LessOrEq(knownValue, joined) || !fixture.factor.LessOrEq(unknownValue, joined) {
		t.Fatal("known and UnknownExternal did not form one may-set")
	}
}

func TestEffectFactorRejectsRowVariableRowArgumentsAndTypeCorrespondence(t *testing.T) {
	rowVariable := newEffectHostileFixture(t, effectHostileSpec(false, target.RowVariable, false, false, false, nil), `local function sink(value) return value end
sink(1)`)
	if _, ok := rowVariable.factor.CallEffectAtom(rowVariable.root, rowVariable.application, rowVariable.owner, 0); ok {
		t.Fatal("RowVariable effect row was admitted as a known atom")
	}

	rowArguments := newEffectHostileFixture(t, effectHostileSpec(false, target.RowClosed, true, false, false, nil), `local function sink(value) return value end
sink(1)`)
	if _, ok := rowArguments.factor.CallEffectAtom(rowArguments.root, rowArguments.application, rowArguments.owner, 0); ok {
		t.Fatal("effect RowArgs substitution was admitted")
	}

	unavailableTypes := newEffectHostileFixture(t, effectHostileSpec(false, target.RowClosed, false, true, false, nil), `local function sink(value) return value end
sink(1)`)
	if _, ok := unavailableTypes.factor.CallEffectAtom(unavailableTypes.root, unavailableTypes.application, unavailableTypes.owner, 0); ok {
		t.Fatal("missing type-formal correspondence was admitted")
	}

	constrainedTypes := newEffectHostileFixture(t, effectHostileSpec(false, target.RowClosed, false, true, false, typ.String), `local function sink<T>(value) return value end
sink::<string>(1)`)
	if _, ok := constrainedTypes.factor.CallEffectAtom(constrainedTypes.root, constrainedTypes.application, constrainedTypes.owner, 0); ok {
		t.Fatal("constrained type-formal correspondence was admitted")
	}
}

func TestEffectFactorBodyAndForeignOwnersAreFenced(t *testing.T) {
	fixture := newEffectHostileFixture(t, effectHostileSpec(false, target.RowClosed, false, false, false, nil), `local function sink(value) return value end
sink(1)`)
	known := effectKnownAtom(t, fixture, 0)
	if fixture.factor.RootCount() < 2 {
		t.Fatal("fixture lacks a second executable body root")
	}
	otherRoot, ok := fixture.factor.RootAt(1)
	if !ok || otherRoot == fixture.root {
		t.Fatal("second body root unavailable")
	}
	if _, ok := fixture.factor.CallEffectAtom(otherRoot, fixture.application, fixture.owner, 0); ok {
		t.Fatal("call occurrence crossed body-root fence")
	}
	foreignFactor, ok := New(fixture.linked, fixture.packs, fixture.contract)
	if !ok {
		t.Fatal("seal foreign same-Link Effect owner")
	}
	foreignRoot, ok := foreignFactor.RootAt(0)
	if !ok {
		t.Fatal("foreign root")
	}
	if _, ok := fixture.factor.CallEffectAtom(foreignRoot, fixture.application, fixture.owner, 0); ok {
		t.Fatal("foreign Effect root crossed owner fence")
	}
	foreignFixture := newEffectHostileFixture(t, effectHostileSpec(false, target.RowClosed, false, false, false, nil), `local function sink(value) return value end
sink(1)`)
	if _, ok := fixture.factor.CallEffectAtom(fixture.root, foreignFixture.application, fixture.owner, 0); ok {
		t.Fatal("foreign Project Application crossed Effect owner fence")
	}
	if _, ok := fixture.factor.AtomID(known); !ok {
		t.Fatal("local known atom was lost while checking foreign fences")
	}
}

func TestEffectFactorAtomTransportAndRankCapacity(t *testing.T) {
	fixture := newEffectHostileFixture(t, effectHostileSpec(false, target.RowClosed, false, false, false, nil), `local function sink(value) return value end
sink(1)`)
	known := effectKnownAtom(t, fixture, 0)
	knownID := effectAtomID(t, fixture.factor, known)
	if fixture.factor.RootCount() < 2 {
		t.Fatal("fixture lacks transport destination root")
	}
	destination, ok := fixture.factor.RootAt(1)
	if !ok {
		t.Fatal("transport destination root")
	}
	transported, ok := fixture.factor.TransportAtom(known, destination)
	if !ok || effectAtomID(t, fixture.factor, transported) != knownID {
		t.Fatal("atom transport changed its certificate identity")
	}
	transportedValue, ok := fixture.factor.Singleton(transported)
	if !ok || !fixture.factor.Admit(destination, transportedValue) {
		t.Fatal("transported atom was not admitted by destination root")
	}
	unknown, ok := fixture.factor.OpenOperationUnknown(fixture.root, fixture.application, mustOpaqueOperation(t, fixture.contract))
	if !ok {
		t.Fatal("UnknownExternal witness")
	}
	knownValue, _ := fixture.factor.Singleton(known)
	unknownValue, _ := fixture.factor.Singleton(unknown)
	joined, ok := fixture.factor.Join(knownValue, unknownValue)
	if !ok {
		t.Fatal("join known and unknown")
	}
	if fixture.factor.capacity < 2 {
		t.Fatalf("Effect capacity = %d, want at least two admitted atoms", fixture.factor.capacity)
	}
	bottomRank := fixture.factor.WidenRank(fixture.root, fixture.factor.Bottom(), 0)
	knownRank := fixture.factor.WidenRank(fixture.root, knownValue, 0)
	joinedRank := fixture.factor.WidenRank(fixture.root, joined, 0)
	if bottomRank != fixture.factor.capacity+1 || !(bottomRank > knownRank && knownRank > joinedRank) || fixture.factor.WidenRank(fixture.root, fixture.factor.Top(), 0) != 0 {
		t.Fatalf("Effect rank did not descend by cardinality: bottom=%d known=%d joined=%d capacity=%d", bottomRank, knownRank, joinedRank, fixture.factor.capacity)
	}
}

func TestEffectFactorTopOrderLaws(t *testing.T) {
	fixture := newEffectHostileFixture(t, effectHostileSpec(false, target.RowClosed, false, false, false, nil), `local function sink(value) return value end
sink(1)`)
	bottom := fixture.factor.Bottom()
	top := fixture.factor.Top()
	atom := effectKnownAtom(t, fixture, 0)
	finite, ok := fixture.factor.Singleton(atom)
	if !ok {
		t.Fatal("finite Effect singleton")
	}
	if !fixture.factor.LessOrEq(bottom, top) {
		t.Fatal("Bottom must be below Top")
	}
	if fixture.factor.LessOrEq(top, bottom) {
		t.Fatal("Top must not be below Bottom")
	}
	if fixture.factor.LessOrEq(top, finite) {
		t.Fatal("Top must not be below a finite value")
	}
	if !fixture.factor.LessOrEq(finite, top) {
		t.Fatal("finite value must be below Top")
	}
	joined, ok := fixture.factor.Join(top, bottom)
	if !ok || !fixture.factor.Equal(joined, top) {
		t.Fatal("Top join Bottom must be Top")
	}
	joined, ok = fixture.factor.Join(bottom, top)
	if !ok || !fixture.factor.Equal(joined, top) {
		t.Fatal("Bottom join Top must be Top")
	}
}

func mustOpaqueOperation(t *testing.T, contract *target.Contract) target.Operation {
	t.Helper()
	op, ok := contract.Opaque()
	if !ok {
		t.Fatal("opaque operation")
	}
	return op
}

func TestEffectFactorHotQueriesAllocateNothing(t *testing.T) {
	fixture := newEffectHostileFixture(t, effectHostileSpec(false, target.RowClosed, false, false, true, nil), `local function sink(value) return value end
sink(function() return 1 end)`)
	known := effectKnownAtom(t, fixture, 0)
	if fixture.callback == 0 {
		t.Fatal("callback unavailable")
	}
	knownValue, ok := fixture.factor.Singleton(known)
	if !ok {
		t.Fatal("known singleton")
	}
	callbackAtom, ok := fixture.factor.CallbackEffectAtom(fixture.root, fixture.application, fixture.owner, fixture.callback, 0)
	if !ok {
		t.Fatal("callback atom unavailable")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := fixture.factor.AtomID(known); !ok {
			panic("atom ID unavailable")
		}
		if _, ok := fixture.factor.AtomID(callbackAtom); !ok {
			panic("callback atom ID unavailable")
		}
		if _, ok := fixture.factor.RootIndex(fixture.root); !ok {
			panic("root index unavailable")
		}
		if _, ok := fixture.factor.RootShard(fixture.root); !ok {
			panic("root shard unavailable")
		}
		if _, ok := fixture.factor.RootBody(fixture.root); !ok {
			panic("root body unavailable")
		}
		if _, ok := fixture.factor.CompareAtoms(known, callbackAtom); !ok {
			panic("atom comparison unavailable")
		}
		if !fixture.factor.Owns(knownValue) || !fixture.factor.Admit(fixture.root, knownValue) {
			panic("known value admission unavailable")
		}
		if fixture.factor.Fingerprint(knownValue) == 0 {
			panic("known value fingerprint unavailable")
		}
		if fixture.factor.WidenRank(fixture.root, knownValue, 0) == 0 {
			panic("rank unavailable")
		}
	}); allocs != 0 {
		t.Fatalf("Effect hot queries allocated %f times", allocs)
	}
}
