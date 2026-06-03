package trace

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestGraphEvidenceIncludesParameterUses(t *testing.T) {
	stmts, err := parse.ParseString(`
		local name = client.name
		return raw, name
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"client", "raw"}},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("expected graph")
	}

	evidence := GraphEvidence(graph, graph.Bindings())
	if len(evidence.ParameterUses) != 2 {
		t.Fatalf("expected parameter uses for client and raw, got %d", len(evidence.ParameterUses))
	}

	nameBySymbol := make(map[cfg.SymbolID]string)
	for _, slot := range graph.ParamSlotsReadOnly() {
		nameBySymbol[slot.Symbol] = slot.Name
	}
	uses := make(map[string]struct {
		whole  bool
		fields []constraint.Segment
	})
	for _, use := range evidence.ParameterUses {
		uses[nameBySymbol[use.Symbol]] = struct {
			whole  bool
			fields []constraint.Segment
		}{whole: use.Whole, fields: use.Fields}
	}
	if raw := uses["raw"]; !raw.whole {
		t.Fatalf("expected raw to be used whole, got %+v", raw)
	}
	if client := uses["client"]; client.whole || len(client.fields) != 1 ||
		client.fields[0].Kind != constraint.SegmentField || client.fields[0].Name != "name" {
		t.Fatalf("expected client.name field use only, got %+v", client)
	}
}
