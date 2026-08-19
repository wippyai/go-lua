package body

import (
	"testing"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

func TestSealNestedBodyAndDirectRootOrder(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	returnTerm := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyValues: 1, keyspace.FamilyReturn: 1,
	}}
	input.Values.Rows = []authored.Value{{Owner: body1}}
	input.Control.Returns = []authored.Return{{Owner: body1, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)}}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{{
		returnTerm, body2,
	}, nil}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()

	result, err := Seal(preimage, view, staticView, body1)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if got := result.BodyCount(); got != 2 {
		t.Fatalf("BodyCount = %d, want 2", got)
	}
	if got, ok := result.BodyAt(0); !ok || got != body1 {
		t.Fatalf("BodyAt(0) = %v/%v", got, ok)
	}
	if parent, ok := result.Parent(body2); !ok || parent != body1 {
		t.Fatalf("Parent(body2) = %v/%v", parent, ok)
	}
	if _, ok := result.Parent(body1); ok {
		t.Fatal("Entry unexpectedly has a parent")
	}
	if count, ok := result.RootCount(body1); !ok || count != 2 {
		t.Fatalf("RootCount(entry) = %d/%v, want 2", count, ok)
	}
	if root, ok := result.RootAt(body1, 0); !ok || root != returnTerm {
		t.Fatalf("RootAt(entry, 0) = %v/%v", root, ok)
	}
	if root, ok := result.RootAt(body1, 1); !ok || root != body2 {
		t.Fatalf("RootAt(entry, 1) = %v/%v", root, ok)
	}
	if count, ok := result.RootCount(body2); !ok || count != 0 {
		t.Fatalf("RootCount(body2) = %d/%v, want 0/true", count, ok)
	}
	if activation, ok := result.Activation(body2); !ok || activation != 0 {
		t.Fatalf("Activation(body2) = %v/%v, want chunk", activation, ok)
	}
	if loop, ok := result.NearestLoop(body2); !ok || loop != 0 {
		t.Fatalf("NearestLoop(body2) = %v/%v, want none", loop, ok)
	}
}

func TestSealNestedCallIsClosedByContainmentNotBody(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	directCall := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	callee := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyCall: 2,
		keyspace.FamilyValues: 1, keyspace.FamilyInteger: 1,
	}}
	input.Values = authored.ValuesInput{
		Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
		Terms: []keyspace.Term{callee},
	}
	input.Calls = []authored.Call{
		{Owner: body, Callee: callee, Actuals: values},
		{Owner: body, Callee: callee, Actuals: values},
	}

	// The second Call is an expression child. Its absence from Source direct
	// order is intentional: containment, not Body, will close that relation.
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{{directCall}}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	result, err := Seal(preimage, view, staticView, body)
	if err != nil {
		t.Fatalf("Seal nested Call: %v", err)
	}
	if count, ok := result.RootCount(body); !ok || count != 1 {
		t.Fatalf("RootCount = %d/%v, want one direct Call root", count, ok)
	}
	if root, ok := result.RootAt(body, 0); !ok || root != directCall {
		t.Fatalf("RootAt(0) = %v/%v, want direct Call %v", root, ok, directCall)
	}
	if _, ok := result.RootAt(body, 1); ok {
		t.Fatal("nested Call was incorrectly published as a Body root")
	}
}

func TestSealDirectCallRemainsBodyRoot(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	firstCall := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	secondCall := keyspace.MakeTerm(keyspace.FamilyCall, 2)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	callee := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyCall: 2,
		keyspace.FamilyValues: 1, keyspace.FamilyInteger: 1,
	}}
	input.Values = authored.ValuesInput{
		Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
		Terms: []keyspace.Term{callee},
	}
	input.Calls = []authored.Call{
		{Owner: body, Callee: callee, Actuals: values},
		{Owner: body, Callee: callee, Actuals: values},
	}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{{firstCall, secondCall}}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	result, err := Seal(preimage, view, staticView, body)
	if err != nil {
		t.Fatalf("Seal direct Calls: %v", err)
	}
	if count, ok := result.RootCount(body); !ok || count != 2 {
		t.Fatalf("RootCount = %d/%v, want two direct Call roots", count, ok)
	}
	for index, want := range []keyspace.Term{firstCall, secondCall} {
		if root, ok := result.RootAt(body, index); !ok || root != want {
			t.Fatalf("RootAt(%d) = %v/%v, want %v", index, root, ok, want)
		}
	}
}

func TestSealFunctionResetsOuterLoopAndBranchHosts(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	body4 := keyspace.MakeTerm(keyspace.FamilyBody, 4)
	loop1 := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	function1 := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 4, keyspace.FamilyNil: 1, keyspace.FamilyLoop: 1,
		keyspace.FamilyFunction: 1,
	}}
	input.Values.Rows = nil
	input.Functions.Rows = []authored.Function{{Owner: body2, Body: body3}}
	input.Control.Loops = []authored.Loop{{Owner: body1, Body: body2, Kind: kind.LoopWhile, Control: keyspace.MakeTerm(keyspace.FamilyNil, 1)}}
	input.Control.Branches = []authored.Branch{{Owner: body3, Condition: keyspace.MakeTerm(keyspace.FamilyValues, 1), WhenTrue: body4, WhenFalse: body1}}
	// The branch's false arm is deliberately changed below to a fresh body in
	// the separate host law; this fixture checks Function reset only.
	input.Counts[keyspace.FamilyBranch] = 0
	input.Control.Branches = nil
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{
		{loop1},
		nil,
		{body4},
		nil,
	}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()

	result, err := Seal(preimage, view, staticView, body1)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if activation, ok := result.Activation(body2); !ok || activation != 0 {
		t.Fatalf("Activation(loop body) = %v/%v, want chunk", activation, ok)
	}
	if loop, ok := result.NearestLoop(body2); !ok || loop != loop1 {
		t.Fatalf("NearestLoop(loop body) = %v/%v, want %v", loop, ok, loop1)
	}
	if activation, ok := result.Activation(body3); !ok || activation != function1 {
		t.Fatalf("Activation(function body) = %v/%v, want %v", activation, ok, function1)
	}
	if loop, ok := result.NearestLoop(body3); !ok || loop != 0 {
		t.Fatalf("NearestLoop(function body) = %v/%v, want reset", loop, ok)
	}
	if activation, ok := result.Activation(body4); !ok || activation != function1 {
		t.Fatalf("Activation(nested body) = %v/%v, want %v", activation, ok, function1)
	}
	if loop, ok := result.NearestLoop(body4); !ok || loop != 0 {
		t.Fatalf("NearestLoop(nested body) = %v/%v, want reset", loop, ok)
	}
	if root, ok := result.RootAt(body1, 0); !ok || root != loop1 {
		t.Fatalf("RootAt(entry, 0) = %v/%v", root, ok)
	}
}

func TestSealNestedFunctionAndLoopReset(t *testing.T) {
	bodies := make([]keyspace.Term, 6)
	for index := range bodies {
		bodies[index] = keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
	}
	loopOuter := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	loopInner := keyspace.MakeTerm(keyspace.FamilyLoop, 2)
	functionOuter := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	functionInner := keyspace.MakeTerm(keyspace.FamilyFunction, 2)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 6, keyspace.FamilyLoop: 2, keyspace.FamilyFunction: 2, keyspace.FamilyNil: 1,
	}}
	input.Functions.Rows = []authored.Function{
		{Owner: bodies[1], Body: bodies[2]},
		{Owner: bodies[3], Body: bodies[4]},
	}
	input.Control.Loops = []authored.Loop{
		{Owner: bodies[0], Body: bodies[1], Kind: kind.LoopWhile, Control: keyspace.MakeTerm(keyspace.FamilyNil, 1)},
		{Owner: bodies[2], Body: bodies[3], Kind: kind.LoopWhile, Control: keyspace.MakeTerm(keyspace.FamilyNil, 1)},
	}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{
		{loopOuter}, nil, {loopInner}, nil, {bodies[5]}, nil,
	}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	result, err := Seal(preimage, view, staticView, bodies[0])
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	cases := []struct {
		body, activation, loop keyspace.Term
	}{
		{bodies[1], 0, loopOuter},
		{bodies[2], functionOuter, 0},
		{bodies[3], functionOuter, loopInner},
		{bodies[4], functionInner, 0},
		{bodies[5], functionInner, 0},
	}
	for _, want := range cases {
		if got, ok := result.Activation(want.body); !ok || got != want.activation {
			t.Fatalf("Activation(%v) = %v/%v, want %v", want.body, got, ok, want.activation)
		}
		if got, ok := result.NearestLoop(want.body); !ok || got != want.loop {
			t.Fatalf("NearestLoop(%v) = %v/%v, want %v", want.body, got, ok, want.loop)
		}
	}
}

func TestSealRejectsSharedBranchArm(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyBranch: 1}}
	input.Control.Branches = []authored.Branch{{Owner: body1, WhenTrue: body2, WhenFalse: body2}}
	if _, err := authored.Build(input); err == nil {
		t.Fatal("authored branch accepted shared arm")
	}
}

func TestSealBranchAndLoopHosts(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 4, keyspace.FamilyNil: 1, keyspace.FamilyBranch: 1, keyspace.FamilyLoop: 1,
	}}
	input.Control.Branches = []authored.Branch{{Owner: body1, Condition: keyspace.MakeTerm(keyspace.FamilyNil, 1), WhenTrue: body2, WhenFalse: body3}}
	input.Control.Loops = []authored.Loop{{Owner: body2, Body: keyspace.MakeTerm(keyspace.FamilyBody, 4), Kind: kind.LoopWhile, Control: keyspace.MakeTerm(keyspace.FamilyNil, 1)}}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{{branch}, {loop}, nil, nil}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()

	result, err := Seal(preimage, view, staticView, body1)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if parent, ok := result.Parent(body2); !ok || parent != body1 {
		t.Fatalf("Parent(true arm) = %v/%v", parent, ok)
	}
	if parent, ok := result.Parent(body3); !ok || parent != body1 {
		t.Fatalf("Parent(false arm) = %v/%v", parent, ok)
	}
	loopBody := keyspace.MakeTerm(keyspace.FamilyBody, 4)
	if loopAt, ok := result.NearestLoop(loopBody); !ok || loopAt != loop {
		t.Fatalf("NearestLoop(loop body) = %v/%v, want %v", loopAt, ok, loop)
	}
}

func TestSealRejectsDuplicateOrphanAndCycle(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyFunction: 1}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2}}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{{body2}, nil}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	if _, err := Seal(preimage, view, staticView, body1); err == nil {
		t.Fatal("shared direct/Function child was accepted")
	}
	_ = function

	input = authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 3}}
	view, staticView, finish, preimage, sourceFinish = prepare(t, [][]keyspace.Term{{body2}, {keyspace.MakeTerm(keyspace.FamilyBody, 1)}, nil}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	if _, err := Seal(preimage, view, staticView, body1); err == nil {
		t.Fatal("cycle was accepted")
	}

	view, staticView, finish, preimage, sourceFinish = prepare(t, [][]keyspace.Term{{body2}, nil, nil}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	if _, err := Seal(preimage, view, staticView, body1); err == nil {
		t.Fatal("orphan Body was accepted")
	}
}

func TestSealRejectsExpiredViewsAndKeepsResultImmutable(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{nil}, input)
	if err := finish.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := sourceFinish.Abort(); err != nil {
		t.Fatal(err)
	}
	if _, err := Seal(preimage, view, staticView, body); err == nil {
		t.Fatal("expired owner views were accepted")
	}

	view, staticView, finish, preimage, sourceFinish = prepare(t, [][]keyspace.Term{{}}, input)
	result, err := Seal(preimage, view, staticView, body)
	if err != nil {
		t.Fatal(err)
	}
	rootCount, rootOK := result.RootCount(body)
	if result.BodyCount() != 1 || !rootOK || rootCount != 0 {
		t.Fatal("valid result changed during query")
	}
	_ = finish
	_ = sourceFinish
}

func TestSealStaticEmptyViewAvailability(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{nil}, input)
	if !staticView.Available() {
		t.Fatal("live empty Static view reported unavailable")
	}
	if _, err := Seal(preimage, view, staticView, body); err != nil {
		t.Fatalf("Seal live empty Static view: %v", err)
	}
	if !staticView.Available() {
		t.Fatal("immutable empty Static view became unavailable")
	}
	if _, err := Seal(preimage, view, staticView, body); err != nil {
		t.Fatalf("Seal reused immutable empty Static view: %v", err)
	}
	_ = finish.Abort()
	_ = sourceFinish.Abort()
}

func TestSealRejectsEachExpiredOwnerIndependently(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("coordinate fixture rejected")
	}
	staticInput := static.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyTypeAlias: 1, keyspace.FamilyTypePrimitive: 1,
	}, Types: statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}}}, Declarations: staticdecl.Input{
		Alias: []staticdecl.TypeAlias{{Owner: body, Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 1, NameCoordinate: coordinate}},
	}}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{{keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)}}, authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}}, staticInput)
	if err := finish.Abort(); err != nil {
		t.Fatal(err)
	}
	if _, err := Seal(preimage, view, staticView, body); err == nil {
		t.Fatal("expired authored View accepted")
	}
	_ = sourceFinish.Abort()

	view, staticView, finish, preimage, sourceFinish = prepare(t, [][]keyspace.Term{{keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)}}, authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}}, staticInput)
	if err := sourceFinish.Abort(); err != nil {
		t.Fatal(err)
	}
	if _, err := Seal(preimage, view, staticView, body); err == nil {
		t.Fatal("expired Source Preimage accepted")
	}
	_ = finish.Abort()
}

func TestSealConcurrentTerminalDoesNotPanic(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{nil}, authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}})
	start := make(chan struct{})
	done := make(chan struct{}, 2)
	go func() {
		<-start
		_, _ = Seal(preimage, view, staticView, body)
		done <- struct{}{}
	}()
	go func() {
		<-start
		_ = finish.Abort()
		_ = sourceFinish.Abort()
		done <- struct{}{}
	}()
	close(start)
	<-done
	<-done
}

func TestSealHandlesDeepBodyChain(t *testing.T) {
	const depth = 8192
	rows := make([][]keyspace.Term, depth)
	for index := 0; index+1 < depth; index++ {
		rows[index] = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+2))}
	}
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: depth}}
	view, staticView, finish, preimage, sourceFinish := prepare(t, rows, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	result, err := Seal(preimage, view, staticView, keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err != nil {
		t.Fatalf("Seal deep chain: %v", err)
	}
	last := keyspace.MakeTerm(keyspace.FamilyBody, depth)
	if parent, ok := result.Parent(last); !ok || parent != keyspace.MakeTerm(keyspace.FamilyBody, depth-1) {
		t.Fatalf("last Parent = %v/%v", parent, ok)
	}
	if !result.Contains(keyspace.MakeTerm(keyspace.FamilyBody, 1), last) ||
		result.Contains(last, keyspace.MakeTerm(keyspace.FamilyBody, 1)) {
		t.Fatal("deep Body intervals have incorrect ancestry direction")
	}
}

func TestResultQueriesDoNotAllocate(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{nil}, input)
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	result, err := Seal(preimage, view, staticView, body)
	if err != nil {
		t.Fatal(err)
	}
	var termSink keyspace.Term
	var boolSink bool
	var intSink int
	allocs := testing.AllocsPerRun(100, func() {
		intSink = result.BodyCount()
		termSink, boolSink = result.BodyAt(0)
		termSink, boolSink = result.Parent(body)
		intSink, boolSink = result.RootCount(body)
		termSink, boolSink = result.RootAt(body, 0)
		termSink, boolSink = result.Activation(body)
		termSink, boolSink = result.NearestLoop(body)
	})
	if allocs != 0 {
		t.Fatalf("queries allocated %f times", allocs)
	}
	_, _, _, _ = termSink, boolSink, intSink, allocs
}

func TestSealAdmitsLabelFaultAliasAndInterfaceRoots(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	iface := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 3)
	if !ok {
		t.Fatal("coordinate fixture rejected")
	}
	staticInput := static.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyTypeAlias: 1, keyspace.FamilyTypeInterface: 1, keyspace.FamilyTypePrimitive: 1,
	}, Types: statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}}}, Declarations: staticdecl.Input{
		Alias:     []staticdecl.TypeAlias{{Owner: body, Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 1, NameCoordinate: coordinate}},
		Interface: []staticdecl.Interface{{Owner: body, Name: 2, NameCoordinate: coordinate}},
	}}
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyLabel: 1}}
	input.Control.Labels = []authored.Label{{Owner: body}}
	view, staticView, finish, preimage, sourceFinish := prepareWithSource(t, [][]keyspace.Term{{label, fault, alias, iface}}, input, staticInput, []source.ControlFault{{Owner: body, Kind: source.ControlFaultUndefinedGoto}})
	defer func() { _ = finish.Abort() }()
	defer func() { _ = sourceFinish.Abort() }()
	result, err := Seal(preimage, view, staticView, body)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if got, ok := result.RootCount(body); !ok || got != 0 {
		t.Fatalf("non-statement roots entered root index: %d/%v", got, ok)
	}
}

func TestSealRejectsMissingAndWrongOwnerDenominatorRows(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("coordinate fixture rejected")
	}
	labelInput := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyLabel: 1}}
	labelInput.Control.Labels = []authored.Label{{Owner: body2}}
	view, staticView, finish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{{label}, nil}, labelInput)
	if _, err := Seal(preimage, view, staticView, body1); err == nil {
		t.Fatal("wrong-body Label accepted")
	}
	_ = finish.Abort()
	_ = sourceFinish.Abort()

	labelInput.Control.Labels[0].Owner = body1
	view, staticView, finish, preimage, sourceFinish = prepare(t, [][]keyspace.Term{nil, nil}, labelInput)
	if _, err := Seal(preimage, view, staticView, body1); err == nil {
		t.Fatal("missing Label source row accepted")
	}
	_ = finish.Abort()
	_ = sourceFinish.Abort()

	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	staticInput := static.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyTypeAlias: 1, keyspace.FamilyTypePrimitive: 1,
	}, Types: statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}}}, Declarations: staticdecl.Input{
		Alias: []staticdecl.TypeAlias{{Owner: body2, Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 1, NameCoordinate: coordinate}},
	}}
	plainInput := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2}}
	view, staticView, finish, preimage, sourceFinish = prepare(t, [][]keyspace.Term{{alias}, nil}, plainInput, staticInput)
	if _, err := Seal(preimage, view, staticView, body1); err == nil {
		t.Fatal("wrong-body TypeAlias accepted")
	}
	_ = finish.Abort()
	_ = sourceFinish.Abort()

	staticInput.Declarations.Alias[0].Owner = body1
	view, staticView, finish, preimage, sourceFinish = prepare(t, [][]keyspace.Term{nil, nil}, plainInput, staticInput)
	if _, err := Seal(preimage, view, staticView, body1); err == nil {
		t.Fatal("missing TypeAlias source row accepted")
	}
	_ = finish.Abort()
	_ = sourceFinish.Abort()

	iface := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	staticInterface := static.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyTypeInterface: 1,
	}, Declarations: staticdecl.Input{
		Interface: []staticdecl.Interface{{Owner: body2, Name: 1, NameCoordinate: coordinate}},
	}}
	view, staticView, finish, preimage, sourceFinish = prepare(t, [][]keyspace.Term{{iface}, nil}, plainInput, staticInterface)
	if _, err := Seal(preimage, view, staticView, body1); err == nil {
		t.Fatal("wrong-body TypeInterface accepted")
	}
	_ = finish.Abort()
	_ = sourceFinish.Abort()

	staticInterface.Declarations.Interface[0].Owner = body1
	view, staticView, finish, preimage, sourceFinish = prepare(t, [][]keyspace.Term{nil, nil}, plainInput, staticInterface)
	if _, err := Seal(preimage, view, staticView, body1); err == nil {
		t.Fatal("missing TypeInterface source row accepted")
	}
	_ = finish.Abort()
	_ = sourceFinish.Abort()
}

func TestSealSourceDenominatorRejectsDuplicateLabelFaultAliasInterface(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cases := []struct {
		name   string
		family keyspace.Family
		term   keyspace.Term
		fault  []source.ControlFault
	}{
		{name: "Label", family: keyspace.FamilyLabel, term: keyspace.MakeTerm(keyspace.FamilyLabel, 1)},
		{name: "ControlFault", family: keyspace.FamilyControlFault, term: keyspace.MakeTerm(keyspace.FamilyControlFault, 1), fault: []source.ControlFault{{Owner: body, Kind: source.ControlFaultUndefinedGoto}}},
		{name: "TypeAlias", family: keyspace.FamilyTypeAlias, term: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
		{name: "TypeInterface", family: keyspace.FamilyTypeInterface, term: keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)},
	}
	for _, test := range cases {
		counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}
		counts[test.family] = 1
		if err := buildSourceForBodyTerms([][]keyspace.Term{{test.term, test.term}}, counts, test.fault); err == nil {
			t.Fatalf("duplicate %s source row accepted", test.name)
		}
	}
}

func buildSourceForBodyTerms(rows [][]keyspace.Term, counts [keyspace.FamilyCount]uint32, faults []source.ControlFault) error {
	input := source.Input{Name: "body-denominator.lua", Faults: append([]source.ControlFault(nil), faults...)}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			spans[index] = source.Span{File: input.Name, StartLine: uint32(index + 1), StartCol: 1, EndLine: uint32(index + 1), EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, len(rows))
	for index, terms := range rows {
		input.Bodies[index] = source.BodySource{Body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)), Terms: append([]keyspace.Term(nil), terms...)}
	}
	_, err := source.Build(input)
	return err
}

// TestSourceRejectsForbiddenFunctionAndLiteralDirectTerms belongs to Source:
// Source owns the authored Body-order denominator and rejects these families
// before a Body view can exist. They are therefore not reachable Body.Seal
// inputs, and asserting their rejection through prepare would test fixture
// construction rather than Body's law.
func TestSourceRejectsForbiddenFunctionAndLiteralDirectTerms(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cases := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyFunction, 1),
		keyspace.MakeTerm(keyspace.FamilyNil, 1),
	}
	for _, forbidden := range cases {
		counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}
		input := source.Input{
			Name:   "body-denominator.lua",
			Bodies: []source.BodySource{{Body: body, Terms: []keyspace.Term{forbidden}}},
		}
		switch keyspace.TermFamily(forbidden) {
		case keyspace.FamilyFunction:
			counts[keyspace.FamilyFunction] = 1
			input.Functions = []source.FunctionFormals{{Function: forbidden}}
		case keyspace.FamilyNil:
			counts[keyspace.FamilyNil] = 1
			input.Nil = []source.NilLiteral{{Owner: body}}
		}
		for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
			spans := make([]source.Span, counts[family])
			for index := range spans {
				spans[index] = source.Span{
					File: input.Name, StartLine: uint32(index + 1), StartCol: 1,
					EndLine: uint32(index + 1), EndCol: 1,
				}
			}
			input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
		}
		if _, err := source.Build(input); err == nil {
			t.Fatalf("forbidden direct term %v accepted", forbidden)
		}
	}
}

func prepare(t *testing.T, rows [][]keyspace.Term, input authored.Input, staticInputs ...static.Input) (authored.View, staticquery.View, authored.Finalizer, source.Preimage, source.Finalizer) {
	staticInput := static.Input{}
	if len(staticInputs) != 0 {
		staticInput = staticInputs[0]
	}
	return prepareWithSourceNamed(t, rows, input, staticInput, nil, "body-test.lua")
}

func prepareWithSource(t *testing.T, rows [][]keyspace.Term, input authored.Input, staticInput static.Input, faults []source.ControlFault) (authored.View, staticquery.View, authored.Finalizer, source.Preimage, source.Finalizer) {
	return prepareWithSourceNamed(t, rows, input, staticInput, faults, "body-test.lua")
}

func prepareNamed(t *testing.T, rows [][]keyspace.Term, input authored.Input, name string) (authored.View, staticquery.View, authored.Finalizer, source.Preimage, source.Finalizer) {
	return prepareWithSourceNamed(t, rows, input, static.Input{}, nil, name)
}

func prepareWithSourceNamed(t *testing.T, rows [][]keyspace.Term, input authored.Input, staticInput static.Input, faults []source.ControlFault, name string) (authored.View, staticquery.View, authored.Finalizer, source.Preimage, source.Finalizer) {
	t.Helper()
	bodyCount := len(rows)
	if bodyCount == 0 {
		t.Fatal("empty body fixture")
	}
	counts := input.Counts
	counts[keyspace.FamilyBody] = uint32(bodyCount)
	for _, row := range rows {
		for _, term := range row {
			family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
			if family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal > counts[family] {
				counts[family] = ordinal
			}
		}
	}
	input.Counts = counts
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if staticInput.Counts[family] > counts[family] {
			counts[family] = staticInput.Counts[family]
		}
	}
	if uint32(len(faults)) > counts[keyspace.FamilyControlFault] {
		counts[keyspace.FamilyControlFault] = uint32(len(faults))
	}
	input.Counts = counts
	flowDraft, err := authored.Build(input)
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinish, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinish.View()
	_, staticView, err := static.Build(staticInput)
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}

	if name == "" {
		name = "body-test.lua"
	}
	sourceInput := source.Input{Name: name}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := int(counts[family])
		spans := make([]source.Span, count)
		for index := range spans {
			spans[index] = source.Span{File: sourceInput.Name, StartLine: uint32(index + 1), StartCol: 1, EndLine: uint32(index + 1), EndCol: 1}
		}
		sourceInput.Families = append(sourceInput.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	sourceInput.Bodies = make([]source.BodySource, bodyCount)
	for index, row := range rows {
		sourceInput.Bodies[index] = source.BodySource{Body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)), Terms: append([]keyspace.Term(nil), row...)}
	}
	sourceInput.Binds = make([]source.BindCells, counts[keyspace.FamilyBind])
	for index := range sourceInput.Binds {
		sourceInput.Binds[index].Bind = keyspace.MakeTerm(keyspace.FamilyBind, uint32(index+1))
	}
	sourceInput.Functions = make([]source.FunctionFormals, counts[keyspace.FamilyFunction])
	for index := range sourceInput.Functions {
		sourceInput.Functions[index].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
	}
	for index := 0; index < int(counts[keyspace.FamilyNil]); index++ {
		sourceInput.Nil = append(sourceInput.Nil, source.NilLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1)})
	}
	for index := 0; index < int(counts[keyspace.FamilyBool]); index++ {
		sourceInput.Bool = append(sourceInput.Bool, source.BoolLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: true})
	}
	for index := 0; index < int(counts[keyspace.FamilyInteger]); index++ {
		sourceInput.Integer = append(sourceInput.Integer, source.IntegerLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: int64(index)})
	}
	for index := 0; index < int(counts[keyspace.FamilyFloat]); index++ {
		sourceInput.Float = append(sourceInput.Float, source.FloatLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Bits: uint64(index)})
	}
	for index := 0; index < int(counts[keyspace.FamilyString]); index++ {
		sourceInput.String = append(sourceInput.String, source.StringLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: "x"})
	}
	sourceInput.Faults = append([]source.ControlFault(nil), faults...)
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinish, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	return flowView, staticView, flowFinish, sourceFinish.Preimage(), sourceFinish
}
