package lower_test

import (
	"strconv"
	"strings"
	"testing"

	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// Activation is a final Flow projection from a Body to its executable
// activation owner. Guard inventories are deliberately not a second global
// query plane; Causal Edges retain their own decisions instead.
func TestActivationProjectionBindsFunctionBody(t *testing.T) {
	p := parseBindLower(t, `
local function child(flag)
  if flag then return end
end
`)
	functions := p.Flow().Authored().Functions()
	function, ok := functions.At(0)
	if !ok {
		t.Fatal("Function is absent")
	}
	_, body, _, ok := functions.Get(function)
	if !ok || body == 0 {
		t.Fatal("Function has no Body")
	}
	if activation, ok := p.Flow().Activation().For(body); !ok || activation != function {
		t.Fatalf("Activation(%v) = %v/%v, want Function %v", body, activation, ok, function)
	}
	if count, ok := p.Flow().Causal().Edges().ActivationCount(body); !ok || count == 0 {
		t.Fatalf("final activation Edge count = %d/%v, want live body edges", count, ok)
	}
}

func TestDeadClosureBodyIsNotExecutable(t *testing.T) {
	p := parseBindLower(t, `
do return end
local function child()
  if liveWhenCalled then end
end
`)
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, functionOK := p.Flow().Authored().Functions().Get(function)
	branch, branchOK := p.Source().Order().BodyAt(body, 0)
	if !functionOK || !branchOK {
		t.Fatal("dead closure shape is absent")
	}
	executable := p.Flow().Executable()
	if executable.Contains(function) || executable.Contains(body) || executable.Contains(branch) {
		t.Fatal("closure behind terminal source control became executable")
	}
	if count, ok := p.Flow().Causal().Edges().ActivationCount(body); ok && count != 0 {
		t.Fatalf("dead Function Body retained activation edges = %d/%v", count, ok)
	}
}

func TestActivationProjectionIsAlphaStable(t *testing.T) {
	left := lowerNamed(t, "activation_alpha.lua", `local function visit(flag) if flag then return end end`)
	right := lowerNamed(t, "activation_alpha.lua", `local function inspect(enabled) if enabled then return end end`)
	leftFunction, leftOK := left.Flow().Authored().Functions().At(0)
	rightFunction, rightOK := right.Flow().Authored().Functions().At(0)
	if !leftOK || !rightOK || leftFunction != rightFunction {
		t.Fatalf("alpha Function identities = %v/%v and %v/%v", leftFunction, leftOK, rightFunction, rightOK)
	}
	_, leftBody, _, _ := left.Flow().Authored().Functions().Get(leftFunction)
	_, rightBody, _, _ := right.Flow().Authored().Functions().Get(rightFunction)
	leftActivation, leftActivationOK := left.Flow().Activation().For(leftBody)
	rightActivation, rightActivationOK := right.Flow().Activation().For(rightBody)
	if !leftActivationOK || !rightActivationOK || leftActivation != rightActivation || leftActivation != leftFunction {
		t.Fatalf("alpha activation owners = %v/%v and %v/%v", leftActivation, leftActivationOK, rightActivation, rightActivationOK)
	}
}

func TestActivationQueryDoesNotAllocate(t *testing.T) {
	p := parseBindLower(t, `local function f() return 1 end`)
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, _ := p.Flow().Authored().Functions().Get(function)
	allocations := testing.AllocsPerRun(1000, func() {
		_, _ = p.Flow().Activation().For(body)
	})
	if allocations != 0 {
		t.Fatalf("Activation.For allocates %f times", allocations)
	}
}

func TestFlowActivationSeparatesNestedFunctionBodies(t *testing.T) {
	p := parseBindLower(t, `
local function outer()
  local function inner()
    return 1
  end
  return inner()
end
return outer()
`)
	functions := p.Flow().Authored().Functions()
	if functions.Count() != 2 {
		t.Fatalf("FunctionCount = %d, want 2", functions.Count())
	}
	outer, _ := functions.At(0)
	inner, _ := functions.At(1)
	_, outerBody, _, outerOK := functions.Get(outer)
	_, innerBody, _, innerOK := functions.Get(inner)
	if !outerOK || !innerOK || outerBody == 0 || innerBody == 0 {
		t.Fatal("nested Functions have malformed Bodies")
	}
	outerActivation, outerActivationOK := p.Flow().Activation().For(outerBody)
	innerActivation, innerActivationOK := p.Flow().Activation().For(innerBody)
	if !outerActivationOK || !innerActivationOK || outerActivation != outer || innerActivation != inner || outerActivation == innerActivation {
		t.Fatalf("nested activation owners = %v/%v and %v/%v", outerActivation, outerActivationOK, innerActivation, innerActivationOK)
	}
}

func TestFlowActivationDoesNotMakeDeadFunctionExecutable(t *testing.T) {
	p := parseBindLower(t, `
do return end
local function dead()
  return deadCall()
end
`)
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, functionOK := p.Flow().Authored().Functions().Get(function)
	call, callOK := p.Flow().Authored().Calls().At(0)
	if !functionOK || !callOK {
		t.Fatal("dead function shape is absent")
	}
	executable := p.Flow().Executable()
	if executable.Contains(function) || executable.Contains(body) || executable.Contains(call) {
		t.Fatal("dead source retained executable function state")
	}
}

func TestFlowActivationIsAlphaStable(t *testing.T) {
	left := lowerNamed(t, "activation_owner.lua", `local function visit(flag) if flag then return flag end end`)
	right := lowerNamed(t, "activation_owner.lua", `local function inspect(enabled) if enabled then return enabled end end`)
	leftFunction, leftOK := left.Flow().Authored().Functions().At(0)
	rightFunction, rightOK := right.Flow().Authored().Functions().At(0)
	if !leftOK || !rightOK || leftFunction != rightFunction {
		t.Fatalf("alpha Function identities = %v/%v and %v/%v", leftFunction, leftOK, rightFunction, rightOK)
	}
	_, leftBody, _, _ := left.Flow().Authored().Functions().Get(leftFunction)
	_, rightBody, _, _ := right.Flow().Authored().Functions().Get(rightFunction)
	leftActivation, leftActivationOK := left.Flow().Activation().For(leftBody)
	rightActivation, rightActivationOK := right.Flow().Activation().For(rightBody)
	if !leftActivationOK || !rightActivationOK || leftActivation != rightActivation || leftActivation != leftFunction {
		t.Fatalf("alpha Activation = %v/%v and %v/%v", leftActivation, leftActivationOK, rightActivation, rightActivationOK)
	}
}

func TestFlowActivationQueryDoesNotAllocate(t *testing.T) {
	p := parseBindLower(t, `local function child() return 1 end; return child()`)
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, _ := p.Flow().Authored().Functions().Get(function)
	allocations := testing.AllocsPerRun(1000, func() {
		_, _ = p.Flow().Activation().For(body)
	})
	if allocations != 0 {
		t.Fatalf("Activation.For allocates %f times", allocations)
	}
}

// TestSourceAlphaRenamingPreservesProgramMeaning fixes the alpha boundary for
// the source Program: lexical spellings are authored reflection, whereas the
// binding, static-resolution, direct-call and source-control relations are
// invariant under a consistent renaming of bound identifiers.
func TestSourceAlphaRenamingPreservesProgramMeaning(t *testing.T) {
	const leftSource = `
local captured = 1
local function recur<T>(formal: T): T
  local shadow = formal
  local function closure<U>(inner: U): U
    local shadow = inner
    captured = captured + 1
    return recur(shadow)
  end
  for loop = 1, 2 do
    local seen = loop
    shadow = seen
  end
  return closure(shadow)
end
local module = require("pkg.core")
type Alias<V> = module.Schema.Box<V>
return recur(captured)
`
	const rightSource = `
local payload = 1
local function cycle<Q>(argument: Q): Q
  local hidden = argument
  local function innerFn<R>(member: R): R
    local hidden = member
    payload = payload + 1
    return cycle(hidden)
  end
  for cursor = 1, 2 do
    local observed = cursor
    hidden = observed
  end
  return innerFn(hidden)
end
local service = require("pkg.core")
type Result<W> = service.Schema.Box<W>
return cycle(payload)
	`
	left := lowerNamed(t, "alpha.lua", leftSource)
	right := lowerNamed(t, "alpha.lua", rightSource)

	assertAlphaProgram(t, left, right)
	if left.ContentID() == right.ContentID() {
		t.Fatal("ContentID hid authored bound spelling")
	}
	relocated := lowerNamed(t, "alpha_relocated.lua", leftSource)
	if left.ContentID() == relocated.ContentID() {
		t.Fatal("ContentID hid authored logical source identity")
	}
}

func lowerNamed(t *testing.T, name, source string) *program.Program {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func assertAlphaProgram(t *testing.T, left, right *program.Program) {
	t.Helper()
	assertAlphaCounts(t, left, right)
	assertAlphaLexicalRoles(t, left, right)
	assertAlphaFunctionsAndCalls(t, left, right)
	assertAlphaQualifiedType(t, left, right)
}

func assertAlphaCounts(t *testing.T, left, right *program.Program) {
	t.Helper()
	leftFlow, rightFlow := left.Flow(), right.Flow()
	leftAuthored, rightAuthored := leftFlow.Authored(), rightFlow.Authored()
	leftStatic, rightStatic := left.Static(), right.Static()
	for _, pair := range []struct {
		name  string
		left  int
		right int
	}{
		{"Body", left.Source().Identity().FamilyCount(keyspace.FamilyBody), right.Source().Identity().FamilyCount(keyspace.FamilyBody)},
		{"Cell", leftAuthored.Storage().Cells().Count(), rightAuthored.Storage().Cells().Count()},
		{"Read", leftAuthored.Storage().Reads().Count(), rightAuthored.Storage().Reads().Count()},
		{"Bind", leftAuthored.Storage().Binds().Count(), rightAuthored.Storage().Binds().Count()},
		{"Assign", leftAuthored.Storage().Assigns().Count(), rightAuthored.Storage().Assigns().Count()},
		{"Function", leftAuthored.Functions().Count(), rightAuthored.Functions().Count()},
		{"Call", leftAuthored.Calls().Count(), rightAuthored.Calls().Count()},
		{"Loop", leftAuthored.Control().Loops().Count(), rightAuthored.Control().Loops().Count()},
		{"Import", left.Module().Count(), right.Module().Count()},
		{"TypeAlias", leftStatic.Declarations().Aliases().Count(), rightStatic.Declarations().Aliases().Count()},
		{"TypeParam", leftStatic.Declarations().TypeParams().Count(), rightStatic.Declarations().TypeParams().Count()},
		{"TypeRef", leftStatic.References().Count(), rightStatic.References().Count()},
	} {
		if pair.left != pair.right {
			t.Fatalf("%s topology = %d/%d", pair.name, pair.left, pair.right)
		}
	}
}

func assertAlphaLexicalRoles(t *testing.T, left, right *program.Program) {
	t.Helper()
	leftFlow, rightFlow := left.Flow(), right.Flow()
	leftCells, rightCells := leftFlow.Authored().Storage().Cells(), rightFlow.Authored().Storage().Cells()
	for index := 0; index < leftCells.Count(); index++ {
		leftCell, _ := leftCells.At(index)
		rightCell, _ := rightCells.At(index)
		leftKind, leftBody, _, leftOK := leftCells.Get(leftCell)
		rightKind, rightBody, _, rightOK := rightCells.Get(rightCell)
		if !leftOK || !rightOK || leftKind != rightKind || (leftBody == 0) != (rightBody == 0) {
			t.Fatalf("Cell[%d] = kind/body %v/%v/%v vs %v/%v/%v", index, leftKind, leftBody, leftOK, rightKind, rightBody, rightOK)
		}
	}

	// Capture topology is a relation between a fresh inner CellCapture and a
	// visible outer cell. Names may differ, but both roles and the ordered
	// capture relation must remain unchanged.
	leftFunctions, rightFunctions := leftFlow.Authored().Functions(), rightFlow.Authored().Functions()
	for functionIndex := 0; functionIndex < leftFunctions.Count(); functionIndex++ {
		leftFunction, _ := leftFunctions.At(functionIndex)
		rightFunction, _ := rightFunctions.At(functionIndex)
		leftCount, leftOK := leftFunctions.CaptureCount(leftFunction)
		rightCount, rightOK := rightFunctions.CaptureCount(rightFunction)
		if !leftOK || !rightOK || leftCount != rightCount {
			t.Fatalf("FunctionCaptureCount[%d] = %d/%v vs %d/%v", functionIndex, leftCount, leftOK, rightCount, rightOK)
		}
		for captureIndex := 0; captureIndex < leftCount; captureIndex++ {
			leftInner, leftOuter := functionCapture(t, left, leftFunction, captureIndex)
			rightInner, rightOuter := functionCapture(t, right, rightFunction, captureIndex)
			if leftInner == 0 || rightInner == 0 || leftOuter == 0 || rightOuter == 0 {
				t.Fatalf("FunctionCapture[%d,%d] contains an absent Cell", functionIndex, captureIndex)
			}
			if alphaCellIndex(t, left, leftInner) != alphaCellIndex(t, right, rightInner) ||
				alphaCellIndex(t, left, leftOuter) != alphaCellIndex(t, right, rightOuter) {
				t.Fatalf("FunctionCapture[%d,%d] changed lexical edge", functionIndex, captureIndex)
			}
		}
	}
}

func alphaCellIndex(t *testing.T, p *program.Program, cell keyspace.Term) int {
	t.Helper()
	cells := p.Flow().Authored().Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		candidate, _ := cells.At(index)
		if candidate == cell {
			return index
		}
	}
	t.Fatalf("%v is not a Cell", cell)
	return -1
}

func assertAlphaFunctionsAndCalls(t *testing.T, left, right *program.Program) {
	t.Helper()
	leftFlow, rightFlow := left.Flow(), right.Flow()
	leftFunctions, rightFunctions := leftFlow.Authored().Functions(), rightFlow.Authored().Functions()
	leftFormalsView, rightFormalsView := left.Source().Formals(), right.Source().Formals()
	leftStatic, rightStatic := left.Static(), right.Static()
	for index := 0; index < leftFunctions.Count(); index++ {
		leftFunction, _ := leftFunctions.At(index)
		rightFunction, _ := rightFunctions.At(index)
		leftFormals, leftOK := leftFormalsView.Len(leftFunction)
		rightFormals, rightOK := rightFormalsView.Len(rightFunction)
		if !leftOK || !rightOK || leftFormals != rightFormals {
			t.Fatalf("FormalLen[%d] = %d/%v vs %d/%v", index, leftFormals, leftOK, rightFormals, rightOK)
		}
		for formal := 0; formal < leftFormals; formal++ {
			leftCell, leftCellOK := leftFormalsView.At(leftFunction, formal)
			rightCell, rightCellOK := rightFormalsView.At(rightFunction, formal)
			if !leftCellOK || !rightCellOK || leftCell == 0 || rightCell == 0 {
				t.Fatalf("Formal[%d,%d] = %v/%v vs %v/%v", index, formal, leftCell, leftCellOK, rightCell, rightCellOK)
			}
		}
		leftTypes, leftOK := leftStatic.Contracts().Functions().TypeParamCount(leftFunction)
		rightTypes, rightOK := rightStatic.Contracts().Functions().TypeParamCount(rightFunction)
		if !leftOK || !rightOK || leftTypes != rightTypes {
			t.Fatalf("FunctionTypeParamCount[%d] = %d/%v vs %d/%v", index, leftTypes, leftOK, rightTypes, rightOK)
		}
		for typeParam := 0; typeParam < leftTypes; typeParam++ {
			leftTerm, _ := leftStatic.Contracts().Functions().TypeParamAt(leftFunction, typeParam)
			rightTerm, _ := rightStatic.Contracts().Functions().TypeParamAt(rightFunction, typeParam)
			_, leftKey, _, leftOK := leftStatic.Declarations().TypeParams().Get(leftTerm)
			_, rightKey, _, rightOK := rightStatic.Declarations().TypeParams().Get(rightTerm)
			if !leftOK || !rightOK {
				t.Fatalf("FunctionTypeParam[%d,%d] absent", index, typeParam)
			}
			alphaExactText(t, left, leftKey, []string{"T", "U"}[index])
			alphaExactText(t, right, rightKey, []string{"Q", "R"}[index])
		}
	}

	leftCalls, rightCalls := leftFlow.Authored().Calls(), rightFlow.Authored().Calls()
	for index := 0; index < leftCalls.Count(); index++ {
		leftCall, _ := leftCalls.At(index)
		rightCall, _ := rightCalls.At(index)
		leftDirect, leftOK := leftFlow.DirectFunctions().Call(leftCall)
		rightDirect, rightOK := rightFlow.DirectFunctions().Call(rightCall)
		if !leftOK || !rightOK || (leftDirect == 0) != (rightDirect == 0) {
			t.Fatalf("Call directness[%d] = %v/%v vs %v/%v", index, leftDirect, leftOK, rightDirect, rightOK)
		}
		if (leftDirect == 0) != (rightDirect == 0) {
			t.Fatalf("Call directness[%d] differs", index)
		}
		if leftDirect != 0 {
			leftIndex, rightIndex := alphaFunctionIndex(t, left, leftDirect), alphaFunctionIndex(t, right, rightDirect)
			if leftIndex != rightIndex {
				t.Fatalf("Call direct target[%d] = Function[%d]/Function[%d]", index, leftIndex, rightIndex)
			}
		}
	}
	leftLoops, rightLoops := leftFlow.Authored().Control().Loops(), rightFlow.Authored().Control().Loops()
	for index := 0; index < leftLoops.Count(); index++ {
		leftLoop, _ := leftLoops.At(index)
		rightLoop, _ := rightLoops.At(index)
		_, _, leftKind, _, leftOK := leftLoops.Get(leftLoop)
		_, _, rightKind, _, rightOK := rightLoops.Get(rightLoop)
		if !leftOK || !rightOK || leftKind != rightKind {
			t.Fatalf("Loop[%d] = %v/%v vs %v/%v", index, leftKind, leftOK, rightKind, rightOK)
		}
		assertLoopMuEdge(t, leftFlow, leftLoop, "left", index)
		assertLoopMuEdge(t, rightFlow, rightLoop, "right", index)
	}
}

func assertLoopMuEdge(t *testing.T, flowView flow.View, loop keyspace.Term, side string, index int) {
	t.Helper()
	edges := flowView.Causal().Edges()
	for edgeIndex := 0; edgeIndex < edges.Count(); edgeIndex++ {
		edge, ok := edges.At(edgeIndex)
		if ok && edge.Mu == loop {
			return
		}
	}
	t.Fatalf("%s Loop[%d] has no final causal recurrence Edge", side, index)
}

func alphaFunctionIndex(t *testing.T, p *program.Program, function keyspace.Term) int {
	t.Helper()
	functions := p.Flow().Authored().Functions()
	for index := 0; index < functions.Count(); index++ {
		candidate, _ := functions.At(index)
		if candidate == function {
			return index
		}
	}
	t.Fatalf("direct target %v is not a Function", function)
	return -1
}

func assertAlphaQualifiedType(t *testing.T, left, right *program.Program) {
	t.Helper()
	leftStatic, rightStatic := left.Static(), right.Static()
	leftAliases, rightAliases := leftStatic.Declarations().Aliases(), rightStatic.Declarations().Aliases()
	leftAlias, _ := leftAliases.At(0)
	rightAlias, _ := rightAliases.At(0)
	_, leftTarget, leftAliasKey, _, leftOK := leftAliases.Get(leftAlias)
	_, rightTarget, rightAliasKey, _, rightOK := rightAliases.Get(rightAlias)
	if !leftOK || !rightOK {
		t.Fatal("renamed static alias is absent")
	}
	alphaExactText(t, left, leftAliasKey, "Alias")
	alphaExactText(t, right, rightAliasKey, "Result")
	leftParam, leftParamOK := leftAliases.ParamAt(leftAlias, 0)
	rightParam, rightParamOK := rightAliases.ParamAt(rightAlias, 0)
	if !leftParamOK || !rightParamOK {
		t.Fatal("renamed alias parameter absent")
	}
	_, leftParamKey, _, leftParamOK := leftStatic.Declarations().TypeParams().Get(leftParam)
	_, rightParamKey, _, rightParamOK := rightStatic.Declarations().TypeParams().Get(rightParam)
	if !leftParamOK || !rightParamOK {
		t.Fatal("renamed alias parameter relation absent")
	}
	alphaExactText(t, left, leftParamKey, "V")
	alphaExactText(t, right, rightParamKey, "W")

	leftRef, _, leftOK := leftStatic.Types().Generics().Get(leftTarget)
	rightRef, _, rightOK := rightStatic.Types().Generics().Get(rightTarget)
	if !leftOK || !rightOK {
		t.Fatal("qualified generic alias target absent")
	}
	leftState, leftTargetTerm, leftRoot, leftStateOK := leftStatic.References().Get(leftRef)
	rightState, rightTargetTerm, rightRoot, rightStateOK := rightStatic.References().Get(rightRef)
	if !leftStateOK || !rightStateOK || leftState != rightState || leftState != static.TypeRefUnresolved || leftTargetTerm != 0 || rightTargetTerm != 0 || leftRoot == 0 || rightRoot == 0 {
		t.Fatalf("qualified TypeRef states = %v/%v/%v/%v vs %v/%v/%v/%v", leftState, leftTargetTerm, leftRoot, leftStateOK, rightState, rightTargetTerm, rightRoot, rightStateOK)
	}
	if left.Module().Count() != 1 || right.Module().Count() != 1 {
		t.Fatalf("imports = %d/%d, want one corresponding literal import", left.Module().Count(), right.Module().Count())
	}
	leftImport, leftOK := left.Module().ImportAt(0)
	rightImport, rightOK := right.Module().ImportAt(0)
	if !leftOK || !rightOK || leftRoot != leftImport.Alias || rightRoot != rightImport.Alias {
		t.Fatalf("qualified roots do not select their renamed import alias")
	}
	leftString, _, leftText, leftStringOK := left.Source().Literals().Strings().At(int(keyspace.TermOrdinal(leftImport.Request) - 1))
	rightString, _, rightText, rightStringOK := right.Source().Literals().Strings().At(int(keyspace.TermOrdinal(rightImport.Request) - 1))
	if !leftStringOK || !rightStringOK || leftString != leftImport.Request || rightString != rightImport.Request || leftText != "pkg.core" || rightText != "pkg.core" {
		t.Fatalf("literal import requests = %q/%v vs %q/%v", leftText, leftStringOK, rightText, rightStringOK)
	}
	leftKey, leftKeyOK := left.Source().Keys().Exact(leftImport.Key)
	rightKey, rightKeyOK := right.Source().Keys().Exact(rightImport.Key)
	if !leftKeyOK || !rightKeyOK || leftKey != rightKey || leftKey.Kind != keyspace.LiteralString || leftKey.String != "pkg.core" {
		t.Fatalf("module exact keys = %#v/%v vs %#v/%v", leftKey, leftKeyOK, rightKey, rightKeyOK)
	}
	alphaTypeRefPath(t, left, leftRef, []string{"module", "Schema", "Box"})
	alphaTypeRefPath(t, right, rightRef, []string{"service", "Schema", "Box"})
}

func alphaExactText(t *testing.T, p *program.Program, key keyspace.Key, want string) {
	t.Helper()
	value, ok := p.Source().Keys().Exact(key)
	if !ok || value.Kind != keyspace.LiteralString || value.String != want {
		t.Fatalf("ExactKey(%v) = %#v/%v, want string %q", key, value, ok, want)
	}
}

func alphaTypeRefPath(t *testing.T, p *program.Program, ref keyspace.Term, want []string) {
	t.Helper()
	references := p.Static().References()
	count, ok := references.SourceCount(ref)
	if !ok || count != len(want) {
		t.Fatalf("TypeRefSourceLen(%v) = %d/%v, want %d", ref, count, ok, len(want))
	}
	for index, text := range want {
		key, ok := references.SourceAt(ref, index)
		if !ok {
			t.Fatalf("TypeRefSourceAt(%v,%d) absent", ref, index)
		}
		alphaExactText(t, p, key, text)
	}
}

// Pending is the evaluation-owned immutable payload set.  It is queried from
// Flow directly; the retired Program-wide continuation-value forwarding plane
// is intentionally not recreated here.
func TestFlowPendingKeepsEarlierOperandsAtCallBoundaries(t *testing.T) {
	p := parseBindLower(t, `local result = left + yieldfn()`)
	call := continuationNamedCall(t, p, "yieldfn")
	binary, ok := p.Flow().Authored().Operators().Binaries().At(0)
	if !ok {
		t.Fatal("missing binary relation")
	}
	_, _, left, _, ok := p.Flow().Authored().Operators().Binaries().Get(binary)
	if !ok {
		t.Fatal("invalid binary relation")
	}
	continuationExactPending(t, p, call, left)

	p = parseBindLower(t, `object[keyfn()] = first(), yieldfn()`)
	call = continuationNamedCall(t, p, "yieldfn")
	first := continuationNamedCall(t, p, "first")
	assign, ok := p.Flow().Authored().Storage().Assigns().At(0)
	if !ok {
		t.Fatal("missing assignment")
	}
	write, ok := p.Flow().Authored().Storage().Assigns().WriteAt(assign, 0)
	if !ok {
		t.Fatal("missing assignment write")
	}
	_, target, ok := p.Flow().Authored().Storage().Writes().Get(write)
	if !ok {
		t.Fatal("invalid assignment write")
	}
	continuationExactPending(t, p, call, target, first)
}

// Continuation is the final Flow owner for lexical Cells and reaching Guards;
// a Call is merely the exact subject term, not a global continuation identity.
func TestFlowContinuationCellsFollowLexicalScope(t *testing.T) {
	t.Run("before and after locals", func(t *testing.T) {
		p := parseBindLower(t, `
local before = 1
yieldfn()
local after = 2
`)
		call := continuationNamedCall(t, p, "yieldfn")
		before := continuationCellAtLine(t, p, 2)
		after := continuationCellAtLine(t, p, 4)
		continuationExactCells(t, p, call, before)
		continuationLacksCell(t, p, call, after)
	})

	t.Run("captured outer cell is not an inner activation cell", func(t *testing.T) {
		p := parseBindLower(t, `
local outer = 0
local function worker()
  yieldfn()
  return outer
end
`)
		call := continuationNamedCall(t, p, "yieldfn")
		function, ok := p.Flow().Authored().Functions().At(0)
		if !ok {
			t.Fatal("missing worker Function")
		}
		inner, outer, ok := p.Flow().Authored().Functions().CaptureAt(function, 0)
		if !ok {
			t.Fatal("missing worker capture")
		}
		continuationExactCells(t, p, call, inner)
		continuationLacksCell(t, p, call, outer)
	})

	t.Run("dead and static calls have no final continuation root", func(t *testing.T) {
		staticProgram := parseBindLower(t, `type Snapshot = typeof(staticfn())`)
		staticCall := continuationNamedCall(t, staticProgram, "staticfn")
		if !staticProgram.Flow().Containment().Static(staticCall) {
			t.Fatal("typeof Call escaped static containment")
		}
		if count, ok := staticProgram.Flow().Continuation().CellCount(staticCall); ok || count != 0 {
			t.Fatalf("static Call continuation Cells = %d/%v, want absent", count, ok)
		}
		if count, ok := staticProgram.Flow().Pending().Count(staticCall); ok || count != 0 {
			t.Fatalf("static Call pending payloads = %d/%v, want absent", count, ok)
		}

		deadProgram := parseBindLower(t, `
goto done
unreachablefn()
::done::
`)
		deadCall := continuationNamedCall(t, deadProgram, "unreachablefn")
		if deadProgram.Flow().Executable().Contains(deadCall) {
			t.Fatal("terminally unreachable Call became executable")
		}
		if count, ok := deadProgram.Flow().Continuation().CellCount(deadCall); ok || count != 0 {
			t.Fatalf("dead Call continuation Cells = %d/%v, want absent", count, ok)
		}
		if count, ok := deadProgram.Flow().Pending().Count(deadCall); ok || count != 0 {
			t.Fatalf("dead Call pending payloads = %d/%v, want absent", count, ok)
		}
	})
}

func TestFlowContinuationCellOrderIsStableAndAllocationFree(t *testing.T) {
	const depth = 256
	p := continuationDeepProgram(t, depth)
	call := continuationNamedCall(t, p, "yielddeep")
	continuation := p.Flow().Continuation()
	want, ok := continuation.CellCount(call)
	if !ok || want != depth {
		t.Fatalf("deep continuation Cell count = %d/%v, want %d", want, ok, depth)
	}
	for index := 0; index < want; index++ {
		cell, cellOK := continuation.CellAt(call, index)
		if !cellOK || cell == 0 {
			t.Fatalf("continuation Cell %d = %v/%v", index, cell, cellOK)
		}
		if index == 0 || index == want-1 {
			span, spanOK := p.Source().Identity().Span(cell)
			if !spanOK {
				t.Fatalf("continuation Cell %d has no Source span", index)
			}
			wantLine := depth - index*(depth-1)
			if int(span.StartLine) != wantLine {
				t.Fatalf("continuation Cell %d line = %d, want %d", index, span.StartLine, wantLine)
			}
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		for index := 0; index < want; index++ {
			continuationCellSink, _ = continuation.CellAt(call, index)
		}
	}); allocations != 0 {
		t.Fatalf("deep continuation Cell enumeration allocations = %v, want 0", allocations)
	}
}

func TestFlowContinuationContentIDHandlesDeepScope(t *testing.T) {
	p := continuationDeepProgram(t, 256)
	first := p.ContentID()
	if !first.Available() {
		t.Fatal("deep continuation Program has no ContentID")
	}
	if second := p.ContentID(); second != first {
		t.Fatalf("deep continuation ContentID is not deterministic: %x != %x", second, first)
	}
}

var continuationCellSink keyspace.Term

type continuationTestingT interface {
	Helper()
	Fatalf(string, ...any)
}

func continuationDeepProgram(t continuationTestingT, depth int) *program.Program {
	t.Helper()
	if depth <= 0 {
		t.Fatalf("invalid continuation depth %d", depth)
	}
	var input strings.Builder
	for index := 0; index < depth; index++ {
		input.WriteString("do local local")
		input.WriteString(strconv.Itoa(index))
		input.WriteString(" = ")
		input.WriteString(strconv.Itoa(index))
		input.WriteString("\n")
	}
	input.WriteString("yielddeep()\n")
	for index := 0; index < depth; index++ {
		input.WriteString("end\n")
	}
	p, err := lowerSource(input.String())
	if err != nil {
		t.Fatalf("deep continuation lower: %v", err)
	}
	return p
}

func continuationNamedCall(t continuationTestingT, p *program.Program, name string) keyspace.Term {
	t.Helper()
	flowView := p.Flow()
	calls := flowView.Authored().Calls()
	reads := flowView.Authored().Storage().Reads()
	cells := flowView.Authored().Storage().Cells()
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		_, callee, _, _, ok := calls.Get(call)
		if !ok {
			continue
		}
		_, cell, _, ok := reads.Get(callee)
		if !ok {
			continue
		}
		_, _, key, ok := cells.Get(cell)
		if !ok {
			continue
		}
		literal, ok := p.Source().Keys().Exact(key)
		if ok && literal.Kind == keyspace.LiteralString && literal.String == name {
			return call
		}
	}
	t.Fatalf("missing Call to %q", name)
	return 0
}

func continuationCellAtLine(t *testing.T, p *program.Program, line int) keyspace.Term {
	t.Helper()
	cells := p.Flow().Authored().Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		cell, _ := cells.At(index)
		span, ok := p.Source().Identity().Span(cell)
		if ok && int(span.StartLine) == line {
			return cell
		}
	}
	t.Fatalf("missing Cell at line %d", line)
	return 0
}

func continuationExactCells(t *testing.T, p *program.Program, call keyspace.Term, want ...keyspace.Term) {
	t.Helper()
	continuation := p.Flow().Continuation()
	count, ok := continuation.CellCount(call)
	if !ok || count != len(want) {
		t.Fatalf("Call %v continuation Cell count = %d/%v, want %d", call, count, ok, len(want))
	}
	for _, expected := range want {
		found := false
		for index := 0; index < count; index++ {
			got, gotOK := continuation.CellAt(call, index)
			found = found || gotOK && got == expected
		}
		if !found {
			t.Fatalf("Call %v continuation Cells do not contain %v", call, expected)
		}
	}
}

func continuationLacksCell(t *testing.T, p *program.Program, call, unwanted keyspace.Term) {
	t.Helper()
	continuation := p.Flow().Continuation()
	count, ok := continuation.CellCount(call)
	if !ok {
		t.Fatalf("Call %v has no continuation Cell projection", call)
	}
	for index := 0; index < count; index++ {
		got, gotOK := continuation.CellAt(call, index)
		if gotOK && got == unwanted {
			t.Fatalf("Call %v continuation Cells unexpectedly contain %v", call, unwanted)
		}
	}
}

func continuationExactPending(t *testing.T, p *program.Program, call keyspace.Term, want ...keyspace.Term) {
	t.Helper()
	pending := p.Flow().Pending()
	count, ok := pending.Count(call)
	if !ok || count != len(want) {
		t.Fatalf("Call %v pending payload count = %d/%v, want %d", call, count, ok, len(want))
	}
	for _, expected := range want {
		found := false
		for index := 0; index < count; index++ {
			got, gotOK := pending.At(call, index)
			found = found || gotOK && got == expected
		}
		if !found {
			t.Fatalf("Call %v pending payloads do not contain %v", call, expected)
		}
	}
}

// Constructor laws consume the exact Source and Flow owner rows.  They do not
// reconstruct a generic Program graph or a per-term recurrence identity.
func TestSourceNestedOperandsKeepAuthoredComposition(t *testing.T) {
	p := parseBindLower(t, "return not ((a()[b()][c()] + d()) and e(f(g()[h()])))")
	returned, ok := p.Flow().Authored().Control().Returns().At(0)
	if !ok {
		t.Fatal("missing Return")
	}
	_, values, returnOK := p.Flow().Authored().Control().Returns().Get(returned)
	if !returnOK {
		t.Fatal("invalid Return row")
	}
	outerUnary := valueAt(t, p, values, 0)
	_, unaryOp, selected, unaryOK := p.Flow().Authored().Operators().Unaries().Get(outerUnary)
	if !unaryOK || unaryOp != kind.UnaryNot || selected == 0 {
		t.Fatalf("outer Unary = op %v operand %v ok %v", unaryOp, selected, unaryOK)
	}
	_, selectOp, binary, outerCall, selectOK := p.Flow().Authored().Operators().Selects().Get(selected)
	if !selectOK || selectOp != kind.SelectAnd || binary == 0 || outerCall == 0 {
		t.Fatalf("Select = op %v left %v right %v ok %v", selectOp, binary, outerCall, selectOK)
	}
	_, binaryOp, outerRead, callD, binaryOK := p.Flow().Authored().Operators().Binaries().Get(binary)
	if !binaryOK || binaryOp != kind.BinaryAdd || outerRead == 0 || callD == 0 {
		t.Fatalf("Binary = op %v left %v right %v ok %v", binaryOp, outerRead, callD, binaryOK)
	}
	_, outerLens, _, readOK := p.Flow().Authored().Storage().Reads().Get(outerRead)
	if !readOK || outerLens == 0 {
		t.Fatalf("Binary left Read source = %v/%v", outerLens, readOK)
	}
	_, innerRead, callC, lensOK := p.Flow().Authored().Access().Dynamic().Get(outerLens)
	if !lensOK || innerRead == 0 || callC == 0 {
		t.Fatalf("outer dynamic Lens = base %v key %v ok %v", innerRead, callC, lensOK)
	}
	_, innerLens, _, innerReadOK := p.Flow().Authored().Storage().Reads().Get(innerRead)
	if !innerReadOK || innerLens == 0 {
		t.Fatalf("inner Read source = %v/%v", innerLens, innerReadOK)
	}
	_, callA, callB, innerLensOK := p.Flow().Authored().Access().Dynamic().Get(innerLens)
	if !innerLensOK || callA == 0 || callB == 0 {
		t.Fatalf("inner dynamic Lens = base %v key %v ok %v", callA, callB, innerLensOK)
	}
	for _, call := range []keyspace.Term{callA, callB, callC, callD, outerCall} {
		if _, _, _, _, ok := p.Flow().Authored().Calls().Get(call); !ok {
			t.Fatalf("nested operand %v is not an authored Call", call)
		}
	}
}

func nestedBranchSource(depth int) string {
	if depth == 0 {
		return "leaf()"
	}
	var input strings.Builder
	for index := 0; index < depth; index++ {
		input.WriteString("if condition")
		input.WriteString(strconv.Itoa(index))
		input.WriteString("() then\n")
	}
	input.WriteString("leaf()\n")
	for index := depth - 1; index >= 0; index-- {
		input.WriteString("else\nfallthrough")
		input.WriteString(strconv.Itoa(index))
		input.WriteString("()\nend\n")
	}
	return input.String()
}

func TestSourceNestedBranchesKeepSourceParents(t *testing.T) {
	for _, depth := range []int{0, 1, 2, 37} {
		t.Run("depth_"+strconv.Itoa(depth), func(t *testing.T) {
			p := parseBindLower(t, nestedBranchSource(depth))
			entry, ok := p.Source().Index().Entry()
			if !ok {
				t.Fatal("missing entry Body")
			}
			if got := p.Flow().Authored().Control().Branches().Count(); got != depth {
				t.Fatalf("Branch count = %d, want %d", got, depth)
			}
			body := entry
			for level := 0; level < depth; level++ {
				branch := controlSourceAt(t, p, body, 0)
				owner, condition, whenTrue, whenFalse, branchOK := p.Flow().Authored().Control().Branches().Get(branch)
				if !branchOK || owner != body || condition == 0 || whenTrue == 0 || whenFalse == 0 {
					t.Fatalf("Branch depth %d = owner %v condition %v true %v false %v ok %v", level, owner, condition, whenTrue, whenFalse, branchOK)
				}
				if parent, ok := p.Source().Index().BodyParent(whenTrue); !ok || parent != body {
					t.Fatalf("truthy Body parent at depth %d = %v/%v, want %v", level, parent, ok, body)
				}
				if parent, ok := p.Source().Index().BodyParent(whenFalse); !ok || parent != body {
					t.Fatalf("false Body parent at depth %d = %v/%v, want %v", level, parent, ok, body)
				}
				body = whenTrue
			}
		})
	}
}

func nestedWhileSource(depth int) string {
	if depth == 0 {
		return "step()"
	}
	var input strings.Builder
	for index := 0; index < depth; index++ {
		input.WriteString("while test")
		input.WriteString(strconv.Itoa(index))
		input.WriteString("() do\n")
	}
	input.WriteString("step()\n")
	for index := 0; index < depth; index++ {
		input.WriteString("end\n")
	}
	return input.String()
}

func TestSourceNestedLoopsKeepCausalResetSupport(t *testing.T) {
	for _, depth := range []int{0, 1, 2, 37} {
		t.Run("depth_"+strconv.Itoa(depth), func(t *testing.T) {
			p := parseBindLower(t, nestedWhileSource(depth))
			loops := p.Flow().Authored().Control().Loops()
			if loops.Count() != depth {
				t.Fatalf("Loop count = %d, want %d", loops.Count(), depth)
			}
			edges := p.Flow().Causal().Edges()
			for index := 0; index < loops.Count(); index++ {
				loop, _ := loops.At(index)
				_, body, loopKind, _, ok := loops.Get(loop)
				if !ok || body == 0 || loopKind != kind.LoopWhile {
					t.Fatalf("Loop[%d] = body %v kind %v ok %v", index, body, loopKind, ok)
				}
				if _, ok := p.Flow().Outcomes().BodyExit(body, kind.OutcomeNormal); !ok {
					t.Fatalf("Loop[%d] Body has no normal Outcome", index)
				}
				found := false
				for edgeIndex := 0; edgeIndex < edges.Count(); edgeIndex++ {
					found = found || edges.ResetContains(edgeIndex, loop)
				}
				if !found {
					t.Fatalf("Loop[%d] has no Causal reset support", index)
				}
			}
		})
	}
}

func nestedGotoSource(depth int) string {
	var input strings.Builder
	input.WriteString("::head::\n")
	for index := 0; index < depth; index++ {
		input.WriteString("do\n")
	}
	input.WriteString("goto head\n")
	for index := 0; index < depth; index++ {
		input.WriteString("end\n")
	}
	return input.String()
}

func TestSourceNestedGotosKeepExactTargetAndRecurrenceEdge(t *testing.T) {
	for _, depth := range []int{0, 1, 2, 37} {
		t.Run("depth_"+strconv.Itoa(depth), func(t *testing.T) {
			p := parseBindLower(t, nestedGotoSource(depth))
			label, labelOK := p.Flow().Authored().Control().Labels().At(0)
			jump, jumpOK := p.Flow().Authored().Control().Gotos().At(0)
			if !labelOK || !jumpOK {
				t.Fatalf("Label/Goto = %v/%v %v/%v", label, labelOK, jump, jumpOK)
			}
			if _, target, ok := p.Flow().Authored().Control().Gotos().Get(jump); !ok || target != label {
				t.Fatalf("Goto target = %v/%v, want %v", target, ok, label)
			}
			if exit, ok := p.Flow().Outcomes().GotoExit(jump); !ok || exit == 0 {
				t.Fatalf("Goto exit = %v/%v", exit, ok)
			}
			found := false
			edges := p.Flow().Causal().Edges()
			for index := 0; index < edges.Count(); index++ {
				edge, edgeOK := edges.At(index)
				found = found || edgeOK && edge.Mu == label
			}
			if !found {
				t.Fatal("backward Goto has no Causal recurrence Edge")
			}
		})
	}
}

func TestSourceNestedFunctionsUseFormalsAndCaptureRows(t *testing.T) {
	p := parseBindLower(t, "\nlocal root = 0\nlocal function outer(first, second)\n  local rootDirect = root\n  local function middle(third)\n    local firstDirect = first\n    local function inner(fourth)\n      return third\n    end\n    return inner\n  end\n  return middle\nend\nreturn outer\n")
	functions := p.Flow().Authored().Functions()
	outer, _ := functions.At(0)
	middle, _ := functions.At(1)
	inner, _ := functions.At(2)
	outerOwner, outerBody, _, outerOK := functions.Get(outer)
	middleOwner, middleBody, _, middleOK := functions.Get(middle)
	innerOwner, innerBody, _, innerOK := functions.Get(inner)
	if !outerOK || !middleOK || !innerOK || outerOwner == 0 || middleOwner != outerBody || innerOwner != middleBody {
		t.Fatalf("Function owners outer=%v/%v middle=%v/%v inner=%v/%v", outerOwner, outerOK, middleOwner, middleOK, innerOwner, innerOK)
	}
	for _, row := range []struct {
		function keyspace.Term
		body     keyspace.Term
		count    int
	}{
		{outer, outerBody, 2}, {middle, middleBody, 1}, {inner, innerBody, 1},
	} {
		if count, ok := p.Source().Formals().Len(row.function); !ok || count != row.count {
			t.Fatalf("Formal count = %d/%v, want %d", count, ok, row.count)
		}
		for index := 0; index < row.count; index++ {
			formal, ok := p.Source().Formals().At(row.function, index)
			if !ok {
				t.Fatalf("missing Formal[%d]", index)
			}
			cellKind, host, _, cellOK := p.Flow().Authored().Storage().Cells().Get(formal)
			if !cellOK || cellKind != flow.CellLocal || host != row.body {
				t.Fatalf("Formal Cell = kind %v host %v ok %v, want local/%v", cellKind, host, cellOK, row.body)
			}
		}
	}
	outerFirst, _ := p.Source().Formals().At(outer, 0)
	middleThird, _ := p.Source().Formals().At(middle, 0)
	_, outerCaptured, outerCaptureOK := functions.CaptureAt(outer, 0)
	_, middleCaptured, middleCaptureOK := functions.CaptureAt(middle, 0)
	_, innerCaptured, innerCaptureOK := functions.CaptureAt(inner, 0)
	if !outerCaptureOK || !middleCaptureOK || !innerCaptureOK || outerCaptured == 0 || middleCaptured != outerFirst || innerCaptured != middleThird {
		t.Fatalf("Capture outers = %v/%v %v/%v %v/%v", outerCaptured, outerCaptureOK, middleCaptured, middleCaptureOK, innerCaptured, innerCaptureOK)
	}
}

var _ *program.Program

// bindingSourceCases is concrete authored source evidence owned by this
// lower semantic test. It proves parse -> bind -> lower -> sealed Program
// behavior, not schema-catalog composition.
var bindingSourceCases = []sourceCase{
	{ID: "binding.case.local.declaration", Form: "LocalAssignStmt", Source: "local value = 1", Line: 1},
	{ID: "binding.case.local.function-assignment", Form: "LocalAssignStmt", Source: "local value = function() end", Line: 1},
	{ID: "binding.case.local.function-recursive", Form: "LocalAssignStmt", Source: "local function value()\n  return value()\nend", Line: 1},
	{ID: "binding.case.identifier.cell-selection", Form: "IdentExpr", Source: "local value = 1\nreturn value", Line: 2},
	{ID: "binding.case.identifier.implicit-global", Form: "IdentExpr", Source: "return unresolved", Line: 1},
	{ID: "binding.case.function.literal-entry", Form: "FunctionExpr", Source: "local captured = 1\nreturn function(parameter, ...)\n  return captured, parameter, ...\nend", Line: 2},
	{ID: "binding.case.function.parameters", Form: "ParList", Source: "return function(first, second)\n  return first\nend", Line: 1},
	{ID: "binding.case.function.vararg", Form: "ParList", Source: "return function(...)\n  return ...\nend", Line: 1},
	{ID: "binding.case.function.declaration", Form: "FuncDefStmt", Source: "function declared() end", Line: 1},
	{ID: "binding.case.function.method-origin", Form: "FuncName", Source: "local receiver = {}\nfunction receiver:method(value)\n  return value\nend", Line: 2},
	{ID: "binding.case.numeric-for.cell", Form: "NumberForStmt", Source: "for index = 1, 2 do end", Line: 1},
	{ID: "binding.case.generic-for.cells", Form: "GenericForStmt", Source: "for key, value in pairs({}) do end", Line: 1},
}

// TestSourceBindingCasesRetainExactLexicalMeaning checks the binding vertical
// at its actual semantic boundary.  Each arm parses the atomic authored
// source, obtains its lexical facts from bind.Result, then follows the sealed
// Program relation produced from that same source.  It intentionally has no
// family registry or structural-count assertion.
