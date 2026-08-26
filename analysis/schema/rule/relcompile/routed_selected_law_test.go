package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// TestRoutedSelectedKeepsTheRouteTupleAndFinalFactorInOneApplyChild is the
// hostile two-candidate shape in declaration form. The route scalar columns
// stay on R, the selected fact stays on the final Factor F, and one Apply
// reads one left-deep C -> R -> F tuple. In particular, R and F are distinct
// identities: any lowering that joins them as independent children could pair
// R(a) with F(b).
func TestRoutedSelectedKeepsTheRouteTupleAndFinalFactorInOneApplyChild(t *testing.T) {
	surfaces, spec, placement, route, writer := installRoutedSelected(t)
	rules, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err != nil {
		t.Fatalf("resolve routed selected: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules=%d, want one canonical route producer and one reader", len(rules))
	}
	if rules[0].Publish == nil {
		t.Fatal("the explicitly selected route has no producer dependency")
	}
	reader := rules[1]
	site := relcompile.Site{Rule: spec.Key, Path: "routed-selected-law"}
	routeID, err := surfaces.registry.Relation(site, route)
	if err != nil {
		t.Fatalf("route relation: %v", err)
	}
	writerID, err := surfaces.registry.Relation(site, writer.relation)
	if err != nil {
		t.Fatalf("writer relation: %v", err)
	}
	if routeID == writerID {
		t.Fatal("route and selected Factor aliases share one relation identity")
	}
	if len(reader.Joins) != 2 || reader.Joins[0].Relation != routeID || reader.Joins[1].Relation != writerID {
		t.Fatalf("reader joins=%+v, want R then final selected F", reader.Joins)
	}
	wantSlots := []relcompile.ReadOccurrence{
		relcompile.JoinOccurrence(0),
		relcompile.JoinOccurrence(0),
		relcompile.JoinOccurrence(0),
		relcompile.JoinOccurrence(1),
	}
	if len(reader.ApplySlots) != len(wantSlots) {
		t.Fatalf("apply slots=%v, want R/R/R/F", reader.ApplySlots)
	}
	for index, want := range wantSlots {
		if reader.ApplySlots[index] != want {
			t.Fatalf("apply slot %d=%v, want exact R/R/R/F occurrence %v", index, reader.ApplySlots[index], want)
		}
	}
	writerKey, err := surfaces.registry.Key(site, writer.key)
	if err != nil {
		t.Fatalf("writer key: %v", err)
	}
	if reader.Publish == nil || reader.Publish.Relation != writerID || reader.Publish.Key != writerKey ||
		len(reader.Publish.Columns) != 1 || reader.Publish.Columns[0] != mustColumn(t, surfaces, site, writer.column) {
		t.Fatalf("publication=%+v, want Output.Column writer F independent of route Destination", reader.Publish)
	}

	compiled := lower(t, surfaces, spec, rules)
	apply, ok := onlyApply(compiled.Expressions()[1].Expression())
	if !ok {
		t.Fatalf("reader expression has no Apply: %T", compiled.Expressions()[1].Expression())
	}
	if len(apply.Inputs()) != 1 {
		t.Fatalf("Apply children=%d, want one correlated C/R/F tuple", len(apply.Inputs()))
	}
	for index, source := range apply.Contract().SlotSource() {
		if source.Child() != 0 {
			t.Fatalf("Apply slot %d child=%d, want the one correlated child", index, source.Child())
		}
	}
}

// TestRoutedSelectedRefusesAForeignDestination keeps the three output roles
// independent: Destination is an R-owned coordinate, while Output.Column is
// an F-owned fact column. Replacing the former with the latter must not be
// accepted merely because both values are keys.
func TestRoutedSelectedRefusesAForeignDestination(t *testing.T) {
	surfaces, spec, placement, _, writer := installRoutedSelected(t)
	output := spec.Program.Fold.Outputs[0]
	output.Destination = member.ProjectionRef{Axis: output.Column.Axis, Member: writer.column.Member}
	spec.Program.Fold.Outputs[0] = output
	_, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err == nil {
		t.Fatal("foreign writer fact column was admitted as a route destination")
	}
	refusal := refusalOf(t, err)
	if refusal.Kind != relcompile.KindColumn || refusal.Reason != relcompile.ReasonForeign {
		t.Fatalf("destination refusal=%v/%v, want foreign column", refusal.Kind, refusal.Reason)
	}
}

// TestRoutedSelectedRefusesAForeignWriterFactor proves a selected F cannot
// be published through an unrelated W absent an explicit correspondence.
func TestRoutedSelectedRefusesAForeignWriterFactor(t *testing.T) {
	surfaces, spec, placement, _, _ := installRoutedSelected(t)
	output := spec.Program.Fold.Outputs[0]
	output.Column.Key = "routed-law/foreign-writer-fact"
	spec.Program.Fold.Outputs[0] = output
	// This is an owner-issued output binding, so the rejection below proves
	// relation mismatch rather than a missing registry installation.
	surfaces.installOutput(output, placement.Candidate)
	_, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err == nil {
		t.Fatal("foreign output writer was accepted for selected Factor")
	}
	refusal := refusalOf(t, err)
	if refusal.Kind != relcompile.KindRelation || refusal.Reason != relcompile.ReasonForeign {
		t.Fatalf("writer refusal=%v/%v, want foreign Factor relation", refusal.Kind, refusal.Reason)
	}
}

func installRoutedSelected(t *testing.T) (*owners, rule.Spec, relcompile.Placement, relcompile.Name, outputInstallation) {
	t.Helper()
	const (
		candidateAxisKey schema.Key = "routed-law-candidate"
		routeAxisKey     schema.Key = "routed-law-route"
		writerAxisKey    schema.Key = "routed-law-writer"
	)
	candidateAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: candidateAxisKey}
	routeAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: routeAxisKey}
	writerAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: writerAxisKey}
	denominator := ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: "routed-law/coordinates"}
	routeRelation := member.RelationRef{Axis: routeAxis, Member: "routed-law/routes"}
	routeKey := member.ProjectionRef{Axis: routeAxis, Member: "routed-law/route-key"}
	routeTag := member.ProjectionRef{Axis: routeAxis, Member: "routed-law/route-tag"}
	routeDestination := member.ProjectionRef{Axis: routeAxis, Member: "routed-law/route-destination"}
	output := ruleprogram.OutputDecl{
		Column:           axis.OutputRef{Axis: writerAxis, Key: "routed-law/fact"},
		Destination:      routeDestination,
		Mode:             ruleprogram.ModeRoute,
		ValueSlot:        0,
		RouteJoin:        0,
		RouteJoinPresent: true,
	}
	spec := rule.Spec{
		Key: "routed-selected-law", Writes: writerAxisKey, Owner: writerAxisKey, Lane: rule.LaneMounted,
		Semantic: "semantic/rule/routed-selected-law", Roles: []schema.Key{"semantic/operand/routed-selected-law"},
		Program: ruleprogram.Program{
			OperandRole: "semantic/operand/routed-selected-law",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: candidateAxis, Member: "routed-law/candidates"}),
			Joins: []ruleprogram.JoinDecl{{
				Sources:   []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation:  routeRelation,
				Key:       routeKey,
				Predicate: routeTag,
				Selection: member.SelectionRef{Axis: routeAxis, Member: "routed-law/route-selection"},
				Read: ruleprogram.ReadDecl{
					Input: 0, Axis: ruleprogram.AxisRef(writerAxis), Form: ruleprogram.Selected, PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseDefault, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne, DenominatorRef: denominator},
				},
			}},
			Fold: ruleprogram.FoldDecl{
				Reducer: member.ReducerRef{Axis: writerAxis, Member: "routed-law/reducer"},
				Inputs:  []ruleprogram.JoinRef{0, 0, 0, 0},
				Outputs: []ruleprogram.OutputDecl{output},
			},
		},
	}
	if problem, valid := spec.Program.Check(); !valid {
		t.Fatalf("routed selected declaration rejected: %+v", problem)
	}

	surfaces := newOwners(t)
	installSyntheticRoutedCatalogs(t, surfaces, candidateAxis, routeAxis, writerAxis)
	candidateScope := surfaces.scope(relcompile.EntryName(schema.SurfaceKindStructure, "routed-law/candidate-scope"))
	portScope := surfaces.scope(relcompile.EntryName(schema.SurfaceKindStructure, "routed-law/port-scope"))
	candidate := relcompile.NewName(candidateAxis, "routed-law/candidates")
	surfaces.relation(candidate, candidateScope)
	writer := surfaces.installOutput(output, candidateScope)
	join, _ := spec.Program.JoinAt(0)
	surfaces.factor(denominator, writerAxis, writer)
	surfaces.installJoin(join, candidateScope, candidate, []relcompile.Name{candidate}, &routedOutputInstallation{output: output, binding: writer})
	route := relcompile.NewName(routeAxis, "routed-law/routes")
	summary := relcompile.NewName(routeAxis, "routed-law/route-summary")
	surfaces.column(summary, route)
	semanticInputs := []signature.Input{
		routedSignatureInput(t, surfaces, route, summary),
		routedSignatureInput(t, surfaces, route, relcompile.NewName(routeAxis, "routed-law/route-key")),
		routedSignatureInput(t, surfaces, route, relcompile.NewName(routeAxis, "routed-law/route-tag")),
		routedSignatureInput(t, surfaces, writer.relation, writer.column),
	}
	surfaces.installOperation(relcompile.NewName(writerAxis, "routed-law/reducer"), writer.column, semanticInputs)
	surfaces.expression(schema.EntryReference{Surface: schema.SurfaceKindRule, Key: spec.Key}, output.Column.Key)
	return surfaces, spec, relcompile.Placement{Candidate: candidateScope, Ports: []relcompile.Name{portScope}}, route, writer
}

func routedSignatureInput(t *testing.T, surfaces *owners, relation, column relcompile.Name) signature.Input {
	t.Helper()
	site := relcompile.Site{Path: "routed-selected-law.signature"}
	relationID, err := surfaces.registry.Relation(site, relation)
	if err != nil {
		t.Fatalf("signature relation %v: %v", relation, err)
	}
	columnID := mustColumn(t, surfaces, site, column)
	key, err := surfaces.registry.RelationPublicationKey(site, relation)
	if err != nil {
		t.Fatalf("signature key %v: %v", relation, err)
	}
	denominator, ok := model.NewDenominatorRef(relationID, key)
	if !ok {
		t.Fatalf("signature denominator %v", relation)
	}
	typeID, err := surfaces.registry.Type(site, relcompile.NewName(column.Entry, column.Member+"#type"))
	if err != nil {
		t.Fatalf("signature type %v: %v", column, err)
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("construct scalar delivery")
	}
	return signature.Input{Relation: relationID, Column: columnID, Type: typeID, Presence: signature.AllowMissing, Delivery: delivery, Denominator: denominator}
}

func mustColumn(t *testing.T, surfaces *owners, site relcompile.Site, name relcompile.Name) model.ColumnID {
	t.Helper()
	column, err := surfaces.registry.Column(site, name)
	if err != nil {
		t.Fatalf("column %v: %v", name, err)
	}
	return column
}

func onlyApply(expression algebra.Expression) (algebra.Apply, bool) {
	switch value := expression.(type) {
	case algebra.Apply:
		return value, true
	case algebra.Publish:
		return onlyApply(value.Child())
	case algebra.ColumnProject:
		return onlyApply(value.Child())
	case algebra.Select:
		return onlyApply(value.Child())
	case algebra.Complete:
		return onlyApply(value.Child())
	default:
		return algebra.Apply{}, false
	}
}
