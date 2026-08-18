package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// cycleCoordinateFixture is the smallest program that separates the two halves
// of the support-axis derivation. Its loop point carries two coordinates: one
// the ingress from outside also establishes, and one only the loop's own self
// boundary establishes. Both reach the head around the cycle, so a derivation
// that reads only the recurrence half would name both, and a derivation that
// reads only the head's scope would name both as well.
type cycleCoordinateFixture struct {
	solver    *Solver
	decision  equation.Decision
	shared    equation.Decision
	dormant   equation.Decision
	loopPoint equation.Point
}

func newCycleCoordinateFixture(t *testing.T) cycleCoordinateFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(951_100))
	writeForm, writeOK := factor.ExactWrite()
	readForm, readOK := factor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(951_101), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(951_102)}, Output: factor.Ref(),
	})
	write, writeRuleOK := SchemaWrite(rule, writeForm)
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(951_103), Freezer: coldKey(951_104)})
	if !factorOK || !writeOK || !readOK || !ruleOK || !writeRuleOK || !queryOK || !SchemaQueryRead(query, readForm) {
		t.Fatal("cycle coordinate schema")
	}
	schema, schemaOK := builder.Seal()
	if !schemaOK || schema == nil {
		t.Fatal("cycle coordinate schema seal")
	}
	binding := NewSchemaBinding(schema)
	ruleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(951_102)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			if _, live := Operand(access); !live {
				return false
			}
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}
	querySpec := hotExactQuerySpec()
	querySpec.Result.Semantic = coldKey(951_104)
	factorSpec := hotUintFactorSpec()
	factorSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	if binding == nil || !BindFactor(binding, factor, factorSpec) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, ruleSpec, testRuleProjector[ruleUnit]) ||
		!BindExactQuery(binding, query, factor, querySpec) || !binding.Seal() {
		t.Fatal("cycle coordinate binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	assembly, assemblyOK := beginReceiptAssembly(binding)
	if !implementationOK || !queryImplementationOK || !assemblyOK || assembly == nil {
		t.Fatal("cycle coordinate assembly")
	}

	decision, decisionOK := equation.NewDecision(compositionKeyOf(coldKey(951_110)))
	shared, sharedOK := equation.NewDecision(compositionKeyOf(coldKey(951_111)))
	dormant, dormantOK := equation.NewDecision(compositionKeyOf(coldKey(951_112)))
	outerScope, outerScopeOK := equation.NewScope(shared)
	cycleScope, cycleScopeOK := equation.NewScope(shared, decision, dormant)
	if !decisionOK || !sharedOK || !dormantOK || !outerScopeOK || !cycleScopeOK {
		t.Fatal("cycle coordinate scope")
	}
	sourceSite, sourceSiteOK := assembly.builder.admitSite(compositionKeyOf(coldKey(951_120)), outerScope, equation.TrueExpr(), equation.InitPresent)
	loopSite, loopSiteOK := assembly.builder.admitSite(compositionKeyOf(coldKey(951_121)), cycleScope, equation.TrueExpr(), equation.InitPresent)
	if !sourceSiteOK || !loopSiteOK {
		t.Fatal("cycle coordinate sites")
	}
	// source rule, loop back rule, loop ingress rule.
	rowSites := []equation.Site{sourceSite, loopSite, loopSite}
	occurrences := make([]equation.Occurrence, len(rowSites))
	operands := make([]equation.Operand, len(rowSites))
	operandValues := make([]ruleUnit, len(rowSites))
	for index, site := range rowSites {
		occurrence, occurrenceOK := assembly.builder.admitAt(site)
		value := ruleUnitForSemantic(coldKey(951_130 + index))
		entity, entityOK := operandEntityForContent(value.content)
		operand, operandOK := assembly.builder.admitOperand(occurrence, entity)
		if !occurrenceOK || !entityOK || !operandOK {
			t.Fatal("cycle coordinate operand")
		}
		occurrences[index], operandValues[index], operands[index] = occurrence, value, operand
	}
	if !assembly.SealSources() {
		t.Fatal("cycle coordinate source seal")
	}
	sourcePointRef, sourcePointOK := assembly.builder.issuePointRow(equation.PointSpec{Site: sourceSite})
	sourceRef, sourceSemanticOK := assembly.builder.addSemanticPoint(receiptAssemblySemanticID(120), sourcePointRef)
	loopPointRef, loopPointOK := assembly.builder.issuePointRow(equation.PointSpec{Site: loopSite})
	loopRef, loopSemanticOK := assembly.builder.addSemanticPoint(receiptAssemblySemanticID(121), loopPointRef)
	if !sourcePointOK || !sourceSemanticOK || !loopPointOK || !loopSemanticOK {
		t.Fatal("cycle coordinate points")
	}
	// The ingress relation carries the shared coordinate identically into the
	// loop's scope and establishes nothing at the cycle's own decision. Both
	// coordinates therefore reach the head around the cycle, and exactly one of
	// them is also established from outside it: that difference is the whole
	// derivation the support-axis widening rests on.
	ingressRelation, ingressRelationOK := equation.NewReindex(outerScope, cycleScope, []equation.DecisionMap{equation.Identity(shared)})
	// The self boundary carries the shared and cycle-owned coordinates and
	// forgets the third, so the head's scope also holds a coordinate neither
	// side of the cycle establishes. A derivation that read only the head's
	// scope would name that one too.
	backRelation, backRelationOK := equation.NewReindex(cycleScope, cycleScope, []equation.DecisionMap{
		equation.Identity(shared), equation.Identity(decision), equation.Forget(dormant),
	})
	if !ingressRelationOK || !backRelationOK {
		t.Fatal("cycle coordinate ingress relation")
	}
	boundaries := []equation.Input{
		{},
		equation.BoundaryInput(loopSite, loopSite, compositionKeyOf(coldKey(951_140)), equation.TrueExpr(), backRelation, equation.TrueExpr()),
		equation.BoundaryInput(sourceSite, loopSite, compositionKeyOf(coldKey(951_141)), equation.TrueExpr(), ingressRelation, equation.TrueExpr()),
	}
	outputs := []bindingPointRowRef{sourceRef, loopRef, loopRef}
	proof := implementation.receipt.proof
	ruleIDs := make([]byte, len(rowSites))
	for index := range rowSites {
		source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrences[index], Operand: operands[index],
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := implementation.BeginBindingRuleRow(source)
		part, partOK := implementation.WritePart(source, 0)
		if !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
			t.Fatal("cycle coordinate rule")
		}
		row, rowOK := assembly.builder.issueRuleRow(draft)
		ruleID := byte(130 + index)
		_, semanticOK := assembly.builder.addSemanticRule(receiptAssemblySemanticID(ruleID), row)
		if !rowOK || !semanticOK || len(assembly.builder.inner.spec.Groups) == 0 {
			t.Fatal("cycle coordinate group")
		}
		last := len(assembly.builder.inner.spec.Groups) - 1
		assembly.builder.inner.spec.Groups[last].Output = outputs[index].ref
		if index != 0 {
			if !boundaries[index].Available() {
				t.Fatalf("cycle coordinate boundary %d is not a valid transport", index)
			}
			assembly.builder.inner.spec.Groups[last].EnvironmentInput = boundaries[index]
		}
		ruleIDs[index] = ruleID
	}
	queryRow, queryRowOK := assembly.builder.issueQueryRow(queryImplementation, equation.QueryInstance{Family: schema.querySemanticAt(0), Point: loopRef.ref, Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}}})
	_, querySemanticOK := assembly.builder.addSemanticQuery(receiptAssemblySemanticID(140), queryRow)
	if !queryRowOK || !querySemanticOK {
		t.Fatal("cycle coordinate query row")
	}
	_, graph, committed := assembly.Commit()
	if !committed || graph == nil {
		t.Fatal("cycle coordinate commit")
	}
	compilation, compilationOK := BeginProgramConstruction(binding, graph)
	if !compilationOK || compilation == nil {
		t.Fatal("cycle coordinate compilation")
	}
	memberOperands := make(map[identity.ContentID]ruleUnit, len(ruleIDs))
	for index, ruleID := range ruleIDs {
		memberOperands[receiptAssemblySemanticID(ruleID)] = operandValues[index]
	}
	if !installMemberOperandResolver(implementation, memberOperands) {
		t.Fatal("cycle coordinate resolver")
	}
	for _, ruleID := range ruleIDs {
		if attached := AttachRuleMember(compilation, implementation, receiptAssemblySemanticID(ruleID)); !attached {
			t.Fatal("cycle coordinate member attachment")
		}
	}
	if !AttachExactQuery(compilation, queryImplementation, receiptAssemblySemanticID(140)) {
		t.Fatal("cycle coordinate query attachment")
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil || solver.runtime == nil {
		t.Fatal("cycle coordinate solver")
	}
	loop, loopLookupOK := graph.lookupPoint(receiptAssemblySemanticID(121))
	if !loopLookupOK {
		t.Fatal("cycle coordinate loop point")
	}
	return cycleCoordinateFixture{solver: solver, decision: decision, shared: shared, dormant: dormant, loopPoint: loop.point}
}

// TestRegionDischargeForgetsOnlyTheCycleOwnCoordinates is the derivation law
// for the support-axis widening. A Region seals a discharge relation exactly
// when its own recurrence establishes a coordinate at its head that nothing
// outside the cycle establishes there, and that relation names precisely those
// coordinates.
func TestRegionDischargeForgetsOnlyTheCycleOwnCoordinates(t *testing.T) {
	fixture := newCycleCoordinateFixture(t)
	runtime := fixture.solver.runtime
	graph := runtime.graph
	if graph.RegionCount() == 0 {
		t.Fatal("cycle coordinate program owns no recurrence Region")
	}
	discharged := 0
	for index := range runtime.regions {
		if !runtime.regions[index].active {
			continue
		}
		region, regionOK := graph.RegionAt(index)
		head, headOK := region.Head()
		if !regionOK || !headOK {
			t.Fatal("cycle coordinate region")
		}
		local, localOK := regionLocalDecisions(graph, region, head)
		if !localOK {
			t.Fatal("cycle coordinate region-local derivation")
		}
		if runtime.regions[index].discharge.atoms != len(local) {
			t.Fatalf("region %d sealed %d discharged coordinates for %d region-local decisions", index, runtime.regions[index].discharge.atoms, len(local))
		}
		if head != fixture.loopPoint {
			if len(local) != 0 {
				t.Fatalf("region %d outside the cycle claims %d region-local decisions", index, len(local))
			}
			continue
		}
		if len(local) != 1 || local[0] != fixture.decision {
			t.Fatalf("cycle head derived %d region-local decisions, want exactly the cycle's own coordinate", len(local))
		}
		if !runtime.regions[index].discharge.available() {
			t.Fatal("cycle head sealed no support-axis widening relation for its own coordinate")
		}
		discharged++
	}
	if discharged != 1 {
		t.Fatalf("%d Regions sealed a support-axis widening, want the one cycle head", discharged)
	}
}

// TestRegionDischargeRetainsCoordinatesTheOutsideEstablishes proves the other
// half of the derivation: once the ingress into the head establishes the same
// coordinate, it is no longer the cycle's own and the Region seals nothing.
// This is what keeps the operator from coarsening a distinction that entered
// the Region from outside its recurrence.
func TestRegionDischargeRetainsCoordinatesTheOutsideEstablishes(t *testing.T) {
	fixture := newCycleCoordinateFixture(t)
	runtime := fixture.solver.runtime
	graph := runtime.graph
	for index := range runtime.regions {
		if !runtime.regions[index].active {
			continue
		}
		region, regionOK := graph.RegionAt(index)
		head, headOK := region.Head()
		if !regionOK || !headOK || head != fixture.loopPoint {
			continue
		}
		recurrence, outside, ingressOK := regionHeadIngress(graph, region, head)
		if !ingressOK {
			t.Fatal("cycle head ingress")
		}
		if !recurrence.has(fixture.decision) || !recurrence.has(fixture.shared) {
			t.Fatal("cycle head recurrence does not establish both coordinates that reach it around the cycle")
		}
		if recurrence.has(fixture.dormant) || outside.has(fixture.dormant) {
			t.Fatal("the forgotten coordinate is established at the cycle head")
		}
		if outside.has(fixture.decision) {
			t.Fatal("ingress from outside the cycle establishes the cycle's own coordinate")
		}
		if !outside.has(fixture.shared) {
			t.Fatal("ingress from outside the cycle does not establish the shared coordinate")
		}
		local, localOK := regionLocalDecisions(graph, region, head)
		if !localOK || len(local) != 1 || local[0] != fixture.decision {
			t.Fatalf("cycle head derived %d region-local decisions from a scope that also carries a coordinate the outside establishes", len(local))
		}
		return
	}
	t.Fatal("cycle head Region is not active")
}

// TestCycleCoordinateSolveCompletes keeps the derivation honest against the
// executor: the same program the laws above read is one the solver runs to a
// complete answer with the support-axis widening applied at its head.
func TestCycleCoordinateSolveCompletes(t *testing.T) {
	fixture := newCycleCoordinateFixture(t)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("cycle coordinate solve = state:%t status:%v", state != nil, status)
	}
}
