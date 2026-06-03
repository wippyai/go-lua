package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

func TestDriverPublishesFunctionFactsFromSolvedSummaries(t *testing.T) {
	chunk, err := parse.ParseString(`
local function get_db()
	return 1, "ok"
end

local value, label = get_db()
`, "function-facts.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	driver := NewDriver(Config{Stdlib: scope.NewWithBuiltins()})
	sess := newCanonicalTestSession("function-facts.lua")
	driver.Run(sess, chunk)
	if sess.rootValue == nil || sess.rootValue.Graph == nil {
		t.Fatal("missing root result")
	}
	root := sess.rootValue.Graph
	sym, ok := root.SymbolAt(root.Exit(), "get_db")
	if !ok || sym == 0 {
		t.Fatal("missing get_db symbol")
	}
	parentHash := sess.store.GraphParentHashOf(root.ID())
	parent := sess.store.Parents()[parentHash]
	facts := sess.store.InterprocFacts(root, parent).FunctionFacts()
	fact, ok := facts[sym]
	if !ok {
		t.Fatalf("missing get_db fact; refs=%v summaries=%v facts=%d", driver.refs, driver.summaries, len(facts))
	}
	returns := product.ProjectVector(fact.Summary)
	if len(returns) != 2 {
		t.Fatalf("return summary arity = %d (%v), want 2", len(returns), returns)
	}
}
