package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPreparedEquationReusesBuilderArenaAcrossSCCRounds(t *testing.T) {
	reg := standard.Registry()
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificate, err := CertifyPlan(plan, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	relation, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True()}})
	if err != nil {
		t.Fatal(err)
	}
	ref := CellRef{Function: 90}
	calls := 0
	prepared, err := NewPreparedEquation(ref, builder, []CellRef{ref, ref}, func(_ context.Context, view RelationView, got *Builder) (Relation, error) {
		calls++
		if got != builder || got.Arena() != relation.arena {
			t.Fatal("prepared equation replaced its persistent builder arena")
		}
		if _, ok := view.Lookup(ref); !ok {
			t.Fatal("declared self dependency missing")
		}
		return relation, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cell, err := prepared.Cell()
	if err != nil {
		t.Fatal(err)
	}
	if len(cell.Dependencies) != 1 || cell.Dependencies[0] != ref {
		t.Fatalf("prepared dependencies = %#v", cell.Dependencies)
	}
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{cell}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := snapshot.Lookup(ref)
	if !ok || !EqualRelation(got, relation) || calls < 2 {
		t.Fatalf("prepared SCC result = %v calls=%d", ok, calls)
	}
}

func TestPreparedEquationRejectsForeignRelationIdentity(t *testing.T) {
	reg := standard.Registry()
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	owner := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	foreign := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	prepared, err := NewPreparedEquation(CellRef{Function: 91}, owner, nil, func(context.Context, RelationView, *Builder) (Relation, error) {
		return Relation{shape: Shape{}, arena: foreign.Arena()}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cell, _ := prepared.Cell()
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{cell}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := snapshot.Lookup(cell.Ref)
	if !ok || got.ContextualReason() == "" {
		t.Fatalf("foreign prepared relation published: %#v/%v", got, ok)
	}
}
