package flow

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	flowtypes "github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIpairs_ElementType(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "ipairs preserves array element type",
			Code: `
				local chunks: {string} = {}
				table.insert(chunks, "hello")
				for _, chunk in ipairs(chunks) do
					local s: string = chunk
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "ipairs wrong element type fails",
			Code: `
				local chunks: {string} = {"a", "b"}
				for _, chunk in ipairs(chunks) do
					local n: number = chunk
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "ipairs index is integer",
			Code: `
				local arr: {string} = {"a", "b", "c"}
				for i, v in ipairs(arr) do
					local idx: integer = i
					local s: string = v
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestIteratorFlowSolutionProjectsLoopVariablesAtBodyPoint(t *testing.T) {
	result := testutil.Check(`
		local arr: {string} = {"a", "b"}
		for i, v in ipairs(arr) do
			local use = v
		end
	`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	root := result.Session.RootResult
	if root == nil || root.FlowInputs == nil || root.FlowSolution == nil {
		t.Fatal("missing root flow result")
	}

	var indexAssign, valueAssign, useAssign *flowtypes.UnifiedAssignment
	for i := range root.FlowInputs.Assignments {
		assign := &root.FlowInputs.Assignments[i]
		switch assign.TargetPath.Root {
		case "i":
			indexAssign = assign
		case "v":
			valueAssign = assign
		case "use":
			useAssign = assign
		}
	}
	if indexAssign == nil || valueAssign == nil || useAssign == nil {
		t.Fatalf("missing iterator/body assignments: index=%v value=%v use=%v", indexAssign, valueAssign, useAssign)
	}

	assertSolved := func(pointPath constraint.Path, p cfg.Point, want typ.Type) {
		t.Helper()
		got := root.FlowSolution.NarrowedTypeAt(p, pointPath)
		if !typ.TypeEquals(got, want) {
			t.Fatalf("NarrowedTypeAt(%s at %d) = %v, want %v", pointPath.Root, p, got, want)
		}
	}

	assertSolved(indexAssign.TargetPath, indexAssign.Point, typ.Integer)
	assertSolved(valueAssign.TargetPath, valueAssign.Point, typ.String)
	assertSolved(useAssign.Source.Path, useAssign.Point, typ.String)

	var useSourceFound bool
	observer := observation.FromFuncResult(root, nil).WithProofValues()
	for _, assign := range root.Evidence.Assignments {
		if assign.Info == nil || assign.Point != useAssign.Point {
			continue
		}
		assign.Info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if target.Symbol != useAssign.TargetPath.Symbol || source == nil {
				return
			}
			useSourceFound = true
			got := observer.AssignmentSourceType(source, assign.Point, typ.String, target.Symbol)
			if !typ.TypeEquals(got, typ.String) {
				t.Fatalf("observed assignment source = %v, want string", got)
			}
		})
	}
	if !useSourceFound {
		t.Fatal("missing use assignment source evidence")
	}
}

func TestPairs_MapType(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "pairs preserves map types",
			Code: `
				local m: {[string]: number} = {a = 1, b = 2}
				for k, v in pairs(m) do
					local key: string = k
					local val: number = v
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "pairs wrong key type fails",
			Code: `
				local m: {[string]: number} = {a = 1}
				for k, v in pairs(m) do
					local key: number = k
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "pairs wrong value type fails",
			Code: `
				local m: {[string]: number} = {a = 1}
				for k, v in pairs(m) do
					local val: string = v
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestTableInsert_TracksType(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "table.insert tracks element type",
			Code: `
				local chunks = {}
				local s: string = "hello"
				table.insert(chunks, s)
				for _, chunk in ipairs(chunks) do
					local x: string = chunk
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "table.insert in loop propagates type",
			Code: `
				local chunks = {}
				while true do
					local chunk: string = "data"
					if chunk == nil then
						break
					end
					table.insert(chunks, chunk)
				end
				for _, c in ipairs(chunks) do
					local s: string = c
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestFP_IpairsPreservesElementFields(t *testing.T) {
	source := `
local items: {id: string, name: string, count: number}[] = {}
table.insert(items, {id = "a", name = "alpha", count = 1})

for _, item in ipairs(items) do
    local id: string = item.id
    local name: string = item.name
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("ipairs loop var should preserve element fields; got: %v",
			testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIterator_CustomIterator(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "custom iterator function",
			Code: `
				local function iter(arr: {number}): (() -> (integer?, number?))
					local i = 0
					return function(): (integer?, number?)
						i = i + 1
						return i, arr[i]
					end
				end

				local arr: {number} = {1, 2, 3}
				for i, v in iter(arr) do
					local idx: integer = i
					local val: number = v
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
