package program

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestObserverProgramKeepsTwoCallContextsCorrelated(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(value: string) return value end
local first = leaf("first")
local second = leaf(false)
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	stats := &Stats{}
	artifact, err := runEvaluatedBoundChunk(context.Background(), stmts, bindings, Config{
		Check: body.Config{
			Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true},
			UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("observer-two-context")),
		},
		Stats: stats,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.program.instances) != 3 || stats.EvaluatedRelationEquationApplications != 2 ||
		stats.EvaluatedRootProjections != 3 || stats.EvaluatedObserverInstanceProjections != 3 ||
		stats.EvaluatedObserverEntryProjections != 1 || stats.EvaluatedObserverTermApplications == 0 {
		t.Fatalf("instances/equations/projections/instance/entry/terms = %d/%d/%d/%d/%d/%d, want 3/2/3/3/1/>0",
			len(artifact.program.instances), stats.EvaluatedRelationEquationApplications, stats.EvaluatedRootProjections,
			stats.EvaluatedObserverInstanceProjections, stats.EvaluatedObserverEntryProjections,
			stats.EvaluatedObserverTermApplications)
	}
	entry, ok, err := artifact.Entry(context.Background(), reg)
	if err != nil || !ok {
		t.Fatalf("entry materialization = ok:%v err:%v", ok, err)
	}
	var first, invalid bool
	for _, slot := range entry.Observations() {
		for _, item := range slot.Observed {
			if item.Kind != observation.CallArgument || !item.HasExpected || !product.Equal(reg, item.Expected, typevalue.String(reg)) {
				continue
			}
			first = first || product.Equal(reg, item.Actual, typevalue.LiteralString(reg, "first"))
			invalid = invalid || product.Equal(reg, item.Actual, typevalue.LiteralBool(reg, false))
		}
	}
	if !first || !invalid {
		t.Fatalf("entry lost correlated actual/expected call evidence: first=%v invalid=%v", first, invalid)
	}
	// Both contexts share the lexical template but retain distinct canonical
	// boundaries and independently sealed local evidence.
	left := artifact.program.instances[1]
	right := artifact.program.instances[2]
	if left.template != right.template || string(left.boundary.values[0].Bytes()) == string(right.boundary.values[0].Bytes()) {
		t.Fatal("two call contexts were merged or assigned different lexical templates")
	}
	for _, instance := range artifact.program.instances {
		if _, err := instance.local.Materialize(context.Background(), reg); err != nil {
			t.Fatalf("instance %d local artifact: %v", instance.id, err)
		}
	}
}
