package canonical_test

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"testing"
)

func TestCanonicalPublishesSolvedLocalFunctionFactsToParentProduct(t *testing.T) {
	res := testutil.Check(`
local function get_db()
	return 1, "ok"
end

local value, label = get_db()
`, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean canonical check, got diagnostics: %v", msgs)
	}
	if res == nil || res.Session == nil || res.Session.Store == nil || res.Session.RootResult == nil || res.Session.RootResult.Graph == nil {
		t.Fatal("missing canonical session root")
	}

	root := res.Session.RootResult.Graph
	sym, ok := root.SymbolAt(root.Exit(), "get_db")
	if !ok || sym == 0 {
		t.Fatal("missing get_db symbol at root exit")
	}
	parentHash := res.Session.Store.GraphParentHashOf(root.ID())
	parent := res.Session.Store.Parents()[parentHash]
	facts := res.Session.Store.FunctionFactsProjection(root, parent)
	fact, ok := facts[sym]
	if !ok {
		key, keyOK := res.Session.Store.ParentGraphKeyForSymbol(sym)
		t.Fatalf(
			"missing get_db function fact; root facts=%d sym=%d parentHash=%d parentKey=%+v/%v nested=%v",
			len(facts),
			sym,
			parentHash,
			key,
			keyOK,
			root.NestedFunctions(),
		)
	}
	returns := product.ProjectVector(fact.Returns.Preflow)
	if len(returns) != 2 {
		t.Fatalf("return summary arity = %d (%v), want 2", len(returns), returns)
	}
	if !subtype.IsSubtype(returns[0], typ.Number) {
		t.Fatalf("return slot 0 = %v, want numeric", returns[0])
	}
	if !subtype.IsSubtype(returns[1], typ.String) {
		t.Fatalf("return slot 1 = %v, want string", returns[1])
	}
}

func TestCanonicalPublishesMutualRecursionReturnSummaries(t *testing.T) {
	res := testutil.Check(`
local function f(n)
	if n <= 0 then
		return 1
	end
	return g(n - 1)
end

local function g(n)
	if n <= 0 then
		return 2
	end
	return f(n - 1)
end

local result = f(10)
`, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean canonical check, got diagnostics: %v", msgs)
	}
	for _, name := range []string{"f", "g"} {
		fact := publishedRootFunctionFact(t, res, name)
		returns := product.ProjectVector(fact.Returns.Preflow)
		if len(returns) != 1 {
			t.Fatalf("%s summary arity = %d (%v), want 1", name, len(returns), returns)
		}
		if !subtype.IsSubtype(returns[0], typ.Number) {
			t.Fatalf("%s summary slot 0 = %v, want numeric", name, returns[0])
		}
	}
}

func publishedRootFunctionFact(t *testing.T, res *testutil.Result, name string) api.FunctionFact {
	t.Helper()
	if res == nil || res.Session == nil || res.Session.Store == nil || res.Session.RootResult == nil || res.Session.RootResult.Graph == nil {
		t.Fatal("missing canonical session root")
	}
	root := res.Session.RootResult.Graph
	sym, ok := root.SymbolAt(root.Exit(), name)
	if !ok || sym == 0 {
		t.Fatalf("missing %s symbol at root exit", name)
	}
	parentHash := res.Session.Store.GraphParentHashOf(root.ID())
	parent := res.Session.Store.Parents()[parentHash]
	facts := res.Session.Store.FunctionFactsProjection(root, parent)
	fact, ok := facts[sym]
	if !ok {
		t.Fatalf("missing %s function fact; root facts=%d", name, len(facts))
	}
	return fact
}
