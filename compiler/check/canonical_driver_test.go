package check_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/canonical"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// TestCanonicalDriver_MultiFunctionModuleSummarizesEachFunction verifies that the
// module driver runs over a small multi-function module without panic and produces
// a converged interprocedural summary for every module function (the root chunk
// and each nested function). It exercises the module walk, the call graph
// (caller -> callee), and a self-recursive function.
func TestCanonicalDriver_MultiFunctionModuleSummarizesEachFunction(t *testing.T) {
	const src = `
local function add(a, b)
	return a + b
end

local function sum_to(n)
	if n <= 0 then
		return 0
	end
	return add(n, sum_to(n - 1))
end

local function describe(n)
	return "total=" .. n
end

return {
	sum = sum_to(10),
	label = describe(42),
}
`
	chunk, err := parse.ParseString(src, "multifn.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	ctx := db.NewQueryContext(db.New())
	sess := check.New(ctx, "multifn.lua")

	driver := canonical.NewDriver(canonical.Config{})
	driver.Run(sess, chunk)

	refs := driver.FuncRefs()
	// Four functions: the module body plus add, sum_to, describe.
	if len(refs) != 4 {
		t.Fatalf("expected 4 module functions (module body + 3 locals); got %d", len(refs))
	}

	for _, ref := range refs {
		if _, ok := driver.SummaryFor(ref); !ok {
			t.Fatalf("function %v has no converged summary", ref)
		}
	}
}

// TestCanonicalDriver_SelfRecursiveModuleTerminates confirms that a module whose
// function calls itself drives the call-graph summary fixed point to convergence
// (bottom seed + db cycle), terminating without a recursion cap. The test process
// -timeout is the only backstop; reaching the assertions proves termination.
func TestCanonicalDriver_SelfRecursiveModuleTerminates(t *testing.T) {
	const src = `
local function walk(node)
	if type(node) ~= "table" then
		return node
	end
	local out = {}
	for i, child in ipairs(node) do
		out[i] = walk(child)
	end
	return out
end

return walk
`
	chunk, err := parse.ParseString(src, "recursive.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	ctx := db.NewQueryContext(db.New())
	sess := check.New(ctx, "recursive.lua")

	driver := canonical.NewDriver(canonical.Config{})
	driver.Run(sess, chunk)

	if len(driver.FuncRefs()) != 2 {
		t.Fatalf("expected the module body + walk; got %d functions", len(driver.FuncRefs()))
	}
}

func TestCanonicalDriver_ParamNarrowsInheritThroughNestedWrapperSummary(t *testing.T) {
	const src = `
local function innerAssert(val: any, msg: string)
	assert(val, msg)
end
local function outerAssert(val: any)
	innerAssert(val, "outer: value is nil")
end
function process(x: string?)
	outerAssert(x)
	local s: string = x
end
`
	chunk, err := parse.ParseString(src, "nested_wrapper.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	ctx := db.NewQueryContext(db.New())
	sess := check.New(ctx, "nested_wrapper.lua")

	driver := canonical.NewDriver(canonical.Config{})
	driver.Run(sess, chunk)

	var outer summary.FuncRef
	var found bool
	for _, ref := range driver.FuncRefs() {
		fn, ok := sessionFuncByRef(sess, ref)
		if !ok || fn == nil || fn.Line() != 5 {
			continue
		}
		outer = ref
		found = true
		break
	}
	if !found {
		t.Fatal("outerAssert function ref not found")
	}

	sum, ok := driver.SummaryFor(outer)
	if !ok {
		t.Fatal("outerAssert has no summary")
	}
	want := constraint.Truthy{Path: constraint.ParamPath(0)}
	for _, c := range sum.Postconditions.Condition().MustConstraints() {
		if c.Equals(want) {
			return
		}
	}
	t.Fatalf("outerAssert Postconditions = %v, want truthy narrow on parameter 0", sum.Postconditions.Condition())
}

// TestCanonicalDriver_ProjectsSessionResults verifies the diagnostic result projection:
// after Run, every module function has an api.FuncResult in the session keyed by
// its *ast.FunctionExpr, carrying the projected sound inputs (the CFG), so
// Checker.runPasses can range over the same result map. It also confirms the
// computed return facts are exposed.
func TestCanonicalDriver_ProjectsSessionResults(t *testing.T) {
	const src = `
local function pick(): number
	return 7
end
return pick()
`
	chunk, err := parse.ParseString(src, "projection.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	ctx := db.NewQueryContext(db.New())
	sess := check.New(ctx, "projection.lua")

	driver := canonical.NewDriver(canonical.Config{})
	driver.Run(sess, chunk)

	refs := driver.FuncRefs()
	if len(refs) != 2 {
		t.Fatalf("expected module body + pick; got %d functions", len(refs))
	}

	// Every analyzed function is projected into the session results under its own
	// function node, with the CFG populated (the sound input the passes read).
	for _, ref := range refs {
		fn, ok := sessionFuncByRef(sess, ref)
		if !ok {
			t.Fatalf("ref %v has no function node", ref)
		}
		result, ok := sess.Results[fn]
		if !ok || result == nil {
			t.Fatalf("ref %v not projected into session results", ref)
		}
		if result.Graph == nil {
			t.Fatalf("ref %v projected result has no graph", ref)
		}
	}

	// The root chunk's result is also reachable as the session root result.
	if sess.RootResult == nil {
		t.Fatal("session projection did not set the root result")
	}

	// pick returns a single number; the computed return fact is exposed for the
	// transfer-fidelity worklist.
	pickRef, found := findRefByFunc(sess, func(fn *ast.FunctionExpr) bool {
		return fn != nil && fn.ParList != nil && len(fn.ParList.Names) == 0 && !fn.ParList.HasVargs
	})
	if !found {
		t.Fatal("could not locate pick among module functions")
	}
	sum, ok := driver.SummaryFor(pickRef)
	if !ok {
		t.Fatal("pick summary missing")
	}
	rets := summary.ReturnTypes(sum)
	if len(rets) != 1 {
		t.Fatalf("pick canonical return arity = %d, want 1", len(rets))
	}
	if rets[0] == nil {
		t.Fatal("pick canonical return slot 0 is nil")
	}
}

func TestCanonicalDriver_ProjectsDirectCallEntryValue(t *testing.T) {
	const src = `
local captured = nil

local function setter(opts)
	captured = opts
end

setter({ retry = { max_attempts = 3, initial_delay = 100 } })

if captured == nil then error("nil") end
if captured.retry == nil then error("nil retry") end

local attempts: number = captured.retry.max_attempts
local delay: number = captured.retry.initial_delay
return attempts, delay
`
	chunk, err := parse.ParseString(src, "captured_entry.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	ctx := db.NewQueryContext(db.New())
	sess := check.New(ctx, "captured_entry.lua")

	driver := canonical.NewDriver(canonical.Config{})
	driver.Run(sess, chunk)

	var root, setter summary.FuncRef
	var foundRoot, foundSetter bool
	for _, ref := range driver.FuncRefs() {
		fn, ok := sessionFuncByRef(sess, ref)
		if !ok || fn == nil || fn.ParList == nil {
			continue
		}
		switch {
		case fn.ParList.HasVargs:
			root, foundRoot = ref, true
		case len(fn.ParList.Names) == 1 && fn.ParList.Names[0] == "opts":
			setter, foundSetter = ref, true
		}
	}
	if !foundRoot || !foundSetter {
		t.Fatalf("root/setter refs not found: root=%v setter=%v refs=%v", foundRoot, foundSetter, driver.FuncRefs())
	}

	rootSum, ok := driver.SummaryFor(root)
	if !ok {
		t.Fatal("root summary missing")
	}
	entry, ok := rootSum.CallEntryPublication[setter].Values[0]
	if !ok {
		t.Fatalf("root CallEntryPublication missing setter slot 0: %#v", rootSum.CallEntryPublication)
	}
	assertNestedNumberField(t, "root call-entry opts", entry, "retry", "max_attempts")
	assertNestedNumberField(t, "root call-entry opts", entry, "retry", "initial_delay")
}

func TestCanonicalDriver_ProjectsMethodCallEntryValue(t *testing.T) {
	const src = `
local captured = nil

local function make()
	return {
		with_options = function(self, opts)
			captured = opts
			return self
		end,
	}
end

local chain = make()
chain:with_options({ retry = { max_attempts = 3, initial_delay = 100 } })

return captured
`
	chunk, err := parse.ParseString(src, "captured_method_entry.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	ctx := db.NewQueryContext(db.New())
	sess := check.New(ctx, "captured_method_entry.lua")

	driver := canonical.NewDriver(canonical.Config{})
	driver.Run(sess, chunk)

	var root, makeRef, withOptions summary.FuncRef
	var foundRoot, foundMake, foundWithOptions bool
	for _, ref := range driver.FuncRefs() {
		fn, ok := sessionFuncByRef(sess, ref)
		if !ok || fn == nil || fn.ParList == nil {
			continue
		}
		switch {
		case fn.ParList.HasVargs:
			root, foundRoot = ref, true
		case len(fn.ParList.Names) == 0:
			makeRef, foundMake = ref, true
		case len(fn.ParList.Names) == 2 && fn.ParList.Names[0] == "self" && fn.ParList.Names[1] == "opts":
			withOptions, foundWithOptions = ref, true
		}
	}
	if !foundRoot || !foundMake || !foundWithOptions {
		t.Fatalf("root/make/with_options refs not found: root=%v make=%v withOptions=%v refs=%v", foundRoot, foundMake, foundWithOptions, driver.FuncRefs())
	}

	makeSum, ok := driver.SummaryFor(makeRef)
	if !ok {
		t.Fatal("make summary missing")
	}
	returnMethodPath := constraint.NewPlaceholder(0).Field("with_options")
	if !returnFunctionRefTreeHasPath(makeSum.ReturnRefs, 0, returnMethodPath) {
		t.Fatalf("make return slot missing with_options function ref at %s: %#v", returnMethodPath.Key(), makeSum.ReturnRefs)
	}
	withOptionsSum, ok := driver.SummaryFor(withOptions)
	if !ok {
		t.Fatal("with_options summary missing")
	}
	if flow.CaptureEffectsDomain.Equal(withOptionsSum.CellEffects, flow.CaptureEffectsDomain.Bottom()) {
		t.Fatalf("with_options cell effects missing: %#v", withOptionsSum)
	}

	rootSum, ok := driver.SummaryFor(root)
	if !ok {
		t.Fatal("root summary missing")
	}
	entry, ok := rootSum.CallEntryPublication[withOptions].Values[1]
	if !ok {
		t.Fatalf("root CallEntryPublication missing with_options opts slot 1: entries=%#v root function refs=%#v make returns=%v make return refs=%#v", rootSum.CallEntryPublication, rootSum.CaptureReferences.FunctionRefs(), makeSum.Returns, makeSum.ReturnRefs)
	}
	assertNestedNumberField(t, "root method call-entry opts", entry, "retry", "max_attempts")
	assertNestedNumberField(t, "root method call-entry opts", entry, "retry", "initial_delay")
}

func TestCanonicalDriver_ProjectsIndirectMethodEffectThroughOpen(t *testing.T) {
	const src = `
local captured = nil
local function get_contract()
	return {
		with_options = function(self, opts)
			captured = opts
			return self
		end,
	}
end
local function open(overrides)
	local c = get_contract()
	c = c:with_options({ retry = overrides.retry })
	return c
end
open({ retry = { max_attempts = 3, initial_delay = 100 } })
return captured
`
	chunk, err := parse.ParseString(src, "captured_indirect_entry.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	ctx := db.NewQueryContext(db.New())
	sess := check.New(ctx, "captured_indirect_entry.lua")

	driver := canonical.NewDriver(canonical.Config{})
	driver.Run(sess, chunk)

	var root, openRef, withOptions summary.FuncRef
	var foundRoot, foundOpen, foundWithOptions bool
	for _, ref := range driver.FuncRefs() {
		fn, ok := sessionFuncByRef(sess, ref)
		if !ok || fn == nil || fn.ParList == nil {
			continue
		}
		switch {
		case fn.ParList.HasVargs:
			root, foundRoot = ref, true
		case len(fn.ParList.Names) == 1 && fn.ParList.Names[0] == "overrides":
			openRef, foundOpen = ref, true
		case len(fn.ParList.Names) == 2 && fn.ParList.Names[0] == "self" && fn.ParList.Names[1] == "opts":
			withOptions, foundWithOptions = ref, true
		}
	}
	if !foundRoot || !foundOpen || !foundWithOptions {
		t.Fatalf("root/open/with_options refs not found: root=%v open=%v withOptions=%v refs=%v", foundRoot, foundOpen, foundWithOptions, driver.FuncRefs())
	}

	rootSum, ok := driver.SummaryFor(root)
	if !ok {
		t.Fatal("root summary missing")
	}
	openEntry, ok := rootSum.CallEntryPublication[openRef].Values[0]
	if !ok {
		t.Fatalf("root CallEntryPublication missing open overrides slot 0: %#v", rootSum.CallEntryPublication)
	}
	assertNestedNumberField(t, "root open call-entry overrides", openEntry, "retry", "max_attempts")

	_ = withOptions
	if len(rootSum.Returns) == 0 {
		t.Fatalf("root return summary missing after open call: %#v", rootSum)
	}
	assertNestedNumberField(t, "root return captured", rootSum.Returns[0], "retry", "max_attempts")
	assertNestedNumberField(t, "root return captured", rootSum.Returns[0], "retry", "initial_delay")
}

func TestCanonicalDriver_FactoryReturnPublishesNestedMethodRefs(t *testing.T) {
	const src = `
type Row = {[string]: any}
type DB = {
	query: fun(self: DB, sql: string): ({Row}?, string?),
}

local M = {}

function M.mock(): DB
	local database: DB = {
		query = function(self: DB, sql: string): ({Row}?, string?)
			return {{ count = 1 }}, nil
		end,
	}
	return database
end

local function table_exists(database: DB): boolean
	local result, query_err = database:query("SELECT 1")
	if query_err then
		return false
	end
	if result and result[1] then
		return result[1].count and result[1].count > 0
	end
	return false
end

return table_exists(M.mock())
`
	chunk, err := parse.ParseString(src, "factory_nested_method_refs.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	ctx := db.NewQueryContext(db.New())
	sess := check.New(ctx, "factory_nested_method_refs.lua")
	driver := canonical.NewDriver(canonical.Config{})
	driver.Run(sess, chunk)

	var mockRef summary.FuncRef
	var foundMock bool
	for _, ref := range driver.FuncRefs() {
		fn, ok := sessionFuncByRef(sess, ref)
		if !ok || fn == nil || fn.ParList == nil {
			continue
		}
		if len(fn.ParList.Names) == 0 && len(fn.ReturnTypes) > 0 {
			mockRef, foundMock = ref, true
			break
		}
	}
	if !foundMock {
		t.Fatalf("mock ref not found: refs=%v", driver.FuncRefs())
	}
	mockSum, ok := driver.SummaryFor(mockRef)
	if !ok {
		t.Fatal("mock summary missing")
	}
	queryPath := constraint.NewPlaceholder(0).Field("query")
	if !returnFunctionRefTreeHasPath(mockSum.ReturnRefs, 0, queryPath) {
		t.Fatalf("mock return slot missing query FunctionRef at %s: %#v", queryPath.Key(), mockSum.ReturnRefs)
	}
}

func returnFunctionRefTreeHasPath(refs flow.ReturnRefs, slot int, path constraint.Path) bool {
	tree, ok := refs.FunctionRefTree(slot)
	if !ok {
		return false
	}
	placeholder := constraint.NewPlaceholder(slot)
	if path.Equal(placeholder) {
		return tree.HasRoot && !tree.Root.IsBottom()
	}
	if len(path.Segments) == 0 {
		return false
	}
	for _, entry := range tree.Entries {
		if reflect.DeepEqual(entry.Segments, path.Segments) && !entry.Set.IsBottom() {
			return true
		}
	}
	return false
}

func assertNestedNumberField(t *testing.T, label string, av product.AbstractValue, first, second string) {
	t.Helper()
	if !nestedNumberField(av, first, second) {
		t.Fatalf("%s.%s.%s = %v, want numeric field", label, first, second, av.ProjectValue())
	}
}

func nestedNumberField(av product.AbstractValue, first, second string) bool {
	child, ok := product.FieldOf(av, first)
	if !ok || child.IsZero() {
		return false
	}
	grandchild, ok := product.FieldOf(child, second)
	if !ok || grandchild.IsZero() {
		return false
	}
	return typeIsNumeric(grandchild.ProjectValue())
}

func typeIsNumeric(t typ.Type) bool {
	if t == nil {
		return false
	}
	if typ.TypeEquals(t, typ.Number) || typ.TypeEquals(t, typ.Integer) {
		return true
	}
	lit, ok := t.(*typ.Literal)
	return ok && (lit.Base == kind.Number || lit.Base == kind.Integer)
}

func sessionFuncByRef(sess *check.Session, ref summary.FuncRef) (*ast.FunctionExpr, bool) {
	if sess == nil {
		return nil, false
	}
	for fn, result := range sess.Results {
		if result == nil || result.Graph == nil {
			continue
		}
		if result.Graph.ID() == ref.GraphID {
			return fn, true
		}
	}
	return nil, false
}

func findRefByFunc(sess *check.Session, match func(*ast.FunctionExpr) bool) (summary.FuncRef, bool) {
	if sess == nil || match == nil {
		return summary.FuncRef{}, false
	}
	for fn, result := range sess.Results {
		if fn == nil || result == nil || result.Graph == nil || !match(fn) {
			continue
		}
		return summary.FuncRef{GraphID: result.Graph.ID()}, true
	}
	return summary.FuncRef{}, false
}
