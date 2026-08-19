package engine

import (
	"context"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

// memberRowBytes is the width the row model is justified by. A silently widened
// row would spend the retained-heap win the cut is taken for.
const memberRowBytes = 24

// TestMemberRowIsTheModelledWidth keeps the row at the width the value-model
// measurement assumed, and records the draft width the row replaces so the
// per-member delta is two measured numbers rather than an estimate.
func TestMemberRowIsTheModelledWidth(t *testing.T) {
	if size := unsafe.Sizeof(memberRow{}); size != memberRowBytes {
		t.Errorf("memberRow is %d bytes, the row model is justified at %d", size, memberRowBytes)
	}
	draft := unsafe.Sizeof(boundRuleMember[uint64, ruleUnit]{})
	if draft <= memberRowBytes {
		t.Errorf("rule draft is %d bytes, no narrower than the %d-byte row it is replaced by", draft, memberRowBytes)
	}
	t.Logf("rule draft %d bytes, activation draft %d bytes, row %d bytes, factor record %d bytes",
		draft, unsafe.Sizeof(boundActivationMember{}), unsafe.Sizeof(memberRow{}), unsafe.Sizeof(factorRecord{}))
}

// TestSolvedRuntimeProgramCoversEveryGraphMember is the row-by-row receipt on
// the live path: the program a real solve ran on covers every graph member
// exactly once, in canonical key order per Group, with each row's output slot
// equal to the sealed slot of the Factor its member writes.
func TestSolvedRuntimeProgramCoversEveryGraphMember(t *testing.T) {
	const width = 6
	order := benchIdentityOrder(width)
	fixture := newReceiptQueryMatrixFixture(t, width, order, order)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("receipt matrix solve = state:%t status:%v", state != nil, status)
	}
	for index, query := range fixture.queries {
		key, keyed := query.PublicationKey()
		if !keyed {
			t.Fatalf("query[%d] has no snapshot key", index)
		}
		value, readable := testSnapshotQueryValue[uint64](fixture.solver, state, key)
		if !readable || value != fixture.expected[index] {
			t.Fatalf("query[%d] over a row-driven solve = %d/%t, want %d/true", index, value, readable, fixture.expected[index])
		}
	}
	assembled := fixture.solver.runtime
	program := assembled.program
	if !program.valid() {
		t.Fatal("a solved runtime carries no sealed program")
	}
	graph := assembled.graph
	if program.groupCount() != graph.GroupCount() || program.groupCount() != len(assembled.producers) {
		t.Fatalf("program spans %d groups, the graph owns %d and the runtime assembled %d producers", program.groupCount(), graph.GroupCount(), len(assembled.producers))
	}
	covered := 0
	for index, producer := range assembled.producers {
		group, groupOK := graph.HyperedgeAt(index)
		span, spanOK := program.groupSpanAt(index)
		if !groupOK || !spanOK || span.count() != group.MemberCount() {
			t.Fatalf("group %d span = %v/%t over %d graph members", index, span, spanOK, group.MemberCount())
		}
		rows := program.memberRows(span)
		if len(rows) != span.count() {
			t.Fatalf("group %d exposes %d rows for a span of %d", index, len(rows), span.count())
		}
		seen := make(map[composition.Key]bool, len(rows))
		for offset, row := range rows {
			memberIdentity, identityOK := memberRowIdentity(group, row)
			if !identityOK || row.exec == nil {
				t.Fatalf("group %d row %d identity = %t exec = %t", index, offset, identityOK, row.exec != nil)
			}
			if seen[memberIdentity.Key()] {
				t.Fatalf("group %d row %d repeats member %v", index, offset, memberIdentity.Key())
			}
			seen[memberIdentity.Key()] = true
			if offset > 0 {
				previous, previousOK := memberRowIdentity(group, rows[offset-1])
				if !previousOK || !lessRuntimeKey(previous.Key(), memberIdentity.Key()) {
					t.Fatalf("group %d rows %d and %d are not in canonical member order", index, offset-1, offset)
				}
			}
			surface, surfaceOK := memberIdentity.WriteAt(0)
			if memberIdentity.WriteCount() == 0 {
				if row.hasSlot {
					t.Fatalf("group %d row %d claims an output slot for an output-free member", index, offset)
				}
				continue
			}
			record, recordOK := program.factorRecordByKey(surface.Factor)
			if !surfaceOK || !recordOK || !row.hasSlot || row.outputSlot != record.slot {
				t.Fatalf("group %d row %d slot = %d/%t, the Factor it writes is sealed at %d/%t", index, offset, row.outputSlot, row.hasSlot, record.slot, recordOK)
			}
			covered++
		}
		// Every dense Group retains its sealed producer descriptor, including a
		// Group outside the initial demand closure.
		if producer.plan == (carrier.ContributionPlan{}) || producer.span != span {
			t.Fatalf("group %d producer addresses %v, the program seals %v", index, producer.span, span)
		}
	}
	if covered != program.memberCount() {
		t.Fatalf("program table holds %d rows, %d were matched to a graph member write", program.memberCount(), covered)
	}
	for index := 0; index < program.factorCount(); index++ {
		record, recordOK := program.factorRecordAt(index)
		owner, ownerOK := program.factorOwnerAt(record.owner)
		slot, slotOK := shape.Slot(0), false
		if ownerOK && owner != nil {
			slot, slotOK = owner.runtimeSlot()
		}
		if !recordOK || !ownerOK || owner == nil || !slotOK || slot != record.slot || compositionKeyOf(owner.semantic()) != record.key {
			t.Fatalf("factor record %d key=%v slot=%d owner slot=%d/%t", index, record.key, record.slot, slot, slotOK)
		}
		resolved, found := program.factorOwnerByKey(record.key)
		if !found || resolved != owner {
			t.Fatalf("factor record %d does not resolve back to its owner through the sealed table", index)
		}
		if index > 0 {
			previous, _ := program.factorRecordAt(index - 1)
			if !lessRuntimeKey(previous.key, record.key) {
				t.Fatalf("factor records %d and %d are not in canonical key order: %v then %v", index-1, index, previous.key, record.key)
			}
		}
	}
	if program.queryCount() != len(assembled.queries) || program.observationCount() != len(assembled.observations) {
		t.Fatalf("program holds %d queries and %d observations, the runtime holds %d and %d", program.queryCount(), program.observationCount(), len(assembled.queries), len(assembled.observations))
	}
}

// TestProgramRowExecutionMatchesDraftExecution drives one member through both
// paths on the same carrier and compares the results. The transfer counter
// proves each path reached the bound rule rather than short-circuiting.
func TestProgramRowExecutionMatchesDraftExecution(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(958_120))
	writeForm, formOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(958_121), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(958_122)}, Output: factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !formOK || !ruleOK || !writeOK || !schemaOK {
		t.Fatal("program row execution schema")
	}
	operand := ruleUnitForSemantic(coldKey(958_123))
	transfers := 0
	hot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(958_122)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transfers++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(11)) })
		},
	}
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, hot, testRuleProjector[ruleUnit]) || !binding.Seal() {
		t.Fatal("program row execution binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	if !implementationOK || implementation == nil {
		t.Fatal("program row execution implementation")
	}
	graph, member := receiptRuleGraph(t, schema, implementation.binding.proof, operand.content)
	compilation, compiled := beginProgramConstruction(binding, graph)
	draft, draftOK := attachProgramRuleMember(compilation, implementation, member, operand)
	if !compiled || compilation == nil || !draftOK || draft == nil {
		t.Fatal("program row execution member draft")
	}
	exec := programMemberExec(draft)
	if exec == nil {
		t.Fatal("program row execution closure")
	}
	slot, slotOK := draft.outputSlot()
	plan, planOK := compilation.carrier.SealContribution(0, []shape.Slot{slot}, nil, false)
	work, workOK := compilation.carrier.NewWork()
	whole, wholeOK := support.True(compilation.runtime.guards)
	if !slotOK || !planOK || !workOK || !wholeOK {
		t.Fatal("program row execution carrier")
	}

	draftBase, draftBaseOK := work.BeginRuleContribution(plan, compilation.carrier.Scope(), nil, whole)
	draftResult := draft.execute(work, draftBase, nil, whole)
	draftContribution, draftFinished := work.FinishRuleContribution(draftBase, []carrier.Patch{draftResult.patch})

	rowBase, rowBaseOK := work.BeginRuleContribution(plan, compilation.carrier.Scope(), nil, whole)
	rowResult := exec(work, rowBase, nil, whole)
	rowContribution, rowFinished := work.FinishRuleContribution(rowBase, []carrier.Patch{rowResult.patch})

	if !draftBaseOK || !rowBaseOK || !draftFinished || !rowFinished || !draftContribution.Valid() || !rowContribution.Valid() {
		t.Fatalf("contributions = draft:%t/%t row:%t/%t", draftBaseOK, draftFinished, rowBaseOK, rowFinished)
	}
	if transfers != 2 {
		t.Fatalf("transfer ran %d times, want one run per path", transfers)
	}
	if draftResult.valid != rowResult.valid || draftResult.wrote != rowResult.wrote || draftResult.boundary != rowResult.boundary ||
		draftResult.hasSupport != rowResult.hasSupport || draftResult.retained.Valid() != rowResult.retained.Valid() ||
		len(draftResult.reads) != len(rowResult.reads) || len(draftResult.activations) != len(rowResult.activations) {
		t.Fatalf("row execution = %+v, draft execution = %+v", rowResult, draftResult)
	}
	if !rowResult.valid || !rowResult.wrote {
		t.Fatalf("row execution did not write through the bound rule: %+v", rowResult)
	}
}

// TestSealRuntimeProgramTakesOneValidityDecision proves the seal is the only
// admission: every inconsistent table set is refused whole, so no partially
// valid program can exist for a later predicate to repair.
func TestSealRuntimeProgramTakesOneValidityDecision(t *testing.T) {
	exec := memberExec(func(*carrier.Work, carrier.RuleContributionBase, []carrier.State, support.Mask) memberResult {
		return memberResult{}
	})
	rows := []memberRow{{exec: exec, memberIndex: 0}}
	spans := []memberSpan{{start: 0, end: 1}}
	if _, sealed := sealRuntimeProgram(rows, spans, nil, nil, nil, nil); !sealed {
		t.Fatal("seal refused a consistent one-row program")
	}
	if _, sealed := sealRuntimeProgram([]memberRow{{memberIndex: 0}}, spans, nil, nil, nil, nil); sealed {
		t.Fatal("seal admitted a row with no execution closure")
	}
	if _, sealed := sealRuntimeProgram([]memberRow{{exec: exec, memberIndex: 1}}, spans, nil, nil, nil, nil); sealed {
		t.Fatal("seal admitted a row positioned outside its own Group")
	}
	if _, sealed := sealRuntimeProgram(rows, []memberSpan{{start: 0, end: 2}}, nil, nil, nil, nil); sealed {
		t.Fatal("seal admitted a span reaching past the member table")
	}
	if _, sealed := sealRuntimeProgram(rows, []memberSpan{{start: 1, end: 1}}, nil, nil, nil, nil); sealed {
		t.Fatal("seal admitted a span set that does not cover the member table")
	}
	if _, sealed := sealRuntimeProgram(rows, spans, []factorRecord{{key: composition.Key{}, owner: 0}}, []runtimeFactor{nil}, nil, nil); sealed {
		t.Fatal("seal admitted a factor record with no owner")
	}
	var unsealed *runtimeProgram
	if unsealed.valid() || unsealed.memberCount() != 0 || unsealed.factorCount() != 0 {
		t.Fatal("an unsealed program reported itself valid")
	}
	if _, ok := unsealed.memberRowAt(0); ok {
		t.Fatal("an unsealed program answered a row read")
	}
}

// TestProgramRowsCarryNoDraft is the retained-shape receipt: a sealed row holds
// the execution closure and dense coordinates only, so the drafts stay
// collectable once the receipt path stops retaining them.
func TestProgramRowsCarryNoDraft(t *testing.T) {
	const width = 3
	order := benchIdentityOrder(width)
	fixture := newReceiptQueryMatrixFixture(t, width, order, order)
	if state, status := fixture.solver.Solve(context.Background()); state == nil || status != SolveComplete {
		t.Fatalf("receipt matrix solve = state:%t status:%v", state != nil, status)
	}
	program := fixture.solver.runtime.program
	if !program.valid() || program.memberCount() == 0 {
		t.Fatal("a solved runtime carries no sealed program")
	}
	if unsafe.Sizeof(program.memberTable[0]) != memberRowBytes {
		t.Fatalf("sealed row is %d bytes, the model is %d", unsafe.Sizeof(program.memberTable[0]), memberRowBytes)
	}
	for index := 0; index < program.memberCount(); index++ {
		row, ok := program.memberRowAt(index)
		if !ok || row.exec == nil {
			t.Fatalf("row %d = %t/%t", index, ok, row.exec != nil)
		}
		mutated := row
		mutated.outputSlot = shape.Slot(identity.Generation(0))
		if sealed, sealedOK := program.memberRowAt(index); !sealedOK || sealed.outputSlot != row.outputSlot {
			t.Fatalf("row %d changed through a returned copy", index)
		}
	}
}
