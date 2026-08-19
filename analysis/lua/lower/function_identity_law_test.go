package lower_test

import (
	"testing"

	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
)

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
		if leftOK != rightOK || (leftDirect == 0) != (rightDirect == 0) {
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
	if !leftStateOK || !rightStateOK || leftState != rightState || leftState != staticrefs.Unresolved || leftTargetTerm != 0 || rightTargetTerm != 0 || leftRoot == 0 || rightRoot == 0 {
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
