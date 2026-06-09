package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
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
	fact, ok := sess.store.CanonicalFunctionFactProjection(root, parent, sym)
	if !ok {
		facts := sess.store.CanonicalFunctionFactsProjectionForExport(root, parent)
		t.Fatalf("missing get_db fact; refs=%v summaries=%v facts=%d", driver.artifact.Refs, driver.artifact.Summaries, len(facts))
	}
	returns := product.ProjectVector(fact.Returns.Preflow)
	if len(returns) != 2 {
		t.Fatalf("return summary arity = %d (%v), want 2", len(returns), returns)
	}
}

func TestDriverFunctionFactsAreFinalProjectionFromCanonicalSummary(t *testing.T) {
	chunk, err := parse.ParseString(`
type Payload = { id: string }

local function consume(payload: Payload): number
	return 1
end

local function ensure_payload(payload)
	consume(payload)
	assert(payload)
	return 1
end

local value = ensure_payload({ id = "db" })
`, "function-facts-projection.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	driver := NewDriver(Config{Stdlib: scope.NewWithBuiltins()})
	sess := newCanonicalTestSession("function-facts-projection.lua")
	driver.Run(sess, chunk)
	if sess.rootValue == nil || sess.rootValue.Graph == nil {
		t.Fatal("missing root result")
	}
	root := sess.rootValue.Graph
	sym, ok := root.SymbolAt(root.Exit(), "ensure_payload")
	if !ok || sym == 0 {
		t.Fatal("missing ensure_payload symbol")
	}
	parentHash := sess.store.GraphParentHashOf(root.ID())
	parent := sess.store.Parents()[parentHash]
	fact, ok := sess.store.CanonicalFunctionFactProjection(root, parent, sym)
	if !ok {
		facts := sess.store.CanonicalFunctionFactsProjectionForExport(root, parent)
		t.Fatalf("missing ensure_payload fact; facts=%#v", facts)
	}
	returns := product.ProjectVector(fact.Returns.Preflow)
	if len(returns) != 1 || !subtype.IsSubtype(returns[0], typ.Number) {
		t.Fatalf("return summary = %v, want numeric", returns)
	}
	consumeSym, ok := root.SymbolAt(root.Exit(), "consume")
	if !ok || consumeSym == 0 {
		t.Fatal("missing consume symbol")
	}
	consumeFact, ok := sess.store.CanonicalFunctionFactProjection(root, parent, consumeSym)
	if !ok {
		facts := sess.store.CanonicalFunctionFactsProjectionForExport(root, parent)
		t.Fatalf("missing consume fact; facts=%#v", facts)
	}
	if consumeFact.Public.Signature == nil || len(consumeFact.Public.Signature.Params) != 1 || len(consumeFact.Public.Signature.Returns) != 1 {
		t.Fatalf("consume signature = %#v, want declared parameter and return", consumeFact.Public.Signature)
	}
	if !typ.TypeEquals(consumeFact.Public.Signature.Returns[0], typ.Number) {
		t.Fatalf("consume signature returns = %#v, want declared number", consumeFact.Public.Signature.Returns)
	}
	publicParams := product.ProjectVector(fact.Call.Params)
	if len(publicParams) != 1 {
		t.Fatalf("public params = %v, want one payload contract", publicParams)
	}
	payload := typ.NewRecord().ReadonlyField("id", typ.String).Build()
	if !subtype.IsSubtype(publicParams[0], payload) {
		t.Fatalf("public param = %v, want payload contract %v", publicParams[0], payload)
	}
	if fact.Effects.Refinement == nil {
		t.Fatal("missing postcondition refinement from assert(payload)")
	}
}
