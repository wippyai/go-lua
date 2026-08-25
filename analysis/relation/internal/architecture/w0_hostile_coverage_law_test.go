package architecture_test

import (
	"testing"
)

// These are semantic shapes, not a package or implementation inventory. The
// table is the W0 hand-off between the architecture and later compiler,
// checker, binding, oracle, and physical lanes. A row names the smallest
// hostile witness that must eventually be executable; it does not require a
// production API to exist during W0.
type w0HostileCoverageShape struct {
	name          string
	category      string
	semanticShape string
	risk          string
	capabilities  []string
}

const (
	w0ReceiverRouteExpansion = "ReceiverRouteExpansion"
	w0RawAccessExpansion     = "RawAccessExpansion"
	w0PayloadExpansion       = "PayloadExpansion"
)

var w0HostileCoverageInventory = []w0HostileCoverageShape{
	{name: "ordinary-exact-family", category: "declaration", semanticShape: "exact candidate read and keyed publication", risk: "ordinary family must not need a form switch"},
	{name: "seed-family", category: "declaration", semanticShape: "seed relation entering solve-local state", risk: "seed is a relation input, not an execution shortcut"},
	{name: "selected-read", category: "read", semanticShape: "selected read with scope and denominator", risk: "selection must remain explicit and range-restricted"},
	{name: "correspondence-read", category: "read", semanticShape: "correspondence/key-vector read", risk: "correspondence identity must not become a physical ordinal"},
	{name: "summary-vector-absent-cells", category: "read", semanticShape: "summary vector containing absent cells", risk: "absence is not a lattice default"},
	{name: "complete-vector-absent-cells", category: "read", semanticShape: "Complete vector with an authenticated denominator and absent cells", risk: "Complete must materialize explicit presence"},
	{name: "routed-publication", category: "publication", semanticShape: "routed output publication", risk: "destination and route authority stay sealed"},
	{name: "transformed-publication", category: "publication", semanticShape: "transformed/correspondence publication", risk: "transformation is typed semantic work, not a physical operator"},
	{name: "activation-publication", category: "publication", semanticShape: "activation branch publication", risk: "activation uses sealed identities and dependencies"},
	{name: "transport-publication", category: "publication", semanticShape: "transport/structural handoff", risk: "transport cannot mint topology"},
	{name: "structural-publication", category: "publication", semanticShape: "structural publication without a domain form", risk: "structural rows still pass the single publication door"},
	{name: "placement-capture-family", category: "placement", semanticShape: "capture route and retained value", risk: "placement semantics stay in typed bindings"},
	{name: "placement-containment-family", category: "placement", semanticShape: "containment route and owner relation", risk: "containment is not inferred from storage layout"},
	{name: "placement-formal-family", category: "placement", semanticShape: "formal/unknown route demand", risk: "formal openness widens only through its certified capability"},
	{name: "placement-publication-escape-family", category: "placement", semanticShape: "publication escape route", risk: "escape remains a declared route, not an engine callback"},
	{name: "placement-suspension-family", category: "placement", semanticShape: "suspension/causal route", risk: "suspension dependencies remain monotone and scoped"},
	{name: "typed-query-fold", category: "query", semanticShape: "grouped typed query fold", risk: "query folds share relation plans and semantic ABI"},
	{name: "selected-query-site-closure", category: "query", semanticShape: "selected query-site closure", risk: "query-site population and filtering must close explicitly"},
	{name: "diagnostic-observation", category: "diagnostic", semanticShape: "diagnostic observation and finding relation", risk: "diagnostics consume the converged schema, not a side channel"},
	{name: "decision-scope", category: "diagnostic", semanticShape: "decision scope and scope entailment", risk: "scope filtering precedes Apply and publication"},
	{name: "raw-get-dependent-six-read-chain", category: "raw-get", semanticShape: "RawGet dependent receiver, key, call, heap, Pack, and Value reads", risk: "the six-read chain must not be flattened into a form"},
	{name: "raw-get-receiver-route-expansion", category: "raw-get", semanticShape: "RawGet typed receiver-route expansion", risk: "route expansion capability is explicit and bounded", capabilities: []string{w0ReceiverRouteExpansion}},
	{name: "raw-get-raw-access-expansion", category: "raw-get", semanticShape: "RawGet typed raw-access expansion", risk: "raw-access expansion cannot read undeclared relation state", capabilities: []string{w0RawAccessExpansion}},
	{name: "raw-get-payload-expansion", category: "raw-get", semanticShape: "RawGet typed heap/Pack/Value payload expansion", risk: "payload expansion has an authenticated denominator", capabilities: []string{w0PayloadExpansion}},
	{name: "raw-set-dependent-chain", category: "raw-set", semanticShape: "RawSet dependent receiver, key, heap, Pack, and Value update chain", risk: "RawSet proposals remain relationally range-restricted"},
	{name: "raw-set-atomic-heap-update", category: "raw-set", semanticShape: "RawSet atomic ascending heap update proposal", risk: "one invocation commits through the sole publication door"},
	{name: "absence-complete", category: "absence", semanticShape: "absence-shaped rule lowered through Complete", risk: "NoCandidate, NoSelection, and ProvenAbsent stay distinct"},
	{name: "selected-recursion", category: "recursion", semanticShape: "positive selected recursion with a certified SCC/WTO head", risk: "selected recursion cannot introduce negative or hidden callbacks"},
}

func TestW0HostileCoverageInventoryNamesRequiredShapes(t *testing.T) {
	if len(w0HostileCoverageInventory) == 0 {
		t.Fatal("W0 hostile coverage inventory is empty")
	}
	seen := make(map[string]struct{}, len(w0HostileCoverageInventory))
	capabilities := make(map[string]struct{})
	for _, shape := range w0HostileCoverageInventory {
		if shape.name == "" || shape.category == "" || shape.semanticShape == "" || shape.risk == "" {
			t.Fatalf("malformed W0 hostile coverage row: %+v", shape)
		}
		if _, duplicate := seen[shape.name]; duplicate {
			t.Fatalf("duplicate W0 hostile coverage row %q", shape.name)
		}
		seen[shape.name] = struct{}{}
		for _, capability := range shape.capabilities {
			if capability == "" {
				t.Fatalf("%s has an empty capability name", shape.name)
			}
			capabilities[capability] = struct{}{}
		}
	}

	required := []string{
		"ordinary-exact-family",
		"seed-family",
		"selected-read",
		"correspondence-read",
		"summary-vector-absent-cells",
		"complete-vector-absent-cells",
		"routed-publication",
		"transformed-publication",
		"activation-publication",
		"transport-publication",
		"structural-publication",
		"placement-capture-family",
		"placement-containment-family",
		"placement-formal-family",
		"placement-publication-escape-family",
		"placement-suspension-family",
		"typed-query-fold",
		"selected-query-site-closure",
		"diagnostic-observation",
		"decision-scope",
		"raw-get-dependent-six-read-chain",
		"raw-get-receiver-route-expansion",
		"raw-get-raw-access-expansion",
		"raw-get-payload-expansion",
		"raw-set-dependent-chain",
		"raw-set-atomic-heap-update",
		"absence-complete",
		"selected-recursion",
	}
	for _, name := range required {
		if _, ok := seen[name]; !ok {
			t.Errorf("W0 hostile coverage inventory omits required shape %q", name)
		}
	}
	for _, capability := range []string{w0ReceiverRouteExpansion, w0RawAccessExpansion, w0PayloadExpansion} {
		if _, ok := capabilities[capability]; !ok {
			t.Errorf("W0 hostile coverage inventory omits explicit capability risk %q", capability)
		}
	}
}

func TestW0HostileCoverageInventoryHasAllSemanticFamilies(t *testing.T) {
	wantCategories := map[string]bool{
		"declaration": false,
		"read":        false,
		"publication": false,
		"placement":   false,
		"query":       false,
		"diagnostic":  false,
		"raw-get":     false,
		"raw-set":     false,
		"absence":     false,
		"recursion":   false,
	}
	for _, shape := range w0HostileCoverageInventory {
		if _, ok := wantCategories[shape.category]; !ok {
			t.Fatalf("unknown W0 hostile coverage category %q in %s", shape.category, shape.name)
		}
		wantCategories[shape.category] = true
	}
	for category, present := range wantCategories {
		if !present {
			t.Errorf("W0 hostile coverage inventory omits semantic family %q", category)
		}
	}
}
