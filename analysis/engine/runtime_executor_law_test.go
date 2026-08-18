// runtime_executor_law_test.go proves the executor postfix and demanded instantiation laws.

package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

func TestRegionPostfixCertificateRejectsExactRecomputeWithUnchangedInputs(t *testing.T) {
	runtime := &solverRuntime{
		regions:       []runtimeRegion{{active: true, head: 0}},
		activeRegions: []bool{true},
	}
	epoch := &executorEpoch{
		runtime:  runtime,
		versions: []uint64{7},
		regions:  []regionEpoch{{phase: phaseAscent, episode: 1, hasExact: true, exactInputsVersion: 3, exactRevision: 1}},
	}
	if !epoch.rememberRegionPostfix(0) || !epoch.regionPostfixProved(0) {
		t.Fatal("initial postfix certificate was not admitted")
	}
	if epoch.regions[0].exactInputsVersion != 3 {
		t.Fatal("test changed exact-input evidence before recompute")
	}
	if !epoch.regions[0].nextExactRevision() || epoch.regions[0].exactRevision != 2 {
		t.Fatal("exact revision did not advance")
	}
	if epoch.regionPostfixProved(0) {
		t.Fatal("stale postfix certificate survived exact recomputation")
	}

	epoch.regions[0].exactRevision = ^uint64(0)
	if epoch.regions[0].nextExactRevision() {
		t.Fatal("exact revision wrapped instead of failing closed")
	}
	if epoch.regionPostfixProved(0) {
		t.Fatal("overflow retained a usable postfix certificate")
	}
}

func TestReceiptRuntimeInstantiatesOnlyDemandedProducerGroups(t *testing.T) {
	schema, factor, rule, write, query := receiptExactQuerySchemaFixture(t)
	operands := [2]ruleUnit{ruleUnitForSemantic(coldKey(957_001)), ruleUnitForSemantic(coldKey(957_002))}
	runs := map[ruleUnit]int{}
	binding := NewSchemaBinding(schema)
	ruleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(948_032)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			operand, operandOK := Operand(access)
			if !operandOK {
				return false
			}
			runs[operand]++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, ruleSpec, testRuleProjector[ruleUnit]) ||
		!BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Seal() {
		t.Fatal("demanded producer binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	assembly, assemblyOK := beginReceiptAssembly(binding)
	if !implementationOK || !queryImplementationOK || !assemblyOK {
		t.Fatal("demanded producer implementations")
	}
	proof := implementation.receipt.proof
	scope := equation.EmptyScope()
	sites := make([]equation.Site, len(operands))
	occurrences := make([]equation.Occurrence, len(operands))
	operandRows := make([]equation.Operand, len(operands))
	pointRefs := make([]bindingPointRowRef, len(operands))
	for index, operandValue := range operands {
		site, siteOK := assembly.builder.admitSite(compositionKeyOf(coldKey(957_010+index)), scope, equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := assembly.builder.admitAt(site)
		entity, entityOK := operandEntityForContent(operandValue.content)
		operand, operandOK := assembly.builder.admitOperand(occurrence, entity)
		if !siteOK || !occurrenceOK || !entityOK || !operandOK {
			t.Fatal("demanded producer sources")
		}
		sites[index] = site
		occurrences[index], operandRows[index] = occurrence, operand
	}
	if !assembly.SealSources() {
		t.Fatal("demanded producer source seal")
	}
	for index, operandValue := range operands {
		point, pointOK := assembly.builder.issuePointRow(equation.PointSpec{Site: sites[index]})
		pointRef, pointSemanticOK := assembly.builder.addSemanticPoint(receiptAssemblySemanticID(byte(170+index)), point)
		source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrences[index], Operand: operandRows[index],
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := implementation.BeginBindingRuleRow(source)
		part, partOK := implementation.WritePart(source, 0)
		if !pointOK || !pointSemanticOK || !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
			t.Fatal("demanded producer row")
		}
		ruleRow, ruleRowOK := assembly.builder.issueRuleRow(draft)
		_, ruleSemanticOK := assembly.builder.addSemanticRule(receiptAssemblySemanticID(byte(180+index)), ruleRow)
		if !ruleRowOK || !ruleSemanticOK || operandValue != operands[index] {
			t.Fatal("demanded producer topology")
		}
		pointRefs[index] = pointRef
	}
	queryRow, queryRowOK := assembly.builder.issueQueryRow(queryImplementation, equation.QueryInstance{
		Family: schema.querySemanticAt(0), Point: pointRefs[0].ref,
		Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}},
	})
	_, querySemanticOK := assembly.builder.addSemanticQuery(receiptAssemblySemanticID(190), queryRow)
	if !queryRowOK || !querySemanticOK {
		t.Fatal("demanded producer query")
	}
	_, graph, committed := assembly.Commit()
	if !committed || graph == nil {
		t.Fatal("demanded producer commit")
	}
	compilation, compilationOK := BeginProgramConstruction(binding, graph)
	_, queryReceiptOK := graph.Query(receiptAssemblySemanticID(190))
	if !compilationOK || !queryReceiptOK {
		t.Fatal("demanded producer compilation")
	}
	groupIndexes := make([]int, len(operands))
	memberOperands := make(map[identity.ContentID]ruleUnit, len(operands))
	for index, operandValue := range operands {
		memberOperands[receiptAssemblySemanticID(byte(180+index))] = operandValue
	}
	if !installMemberOperandResolver(implementation, memberOperands) {
		t.Fatal("demanded producer resolver")
	}
	for index := range operands {
		if _, memberOK := graph.RuleMember(receiptAssemblySemanticID(byte(180 + index))); !memberOK {
			t.Fatal("demanded producer member")
		}
		if attached := AttachRuleMember(compilation, implementation, receiptAssemblySemanticID(byte(180+index))); !attached {
			t.Fatal("demanded producer attachment")
		}
		point, pointOK := graph.lookupPoint(receiptAssemblySemanticID(byte(170 + index)))
		group, groupOK := graph.graph.ProducerAt(point.point, 0)
		groupIndex, groupIndexed := graph.graph.GroupIndex(group)
		if !pointOK || !groupOK || !groupIndexed {
			t.Fatal("demanded producer group")
		}
		groupIndexes[index] = groupIndex
	}
	if !AttachExactQuery(compilation, queryImplementation, receiptAssemblySemanticID(190)) {
		t.Fatal("demanded producer query attachment")
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil || solver.runtime == nil {
		t.Fatal("demanded producer solver")
	}
	active := solver.runtime.producers[groupIndexes[0]]
	inactive := solver.runtime.producers[groupIndexes[1]]
	if active.span.count() != 1 || active.plan == (carrier.ContributionPlan{}) || inactive.span.count() != 1 || inactive.plan == (carrier.ContributionPlan{}) || !inactive.group.Key().Available() {
		t.Fatal("runtime did not retain the sealed producer descriptors")
	}
	// The undemanded Group stays inactive for this solve even though its complete
	// descriptor is already sealed for a later demand revision.
	inactiveSpan, inactiveSpanOK := solver.runtime.program.groupSpanAt(groupIndexes[1])
	if !inactiveSpanOK || inactiveSpan.count() != 1 {
		t.Fatal("the sealed program dropped the undemanded Group's member row")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil || runs[operands[0]] == 0 || runs[operands[1]] != 0 {
		t.Fatalf("producer runs = %d/%d status=%v", runs[operands[0]], runs[operands[1]], status)
	}
}
