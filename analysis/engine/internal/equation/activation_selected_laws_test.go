package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func selectedStructuralFixture(t testing.TB) (*Topology, AcceptedMember) {
	t.Helper()
	factor := boundaryKey(101)
	family, query, freezer := boundaryKey(102), boundaryKey(118), boundaryKey(119)
	triggerRule, triggerFamily, triggerProof := boundaryKey(103), boundaryKey(104), boundaryKey(105)
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors:            []composition.Factor{{Key: factor}},
		ActivationFamilies: []composition.ActivationFamily{{Semantic: family}},
		Rules: []composition.Rule{{
			Key: triggerRule, OperandFamily: triggerFamily,
			Admission:   composition.Admission{Kind: composition.AdmissionTrustedTheorem, Identity: triggerProof},
			OutputKind:  composition.StructuralOutput,
			Activations: []composition.ActivationRange{{Family: family}},
		}},
		Queries: []composition.QueryFamily{{
			Key: query, Freezer: freezer, Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factor}},
		}},
	})
	if !sourceOK || source == nil {
		t.Fatal("source")
	}
	first, second := boundaryDecision(t, 106), boundaryDecision(t, 107)
	ambient, ambientOK := NewScope(first, second)
	premise, premiseOK := DecisionExpr(first)
	if !ambientOK || !premiseOK {
		t.Fatal("ambient")
	}
	batch := NewBatch()
	sourceSite, sourceSiteOK := batch.AdmitSite(boundaryKey(108), ambient, TrueExpr(), InitPresent)
	targetSite, targetSiteOK := batch.AdmitSite(boundaryKey(109), ambient, TrueExpr(), InitPresent)
	triggerSite, triggerSiteOK := batch.AdmitSite(boundaryKey(110), ambient, TrueExpr(), InitPresent)
	occurrence, occurrenceOK := batch.At(triggerSite)
	operand, operandOK := batch.AdmitOperand(occurrence, boundaryKey(111))
	forgetAmbient, forgetOK := NewReindex(ambient, EmptyScope(), []DecisionMap{Forget(first), Forget(second)})
	if !sourceSiteOK || !targetSiteOK || !triggerSiteOK || !occurrenceOK || !operandOK || !forgetOK || !batch.Seal() {
		t.Fatal("batch")
	}
	exportRole := boundaryKey(113)
	target, endpoint, application := boundaryKey(114), boundaryKey(115), boundaryKey(116)
	plan, planOK := NewVariantPlan(source, family, []Variant{{
		Target: target, Endpoint: endpoint,
		Template: Template{
			Ports: []Port{{Role: exportRole, Mode: PortExport}},
			FactorEdges: []FragmentFactorEdge{{
				ExternalSource: sourceSite, Target: FragmentPoint{Port: exportRole},
				Factor: factor, Provenance: boundaryKey(117), Pre: TrueExpr(), Reindex: forgetAmbient, Post: TrueExpr(),
			}},
		},
	}})
	if !planOK {
		t.Fatal("structural plan")
	}
	spec := TopologySpec{
		Batch:  batch,
		Rules:  []RuleInstance{{Schema: triggerRule, OperandFamily: triggerFamily, Occurrence: occurrence, Operand: operand}},
		Points: []PointSpec{{Site: sourceSite}, {Site: targetSite}, {Site: triggerSite}},
		Groups: []Group{{Members: []RuleRef{RuleAt(0)}, Output: PointAt(2)}},
		Queries: []QueryInstance{{
			Family: query, Point: PointAt(2), Surfaces: []Surface{{Factor: factor, Form: SurfaceReadExact, Local: 1}},
		}},
		ActivationBindings: []ActivationBinding{{
			Family: family, Trigger: RuleAt(0), Application: application, Plan: plan,
			PortBindings: []PortBinding{{Role: exportRole, Base: PointAt(1)}},
		}},
	}
	topology, topologyOK := SealTopology(source, spec)
	if !topologyOK || topology == nil || len(topology.bindings) != 1 {
		t.Fatal("topology")
	}
	member, memberOK := topology.SelectMember(topology.bindings[0].trigger, PairLocator{Application: application, Target: target, Endpoint: endpoint})
	accepted, acceptedOK := topology.Accept(member, premise)
	if !memberOK || !acceptedOK {
		t.Fatal("accepted structural member")
	}
	return topology, accepted
}

func TestSelectedStructuralFactorEdgesMatchOrdinaryGraphExpansion(t *testing.T) {
	topology, accepted := selectedStructuralFixture(t)
	base, baseOK := topology.Graph(nil)
	edges, materialized := topology.SelectedStructuralFactorEdges(base, []AcceptedMember{accepted})
	graph, compiled := topology.Graph([]AcceptedMember{accepted})
	if !baseOK || base == nil || !materialized || len(edges) != 1 || !compiled || graph == nil || graph.FactorEdgeTotal() != 1 {
		t.Fatal("selected structural edge materialization")
	}
	compiledEdge, edgeOK := graph.FactorEdgeAtIndex(0)
	selected := edges[0]
	if !edgeOK || !selected.Available() || selected.Key() != compiledEdge.Key() || selected.Source().Key() != compiledEdge.Input().Point().Key() ||
		selected.Target().Key() != compiledEdge.Target().Key() || selected.Input().Key() != compiledEdge.Input().Key() || selected.Factor() != compiledEdge.Factor() {
		t.Fatal("selected descriptor diverged from ordinary graph FactorEdge")
	}
	if selected.Input().Pre().IsTrue() {
		t.Fatal("selected descriptor dropped accepted premise")
	}
	if !base.OwnsPoint(selected.Source()) || !base.OwnsPoint(selected.Target()) || !base.OwnsPoint(selected.Input().Point()) {
		t.Fatal("selected descriptor did not retain graph-owned endpoints")
	}
	if _, indexed := base.PointIndex(selected.Source()); !indexed {
		t.Fatal("selected source is not indexable in base graph")
	}
	if _, indexed := base.PointIndex(selected.Target()); !indexed {
		t.Fatal("selected target is not indexable in base graph")
	}
}

func TestSelectedStructuralFactorEdgesRejectTypedOrLocalTemplate(t *testing.T) {
	topology, accepted := selectedStructuralFixture(t)
	base, baseOK := topology.Graph(nil)
	if !baseOK || base == nil {
		t.Fatal("base graph")
	}
	variant := &topology.bindings[0].plan.data.variants[0].template
	variant.value.Points = []PointSpec{{}}
	if edges, materialized := topology.SelectedStructuralFactorEdges(base, []AcceptedMember{accepted}); materialized || edges != nil {
		t.Fatal("materializer admitted non-base structural template")
	}
}
