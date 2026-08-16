package engine

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

type receiptAssemblyRuleFixture struct {
	assembly       *ReceiptAssembly
	implementation *RuleImplementation[uint64, uint64, struct{}]
	site           equation.Site
	occurrence     equation.Occurrence
	operand        equation.Operand
}

func receiptAssemblySemanticID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func newReceiptAssemblyRuleFixture(t testing.TB) receiptAssemblyRuleFixture {
	t.Helper()
	schema, factor, rule, write := zeroWriteRuleSchema(t, 0)
	binding, implementation := sealedSchemaRuleImplementation(t, schema, factor, rule, write)
	assembly, ok := beginReceiptAssembly(binding)
	if !ok || assembly == nil || assembly.builder == nil {
		t.Fatal("receipt assembly")
	}
	site, siteOK := assembly.builder.admitSite(coldKey(949_001).compositionKey(), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.builder.admitAt(site)
	entity, entityOK := operandEntityForContent([32]byte{3})
	operand, operandOK := assembly.builder.admitOperand(occurrence, entity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK {
		t.Fatal("receipt assembly source rows")
	}
	return receiptAssemblyRuleFixture{assembly: assembly, implementation: implementation, site: site, occurrence: occurrence, operand: operand}
}

func (fixture receiptAssemblyRuleFixture) sourceSpec() equation.RuleSurfaceSourceSpec {
	proof := fixture.implementation.receipt.proof
	return equation.RuleSurfaceSourceSpec{
		Schema:        proof.semantic,
		OperandFamily: proof.operandFamily,
		Occurrence:    fixture.occurrence,
		Operand:       fixture.operand,
		Writes: []equation.ResolvedWrite{{
			Index: 0,
			Surface: equation.Surface{
				Factor: proof.output,
				Form:   equation.SurfaceWriteExact,
				Local:  1,
				Mode:   equation.TargetModeStrong,
			},
		}},
	}
}

func (fixture receiptAssemblyRuleFixture) addTopology(t testing.TB) {
	t.Helper()
	builder := fixture.assembly.builder
	point, pointOK := builder.issuePointRow(equation.PointSpec{Site: fixture.site})
	if !pointOK {
		t.Fatal("receipt assembly Point row")
	}
	if _, ok := builder.addSemanticPoint(receiptAssemblySemanticID(1), point); !ok {
		t.Fatal("receipt assembly semantic Point")
	}
	source, sourceOK := builder.issueRuleSurfaceSource(fixture.sourceSpec())
	draft, draftOK := fixture.implementation.BeginBindingRuleRow(source)
	write, writeOK := fixture.implementation.WritePart(source, 0)
	if !sourceOK || !draftOK || !writeOK || !draft.AddWrite(write) {
		t.Fatal("receipt assembly typed Rule row")
	}
	row, rowOK := builder.issueRuleRow(draft)
	if _, refOK := builder.addSemanticRule(receiptAssemblySemanticID(2), row); !rowOK || !refOK {
		t.Fatal("receipt assembly topology rows")
	}
}

func TestReceiptAssemblyThreePhaseAliasLifecycle(t *testing.T) {
	fixture := newReceiptAssemblyRuleFixture(t)
	alias := *fixture.assembly
	if _, ok := fixture.assembly.builder.issueRuleSurfaceSource(fixture.sourceSpec()); ok || fixture.assembly.builder.addPoint(equation.PointSpec{Site: fixture.site}) {
		t.Fatal("topology authoring entered before source seal")
	}
	if !alias.SealSources() || fixture.assembly.SealSources() {
		t.Fatal("copied assembly did not share one source-seal transition")
	}
	if _, ok := fixture.assembly.builder.admitSite(coldKey(949_002).compositionKey(), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent); ok {
		t.Fatal("source admission remained open during topology construction")
	}
	fixture.addTopology(t)
	topology, graph, committed := fixture.assembly.Commit()
	if !committed || topology == nil || graph == nil || !topology.valid() || !graph.valid() {
		t.Fatal("receipt assembly atomic topology/graph publication")
	}
	if _, _, ok := alias.Commit(); ok || alias.Abort() || alias.SealSources() {
		t.Fatal("terminal receipt assembly alias admitted another transition")
	}
}

func TestReceiptAssemblyFailureIsTerminalAndFailClosed(t *testing.T) {
	schema, factor := factorOnlySlotSchema(t, coldKey(949_010))
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !binding.Seal() {
		t.Fatal("receipt assembly failure binding")
	}
	empty, emptyOK := beginReceiptAssembly(binding)
	if !emptyOK || empty == nil || empty.SealSources() || empty.Abort() {
		t.Fatal("failed source seal was not terminal")
	}
	if _, _, ok := empty.Commit(); ok {
		t.Fatal("failed source seal recovered through commit")
	}

	fixture := newReceiptAssemblyRuleFixture(t)
	if topology, graph, ok := fixture.assembly.Commit(); ok || topology != nil || graph != nil {
		t.Fatal("commit admitted an unsealed source batch")
	}
	if fixture.assembly.SealSources() || fixture.assembly.Abort() {
		t.Fatal("early commit failure was repairable")
	}
	if _, ok := fixture.assembly.builder.admitAt(fixture.site); ok {
		t.Fatal("failed assembly retained source authoring authority")
	}

	fixture = newReceiptAssemblyRuleFixture(t)
	if !fixture.assembly.SealSources() {
		t.Fatal("failed topology commit setup")
	}
	point, pointOK := fixture.assembly.builder.issuePointRow(equation.PointSpec{Site: fixture.site})
	if !pointOK {
		t.Fatal("failed topology Point receipt")
	}
	if _, ok := fixture.assembly.builder.addSemanticPoint(receiptAssemblySemanticID(3), point); !ok {
		t.Fatal("failed topology semantic Point")
	}
	// The admitted Operand is deliberately left outside every Rule realm. The
	// topology must fail at Commit without publishing either half of the pair.
	if topology, graph, ok := fixture.assembly.Commit(); ok || topology != nil || graph != nil {
		t.Fatal("malformed topology published a receipt")
	}
	inner := fixture.assembly.builder.inner
	inner.mu.Lock()
	failedClosed := inner.phase == bindingTopologyBuilderAborted && inner.batch == nil && !inner.sourceKey.Available() && inner.spec.Batch == nil && inner.topology == nil && inner.receipt == nil && len(inner.factors) == 0 && inner.semantic == nil
	inner.mu.Unlock()
	if !failedClosed || fixture.assembly.Abort() || fixture.assembly.SealSources() {
		t.Fatal("failed topology commit was not terminal and unpublished")
	}
}

func TestReceiptAssemblySnapshotsAdmittedRowsBeforeCommit(t *testing.T) {
	fixture := newReceiptAssemblyRuleFixture(t)
	if !fixture.assembly.SealSources() {
		t.Fatal("snapshot source seal")
	}
	builder := fixture.assembly.builder
	point, pointOK := builder.issuePointRow(equation.PointSpec{Site: fixture.site})
	if !pointOK {
		t.Fatal("snapshot Point receipt")
	}
	if _, ok := builder.addSemanticPoint(receiptAssemblySemanticID(4), point); !ok {
		t.Fatal("snapshot semantic Point")
	}
	spec := fixture.sourceSpec()
	source, sourceOK := builder.issueRuleSurfaceSource(spec)
	// The source Batch owns its own immutable copy; caller mutation cannot
	// rewrite the later typed row.
	spec.Writes[0].Surface.Local = 2
	draft, draftOK := fixture.implementation.BeginBindingRuleRow(source)
	write, writeOK := fixture.implementation.WritePart(source, 0)
	if !sourceOK || !draftOK || !writeOK || !draft.AddWrite(write) {
		t.Fatal("snapshot Rule source")
	}
	row, rowOK := builder.issueRuleRow(draft)
	if _, refOK := builder.addSemanticRule(receiptAssemblySemanticID(5), row); !rowOK || !refOK {
		t.Fatal("snapshot Rule row")
	}
	// Admission clones the issued row into the spec, so mutating the receipt
	// copy afterwards cannot rewrite the committed topology.
	row.row.Writes[0].Surface.Local = 3
	topology, graph, ok := fixture.assembly.Commit()
	if !ok || topology == nil || graph == nil || !topology.valid() || !graph.valid() {
		t.Fatal("caller mutation changed committed topology snapshot")
	}
}

func TestReceiptAssemblyRejectsForeignEqualBindingSourcesAndRows(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleSchema(t, 0)
	firstBinding, firstImplementation := sealedSchemaRuleImplementation(t, schema, factor, rule, write)
	secondBinding, secondImplementation := sealedSchemaRuleImplementation(t, schema, factor, rule, write)
	firstAssembly, firstOK := beginReceiptAssembly(firstBinding)
	secondAssembly, secondOK := beginReceiptAssembly(secondBinding)
	if !firstOK || !secondOK {
		t.Fatal("foreign receipt assemblies")
	}
	admit := func(t testing.TB, assembly *ReceiptAssembly, semantic SemanticKey) (equation.Site, equation.Occurrence, equation.Operand) {
		t.Helper()
		site, siteOK := assembly.builder.admitSite(semantic.compositionKey(), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := assembly.builder.admitAt(site)
		entity, entityOK := operandEntityForContent([32]byte{3})
		operand, operandOK := assembly.builder.admitOperand(occurrence, entity)
		if !siteOK || !occurrenceOK || !entityOK || !operandOK || !assembly.SealSources() {
			t.Fatal("foreign receipt source")
		}
		return site, occurrence, operand
	}
	_, firstOccurrence, firstOperand := admit(t, firstAssembly, coldKey(949_020))
	_, secondOccurrence, secondOperand := admit(t, secondAssembly, coldKey(949_020))
	proof := firstImplementation.receipt.proof
	writeSurface := []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}
	foreignSpec := equation.RuleSurfaceSourceSpec{Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: secondOccurrence, Operand: secondOperand, Writes: writeSurface}
	if _, ok := firstAssembly.builder.issueRuleSurfaceSource(foreignSpec); ok {
		t.Fatal("foreign equal-Binding source identities crossed Batch fence")
	}
	firstSpec := foreignSpec
	firstSpec.Occurrence, firstSpec.Operand = firstOccurrence, firstOperand
	source, sourceOK := firstAssembly.builder.issueRuleSurfaceSource(firstSpec)
	foreignDraft, draftOK := secondImplementation.BeginBindingRuleRow(source)
	foreignWrite, writeOK := secondImplementation.WritePart(source, 0)
	if !sourceOK || !draftOK || !writeOK || !foreignDraft.AddWrite(foreignWrite) {
		t.Fatal("foreign row setup")
	}
	if _, ok := firstAssembly.builder.issueRuleRow(foreignDraft); ok {
		t.Fatal("foreign equal-Binding Rule authority crossed row fence")
	}
	if !firstAssembly.Abort() || !secondAssembly.Abort() {
		t.Fatal("foreign receipt assembly abort")
	}
}

func TestReceiptAssemblyConcurrentTransitionsHaveOneWinner(t *testing.T) {
	fixture := newReceiptAssemblyRuleFixture(t)
	aliases := make([]ReceiptAssembly, 16)
	for index := range aliases {
		aliases[index] = *fixture.assembly
	}
	start := make(chan struct{})
	results := make(chan bool, len(aliases))
	var wait sync.WaitGroup
	for index := range aliases {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results <- aliases[index].SealSources()
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)
	winners := 0
	for result := range results {
		if result {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("source seal winners = %d, want 1", winners)
	}
	fixture.addTopology(t)

	commitResult := make(chan bool, 1)
	abortResult := make(chan bool, 1)
	start = make(chan struct{})
	go func() {
		<-start
		_, _, ok := aliases[0].Commit()
		commitResult <- ok
	}()
	go func() {
		<-start
		abortResult <- aliases[1].Abort()
	}()
	close(start)
	committed, aborted := <-commitResult, <-abortResult
	if committed == aborted {
		t.Fatalf("commit=%t abort=%t, want one terminal winner", committed, aborted)
	}
	if _, _, ok := fixture.assembly.Commit(); ok || fixture.assembly.Abort() || fixture.assembly.SealSources() {
		t.Fatal("concurrent terminal winner did not close every alias")
	}
}
