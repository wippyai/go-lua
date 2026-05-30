package check_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/canonical"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
)

// TestCanonicalDriver_MultiFunctionModuleSummarizesEachFunction is gate (b): the
// canonical driver runs over a small multi-function module without panic and
// produces a converged interprocedural summary for every module function (the
// root chunk and each nested function). It exercises the module walk, the call
// graph (caller -> callee), and a self-recursive function.
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

// TestCanonicalDriver_BridgePopulatesSessionResults verifies the diagnostic bridge
// (component 11b): after Run, every module function has an api.FuncResult in the
// session keyed by its *ast.FunctionExpr, carrying the bridged sound inputs (the
// CFG), so Checker.runPasses can range over the same result map the legacy flow
// produces. It also confirms the canonical-computed return facts are exposed.
func TestCanonicalDriver_BridgePopulatesSessionResults(t *testing.T) {
	const src = `
local function pick(): number
	return 7
end
return pick()
`
	chunk, err := parse.ParseString(src, "bridge.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	ctx := db.NewQueryContext(db.New())
	sess := check.New(ctx, "bridge.lua")

	driver := canonical.NewDriver(canonical.Config{})
	driver.Run(sess, chunk)

	refs := driver.FuncRefs()
	if len(refs) != 2 {
		t.Fatalf("expected module body + pick; got %d functions", len(refs))
	}

	// Every analyzed function is bridged into the session results under its own
	// function node, with the CFG populated (the sound input the passes read).
	for _, ref := range refs {
		fn, ok := driver.FuncExprFor(ref)
		if !ok {
			t.Fatalf("ref %v has no function node", ref)
		}
		result, ok := sess.Results[fn]
		if !ok || result == nil {
			t.Fatalf("ref %v not bridged into session results", ref)
		}
		if result.Graph == nil {
			t.Fatalf("ref %v bridged result has no graph", ref)
		}
	}

	// The root chunk's result is also reachable as the session root result.
	if sess.RootResult == nil {
		t.Fatal("bridge did not set the session root result")
	}

	// pick returns a single number; the canonical-computed return fact is exposed
	// for the transfer-fidelity worklist even though the bridge does not yet route
	// it into a legacy diagnostic field.
	var pickRef summary.FuncRef
	found := false
	for _, ref := range refs {
		fn, _ := driver.FuncExprFor(ref)
		if fn != nil && fn.ParList != nil && len(fn.ParList.Names) == 0 && !fn.ParList.HasVargs {
			// pick has no params and is not the vararg module body.
			pickRef = ref
			found = true
		}
	}
	if !found {
		t.Fatal("could not locate pick among module functions")
	}
	rets := driver.ReturnTypes(pickRef)
	if len(rets) != 1 {
		t.Fatalf("pick canonical return arity = %d, want 1", len(rets))
	}
	if rets[0] == nil {
		t.Fatal("pick canonical return slot 0 is nil")
	}
}
