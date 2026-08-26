package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

func heapAxisRef() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
}

func heapRelation(key schema.Key) member.RelationRef {
	return member.RelationRef{Axis: heapAxisRef(), Member: key}
}

func heapProjection(key schema.Key) member.ProjectionRef {
	return member.ProjectionRef{Axis: heapAxisRef(), Member: key}
}

// selectedSpecimen is one rule whose second read is produced: its rows are
// published by a named operation and correlated by the tag that operation
// stamps. It is the shape sixteen census rows are waiting to become.
func selectedSpecimen() rule.Spec {
	exact := ruleprogram.ReadContract{
		Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
		OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne,
	}
	return rule.Spec{
		Key: "heap-selected-specimen", Writes: "heap", Owner: "heap", Lane: rule.LaneMounted,
		Semantic: "semantic/rule/heap/selected-specimen",
		Roles:    []schema.Key{"semantic/operand/heap/selected-specimen"},
		Issues: []rule.Issuance{{
			Occurrence: "occurrence/index-read", Requirement: "program-requirement/unrestricted",
			Form: "program-form/computation",
		}},
		Program: ruleprogram.Program{
			OperandRole: "semantic/operand/heap/selected-specimen",
			Candidate:   member.AxisRelationCandidate(heapRelation("heap/candidates")),
			Joins: []ruleprogram.JoinDecl{
				{
					Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
					Relation: heapRelation("heap/facts"),
					Key:      heapProjection("heap/fact-key"),
					Read: ruleprogram.ReadDecl{
						Input: 0, Axis: ruleprogram.AxisRef(heapAxisRef()),
						Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: exact,
					},
				},
				{
					Sources:   []ruleprogram.SourceRef{ruleprogram.CandidateSource(), ruleprogram.PriorSource(0)},
					Relation:  heapRelation("heap/routes"),
					Key:       heapProjection("heap/route-key"),
					Predicate: heapProjection("heap/route-tag"),
					Selection: member.SelectionRef{Axis: heapAxisRef(), Member: "heap/route-selection"},
					Read: ruleprogram.ReadDecl{
						Input: 0, Axis: ruleprogram.AxisRef(heapAxisRef()),
						Form: ruleprogram.Selected, PointBound: ruleprogram.PointBoundSelf, Contract: exact,
					},
				},
			},
			Fold: ruleprogram.FoldDecl{
				Reducer: member.ReducerRef{Axis: heapAxisRef(), Member: "heap/specimen-reducer"},
				Inputs:  []ruleprogram.JoinRef{1},
				Outputs: []ruleprogram.OutputDecl{{
					Column:      axis.OutputRef{Axis: heapAxisRef(), Key: "heap/published"},
					Destination: heapProjection("heap/publication"),
					Mode:        ruleprogram.ModeExact,
					ValueSlot:   0,
				}},
			},
		},
	}
}

// TestAProducedReadLowersToOneApplyAndOneJoin states the lowering the whole
// remaining corpus is waiting on. A read whose rows an operation publishes
// becomes that operation's own dependency plus an ordinary equijoin onto the
// tag it stamps, so nothing about a selection is a form.
func TestAProducedReadLowersToOneApplyAndOneJoin(t *testing.T) {
	surfaces := newOwners(t)
	installSyntheticHeapCatalog(t, surfaces)
	spec := selectedSpecimen()
	placement := surfaces.install(spec)

	resolution, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	rules := resolution.Rules
	if len(rules) != 2 {
		t.Fatalf("resolved rules = %d, want the selection and the rule that reads it", len(rules))
	}
	producer, reader := rules[0], rules[1]
	if producer.Publish == nil || producer.Publish.Relation != reader.Joins[len(reader.Joins)-1].Relation {
		t.Fatal("the selection does not publish into the relation the read consumes")
	}
	if !producer.Apply.Available() {
		t.Fatal("the selection names no semantic operation")
	}
	if len(producer.Joins) != 1 {
		t.Fatalf("selection joins = %d, want the one earlier result it consumes", len(producer.Joins))
	}

	compiled := lower(t, surfaces, spec, rules)
	if len(compiled.Expressions()) != 2 {
		t.Fatalf("expressions = %d, want one per dependency", len(compiled.Expressions()))
	}
	for _, expression := range compiled.Expressions() {
		published, ok := expression.Expression().(algebra.Publish)
		if !ok {
			t.Fatalf("root = %T, want Publish", expression.Expression())
		}
		if !containsApply(published.Child()) {
			t.Fatal("a dependency publishes rows no operation produced")
		}
	}
}

// TestAnUnselectedReadConsumesEveryDeclaredSource states the consumer
// boundary: Selection absence prevents a second producer dependency, not use
// of ordered predecessor results. Dropping the prior source would turn a
// two-input consumer into an accidental source-zero read.
func TestAnUnselectedReadConsumesEveryDeclaredSource(t *testing.T) {
	surfaces := newOwners(t)
	installSyntheticHeapCatalog(t, surfaces)
	spec := selectedSpecimen()
	joins := spec.Program.Joins
	joins[1].Selection = member.SelectionRef{}
	spec.Program.Joins = joins
	placement := surfaces.install(spec)

	rules, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err != nil {
		t.Fatalf("resolve selection-absent consumer: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("resolved rules=%d, want consumer only and no rerun producer", len(rules))
	}
	reader := rules[0]
	if len(reader.Joins) != 3 {
		t.Fatalf("consumer joins=%d, want first read plus candidate and prior correlations", len(reader.Joins))
	}
	candidate := relcompile.NewName(heapAxisRef(), "heap/candidates")
	first := relcompile.NewName(heapAxisRef(), "heap/facts")
	routes := relcompile.NewName(heapAxisRef(), "heap/routes")
	site := relcompile.Site{Rule: spec.Key, Path: "law"}
	candidateAddress, err := surfaces.registry.Addressed(site, candidate, relcompile.CoordinateAddress)
	if err != nil {
		t.Fatalf("candidate address: %v", err)
	}
	firstAddress, err := surfaces.registry.Addressed(site, first, relcompile.CoordinateAddress)
	if err != nil {
		t.Fatalf("prior address: %v", err)
	}
	routeKey, err := surfaces.registry.Column(site, relcompile.NewName(heapAxisRef(), "heap/route-key"))
	if err != nil {
		t.Fatalf("route key: %v", err)
	}
	for index, want := range []struct{ left model.ColumnID }{{candidateAddress}, {firstAddress}} {
		join := reader.Joins[index+1]
		if join.Relation != routeKey.Relation() || len(join.LeftColumns) != 1 || len(join.RightColumns) != 1 ||
			join.LeftColumns[0] != want.left || join.RightColumns[0] != routeKey {
			t.Fatalf("consumer correlation %d=%+v, want source address %v -> canonical route key %v", index, join, want.left, routeKey)
		}
	}
	routeID, err := surfaces.registry.Relation(site, routes)
	if err != nil {
		t.Fatalf("route relation: %v", err)
	}
	if reader.Joins[1].LeftColumns[0] == reader.Joins[2].LeftColumns[0] || reader.Joins[1].Relation != reader.Joins[2].Relation || reader.Joins[1].Relation != routeID {
		t.Fatalf("consumer did not retain distinct candidate/prior source correlations: %+v", reader.Joins[1:])
	}
}

func containsApply(expression algebra.Expression) bool {
	switch value := expression.(type) {
	case algebra.Apply:
		return true
	case algebra.Merge:
		for _, input := range value.Inputs() {
			if containsApply(input) {
				return true
			}
		}
	case algebra.Join:
		return containsApply(value.Left()) || containsApply(value.Right())
	case algebra.Select:
		return containsApply(value.Child())
	case algebra.Complete:
		return containsApply(value.Child())
	case algebra.Expand:
		return containsApply(value.Child())
	}
	return false
}
