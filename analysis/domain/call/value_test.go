package call

import (
	"testing"

	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

var callJoinSink Value
var callTargetSink Target
var callBodySink Body
var callBodyIndexSink int
var callOperationBinding = target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"call_test_operation"}}

func callSource(t *testing.T, name, text string, contract *target.Contract) (*link.Link, *program.Program) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	if contract == nil {
		contract, err = target.Seal(&target.Spec{Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{callOperationBinding},
			Input:    target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	return linked, p
}

func callApplication(t *testing.T, source *link.Link, p *program.Program, ordinal int) linkproject.Application {
	t.Helper()
	want, ok := p.Flow().Authored().Calls().At(ordinal)
	if !ok {
		t.Fatalf("CallAt(%d)", ordinal)
	}
	calls := source.Project().Applications().Calls()
	applications := source.Project().Applications()
	for index := 0; index < calls.Count(); index++ {
		application, present := calls.At(index)
		_, occurrence, mapped := applications.Call(application)
		if !present || !mapped {
			t.Fatal("malformed Link Call application table")
		}
		if occurrence == want {
			return application
		}
	}
	t.Fatalf("application for CallAt(%d) not found", ordinal)
	return linkproject.Application{}
}

func dynamicAlgebra(t *testing.T) (*Algebra, Key, *link.Link, *program.Program) {
	t.Helper()
	source, p := callSource(t, "dynamic_call", `
local function first() return 1 end
local function second() return 2 end
local function invoke(selected) selected() end
first()
`, nil)
	algebra, ok := New(source)
	if !ok {
		t.Fatal("algebra rejected")
	}
	key, ok := algebra.KeyForApplication(callApplication(t, source, p, 0))
	if !ok {
		t.Fatal("dynamic key rejected")
	}
	return algebra, key, source, p
}

func selectorForFunction(t *testing.T, algebra *Algebra, key Key, source *link.Link, p *program.Program, ordinal int) Target {
	t.Helper()
	_ = key
	_ = source
	function, ok := p.Flow().Authored().Functions().At(ordinal)
	if !ok {
		t.Fatalf("FunctionAt(%d)", ordinal)
	}
	shard, ok := source.Project().Mounts().At(0)
	if !ok {
		t.Fatal("mounted shard")
	}
	capability, ok := algebra.TargetForFunction(shard, function)
	if !ok {
		t.Fatalf("TargetForFunction(%d)", ordinal)
	}
	return capability
}

func selectorForOperation(t *testing.T, algebra *Algebra, key Key, source *link.Link) Target {
	t.Helper()
	_ = key
	contract, ok := source.Boundary().Target()
	if !ok {
		t.Fatal("target")
	}
	operation, ok := contract.Lookup(callOperationBinding)
	if !ok {
		t.Fatal("operation")
	}
	boundary := source.Boundary()
	if boundary == nil {
		t.Fatal("boundary")
	}
	seed, ok := boundary.Seeds().ForOperation(operation)
	if !ok {
		t.Fatal("operation seed")
	}
	capability, ok := algebra.TargetForSeed(seed)
	if !ok {
		t.Fatal("TargetForSeed(operation)")
	}
	return capability
}

func openCallValue(t *testing.T, algebra *Algebra, key Key, targets ...Target) Value {
	t.Helper()
	value, ok := algebra.DispatchValue(key, targets, true)
	if !ok {
		t.Fatal("call value rejected")
	}
	return value
}

func TestCallOneLinkScopedOwnerAcrossApplicationKeys(t *testing.T) {
	source, p := callSource(t, "two_dynamic_calls", `
local function first(x) x() end
local function second(x) x() end
`, nil)
	algebra, ok := New(source)
	if !ok {
		t.Fatal("algebra")
	}
	firstKey, firstOK := algebra.KeyForApplication(callApplication(t, source, p, 0))
	secondKey, secondOK := algebra.KeyForApplication(callApplication(t, source, p, 1))
	if !firstOK || !secondOK || firstKey == secondKey {
		t.Fatalf("application keys: first=%v second=%v distinct=%v", firstOK, secondOK, firstKey != secondKey)
	}
	firstSelector := selectorForFunction(t, algebra, firstKey, source, p, 0)
	secondSelector := selectorForFunction(t, algebra, secondKey, source, p, 1)
	left := openCallValue(t, algebra, firstKey, firstSelector)
	right := openCallValue(t, algebra, secondKey, secondSelector)
	joined, joinedOK := algebra.Join(left, right)
	if !joinedOK || !algebra.LessOrEq(left, joined) || !algebra.LessOrEq(right, joined) {
		t.Fatal("application keys did not share one Call carrier")
	}
}

func TestCallEmptyProjectStillIncludesTargetEndpointKeys(t *testing.T) {
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, _ := callSource(t, "no_calls", `local value = 1`, contract)
	algebra, ok := New(source)
	opaque, opaqueOK := contract.Opaque()
	wantKeys := contract.CallbackCount(opaque)
	if !opaqueOK || !ok || !algebra.Valid() || algebra.KeyCount() != wantKeys {
		t.Fatal("empty Call family rejected")
	}
	domain, ok := algebra.Lattice()
	if !ok || !domain.Equal(domain.Bottom(), algebra.Bottom()) || !domain.Equal(domain.Top(), algebra.Top()) {
		t.Fatal("empty Call family lost shared carrier")
	}
}

func TestDynamicCallUsesFactorizedGlobalTargets(t *testing.T) {
	algebra, key, source, p := dynamicAlgebra(t)
	second := selectorForFunction(t, algebra, key, source, p, 1)
	operation := selectorForOperation(t, algebra, key, source)
	expectedSupport := p.Flow().Authored().Functions().Count()
	boundary := source.Boundary()
	if boundary == nil {
		t.Fatal("boundary")
	}
	seeds := boundary.Seeds()
	for index := 0; index < seeds.Count(); index++ {
		seed, ok := seeds.At(index)
		if !ok {
			t.Fatal("seed range")
		}
		_, operationSeed := seeds.Operation(seed)
		_, loaderSeed := seeds.Loader(seed)
		if operationSeed || loaderSeed {
			expectedSupport++
		}
	}
	if algebra.SupportCount(key) != expectedSupport || !algebra.OpaqueAdmitted(key) {
		t.Fatal("dynamic support law")
	}
	value := openCallValue(t, algebra, key, second, operation)
	if !value.IsOpen() || !value.HasTarget(second) || !value.HasTarget(operation) || !algebra.Admits(key, value) {
		t.Fatal("factorized relation lost alternative")
	}
}

// A dynamic Call may range over every executable closure summary, including
// an uncalled lexical closure, but never over a source-dead Function merely
// because its Program row exists. This is the Call-side law for Link v22.
func TestDynamicSupportExcludesSourceDeadFunctions(t *testing.T) {
	source, p := callSource(t, "dead_function", `
local function uncalled() return 1 end
local function invoke(selected) return selected() end
do return uncalled end
local function hidden() return 2 end
hidden()
`, nil)
	algebra, ok := New(source)
	if !ok {
		t.Fatal("algebra")
	}
	key, ok := algebra.KeyForApplication(callApplication(t, source, p, 0))
	if !ok {
		t.Fatal("dynamic Call key")
	}
	functions := p.Flow().Authored().Functions()
	executable := p.Flow().Executable()
	deadSeen := 0
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			t.Fatalf("FunctionAt(%d)", index)
		}
		shard, shardOK := source.Project().Mounts().At(0)
		if !shardOK {
			t.Fatal("mounted shard")
		}
		capability, admitted := algebra.TargetForFunction(shard, function)
		if executable.Contains(function) {
			if !admitted {
				t.Fatalf("executable Function %v missing from Call support", function)
			}
			if !capability.Valid() {
				t.Fatalf("Call support has invalid Function selector %v", function)
			}
			continue
		}
		if admitted {
			t.Fatalf("source-dead Function %v entered Call support", function)
		}
		deadSeen++
	}
	if deadSeen == 0 {
		t.Fatal("fixture did not contain a source-dead Function")
	}
	if algebra.SupportCount(key) != len(algebra.targets) {
		t.Fatal("Call support did not expose its direct target vocabulary")
	}
}

func TestCallOpenJoinAndKeySpecificWidenRank(t *testing.T) {
	algebra, key, source, p := dynamicAlgebra(t)
	second := selectorForFunction(t, algebra, key, source, p, 1)
	operation := selectorForOperation(t, algebra, key, source)
	staticKey, staticOK := algebra.KeyForApplication(callApplication(t, source, p, 1))
	if !staticOK {
		t.Fatal("static Call key")
	}
	complete, completeOK := algebra.Initial(staticKey)
	if !completeOK || !complete.IsComplete() || complete.HasOpaqueAlternative() {
		t.Fatal("application seed was not neutral")
	}
	open := openCallValue(t, algebra, key, second)
	joined, ok := algebra.Join(complete, open)
	if !ok || !joined.IsOpen() || !algebra.LessOrEq(complete, joined) || !algebra.LessOrEq(open, joined) {
		t.Fatal("open join")
	}
	rank, ok := algebra.WidenRank(key)
	if !ok {
		t.Fatal("rank")
	}
	state := algebra.Bottom()
	prior, valid := rank.At(state, 0)
	if !valid {
		t.Fatal("bottom rank")
	}
	for _, next := range []Value{complete, openCallValue(t, algebra, key, second), openCallValue(t, algebra, key, operation)} {
		widened, widenedOK := algebra.Widen(state, next)
		nextRank, rankOK := rank.At(widened, 0)
		if !widenedOK || !rankOK || !algebra.Equal(widened, state) && nextRank >= prior {
			t.Fatal("strict widen did not descend")
		}
		state, prior = widened, nextRank
	}
}

func TestCallUnionPreservesOpenAndCombinesKnownAlternatives(t *testing.T) {
	algebra, key, source, p := dynamicAlgebra(t)
	first := selectorForFunction(t, algebra, key, source, p, 0)
	second := selectorForFunction(t, algebra, key, source, p, 1)
	left := openCallValue(t, algebra, key, first)
	right := openCallValue(t, algebra, key, second)
	union, ok := algebra.Join(left, right)
	if !ok || !union.IsOpen() || !union.HasTarget(first) || !union.HasTarget(second) || !algebra.LessOrEq(left, union) || !algebra.LessOrEq(right, union) {
		t.Fatal("Call union did not preserve open or known alternatives")
	}
	staticKey, staticOK := algebra.KeyForApplication(callApplication(t, source, p, 1))
	complete, completeOK := algebra.Initial(staticKey)
	if !staticOK || !completeOK || !complete.IsComplete() || complete.HasOpaqueAlternative() {
		t.Fatal("application seed")
	}
	for _, operation := range []func(Value, Value) (Value, bool){algebra.Join, algebra.Widen} {
		value, ok := operation(complete, left)
		if !ok || !value.IsOpen() {
			t.Fatal("Call union erased an open alternative")
		}
	}
}

func TestCallNoPublicOperationCanTurnOpenComplete(t *testing.T) {
	algebra, key, source, p := dynamicAlgebra(t)
	dynamicInitial, initialOK := algebra.Initial(key)
	if !initialOK || !dynamicInitial.IsComplete() || dynamicInitial.HasOpaqueAlternative() {
		t.Fatal("dynamic Application did not start at neutral Bottom")
	}
	staticKey, staticOK := algebra.KeyForApplication(callApplication(t, source, p, 1))
	staticInitial, staticInitialOK := algebra.Initial(staticKey)
	if !staticOK || !staticInitialOK || !staticInitial.IsComplete() || staticInitial.HasOpaqueAlternative() {
		t.Fatal("Call application source retained an unconditional opaque dispatch")
	}
	first := selectorForFunction(t, algebra, key, source, p, 0)
	open := openCallValue(t, algebra, key, first)
	if open.IsComplete() {
		t.Fatal("ordinary Call constructor self-asserted completeness")
	}
	domain, domainOK := algebra.Lattice()
	if !domainOK || domain.Meet != nil || domain.Narrow != nil {
		t.Fatal("generic Call lattice exposed a narrowing path")
	}
	joined, joinedOK := algebra.Join(open, staticInitial)
	widened, widenedOK := algebra.Widen(open, staticInitial)
	rebound, reboundOK := algebra.Rebind(open)
	if !joinedOK || !widenedOK || !reboundOK {
		t.Fatal("public Call operation rejected a valid open value")
	}
	for _, value := range []Value{
		openCallValue(t, algebra, key, first),
		joined,
		widened,
		rebound,
		domain.Join(open, staticInitial),
		domain.Widen(open, staticInitial),
	} {
		if value.IsComplete() || !value.HasOpaqueAlternative() {
			t.Fatal("public Call operation erased the opaque alternative")
		}
	}
	if !algebra.Top().HasOpaqueAlternative() {
		t.Fatal("Call Top omitted the conservative opaque alternative")
	}
}

func TestCallLatticeLaws(t *testing.T) {
	algebra, key, source, p := dynamicAlgebra(t)
	staticKey, staticOK := algebra.KeyForApplication(callApplication(t, source, p, 1))
	if !staticOK {
		t.Fatal("static Call key")
	}
	first, firstOK := algebra.Initial(staticKey)
	second := openCallValue(t, algebra, key, selectorForFunction(t, algebra, key, source, p, 1))
	joined, ok := algebra.Join(first, second)
	domain, domainOK := algebra.Lattice()
	if !firstOK || !ok || !domainOK {
		t.Fatal("lattice")
	}
	latticelaws.LawSuite[Value]{Name: "call", Domain: domain, Sample: []Value{algebra.Bottom(), first, second, joined, algebra.Top()}}.Run(t)
}

func TestCallReplayRebindsByContentAndForeignLinkFailsClosed(t *testing.T) {
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	text := `local function f() end; local function invoke(x) x() end`
	leftLink, leftProgram := callSource(t, "replay", text, contract)
	rightLink, rightProgram := callSource(t, "replay", text, contract)
	left, leftOK := New(leftLink)
	right, rightOK := New(rightLink)
	leftKey, leftKeyOK := left.KeyForApplication(callApplication(t, leftLink, leftProgram, 0))
	rightKey, rightKeyOK := right.KeyForApplication(callApplication(t, rightLink, rightProgram, 0))
	if !leftOK || !rightOK || !leftKeyOK || !rightKeyOK {
		t.Fatal("replay algebra")
	}
	leftSelector := selectorForFunction(t, left, leftKey, leftLink, leftProgram, 0)
	rightSelector := selectorForFunction(t, right, rightKey, rightLink, rightProgram, 0)
	leftValue := openCallValue(t, left, leftKey, leftSelector)
	rightValue := openCallValue(t, right, rightKey, rightSelector)
	if left.ContentID() != right.ContentID() || !left.Equivalent(right) {
		t.Fatal("semantic replay")
	}
	if _, ok := left.Join(leftValue, rightValue); ok {
		t.Fatal("foreign owner joined before rebind")
	}
	rebound, ok := left.Rebind(rightValue)
	if !ok || !left.Equal(leftValue, rebound) {
		t.Fatal("replay rebind")
	}
	foreignLink, _ := callSource(t, "foreign", `local function g() end; local function invoke(x) x() end`, contract)
	foreign, ok := New(foreignLink)
	if !ok {
		t.Fatal("foreign algebra")
	}
	if left.Equivalent(foreign) {
		t.Fatal("foreign Link accepted")
	}
	if _, ok := foreign.Rebind(leftValue); ok {
		t.Fatal("foreign value rebound")
	}
}

func TestCallTargetCapabilityRejectsForeignSameOrdinal(t *testing.T) {
	leftLink, leftProgram := callSource(t, "same_ordinal", `local function f() end; f()`, nil)
	contract, ok := leftLink.Boundary().Target()
	if !ok {
		t.Fatal("target")
	}
	rightLink, rightProgram := callSource(t, "same_ordinal", `local function f() end; f()`, contract)
	left, leftOK := New(leftLink)
	right, rightOK := New(rightLink)
	leftKey, leftKeyOK := left.KeyForApplication(callApplication(t, leftLink, leftProgram, 0))
	rightKey, rightKeyOK := right.KeyForApplication(callApplication(t, rightLink, rightProgram, 0))
	leftShard, leftShardOK := leftLink.Project().Mounts().At(0)
	rightShard, rightShardOK := rightLink.Project().Mounts().At(0)
	leftFunction, leftFunctionOK := leftProgram.Flow().Authored().Functions().At(0)
	rightFunction, rightFunctionOK := rightProgram.Flow().Authored().Functions().At(0)
	leftTarget, leftTargetOK := left.TargetForFunction(leftShard, leftFunction)
	rightTarget, rightTargetOK := right.TargetForFunction(rightShard, rightFunction)
	resealed, resealedOK := New(leftLink)
	resealedTarget, resealedTargetOK := resealed.TargetForFunction(leftShard, leftFunction)
	if !leftOK || !rightOK || !leftKeyOK || !rightKeyOK || !leftShardOK || !rightShardOK || !leftFunctionOK || !rightFunctionOK || !leftTargetOK || !rightTargetOK || !resealedOK || !resealedTargetOK {
		t.Fatal("same-ordinal target setup")
	}
	if leftTarget.Same(rightTarget) || leftTarget == rightTarget || leftTarget.Same(resealedTarget) || leftTarget == resealedTarget {
		t.Fatal("foreign same-ordinal targets collapsed")
	}
	leftValue := openCallValue(t, left, leftKey, leftTarget)
	rightValue := openCallValue(t, right, rightKey, rightTarget)
	if leftValue.HasTarget(rightTarget) || rightValue.HasTarget(leftTarget) || leftValue.HasTarget(resealedTarget) {
		t.Fatal("foreign target capability crossed Value owner fence")
	}
}

func TestCallCanonicalJoinAllocationBounds(t *testing.T) {
	algebra, key, source, p := dynamicAlgebra(t)
	left := openCallValue(t, algebra, key, selectorForFunction(t, algebra, key, source, p, 0))
	right := openCallValue(t, algebra, key, selectorForFunction(t, algebra, key, source, p, 1))
	if allocations := testing.AllocsPerRun(1_000, func() {
		value, ok := algebra.Join(left, left)
		if !ok {
			panic("join")
		}
		callJoinSink = value
	}); allocations != 0 {
		t.Fatalf("idempotent join allocations=%g", allocations)
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		value, ok := algebra.Join(left, right)
		if !ok {
			panic("join")
		}
		callJoinSink = value
	}); allocations != 1 {
		t.Fatalf("strict join allocations=%g, want 1", allocations)
	}
}

func TestCallKnownTargetsAreOpaqueCapabilitiesForOpenAndCompleteValues(t *testing.T) {
	algebra, key, source, p := dynamicAlgebra(t)
	first := selectorForFunction(t, algebra, key, source, p, 0)
	second := selectorForFunction(t, algebra, key, source, p, 1)

	open := openCallValue(t, algebra, key, first)
	complete, ok := algebra.DispatchValue(key, []Target{first, second}, false)
	if !ok || !complete.IsComplete() {
		t.Fatal("complete value")
	}
	for name, value := range map[string]Value{"open": open, "complete": complete} {
		if value.KnownTargetCount() != len(value.selectors) {
			t.Fatalf("%s projected global support instead of known alternatives", name)
		}
		for index := 0; index < value.KnownTargetCount(); index++ {
			target, present := value.KnownTargetAt(index)
			if !present || !target.Valid() {
				t.Fatalf("%s target %d", name, index)
			}
			if !present || target.owner != algebra || !value.HasTarget(target) {
				t.Fatalf("%s capability identity %d", name, index)
			}
		}
	}
	if !open.HasOpaqueAlternative() || open.KnownTargetCount() != 1 {
		t.Fatal("open capability projection erased or enumerated opaque dispatch")
	}
	_, topTargetPresent := algebra.Top().KnownTargetAt(0)
	if algebra.Top().KnownTargetCount() != 0 || topTargetPresent {
		t.Fatal("top exposed a finite target image")
	}
}

func TestCallTargetOperationProjectsOnlyExactSeedAlternatives(t *testing.T) {
	algebra, key, source, p := dynamicAlgebra(t)
	want, ok := source.Boundary().Target()
	if !ok {
		t.Fatal("target")
	}
	wantOperation, ok := want.Lookup(callOperationBinding)
	if !ok {
		t.Fatal("operation")
	}

	seedSelector := selectorForOperation(t, algebra, key, source)
	known := openCallValue(t, algebra, key, seedSelector)
	seedTarget, ok := known.KnownTargetAt(0)
	if !ok {
		t.Fatal("seed target")
	}
	operation, ok := seedTarget.Operation()
	if !ok || operation != wantOperation {
		t.Fatalf("seed operation = (%v, %v), want (%v, true)", operation, ok, wantOperation)
	}
	if _, ok := seedTarget.Body(); ok {
		t.Fatal("operation target projected a Program body")
	}

	bodyTarget := selectorForFunction(t, algebra, key, source, p, 0)
	bodySelector := bodyTarget
	if _, ok := bodyTarget.Operation(); ok {
		t.Fatal("function-body target projected an operation")
	}
	open := openCallValue(t, algebra, key, bodySelector)
	if !open.HasOpaqueAlternative() || open.KnownTargetCount() != 1 {
		t.Fatal("open Call did not retain an unprojectable opaque remainder")
	}
	if _, ok := (Target{}).Operation(); ok {
		t.Fatal("invalid capability projected an operation")
	}
}

func TestCallKnownTargetProjectionIsAllocationFree(t *testing.T) {
	algebra, key, source, p := dynamicAlgebra(t)
	value := openCallValue(t, algebra, key, selectorForFunction(t, algebra, key, source, p, 0))
	if allocations := testing.AllocsPerRun(1_000, func() {
		for index := 0; index < value.KnownTargetCount(); index++ {
			target, ok := value.KnownTargetAt(index)
			if !ok {
				panic("known target")
			}
			if !target.Valid() {
				panic("known target")
			}
			callTargetSink = target
		}
	}); allocations != 0 {
		t.Fatalf("known target iteration allocations=%g", allocations)
	}
}

func TestCallBodiesAreOneCanonicalOwnerFencedPrefix(t *testing.T) {
	const text = `
local function first() return 1 end
local function second() return 2 end
local function invoke(selected) return selected() end
first()
second()
`
	leftLink, leftProgram := callSource(t, "body_cursor", text, nil)
	contract, contractOK := leftLink.Boundary().Target()
	rightLink, _ := callSource(t, "body_cursor", text, contract)
	left, leftOK := New(leftLink)
	right, rightOK := New(rightLink)
	resealed, resealedOK := New(leftLink)
	if !contractOK || !leftOK || !rightOK || !resealedOK {
		t.Fatal("body cursor fixture")
	}
	leftBodies, rightBodies, resealedBodies := left.Bodies(), right.Bodies(), resealed.Bodies()
	if leftBodies.Count() == 0 || leftBodies.Count() != rightBodies.Count() || leftBodies.Count() != resealedBodies.Count() || leftBodies.Count() >= len(left.targets) {
		t.Fatalf("body prefix counts: left=%d right=%d resealed=%d targets=%d", leftBodies.Count(), rightBodies.Count(), resealedBodies.Count(), len(left.targets))
	}

	functions := leftProgram.Flow().Authored().Functions()
	executable := leftProgram.Flow().Executable()
	shard, shardOK := leftLink.Project().Mounts().At(0)
	if !shardOK {
		t.Fatal("body cursor shard")
	}
	wantIndex := 0
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			t.Fatalf("function %d", index)
		}
		if !executable.Contains(function) {
			continue
		}
		_, wantBody, _, relationOK := functions.Get(function)
		body, bodyOK := leftBodies.At(wantIndex)
		bodyIndex, indexed := leftBodies.Index(body)
		resolvedShard, resolvedBody, resolved := left.ResolveBody(body)
		target, targetOK := left.TargetForFunction(shard, function)
		projected, projectedOK := target.Body()
		if !relationOK || !bodyOK || !indexed || bodyIndex != wantIndex || !resolved || resolvedShard != shard || resolvedBody != wantBody || !targetOK || !projectedOK || !body.Same(projected) {
			t.Fatalf("canonical body %d", wantIndex)
		}
		foreign, foreignOK := rightBodies.At(wantIndex)
		resealedBody, resealedBodyOK := resealedBodies.At(wantIndex)
		leftID, leftIDOK := body.ContentID()
		foreignID, foreignIDOK := foreign.ContentID()
		if !foreignOK || !resealedBodyOK || !leftIDOK || !foreignIDOK || leftID != foreignID || body.Same(foreign) || body.Same(resealedBody) {
			t.Fatalf("body replay fence %d", wantIndex)
		}
		if _, ok := leftBodies.Index(foreign); ok {
			t.Fatalf("foreign body indexed at %d", wantIndex)
		}
		if _, ok := leftBodies.Index(resealedBody); ok {
			t.Fatalf("resealed body indexed at %d", wantIndex)
		}
		wantIndex++
	}
	if wantIndex != leftBodies.Count() {
		t.Fatalf("canonical body count=%d, want %d", leftBodies.Count(), wantIndex)
	}
	if _, ok := leftBodies.At(-1); ok {
		t.Fatal("negative body cursor index")
	}
	if _, ok := leftBodies.At(leftBodies.Count()); ok {
		t.Fatal("past-end body cursor index")
	}
	if _, ok := (Bodies{}).At(0); ok || (Bodies{}).Count() != 0 {
		t.Fatal("zero body cursor became available")
	}

	first, _ := leftBodies.At(0)
	if allocations := testing.AllocsPerRun(1_000, func() {
		for index := 0; index < leftBodies.Count(); index++ {
			body, ok := leftBodies.At(index)
			if !ok {
				panic("body cursor")
			}
			bodyIndex, ok := leftBodies.Index(body)
			if !ok || bodyIndex != index {
				panic("body cursor inverse")
			}
			callBodySink, callBodyIndexSink = body, bodyIndex
		}
	}); allocations != 0 {
		t.Fatalf("body cursor allocations=%g", allocations)
	}
	callBodySink = first
}

// Call's external target vocabulary is deliberately a projection of the
// Boundary authority.  It admits ordinary Target operations, scoped loaders,
// and each nominal endpoint independently, but never turns a denied boot
// value into a dispatch target.
func TestCallBoundarySeedCandidatesAreExactAndDeniedValuesStayOut(t *testing.T) {
	contract := callBoundarySeedContract(t)
	p, err := programlower.Lower(programlower.Source{Name: "call_boundary_seed", Text: []byte(`local function invoke(f) f() end; invoke(invoke)`)})
	if err != nil {
		t.Fatal(err)
	}
	provider := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"host"}, Member: []string{"dispatch"}}
	source, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: "call_boundary_seed", Program: p}},
		EndpointRequests: []linkboundary.EndpointRequest{
			{Identity: "first", Binding: provider},
			{Identity: "second", Binding: provider},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	algebra, ok := New(source)
	if !ok {
		t.Fatal("Call algebra")
	}
	boundary := source.Boundary()
	if boundary == nil {
		t.Fatal("Boundary")
	}
	seeds := boundary.Seeds()
	var admitted, denied, loaders int
	for index := 0; index < seeds.Count(); index++ {
		seed, present := seeds.At(index)
		if !present {
			t.Fatalf("SeedAt(%d)", index)
		}
		disposition, _, _, classified := seeds.CallableDisposition(seed)
		_, operation := seeds.Operation(seed)
		_, loader := seeds.Loader(seed)
		_, selected := algebra.TargetForSeed(seed)
		if classified && disposition == linkboundary.CallableDeniedTarget {
			denied++
			if operation || loader || selected {
				t.Fatal("denied bootstrap value entered Call target vocabulary")
			}
			continue
		}
		if !operation && !loader {
			t.Fatalf("unclassified Boundary seed %d", index)
		}
		admitted++
		if loader {
			loaders++
		}
		if !selected {
			t.Fatalf("admitted Boundary seed %d missing Call selector", index)
		}
	}
	if admitted == 0 || denied == 0 || loaders == 0 {
		t.Fatalf("admitted=%d denied=%d loaders=%d", admitted, denied, loaders)
	}

	endpoints := boundary.Endpoints()
	if endpoints.Count() != 2 {
		t.Fatalf("endpoint count=%d, want 2", endpoints.Count())
	}
	first, firstOK := endpoints.At(0)
	second, secondOK := endpoints.At(1)
	firstSeed, firstSeedOK := endpoints.Seed(first)
	secondSeed, secondSeedOK := endpoints.Seed(second)
	firstTarget, firstTargetOK := algebra.TargetForSeed(firstSeed)
	secondTarget, secondTargetOK := algebra.TargetForSeed(secondSeed)
	if !firstOK || !secondOK || !firstSeedOK || !secondSeedOK || !firstTargetOK || !secondTargetOK || firstTarget == secondTarget {
		t.Fatal("same-operation endpoints lost nominal Call targets")
	}
	providerOperation, providerOK := contract.Lookup(provider)
	firstOperation, firstOperationOK := firstTarget.Operation()
	secondOperation, secondOperationOK := secondTarget.Operation()
	if !firstTargetOK || !secondTargetOK || !providerOK || !firstOperationOK || !secondOperationOK || firstOperation != providerOperation || secondOperation != providerOperation {
		t.Fatal("endpoint targets did not project the exact provider operation")
	}
}

func TestCallSelectorForSeedRejectsForeignEquivalentBoundary(t *testing.T) {
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{callOperationBinding},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	left, _ := callSource(t, "call_boundary_fence", `local function invoke(f) f() end`, contract)
	right, _ := callSource(t, "call_boundary_fence", `local function invoke(f) f() end`, contract)
	leftAlgebra, leftOK := New(left)
	rightAlgebra, rightOK := New(right)
	operation, operationOK := contract.Lookup(callOperationBinding)
	if !leftOK || !rightOK || !operationOK {
		t.Fatal("Call setup")
	}
	leftSeed, leftSeedOK := left.Boundary().Seeds().ForOperation(operation)
	rightSeed, rightSeedOK := right.Boundary().Seeds().ForOperation(operation)
	if !leftSeedOK || !rightSeedOK {
		t.Fatal("operation seed")
	}
	if _, ok := leftAlgebra.TargetForSeed(leftSeed); !ok {
		t.Fatal("local Boundary seed rejected")
	}
	if _, ok := rightAlgebra.TargetForSeed(leftSeed); ok {
		t.Fatal("foreign equivalent Boundary seed crossed owner fence")
	}
	if _, ok := leftAlgebra.TargetForSeed(rightSeed); ok {
		t.Fatal("foreign equivalent Boundary seed crossed reverse owner fence")
	}
}

func callBoundarySeedContract(t *testing.T) *target.Contract {
	t.Helper()
	op := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"op"}}
	require := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	denied := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"load"}}
	provider := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"host"}, Member: []string{"dispatch"}}
	operation := func(binding target.BindingSpec) target.OperationSpec {
		return target.OperationSpec{Bindings: []target.BindingSpec{binding}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}}
	}
	contract, err := target.Seal(&target.Spec{
		Operations:   []target.OperationSpec{operation(op), operation(require), operation(provider)},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "load"}, Value: target.InitialValueSpec{Kind: target.InitialValueDeniedOperation, Operation: denied}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}},
			{Name: "load", Root: "GlobalEnvRoot", Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "load"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}
