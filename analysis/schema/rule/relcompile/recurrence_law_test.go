package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

func recurrenceAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
}

func recurrenceRelation(key schema.Key) member.RelationRef {
	return member.RelationRef{Axis: recurrenceAxis(), Member: key}
}

func recurrenceProjection(key schema.Key) member.ProjectionRef {
	return member.ProjectionRef{Axis: recurrenceAxis(), Member: key}
}

// selfReadingSpecimen is one family whose fold reads the very relation it
// publishes to. Its dependency is its own producer, so the plan it lowers to
// has one component with a cycle through it: the shape every ascending family
// has, and the shape a certificate cannot be issued over while the component
// goes undeclared.
func selfReadingSpecimen() rule.Spec {
	exact := ruleprogram.ReadContract{
		Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
		OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne,
	}
	return rule.Spec{
		Key: "heap-self-reading", Writes: "heap", Owner: "heap", Lane: rule.LaneMounted,
		Semantic: "semantic/rule/heap/self-reading",
		Roles:    []schema.Key{"semantic/operand/heap/self-reading"},
		Issues: []rule.Issuance{{
			Occurrence: "occurrence/index-read", Requirement: "program-requirement/unrestricted",
			Form: "program-form/computation",
		}},
		Program: ruleprogram.Program{
			OperandRole: "semantic/operand/heap/self-reading",
			Candidate:   member.AxisRelationCandidate(recurrenceRelation("heap/candidates")),
			Joins: []ruleprogram.JoinDecl{{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: recurrenceRelation("heap/published"),
				Key:      recurrenceProjection("heap/published-key"),
				Read: ruleprogram.ReadDecl{
					Input: 0, Axis: ruleprogram.AxisRef(recurrenceAxis()),
					Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: exact,
				},
			}},
			Fold: ruleprogram.FoldDecl{
				Reducer: member.ReducerRef{Axis: recurrenceAxis(), Member: "heap/self-reading-reducer"},
				Inputs:  []ruleprogram.JoinRef{0},
				Outputs: []ruleprogram.OutputDecl{{
					Column:      axis.OutputRef{Axis: recurrenceAxis(), Key: "heap/published"},
					Destination: recurrenceProjection("heap/publication"),
					Mode:        ruleprogram.ModeExact,
					ValueSlot:   0,
				}},
			},
		},
	}
}

// TestASelfReadingFamilyLowersToADeclaredComponent states what the certificate
// stage needs from the compiler: a lowered plan declares the components of its
// own dependency graph. A family that reads the relation it publishes is its
// own producer, so its component carries a cycle and is declared positive.
func TestASelfReadingFamilyLowersToADeclaredComponent(t *testing.T) {
	surfaces := newOwners(t)
	spec := selfReadingSpecimen()
	placement := surfaces.install(spec)

	rules, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	compiled := lower(t, surfaces, spec, rules)

	components := compiled.SCCs()
	if len(components) == 0 {
		t.Fatal("a lowered plan declared no component of its own dependency graph")
	}
	positive := 0
	for _, component := range components {
		if component.Recurrence().Kind() == planPositive() {
			positive++
			if len(component.Edges()) == 0 {
				t.Fatal("a component that recurs declares no edge that carries the recurrence")
			}
		}
		if len(component.Recurrence().Heads()) != 0 {
			t.Fatal("the compiler chose a widening head nobody declared")
		}
	}
	if positive != 1 {
		t.Fatalf("positive components = %d, want the one the self-read forms", positive)
	}
}

// TestALoweredPlanRaisesNoRecurrenceRefusal is the gate this closes. The
// checker derives the dependency graph itself and refuses a schema that
// declares fewer components than the graph has, so every lowered plan - a
// self-reading family most of all - used to die before its certificate for a
// component nobody stated. The compiler states them, and the recurrence pass
// has nothing left to report.
//
// The remaining passes are a separate gate: the census surface owes the
// checker shared join TypeIDs, plan-fenced signatures and source-column
// inputs, and their findings are reported there rather than folded in here.
func TestALoweredPlanRaisesNoRecurrenceRefusal(t *testing.T) {
	surfaces := newOwners(t)
	spec := selfReadingSpecimen()
	placement := surfaces.install(spec)

	rules, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	compiled := lower(t, surfaces, spec, rules)

	_, refusal := certificate.Check(compiled)
	for _, issue := range refusal.Issues() {
		if issue.Pass == certificate.PassRecurrence || issue.Pass == certificate.PassStructural {
			t.Fatalf("a lowered plan was refused over its own dependency graph: %s", issue)
		}
	}
}

func planPositive() plan.RecurrenceKind { return plan.Positive }
