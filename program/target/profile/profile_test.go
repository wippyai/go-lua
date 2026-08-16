package profile

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

func TestCatalogueCoversItsClosedInventory(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	admitted := Admitted()
	bound := make(map[target.Operation]struct{})
	for _, binding := range admitted {
		op, ok := contract.Lookup(binding)
		if !ok {
			t.Fatalf("missing admitted binding %#v", binding)
		}
		bound[op] = struct{}{}
	}
	if got, want := contract.BoundOperationCount(), len(bound); got != want {
		t.Fatalf("bound operations = %d, want %d", got, want)
	}
	// The frozen denials below are deliberately absent from the historical
	// catalogue. coroutine.spawn is now an admitted typed detached operation.
	if got := len(admitted); got != 103 {
		t.Fatalf("admitted bindings = %d, want 103", got)
	}
	if got := contract.BoundOperationCount(); got != 101 {
		t.Fatalf("sealed bound operations = %d, want 101", got)
	}
	for _, binding := range Excluded() {
		if _, ok := contract.Lookup(binding); ok {
			t.Fatalf("excluded binding admitted %#v", binding)
		}
	}
}

func TestRequiredCallableAndSuspensionRelations(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range []target.BindingSpec{
		builtinBinding("pairs"), builtinBinding("ipairs"), moduleBinding("string", "gmatch"),
		moduleBinding("utf8", "codes"), moduleBinding("coroutine", "wrap"),
	} {
		op, ok := contract.Lookup(binding)
		if !ok {
			t.Fatalf("lookup %#v", binding)
		}
		found := false
		for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
			if _, _, ok := contract.ProducedForResult(op, outcome, 0); ok {
				found = true
			}
		}
		if !found {
			t.Fatalf("%#v has no produced callable result", binding)
		}
	}
	yield, ok := contract.Lookup(moduleBinding("coroutine", "yield"))
	if !ok || contract.SuspensionCount(yield) != 1 {
		t.Fatalf("coroutine.yield suspension count = %d", contract.SuspensionCount(yield))
	}
	resume, ok := contract.Lookup(moduleBinding("coroutine", "resume"))
	if !ok || contract.ResumeCount(resume) != 1 {
		t.Fatalf("coroutine.resume resumption count = %d", contract.ResumeCount(resume))
	}
	create, _ := contract.Lookup(moduleBinding("coroutine", "create"))
	if contract.CallbackCount(create) != 1 {
		t.Fatal("coroutine.create lacks its callback correspondence")
	}
	callback, _ := contract.CallbackAt(create, 0)
	linked := false
	for outcome := 0; outcome < contract.OutcomeCount(create); outcome++ {
		if got, _, ok := contract.CallbackForResult(create, outcome, 0); ok && got == callback {
			linked = true
		}
	}
	if !linked {
		t.Fatal("coroutine.create thread result lacks its callback result link")
	}
	wrap, _ := contract.Lookup(moduleBinding("coroutine", "wrap"))
	if contract.CallbackCount(wrap) != 1 {
		t.Fatal("coroutine.wrap lacks its callback correspondence")
	}
	wrapCallback, _ := contract.CallbackAt(wrap, 0)
	foundCapture := false
	for outcome := 0; outcome < contract.OutcomeCount(wrap); outcome++ {
		if _, producedIndex, ok := contract.ProducedForResult(wrap, outcome, 0); ok {
			if kind, ordinal, ok := contract.ProducedCaptureAt(wrap, outcome, producedIndex, 0); ok && kind == target.CaptureCallback && ordinal == uint32(wrapCallback) {
				foundCapture = true
			}
		}
	}
	if !foundCapture {
		t.Fatal("coroutine.wrap result does not retain its callback")
	}
}

func TestFreshProfileResultsArePreciseAndConjunctive(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		binding target.BindingSpec
		kind    target.FreshKind
	}{
		{moduleBinding("table", "pack"), target.FreshTable},
		{moduleBinding("table", "create"), target.FreshTable},
		{moduleBinding("coroutine", "create"), target.FreshThread},
		{moduleBinding("coroutine", "wrap"), target.FreshFunction},
		{moduleBinding("string", "gmatch"), target.FreshFunction},
		{moduleBinding("errors", "new"), target.FreshError},
		{moduleBinding("errors", "wrap"), target.FreshError},
	} {
		op, ok := contract.Lookup(item.binding)
		if !ok {
			t.Fatalf("lookup %#v", item.binding)
		}
		found := false
		for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
			if _, kind, _, ok := contract.FreshResultForResult(op, outcome, 0); ok && kind == item.kind {
				found = true
			}
		}
		if !found {
			t.Fatalf("%#v lacks FreshResult(%d)", item.binding, item.kind)
		}
	}
	details, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingModule, Owner: []string{"errors"}, Member: []string{"Error", "details"}})
	if !ok {
		t.Fatal("errors.Error.details missing")
	}
	tableFresh, nilFresh := false, false
	for outcome := 0; outcome < contract.OutcomeCount(details); outcome++ {
		_, values, _ := contract.OutcomeAt(details, outcome)
		_, kind, _, fresh := contract.FreshResultForResult(details, outcome, 0)
		if hasFixedOutcomeForValues(contract, values, []typ.Type{typ.BuiltinTableTopMarker()}) {
			tableFresh = fresh && kind == target.FreshTable
		}
		if hasFixedOutcomeForValues(contract, values, []typ.Type{typ.Nil}) {
			nilFresh = !fresh
		}
	}
	if !tableFresh || !nilFresh {
		t.Fatalf("errors.Error.details freshness = table:%v nil:%v", tableFresh, nilFresh)
	}
}

func TestProfileOpenValuesTailClassesAndMinMaxIdentityEnvelope(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	// Only char consumes every member of its input tail. The other open
	// envelopes below read at most a finite set of optional arguments, so an
	// arbitrary ignored suffix must remain Any.
	for _, item := range []struct {
		binding target.BindingSpec
		want    typ.Type
	}{
		{moduleBinding("string", "char"), typ.Integer},
		{moduleBinding("utf8", "char"), typ.Integer},
		{builtinBinding("tonumber"), typ.Any},
		{moduleBinding("table", "remove"), typ.Any},
		{moduleBinding("string", "byte"), typ.Any},
		{moduleBinding("string", "gsub"), typ.Any},
		{moduleBinding("string", "match"), typ.Any},
		{moduleBinding("string", "rep"), typ.Any},
		{moduleBinding("string", "sub"), typ.Any},
		{moduleBinding("string", "unpack"), typ.Any},
		{moduleBinding("math", "random"), typ.Any},
		{moduleBinding("math", "randomseed"), typ.Any},
		{moduleBinding("math", "atan"), typ.Any},
		{moduleBinding("math", "log"), typ.Any},
		{moduleBinding("utf8", "codepoint"), typ.Any},
		{moduleBinding("utf8", "len"), typ.Any},
		{moduleBinding("utf8", "offset"), typ.Any},
	} {
		op, ok := contract.Lookup(item.binding)
		if !ok {
			t.Fatalf("lookup %#v", item.binding)
		}
		input, _ := contract.Input(op)
		if !frozenTailClassMatches(t, contract, input, item.want) {
			t.Fatalf("%#v input tail class is not %s", item.binding, item.want)
		}
	}
	for _, binding := range []target.BindingSpec{moduleBinding("string", "byte"), moduleBinding("utf8", "codepoint")} {
		op, _ := contract.Lookup(binding)
		integerTail := false
		for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
			_, values, _ := contract.OutcomeAt(op, outcome)
			if frozenTailClassMatches(t, contract, values, typ.Integer) {
				integerTail = true
			}
		}
		if !integerTail {
			t.Fatalf("%#v lacks its Integer result tail class", binding)
		}
	}
	for _, name := range []string{"min", "max"} {
		op, _ := contract.Lookup(moduleBinding("math", name))
		input, _ := contract.Input(op)
		if contract.ValuesCount(input) != 1 || !frozenValuesAtMatches(t, contract, input, 0, typ.Any) || !frozenTailClassMatches(t, contract, input, typ.Any) {
			t.Fatalf("math.%s is not the Any head/tail identity envelope", name)
		}
	}
}

func TestCoroutineSpawnProfileCarriesTypedDetachedRelation(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Lookup(moduleBinding("coroutine", "spawn"))
	if !ok || contract.SpawnCount(op) != 1 {
		t.Fatalf("spawn authority = %v/%d", ok, contract.SpawnCount(op))
	}
	id, found := contract.SpawnIDAt(op, 0)
	owner, function, child, yield, resume, entry, resumed, found := contract.Spawn(id)
	if !found || owner != op || function.Kind != target.InputSourceValueFormal || function.Ordinal != 0 || child == 0 || yield == resume || entry != resumed {
		t.Fatalf("spawn = %d/%#v/%d/%d/%d/%d/%d/%v", owner, function, child, yield, resume, entry, resumed, found)
	}
	if source, found := contract.CallbackFunction(child); !found || source != function {
		t.Fatalf("spawn child source = %#v/%v", source, found)
	}
	if count := contract.SpawnSiblingCount(id); count != 2 {
		t.Fatalf("spawn sibling alternatives = %d", count)
	}
}

func TestCoroutineResumeProfilesCarryExactCrossActivationOutcomes(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		binding target.BindingSpec
		source  target.ResumeSource
	}{
		{moduleBinding("coroutine", "resume"), target.ResumeSourceValueFormal},
	} {
		op, ok := contract.Lookup(item.binding)
		if !ok || contract.ResumeCount(op) != 1 {
			t.Fatalf("resume relation missing for %#v", item.binding)
		}
		resume, found := contract.ResumeIDAt(op, 0)
		owner, source, carrier, arguments, found := contract.Resume(resume)
		tail, variable, tailOK := contract.ValuesTail(arguments)
		if !found || owner != op || source != item.source || carrier != 0 || !tailOK || tail != target.ValuesVariable || variable != 0 {
			t.Fatalf("resume relation = %d/%d/%d/%d/%v", owner, source, carrier, arguments, found)
		}
		assertResumeEnvelopeOutcomes(t, contract, op)
	}
	wrap, ok := contract.Lookup(moduleBinding("coroutine", "wrap"))
	if !ok {
		t.Fatal("coroutine.wrap missing")
	}
	var invoke target.Operation
	for outcome := 0; outcome < contract.OutcomeCount(wrap); outcome++ {
		candidate, _, found := contract.ProducedForResult(wrap, outcome, 0)
		if found {
			invoke = candidate
			break
		}
	}
	if invoke == 0 || contract.ResumeCount(invoke) != 1 {
		t.Fatal("coroutine.wrap produced invocation lacks resume relation")
	}
	resume, found := contract.ResumeIDAt(invoke, 0)
	owner, source, carrier, arguments, found := contract.Resume(resume)
	tail, variable, tailOK := contract.ValuesTail(arguments)
	if !found || owner != invoke || source != target.ResumeSourceProduced || carrier != 0 || !tailOK || tail != target.ValuesVariable || variable != 0 {
		t.Fatalf("wrap invocation resume relation = %d/%d/%d/%d/%v", owner, source, carrier, arguments, found)
	}
	assertResumeEnvelopeOutcomes(t, contract, invoke)
}

func assertResumeEnvelopeOutcomes(t *testing.T, contract *target.Contract, op target.Operation) {
	t.Helper()
	var normal, thrown uint32
	normalFound, thrownFound := false, false
	for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
		kind, values, ok := contract.OutcomeAt(op, outcome)
		if !ok {
			t.Fatal("malformed operation outcome")
		}
		tail, variable, tailOK := contract.ValuesTail(values)
		if tailOK && tail == target.ValuesVariable && variable == 1 {
			normal = uint32(outcome)
			normalFound = true
		}
		if kind == flowkind.OutcomeThrow || (tailOK && tail == target.ValuesVariable && variable == 2) {
			thrown = uint32(outcome)
			thrownFound = true
		}
	}
	if !normalFound || !thrownFound {
		t.Fatalf("profile resume envelope lacks exact success/failure Values rows: success=%v failure=%v", normalFound, thrownFound)
	}
	for index, want := range [...]struct {
		kind    flowkind.OutcomeKind
		outcome uint32
	}{
		{flowkind.OutcomeNormal, normal}, {flowkind.OutcomeReturn, normal}, {flowkind.OutcomeThrow, thrown},
		{flowkind.OutcomeYield, normal}, {flowkind.OutcomeCancel, thrown},
	} {
		resume, ok := contract.ResumeIDAt(op, 0)
		if !ok {
			t.Fatal("resume identity missing")
		}
		kind, outcome, ok := contract.ResumeOutcomeAt(resume, index)
		if !ok || kind != want.kind || outcome != want.outcome {
			t.Fatalf("resume outcome %d = %d/%d/%v, want %d/%d/true", index, kind, outcome, ok, want.kind, want.outcome)
		}
	}
}

func TestProtectedCallbacksExposeOwnerYieldWithoutSyntheticSuspension(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range []target.BindingSpec{builtinBinding("pcall"), builtinBinding("xpcall")} {
		op, ok := contract.Lookup(binding)
		if !ok {
			t.Fatalf("lookup %#v", binding)
		}
		if contract.SuspensionCount(op) != 0 {
			t.Fatalf("%#v has synthetic suspension", binding)
		}
		yield := false
		for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
			kind, _, ok := contract.OutcomeAt(op, outcome)
			yield = yield || ok && kind == flowkind.OutcomeYield
		}
		if !yield {
			t.Fatalf("%#v lacks forwarded owner yield", binding)
		}
	}
}

// TestFrozenInternalApplicationMatrix is the semantic inventory for every
// profile whose Lua behavior requires a hidden operation.  It deliberately
// observes only Target's public relations: an implementation may be rearranged
// freely, but may not lose an application family, terminal transport, or
// callable-admission failure.
func TestFrozenInternalApplicationMatrix(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	type edge struct {
		role   uint32
		family target.SubedgeFamily
	}
	for _, item := range []struct {
		binding target.BindingSpec
		edges   []edge
	}{
		{builtinBinding("print"), []edge{{1, target.SubedgeFamilyCall}}},
		{builtinBinding("tostring"), []edge{{1, target.SubedgeFamilyCall}}},
		{moduleBinding("string", "format"), []edge{{1, target.SubedgeFamilyCall}}},
		{builtinBinding("pairs"), []edge{{1, target.SubedgeFamilyCall}}},
		{moduleBinding("table", "concat"), []edge{{1, target.SubedgeFamilyLength}, {2, target.SubedgeFamilyIndexGet}}},
		{moduleBinding("table", "insert"), []edge{{1, target.SubedgeFamilyLength}, {2, target.SubedgeFamilyIndexGet}, {3, target.SubedgeFamilyIndexSet}, {4, target.SubedgeFamilyIndexSet}}},
		{moduleBinding("table", "remove"), []edge{{1, target.SubedgeFamilyLength}, {2, target.SubedgeFamilyIndexGet}, {3, target.SubedgeFamilyIndexGet}, {4, target.SubedgeFamilyIndexSet}, {5, target.SubedgeFamilyIndexSet}}},
		{moduleBinding("table", "move"), []edge{{1, target.SubedgeFamilyIndexGet}, {2, target.SubedgeFamilyIndexSet}, {3, target.SubedgeFamilyEqual}}},
		{moduleBinding("table", "unpack"), []edge{{1, target.SubedgeFamilyLength}, {2, target.SubedgeFamilyIndexGet}}},
		{moduleBinding("table", "sort"), []edge{{1, target.SubedgeFamilyLength}, {2, target.SubedgeFamilyIndexGet}, {3, target.SubedgeFamilyCall}, {4, target.SubedgeFamilyLess}, {5, target.SubedgeFamilyIndexSet}, {6, target.SubedgeFamilyIndexSet}}},
		{moduleBinding("string", "gsub"), []edge{{1, target.SubedgeFamilyCall}, {2, target.SubedgeFamilyIndexGet}}},
		{moduleBinding("math", "min"), []edge{{1, target.SubedgeFamilyLess}}},
		{moduleBinding("math", "max"), []edge{{1, target.SubedgeFamilyLess}}},
	} {
		op, ok := contract.Lookup(item.binding)
		if !ok {
			t.Fatalf("missing operation %#v", item.binding)
		}
		if got := contract.SubedgeCount(op); got != len(item.edges) {
			t.Fatalf("%#v subedge count = %d, want %d", item.binding, got, len(item.edges))
		}
		for _, want := range item.edges {
			edge := subedgeRole(t, contract, op, want.role)
			if family, ok := contract.SubedgeFamily(edge); !ok || family != want.family {
				t.Fatalf("%#v role %d family = %d/%v, want %d", item.binding, want.role, family, ok, want.family)
			}
			if _, ok := contract.AdmissionFailure(edge); !ok {
				t.Fatalf("%#v role %d lacks explicit admission failure", item.binding, want.role)
			}
			if route, _, _, _, _, _, _, _, ok := contract.AdmissionRoute(edge); !ok || (route != target.RouteOutcome && route != target.RouteSubedge) {
				t.Fatalf("%#v role %d admission route = %d/%v", item.binding, want.role, route, ok)
			}
			for _, terminal := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
				if _, ok := contract.SubedgeTerminal(edge, terminal); !ok {
					t.Fatalf("%#v role %d lacks terminal %d", item.binding, want.role, terminal)
				}
				if route, _, _, _, _, _, _, _, ok := contract.SubedgeRouteAt(edge, terminal); !ok || route == target.RouteInvalid {
					t.Fatalf("%#v role %d terminal %d route = %d/%v", item.binding, want.role, terminal, route, ok)
				}
			}
			if route, _, _, _, _, _, _, _, ok := contract.SubedgeRouteAt(edge, flowkind.OutcomeYield); !ok || route != target.RouteRejectYield {
				t.Fatalf("%#v role %d yield route = %d/%v, want C-boundary rejection", item.binding, want.role, route, ok)
			}
		}
	}
	// ipairs.aux is intentionally produced-only, so it has no catalogue
	// binding. Its index operation is still a public Target relation.
	ipairs, ok := contract.Lookup(builtinBinding("ipairs"))
	if !ok {
		t.Fatal("ipairs missing")
	}
	aux, _, ok := contract.ProducedForResult(ipairs, 0, 0)
	if !ok || contract.SubedgeCount(aux) != 1 {
		t.Fatalf("ipairs produced auxiliary = %d/%v", aux, ok)
	}
	edgeID := subedgeRole(t, contract, aux, 1)
	if family, ok := contract.SubedgeFamily(edgeID); !ok || family != target.SubedgeFamilyIndexGet {
		t.Fatalf("ipairs auxiliary family = %d/%v", family, ok)
	}
	if route, _, _, _, _, _, _, _, ok := contract.SubedgeRouteAt(edgeID, flowkind.OutcomeYield); !ok || route != target.RouteRejectYield {
		t.Fatalf("ipairs auxiliary yield route = %d/%v", route, ok)
	}
}

func TestInternalApplicationCalleeSourcesAreExact(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	assertKey := func(edge target.SubedgeID, wantCallee target.SubedgeCalleeKind, wantKey string) {
		t.Helper()
		if callee, ok := contract.SubedgeCallee(edge); !ok || callee != wantCallee {
			t.Fatalf("subedge %d callee = %d/%v, want %d", edge, callee, ok, wantCallee)
		}
		var key target.ExactKey
		var ok bool
		if wantCallee == target.SubedgeCalleeCapturedInitialRead {
			root, captured, found := contract.SubedgeCapturedInitialRead(edge)
			identity, identityOK := contract.InitialRootIdentity(root)
			if !found || !identityOK || identity != globalEnvRoot {
				t.Fatalf("subedge %d capture = %d/%d/%v", edge, root, captured, found)
			}
			key, ok = captured, found
		} else {
			key, ok = contract.SubedgeMetaKey(edge)
		}
		literal, literalOK := contract.ExactKeyValue(key)
		if !ok || !literalOK || literal.Kind != keyspace.LiteralString || literal.String != wantKey {
			t.Fatalf("subedge %d key = %#v/%v", edge, literal, ok && literalOK)
		}
	}
	lookup := func(binding target.BindingSpec) target.Operation {
		t.Helper()
		op, ok := contract.Lookup(binding)
		if !ok {
			t.Fatalf("lookup %#v", binding)
		}
		return op
	}
	assertKey(subedgeRole(t, contract, lookup(builtinBinding("print")), 1), target.SubedgeCalleeCapturedInitialRead, "tostring")
	for _, item := range []struct {
		binding target.BindingSpec
		key     string
	}{
		{builtinBinding("tostring"), "__tostring"},
		{moduleBinding("string", "format"), "__tostring"},
		{builtinBinding("pairs"), "__pairs"},
	} {
		assertKey(subedgeRole(t, contract, lookup(item.binding), 1), target.SubedgeCalleeMetaKey, item.key)
	}
}

func TestPairsSeparatesMetaHookFromRawFallback(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	pairs, ok := contract.Lookup(builtinBinding("pairs"))
	if !ok || contract.OutcomeCount(pairs) != 4 {
		t.Fatalf("pairs outcomes = %v/%d", ok, contract.OutcomeCount(pairs))
	}
	metaOutcome, fallbackOutcome := -1, -1
	for outcome := 0; outcome < contract.OutcomeCount(pairs); outcome++ {
		kind, values, found := contract.OutcomeAt(pairs, outcome)
		if !found || kind != flowkind.OutcomeNormal {
			continue
		}
		if hasFixedOutcomeForValues(contract, values, []typ.Type{typ.Any, typ.Any, typ.Any}) {
			metaOutcome = outcome
		}
		if hasFixedOutcomeForValues(contract, values, []typ.Type{typ.Any, typ.Any, typ.Nil}) {
			fallbackOutcome = outcome
		}
	}
	if metaOutcome < 0 {
		t.Fatal("pairs meta-hook outcome does not retain an arbitrary third result")
	}
	if fallbackOutcome < 0 {
		t.Fatal("pairs fallback is not next/input/nil")
	}
	if contract.ProducedCount(pairs, metaOutcome) != 0 || contract.ResultAliasCount(pairs, metaOutcome) != 0 {
		t.Fatal("pairs meta-hook outcome spuriously produces raw next/input")
	}
	next, ok := contract.Lookup(builtinBinding("next"))
	produced, _, producedOK := contract.ProducedForResult(pairs, fallbackOutcome, 0)
	if !ok || !producedOK || produced != next {
		t.Fatalf("pairs fallback produced = %d/%v, want next %d", produced, producedOK, next)
	}
	kind, source, _, aliasOK := contract.ResultAliasForResult(pairs, fallbackOutcome, 1)
	if !aliasOK || kind != target.InputSourceValueFormal || source != 0 {
		t.Fatalf("pairs fallback iterator state alias = %d/%d/%v", kind, source, aliasOK)
	}
	edge := subedgeRole(t, contract, pairs, 1)
	for _, terminal := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn} {
		route, _, result, placement, offset, outcome, _, _, found := contract.SubedgeRouteAt(edge, terminal)
		if !found || route != target.RouteOutcome || placement != target.PlacementFixed || offset != 0 || int(outcome) != metaOutcome || !hasFixedOutcomeForValues(contract, result, []typ.Type{typ.Any, typ.Any, typ.Any}) {
			t.Fatalf("pairs meta-hook terminal %d = route:%d placement:%d offset:%d outcome:%d", terminal, route, placement, offset, outcome)
		}
	}
}

func TestInternalApplicationArgumentOriginsRemainConcrete(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	assertOrigin := func(edge target.SubedgeID, index int, wantSegment target.ArgumentSegment, wantOrdinal uint32, wantKind target.ArgumentSource, wantInput target.InputSource) {
		t.Helper()
		segment, ordinal, kind, input, ok := contract.ArgumentOriginAt(edge, index)
		if !ok || segment != wantSegment || ordinal != wantOrdinal || kind != wantKind || input != wantInput {
			t.Fatalf("subedge %d argument origin %d = %d/%d/%d/%#v/%v", edge, index, segment, ordinal, kind, input, ok)
		}
	}
	tostring, ok := contract.Lookup(builtinBinding("tostring"))
	if !ok {
		t.Fatal("tostring missing")
	}
	tostringEdge := subedgeRole(t, contract, tostring, 1)
	if contract.ArgumentOriginCount(tostringEdge) != 1 {
		t.Fatal("tostring does not have one exact meta-call operand")
	}
	assertOrigin(tostringEdge, 0, target.ArgumentFixed, 0, target.ArgumentSourceInput, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0})

	ipairs, ok := contract.Lookup(builtinBinding("ipairs"))
	if !ok {
		t.Fatal("ipairs missing")
	}
	aux, _, ok := contract.ProducedForResult(ipairs, 0, 0)
	if !ok {
		t.Fatal("ipairs auxiliary missing")
	}
	auxEdge := subedgeRole(t, contract, aux, 1)
	if contract.ArgumentOriginCount(auxEdge) != 2 {
		t.Fatal("ipairs auxiliary does not have its exact base/successor operands")
	}
	assertOrigin(auxEdge, 0, target.ArgumentFixed, 0, target.ArgumentSourceInput, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0})
	assertOrigin(auxEdge, 1, target.ArgumentFixed, 1, target.ArgumentSourceRule, target.InputSource{})
}

func TestProtectedSubedgeMatrix(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	pcall, ok := contract.Lookup(builtinBinding("pcall"))
	if !ok || contract.SubedgeCount(pcall) != 1 {
		t.Fatalf("pcall subedge matrix = %v/%d", ok, contract.SubedgeCount(pcall))
	}
	protected, _ := contract.SubedgeAt(pcall, 0)
	route, _, _, _, _, outcome, sibling, _, ok := contract.AdmissionRoute(protected)
	if !ok || route != target.RouteOutcome || sibling != 0 {
		t.Fatalf("pcall admission route = %d/%d/%v", route, sibling, ok)
	}
	kind, values, ok := contract.OutcomeAt(pcall, int(outcome))
	if !ok || kind != flowkind.OutcomeNormal || !hasFixedOutcomeForValues(contract, values, []typ.Type{typ.LiteralBool(false), typ.Any}) {
		t.Fatal("pcall admission failure does not produce false/error")
	}

	xpcall, ok := contract.Lookup(builtinBinding("xpcall"))
	if !ok || contract.SubedgeCount(xpcall) != 2 {
		t.Fatalf("xpcall subedge matrix = %v/%d", ok, contract.SubedgeCount(xpcall))
	}
	protected, handler := subedgeRole(t, contract, xpcall, 1), subedgeRole(t, contract, xpcall, 2)
	if admission, ok := contract.SubedgeAdmission(handler); !ok || admission != target.DirectFunction {
		t.Fatalf("xpcall handler admission = %d/%v", admission, ok)
	}
	if callback, ok := contract.SubedgeCallback(handler); !ok {
		t.Fatalf("xpcall handler lifecycle = %d/%v", callback, ok)
	} else if lifecycle, found := contract.CallbackLifecycle(callback); !found || lifecycle != target.CallbackSyncOptionalMany {
		t.Fatalf("xpcall handler lifecycle = %d/%v", lifecycle, found)
	}
	_, _, result, placement, offset, _, sibling, _, ok := contract.SubedgeRouteAt(protected, flowkind.OutcomeThrow)
	if !ok || sibling != handler || placement != target.PlacementFixed || offset != 0 || contract.ValuesCount(result) != 1 {
		t.Fatalf("xpcall protected throw route lost scalar handler input")
	}
	for _, terminal := range []flowkind.OutcomeKind{flowkind.OutcomeThrow, flowkind.OutcomeYield} {
		route, _, result, placement, offset, _, sibling, _, ok := contract.SubedgeRouteAt(handler, terminal)
		if !ok || sibling != handler || placement != target.PlacementFixed || offset != 0 || contract.ValuesCount(result) != 1 {
			t.Fatalf("xpcall handler terminal %d is not recursive scalar transport", terminal)
		}
		if (terminal == flowkind.OutcomeThrow && route != target.RouteSubedge) || (terminal == flowkind.OutcomeYield && route != target.RouteRejectYield) {
			t.Fatalf("xpcall handler terminal %d route = %d", terminal, route)
		}
	}
}

func subedgeRole(t *testing.T, contract *target.Contract, op target.Operation, role uint32) target.SubedgeID {
	t.Helper()
	for index := 0; index < contract.SubedgeCount(op); index++ {
		edge, _ := contract.SubedgeAt(op, index)
		if found, ok := contract.SubedgeRole(edge); ok && found == role {
			return edge
		}
	}
	t.Fatalf("operation %d lacks subedge role %d", op, role)
	return 0
}

func TestCallbackSourcesUseExactAdjustedOrFixedFormals(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		binding target.BindingSpec
		formal  uint32
	}{
		{moduleBinding("table", "sort"), 1}, {moduleBinding("string", "gsub"), 2},
	} {
		op, ok := contract.Lookup(item.binding)
		if !ok || contract.CallbackCount(op) != 1 {
			t.Fatalf("callback relation missing for %#v", item.binding)
		}
		id, _ := contract.CallbackAt(op, 0)
		source, ok := contract.CallbackFunction(id)
		if !ok || source.Kind != target.InputSourceValueFormal || source.Ordinal != item.formal {
			t.Fatalf("callback source for %#v = %#v/%v", item.binding, source, ok)
		}
	}
}

func TestCallbackProfilesCarryExactLifecycle(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		binding    target.BindingSpec
		lifecycles []target.CallbackLifecycle
	}{
		{builtinBinding("pcall"), []target.CallbackLifecycle{target.CallbackSyncRequiredOnce}},
		{builtinBinding("xpcall"), []target.CallbackLifecycle{target.CallbackSyncRequiredOnce, target.CallbackSyncOptionalMany}},
		{moduleBinding("table", "sort"), []target.CallbackLifecycle{target.CallbackSyncOptionalMany}},
		{moduleBinding("string", "gsub"), []target.CallbackLifecycle{target.CallbackSyncOptionalMany}},
		{moduleBinding("coroutine", "create"), []target.CallbackLifecycle{target.CallbackRetainedOptionalOnce}},
		{moduleBinding("coroutine", "wrap"), []target.CallbackLifecycle{target.CallbackRetainedOptionalOnce}},
	}
	for _, test := range tests {
		op, ok := contract.Lookup(test.binding)
		if !ok || contract.CallbackCount(op) != len(test.lifecycles) {
			t.Fatalf("callback lifecycle inventory for %#v = %d/%v", test.binding, contract.CallbackCount(op), ok)
		}
		for index, want := range test.lifecycles {
			id, found := contract.CallbackAt(op, index)
			got, lifecycleFound := contract.CallbackLifecycle(id)
			if !found || !lifecycleFound || got != want {
				t.Fatalf("callback lifecycle for %#v[%d] = %d/%v/%v, want %d", test.binding, index, got, found, lifecycleFound, want)
			}
			for _, kind := range []flowkind.OutcomeKind{
				flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
				flowkind.OutcomeYield, flowkind.OutcomeCancel,
			} {
				values, outcomeFound := contract.CallbackOutcome(id, kind)
				if !outcomeFound || values == 0 {
					t.Fatalf("callback outcome for %#v[%d]/%d missing", test.binding, index, kind)
				}
			}
		}
	}
}

func TestExactOutcomeLedger(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		binding  target.BindingSpec
		outcomes [][]typ.Type
	}{
		{moduleBinding("math", "ceil"), [][]typ.Type{{typ.Number}}},
		{moduleBinding("math", "floor"), [][]typ.Type{{typ.Number}}},
		{moduleBinding("math", "modf"), [][]typ.Type{{typ.Number, typ.Number}}},
		{moduleBinding("math", "random"), [][]typ.Type{{typ.Number}, {typ.Integer}}},
		{moduleBinding("utf8", "len"), [][]typ.Type{{typ.Integer}, {typ.Nil, typ.Integer}}},
		{target.BindingSpec{Namespace: target.BindingModule, Owner: []string{"errors"}, Member: []string{"Error", "details"}}, [][]typ.Type{{typ.BuiltinTableTopMarker()}, {typ.Nil}}},
	} {
		op, ok := contract.Lookup(item.binding)
		if !ok {
			t.Fatalf("lookup %#v", item.binding)
		}
		for _, want := range item.outcomes {
			if !hasFixedOutcome(contract, op, want) {
				t.Fatalf("%#v lacks outcome %#v", item.binding, want)
			}
		}
	}
	find, _ := contract.Lookup(moduleBinding("string", "find"))
	if !hasOpenPrefixOutcome(contract, find, []typ.Type{typ.Integer, typ.Integer}) {
		t.Fatal("string.find lacks capture tail")
	}
}

func hasFixedOutcome(contract *target.Contract, op target.Operation, want []typ.Type) bool {
	for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
		_, values, ok := contract.OutcomeAt(op, outcome)
		if !ok || contract.ValuesCount(values) != len(want) {
			continue
		}
		match := true
		for index, expected := range want {
			frozen, _ := contract.ValuesAt(values, index)
			got, _ := contract.TypeBytes(frozen)
			want, err := typ.EncodeCanonicalFormals(context.Background(), expected, nil)
			match = match && err == nil && bytes.Equal(got, want)
		}
		if match {
			return true
		}
	}
	return false
}

func frozenTailClassMatches(t *testing.T, contract *target.Contract, values target.Values, want typ.Type) bool {
	t.Helper()
	got, ok := contract.ValuesTailType(values)
	if !ok {
		return false
	}
	gotBytes, ok := contract.TypeBytes(got)
	expected, err := typ.EncodeCanonicalFormals(context.Background(), want, nil)
	return ok && err == nil && bytes.Equal(gotBytes, expected)
}

func frozenValuesAtMatches(t *testing.T, contract *target.Contract, values target.Values, index int, want typ.Type) bool {
	t.Helper()
	got, ok := contract.ValuesAt(values, index)
	if !ok {
		return false
	}
	gotBytes, ok := contract.TypeBytes(got)
	expected, err := typ.EncodeCanonicalFormals(context.Background(), want, nil)
	return ok && err == nil && bytes.Equal(gotBytes, expected)
}

func hasOpenPrefixOutcome(contract *target.Contract, op target.Operation, want []typ.Type) bool {
	for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
		_, values, ok := contract.OutcomeAt(op, outcome)
		if !ok {
			continue
		}
		tail, _, _ := contract.ValuesTail(values)
		if tail == target.ValuesVariable && hasFixedOutcomeForValues(contract, values, want) {
			return true
		}
	}
	return false
}

func hasFixedOutcomeForValues(contract *target.Contract, values target.Values, want []typ.Type) bool {
	if contract.ValuesCount(values) != len(want) {
		return false
	}
	for index, expected := range want {
		frozen, _ := contract.ValuesAt(values, index)
		got, _ := contract.TypeBytes(frozen)
		want, err := typ.EncodeCanonicalFormals(context.Background(), expected, nil)
		if err != nil || !bytes.Equal(got, want) {
			return false
		}
	}
	return true
}

func TestAliasesAndValuesSuffixAreSealedSemantics(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		binding target.BindingSpec
		result  uint32
		source  uint32
	}{
		{builtinBinding("setmetatable"), 0, 0}, {builtinBinding("rawset"), 0, 0},
		{moduleBinding("table", "freeze"), 0, 0}, {builtinBinding("pairs"), 1, 0},
		{builtinBinding("ipairs"), 1, 0}, {moduleBinding("utf8", "codes"), 1, 0},
	} {
		op, ok := contract.Lookup(item.binding)
		if !ok {
			t.Fatalf("lookup %#v", item.binding)
		}
		found := false
		for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
			kind, source, _, ok := contract.ResultAliasForResult(op, outcome, item.result)
			if ok {
				found = kind == target.InputSourceValueFormal && source == item.source
			}
		}
		if !found {
			t.Fatalf("missing result alias %#v result %d", item.binding, item.result)
		}
	}
	op, ok := contract.Lookup(moduleBinding("string", "unpack"))
	if !ok {
		t.Fatal("string.unpack missing")
	}
	foundSuffix := false
	for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
		_, values, ok := contract.OutcomeAt(op, outcome)
		if ok && contract.ValuesSuffixCount(values) == 1 {
			foundSuffix = true
		}
	}
	if !foundSuffix {
		t.Fatal("string.unpack lacks the fixed post-tail position result")
	}
}

func TestExcludedBindingsDoNotAliasProfileStorage(t *testing.T) {
	first := Excluded()
	first[0].Member[0] = "mutated"
	second := Excluded()
	if second[0].Member[0] == "mutated" {
		t.Fatal("Excluded returned mutable shared binding storage")
	}
}

func TestAuthoredCatalogueRejectsMissingIdentityBeforeSpecRef(t *testing.T) {
	var catalogue authoredCatalogue
	catalogue.add("known", normal(builtin("known"), nil, false, nil, false))
	if err := catalogue.produce("misspelled", "known"); err == nil {
		t.Fatal("missing authored producer created a produced-operation relation")
	}
	if len(catalogue.operations[0].Outcomes[0].Produced) != 0 {
		t.Fatal("missing authored producer mutated the first operation")
	}
	// A missing identity must fail before conversion to target.SpecRef.  The
	// untouched catalogue remains independently sealable, proving that a
	// misspelling did not silently wire its edge to operation zero.
	spec := target.Spec{Operations: catalogue.operations}
	if _, err := target.Seal(&spec); err != nil {
		t.Fatalf("untouched catalogue did not seal: %v", err)
	}
}

func TestAuthoringPermutationHasSameBoundCatalogue(t *testing.T) {
	first, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec()
	spec.Operations = reverseAndRemap(spec.Operations)
	second, err := target.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := boundSnapshot(second), boundSnapshot(first); !reflect.DeepEqual(got, want) {
		t.Fatalf("permuted catalogue differs\n got: %#v\nwant: %#v", got, want)
	}
}

// This assertion deliberately does not derive the frozen denied surface from
// Admitted, Excluded, or Spec. It prevents the profile from validating a
// self-authored omission while still accidentally exposing a forbidden target
// operation or effect edge.
func TestFrozenDeniedBindingsHaveNoOrdinaryTargetOperationOrEffect(t *testing.T) {
	frozenDenied := []target.BindingSpec{
		moduleBinding("string", "dump"),
		moduleBinding("debug", "getinfo"),
		moduleBinding("debug", "getlocal"),
		moduleBinding("debug", "traceback"),
		moduleBinding("errors", "call_stack"),
		{Namespace: target.BindingModule, Owner: []string{"errors"}, Member: []string{"Error", "stack"}},
	}
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	admitted := make(map[string]struct{}, len(Admitted()))
	for _, binding := range Admitted() {
		admitted[bindingKey(binding)] = struct{}{}
	}
	excluded := make(map[string]struct{}, len(Excluded()))
	for _, binding := range Excluded() {
		excluded[bindingKey(binding)] = struct{}{}
	}
	denied := make(map[string]struct{}, len(frozenDenied))
	for _, binding := range frozenDenied {
		key := bindingKey(binding)
		denied[key] = struct{}{}
		if _, ok := excluded[key]; !ok {
			t.Fatalf("frozen denied binding missing from Excluded: %#v", binding)
		}
		if _, ok := admitted[key]; ok {
			t.Fatalf("frozen denied binding admitted: %#v", binding)
		}
		if operation, ok := contract.Lookup(binding); ok {
			t.Fatalf("frozen denied binding has target operation %d: %#v", operation, binding)
		}
	}

	spec := Spec()
	for operationIndex, operation := range spec.Operations {
		for _, binding := range operation.Bindings {
			if _, forbidden := denied[bindingKey(binding)]; forbidden {
				t.Fatalf("ordinary operation %d binds frozen denied identity %#v", operationIndex, binding)
			}
		}
		for effectIndex, effect := range operation.Effects.Occurrences {
			targetIndex := int(effect.Target) - 1
			if targetIndex < 0 || targetIndex >= len(spec.Operations) {
				t.Fatalf("operation %d effect %d has invalid target %d", operationIndex, effectIndex, effect.Target)
			}
			for _, binding := range spec.Operations[targetIndex].Bindings {
				if _, forbidden := denied[bindingKey(binding)]; forbidden {
					t.Fatalf("operation %d effect %d targets frozen denied identity %#v", operationIndex, effectIndex, binding)
				}
			}
		}
	}
}

func reverseAndRemap(input []target.OperationSpec) []target.OperationSpec {
	out := make([]target.OperationSpec, len(input))
	for old := range input {
		item := input[old]
		item.Outcomes = append([]target.OutcomeSpec(nil), item.Outcomes...)
		for outcome := range item.Outcomes {
			produced := append([]target.ProducedSpec(nil), item.Outcomes[outcome].Produced...)
			for index := range produced {
				oldTarget := int(produced[index].Operation) - 1
				produced[index].Operation = target.SpecRef(len(input) - oldTarget)
			}
			item.Outcomes[outcome].Produced = produced
		}
		item.Effects.Occurrences = append([]target.EffectSpec(nil), item.Effects.Occurrences...)
		for effect := range item.Effects.Occurrences {
			oldTarget := int(item.Effects.Occurrences[effect].Target) - 1
			item.Effects.Occurrences[effect].Target = target.SpecRef(len(input) - oldTarget)
		}
		out[len(input)-1-old] = item
	}
	return out
}

func boundSnapshot(contract *target.Contract) map[string]target.Operation {
	out := make(map[string]target.Operation)
	for _, binding := range Admitted() {
		op, ok := contract.Lookup(binding)
		if !ok {
			panic("profile test: missing bound operation")
		}
		out[fmt.Sprintf("%#v", binding)] = op
	}
	return out
}

func builtinBinding(name string) target.BindingSpec {
	return target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{name}}
}

func moduleBinding(owner, name string) target.BindingSpec {
	return target.BindingSpec{Namespace: target.BindingModule, Owner: []string{owner}, Member: []string{name}}
}

func bindingKey(binding target.BindingSpec) string {
	return fmt.Sprintf("%d/%q/%q", binding.Namespace, binding.Owner, binding.Member)
}
