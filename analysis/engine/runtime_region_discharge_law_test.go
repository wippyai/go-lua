package engine

import (
	"context"
	"sort"
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
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
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
	sourceSite, sourceSiteOK := assembly.admitSite(compositionKeyOf(coldKey(951_120)), outerScope, equation.TrueExpr(), equation.InitPresent)
	loopSite, loopSiteOK := assembly.admitSite(compositionKeyOf(coldKey(951_121)), cycleScope, equation.TrueExpr(), equation.InitPresent)
	if !sourceSiteOK || !loopSiteOK {
		t.Fatal("cycle coordinate sites")
	}
	// source rule, loop back rule, loop ingress rule.
	rowSites := []equation.Site{sourceSite, loopSite, loopSite}
	occurrences := make([]equation.Occurrence, len(rowSites))
	operands := make([]equation.Operand, len(rowSites))
	operandValues := make([]ruleUnit, len(rowSites))
	for index, site := range rowSites {
		occurrence, occurrenceOK := assembly.admitAt(site)
		value := ruleUnitForSemantic(coldKey(951_130 + index))
		entity, entityOK := operandEntityForContent(value.content)
		operand, operandOK := assembly.admitOperand(occurrence, entity)
		if !occurrenceOK || !entityOK || !operandOK {
			t.Fatal("cycle coordinate operand")
		}
		occurrences[index], operandValues[index], operands[index] = occurrence, value, operand
	}
	if !assembly.SealSources() {
		t.Fatal("cycle coordinate source seal")
	}
	declaration := topologyDeclaration{binding: binding, batch: assembly.inner.batch}
	declaration.points = append(declaration.points,
		declaredPointRow{ID: receiptAssemblySemanticID(120), Site: sourceSite},
		declaredPointRow{ID: receiptAssemblySemanticID(121), Site: loopSite},
	)
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
	proof := implementation.binding.proof
	ruleIDs := make([]byte, len(rowSites))
	for index := range rowSites {
		source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrences[index], Operand: operands[index],
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := implementation.beginBindingRuleRow(source)
		part, partOK := implementation.WritePart(source, 0)
		if !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
			t.Fatal("cycle coordinate rule")
		}
		row, rowOK := assembly.issueRuleRow(draft)
		if !rowOK {
			t.Fatal("cycle coordinate group")
		}
		ruleID := byte(130 + index)
		member := declaredMemberRow{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(ruleID), Row: row.row}
		if index != 0 {
			if !boundaries[index].Available() {
				t.Fatalf("cycle coordinate boundary %d is not a valid transport", index)
			}
			member.EnvironmentInput = boundaries[index]
		}
		declaration.members = append(declaration.members, member)
		ruleIDs[index] = ruleID
	}
	declaration.queries = append(declaration.queries, declaredQueryRow{
		ID:  receiptAssemblySemanticID(140),
		Row: equation.QueryInstance{Family: schema.querySemanticAt(0), Point: equation.PointAt(1), Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}}},
	})
	constructed, refusal := constructTopology(declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("cycle coordinate commit stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	program := CommittedProgramFrom(constructed.topology, constructed.graph)
	if program == nil {
		t.Fatal("cycle coordinate committed program")
	}
	compilation, compilationOK := BeginProgramConstruction(binding, program)
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
	loop, loopLookupOK := program.lookupPoint(receiptAssemblySemanticID(121))
	if !loopLookupOK {
		t.Fatal("cycle coordinate loop point")
	}
	return cycleCoordinateFixture{solver: solver, decision: decision, shared: shared, dormant: dormant, loopPoint: loop}
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

// guardedCycleFixture is the cycle fixture that makes the support-axis widening
// observable at the head's own publication. Its recurrence head has two back
// producers premised on opposite valuations of the Region's own coordinate:
// one contributes only where d holds, the other only where d fails, and the two
// stage different values. Nothing outside the cycle establishes d, so the two
// contributions are exactly the guard cells the discharge is defined to join.
//
// The value lattice is bounded by construction: every producer stages a
// constant, so the head reaches its fixpoint in one step per guard cell. That
// keeps the fixture terminating whether or not the discharge runs, which is
// what makes a removed discharge a failed assertion rather than a hang.
type guardedCycleFixture struct {
	solver   *Solver
	queryKey identity.ContentID
	decision equation.Decision
	shared   equation.Decision
	head     equation.Point
}

// guardedCycleCellQuerySpec folds one value per observed guard cell of the head
// plane. The result is therefore the head's published partition: its length is
// how finely the head separates its guard support, and its entries are the
// values that separation retains.
func guardedCycleCellQuerySpec(semantic identity.SemanticKey) HotExactQuerySpec[uint64, []uint64] {
	clone := func(value []uint64) []uint64 { return append([]uint64(nil), value...) }
	return HotExactQuerySpec[uint64, []uint64]{
		Fold: QueryFold[OrderedCells[uint64], []uint64]{
			Begin: func() []uint64 { return nil },
			Accumulate: func(observed []uint64, cells OrderedCells[uint64]) ([]uint64, bool) {
				for index := 0; index < cells.Count(); index++ {
					value, present, ok := cells.At(index)
					if !ok {
						return nil, false
					}
					if present {
						observed = append(observed, value)
					}
				}
				sort.Slice(observed, func(left, right int) bool { return observed[left] < observed[right] })
				return observed, true
			},
		},
		Result: FrozenResult[[]uint64]{
			Semantic: semantic, Freeze: clone, Clone: clone,
			Equal: func(left, right []uint64) bool {
				if len(left) != len(right) {
					return false
				}
				for index := range left {
					if left[index] != right[index] {
						return false
					}
				}
				return true
			},
			Fingerprint: func(value []uint64) uint64 {
				var fingerprint uint64
				for index, item := range value {
					fingerprint ^= uint64(index+1)*0x9e3779b97f4a7c15 ^ item
				}
				return fingerprint
			},
			Present: func(value []uint64) bool { return true },
		},
	}
}

// guardedCycleStagedValues is the staged constant of each fixture producer, in
// row order: the source point, the back producer premised on d, the back
// producer premised on the negation of d, and the ingress into the head.
var guardedCycleStagedValues = []uint64{3, 5, 9, 1}

func newGuardedCycleFixture(t *testing.T) guardedCycleFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(952_100))
	writeForm, writeOK := factor.ExactWrite()
	readForm, readOK := factor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(952_101), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(952_102)}, Output: factor.Ref(),
	})
	write, writeRuleOK := SchemaWrite(rule, writeForm)
	query, queryOK := DeclareQuerySlot[[]uint64](builder, SchemaQuerySpec{Semantic: coldKey(952_103), Freezer: coldKey(952_104)})
	if !factorOK || !writeOK || !readOK || !ruleOK || !writeRuleOK || !queryOK || !SchemaQueryRead(query, readForm) {
		t.Fatal("guarded cycle schema")
	}
	schema, schemaOK := builder.Seal()
	if !schemaOK || schema == nil {
		t.Fatal("guarded cycle schema seal")
	}

	// Each producer is identified by its operand digest and stages its own
	// constant, so the two back producers publish different values under the
	// two valuations of the Region's own coordinate.
	staged := make(map[[32]byte]uint64, len(guardedCycleStagedValues))
	for index, value := range guardedCycleStagedValues {
		staged[ruleUnitForSemantic(coldKey(952_130+index)).content] = value
	}
	binding := NewSchemaBinding(schema)
	ruleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(952_102)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			operand, live := Operand(access)
			if !live {
				return false
			}
			value, known := staged[operand.content]
			if !known {
				return false
			}
			return Product(access, func(row Row) bool { return StageValue(access, row, value) })
		},
	}
	querySpec := guardedCycleCellQuerySpec(coldKey(952_104))
	factorSpec := hotUintFactorSpec()
	factorSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	if binding == nil || !BindFactor(binding, factor, factorSpec) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, ruleSpec, testRuleProjector[ruleUnit]) ||
		!BindExactQuery(binding, query, factor, querySpec) || !binding.Seal() {
		t.Fatal("guarded cycle binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, []uint64](binding, query)
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !implementationOK || !queryImplementationOK || !assemblyOK || assembly == nil {
		t.Fatal("guarded cycle assembly")
	}

	decision, decisionOK := equation.NewDecision(compositionKeyOf(coldKey(952_110)))
	shared, sharedOK := equation.NewDecision(compositionKeyOf(coldKey(952_111)))
	outerScope, outerScopeOK := equation.NewScope(shared)
	cycleScope, cycleScopeOK := equation.NewScope(shared, decision)
	if !decisionOK || !sharedOK || !outerScopeOK || !cycleScopeOK {
		t.Fatal("guarded cycle scope")
	}
	sourceSite, sourceSiteOK := assembly.admitSite(compositionKeyOf(coldKey(952_120)), outerScope, equation.TrueExpr(), equation.InitPresent)
	loopSite, loopSiteOK := assembly.admitSite(compositionKeyOf(coldKey(952_121)), cycleScope, equation.TrueExpr(), equation.InitPresent)
	if !sourceSiteOK || !loopSiteOK {
		t.Fatal("guarded cycle sites")
	}
	// source rule, back rule under d, back rule under not-d, loop ingress rule.
	rowSites := []equation.Site{sourceSite, loopSite, loopSite, loopSite}
	occurrences := make([]equation.Occurrence, len(rowSites))
	operands := make([]equation.Operand, len(rowSites))
	operandValues := make([]ruleUnit, len(rowSites))
	for index, site := range rowSites {
		occurrence, occurrenceOK := assembly.admitAt(site)
		value := ruleUnitForSemantic(coldKey(952_130 + index))
		entity, entityOK := operandEntityForContent(value.content)
		operand, operandOK := assembly.admitOperand(occurrence, entity)
		if !occurrenceOK || !entityOK || !operandOK {
			t.Fatal("guarded cycle operand")
		}
		occurrences[index], operandValues[index], operands[index] = occurrence, value, operand
	}
	if !assembly.SealSources() {
		t.Fatal("guarded cycle source seal")
	}
	declaration := topologyDeclaration{binding: binding, batch: assembly.inner.batch}
	declaration.points = append(declaration.points,
		declaredPointRow{ID: receiptAssemblySemanticID(220), Site: sourceSite},
		declaredPointRow{ID: receiptAssemblySemanticID(221), Site: loopSite},
	)
	ingressRelation, ingressRelationOK := equation.NewReindex(outerScope, cycleScope, []equation.DecisionMap{equation.Identity(shared)})
	backRelation, backRelationOK := equation.NewReindex(cycleScope, cycleScope, []equation.DecisionMap{
		equation.Identity(shared), equation.Identity(decision),
	})
	holds, holdsOK := equation.DecisionExpr(decision)
	fails, failsOK := equation.NotExpr(holds)
	if !ingressRelationOK || !backRelationOK || !holdsOK || !failsOK {
		t.Fatal("guarded cycle relations")
	}
	// The two self boundaries carry the same coordinates around the cycle and
	// differ only in the valuation of the cycle's own coordinate they admit.
	// Their guard cells are therefore genuinely separated at the head.
	boundaries := []equation.Input{
		{},
		equation.BoundaryInput(loopSite, loopSite, compositionKeyOf(coldKey(952_140)), equation.TrueExpr(), backRelation, holds),
		equation.BoundaryInput(loopSite, loopSite, compositionKeyOf(coldKey(952_141)), equation.TrueExpr(), backRelation, fails),
		equation.BoundaryInput(sourceSite, loopSite, compositionKeyOf(coldKey(952_142)), equation.TrueExpr(), ingressRelation, equation.TrueExpr()),
	}
	proof := implementation.binding.proof
	ruleIDs := make([]byte, len(rowSites))
	for index := range rowSites {
		source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrences[index], Operand: operands[index],
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := implementation.beginBindingRuleRow(source)
		part, partOK := implementation.WritePart(source, 0)
		if !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
			t.Fatal("guarded cycle rule")
		}
		row, rowOK := assembly.issueRuleRow(draft)
		if !rowOK {
			t.Fatal("guarded cycle group")
		}
		ruleID := byte(230 + index)
		member := declaredMemberRow{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(ruleID), Row: row.row}
		if index != 0 {
			if !boundaries[index].Available() {
				t.Fatalf("guarded cycle boundary %d is not a valid transport", index)
			}
			member.EnvironmentInput = boundaries[index]
		}
		declaration.members = append(declaration.members, member)
		ruleIDs[index] = ruleID
	}
	declaration.queries = append(declaration.queries, declaredQueryRow{
		ID:  receiptAssemblySemanticID(240),
		Row: equation.QueryInstance{Family: schema.querySemanticAt(0), Point: equation.PointAt(1), Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}}},
	})
	constructed, refusal := constructTopology(declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("guarded cycle commit stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	program := CommittedProgramFrom(constructed.topology, constructed.graph)
	if program == nil {
		t.Fatal("guarded cycle committed program")
	}
	compilation, compilationOK := BeginProgramConstruction(binding, program)
	if !compilationOK || compilation == nil {
		t.Fatal("guarded cycle compilation")
	}
	memberOperands := make(map[identity.ContentID]ruleUnit, len(ruleIDs))
	for index, ruleID := range ruleIDs {
		memberOperands[receiptAssemblySemanticID(ruleID)] = operandValues[index]
	}
	if !installMemberOperandResolver(implementation, memberOperands) {
		t.Fatal("guarded cycle resolver")
	}
	for _, ruleID := range ruleIDs {
		if attached := AttachRuleMember(compilation, implementation, receiptAssemblySemanticID(ruleID)); !attached {
			t.Fatal("guarded cycle member attachment")
		}
	}
	if !AttachExactQuery(compilation, queryImplementation, receiptAssemblySemanticID(240)) {
		t.Fatal("guarded cycle query attachment")
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil || solver.runtime == nil {
		t.Fatal("guarded cycle solver")
	}
	loop, loopLookupOK := program.lookupPoint(receiptAssemblySemanticID(221))
	published, publishedOK := program.Query(receiptAssemblySemanticID(240))
	queryKey, queryKeyOK := published.PublicationKey()
	if !loopLookupOK || !publishedOK || !queryKeyOK {
		t.Fatal("guarded cycle head")
	}
	return guardedCycleFixture{solver: solver, queryKey: queryKey, decision: decision, shared: shared, head: loop}
}

// guardedCycleHeadPlane solves the fixture and returns the head's published
// guard partition: one entry per cell the head separates its support into.
func guardedCycleHeadPlane(t *testing.T, fixture guardedCycleFixture) []uint64 {
	t.Helper()
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("guarded cycle solve = state:%t status:%v", state != nil, status)
	}
	plane, readable := testSnapshotQueryValue[[]uint64](fixture.solver, state, fixture.queryKey)
	if !readable {
		t.Fatal("guarded cycle head plane is not published")
	}
	return plane
}

// guardedCycleRegion returns the fixture's recurrence Region together with the
// evidence that its head really does separate its guard support: two back
// producers whose environment boundaries admit opposite valuations of the one
// coordinate the Region itself establishes. Without that separation the head
// has nothing to join and the laws below would hold vacuously.
func guardedCycleRegion(t *testing.T, fixture guardedCycleFixture) int {
	t.Helper()
	runtime := fixture.solver.runtime
	graph := runtime.graph
	found := -1
	for index := range runtime.regions {
		if !runtime.regions[index].active {
			continue
		}
		region, regionOK := graph.RegionAt(index)
		head, headOK := region.Head()
		if !regionOK || !headOK {
			t.Fatal("guarded cycle region")
		}
		if head != fixture.head {
			continue
		}
		if found >= 0 {
			t.Fatal("the guarded cycle head owns more than one Region")
		}
		found = index
		local, localOK := regionLocalDecisions(graph, region, head)
		if !localOK || len(local) != 1 || local[0] != fixture.decision {
			t.Fatalf("guarded cycle head derived %d region-local decisions, want exactly the cycle's own coordinate", len(local))
		}
		if region.BackHeadProducerCount() != 2 {
			t.Fatalf("guarded cycle head owns %d back producers, want the two that stage opposite valuations", region.BackHeadProducerCount())
		}
		premises := make([]equation.Expr, 0, 2)
		for producer := 0; producer < region.BackHeadProducerCount(); producer++ {
			group, groupOK := region.BackHeadProducerAt(producer)
			environment, present := group.EnvironmentInput()
			if !groupOK || !present || !environment.Available() {
				t.Fatalf("guarded cycle back producer %d carries no recurrence boundary", producer)
			}
			post := environment.Post()
			decisions := post.Decisions()
			if !post.Available() || post.IsTrue() || len(decisions) != 1 || decisions[0] != fixture.decision {
				t.Fatalf("guarded cycle back producer %d is not premised on the Region's own coordinate alone", producer)
			}
			premises = append(premises, post)
		}
		// Complementary premises are the exact separation proof: the two back
		// contributions never share a point of the head's support, and together
		// they cover all of it.
		overlap, overlapOK := equation.AndExpr(premises[0], premises[1])
		cover, coverOK := equation.OrExpr(premises[0], premises[1])
		if !overlapOK || !coverOK || !overlap.IsFalse() || !cover.IsTrue() {
			t.Fatal("the guarded cycle back producers do not partition the head's support, so the head separates no guard cell")
		}
	}
	if found < 0 {
		t.Fatal("the guarded cycle head owns no active Region")
	}
	return found
}

// TestGuardedCycleSolveCompletes keeps the guard-separating fixture honest
// against the executor. Every producer stages a constant, so the head settles
// whether or not the support-axis widening runs: a broken discharge shows up
// below as a wrong published partition, never as a fixture that fails to
// terminate.
func TestGuardedCycleSolveCompletes(t *testing.T) {
	fixture := newGuardedCycleFixture(t)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("guarded cycle solve = state:%t status:%v", state != nil, status)
	}
}

// TestRegionDischargeJoinsTheCycleOwnGuardCellsAtTheHead is the law for the
// ascent discharge call site. The derivation laws above prove which coordinate
// a Region may discharge; this one proves the sealed relation is actually
// applied at the one recurrence publication that widens.
//
// The fixture's head receives two back contributions that are separated by the
// Region's own coordinate d and carry different values. Discharging d joins
// them, so the head publishes one guard cell holding their join. A head that
// still distinguished d would publish both cells instead, which is exactly the
// partition growth the operator exists to bound.
func TestRegionDischargeJoinsTheCycleOwnGuardCellsAtTheHead(t *testing.T) {
	fixture := newGuardedCycleFixture(t)
	region := guardedCycleRegion(t, fixture)
	discharge := fixture.solver.runtime.regions[region].discharge
	if !discharge.available() || discharge.atoms != 1 {
		t.Fatalf("guarded cycle head sealed %d discharged coordinates with plan:%t, want the one cycle coordinate", discharge.atoms, discharge.plan.Valid())
	}
	plane := guardedCycleHeadPlane(t, fixture)
	joined := guardedCycleStagedValues[1]
	if guardedCycleStagedValues[2] > joined {
		joined = guardedCycleStagedValues[2]
	}
	if len(plane) != 1 || plane[0] != joined {
		t.Fatalf("guarded cycle head published %v, want one guard cell holding %d: the head is still partitioned by its own coordinate", plane, joined)
	}
}
