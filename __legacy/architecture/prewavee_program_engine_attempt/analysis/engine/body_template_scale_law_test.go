package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program"
)

// TestNestedBodyDemandPreservesOnePublicFlow is a semantic scale law: a
// signal entering a deeply nested lexical program must reach the enclosing
// normal outcome through the one public Program/engine path. It intentionally
// asserts neither body-index representation nor work counts.
func TestNestedBodyDemandPreservesOnePublicFlow(t *testing.T) {
	const depth = 96
	var source strings.Builder
	for range depth {
		source.WriteString("do\n")
	}
	source.WriteString("local payload = 1\n")
	for range depth {
		source.WriteString("end\n")
	}
	source.WriteString("local completed = true\n")

	p := localLawProgram(t, source.String())
	if p.BodyCount() < depth+1 {
		t.Fatalf("nested source produced %d Bodies, want at least %d", p.BodyCount(), depth+1)
	}
	project, shard := localLawLink(t, p)
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program.Entry")
	}
	exit, ok := p.BodyNormalExit(entry)
	if !ok {
		t.Fatal("BodyNormalExit")
	}
	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	facts := localLawFactor(t, solver, 0xe1)
	if _, ok := engine.DeclareRule(solver, facts, bodyTemplateScaleSemantic(0), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, entry)
	}, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(0, localLawOne)
	}); !ok {
		t.Fatal("declare entry fact")
	}
	count, ok := p.ActivationEdgeCount(entry)
	if !ok || count == 0 {
		t.Fatal("ActivationEdgeCount")
	}
	for index := 0; index < count; index++ {
		edge, ok := p.ActivationEdgeAt(entry, index)
		if !ok {
			t.Fatalf("ActivationEdgeAt(%d)", index)
		}
		var carried engine.ReadRef[uint64, localLawBits]
		rule, ok := engine.DeclareRule(solver, facts, bodyTemplateScaleSemantic(index+1), func(binding *engine.RuleBinding) bool {
			return binding.From(shard, edge)
		}, func(access engine.Access[uint64, localLawBits]) bool {
			return engine.Carry(access, carried)
		})
		if !ok {
			t.Fatalf("declare transfer %d", index)
		}
		if carried, ok = engine.Read(rule, 0, facts); !ok {
			t.Fatalf("bind transfer read %d", index)
		}
	}
	query, ok := engine.DeclareQuery(solver, facts, shard, exit, 0)
	if !ok {
		t.Fatal("DeclareQuery")
	}
	if !solver.Seal() {
		t.Fatal("Seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solve")
	}
	if got, present := query.Read(state); !present || got != localLawOne {
		t.Fatalf("nested normal outcome = %d/%t, want %d/true", got, present, localLawOne)
	}
}

func bodyTemplateScaleSemantic(index int) engine.SemanticKey {
	return engine.SemanticKey{ID: program.ContentID{byte(index), byte(index >> 8), 0x7a}, Version: 1}
}
