package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func pendingSourceSpans(name string, counts [keyspace.FamilyCount]uint32) []source.FamilySpans {
	rows := make([]source.FamilySpans, 0, keyspace.FamilyCount-1)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		rows = append(rows, source.FamilySpans{Family: family, Spans: spans})
	}
	return rows
}

func TestSealPendingProductionRejectsDuplicateDirectSourceBeforeSeal(t *testing.T) {
	name := "pending-duplicate-direct.lua"
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyReturn: 1}
	returnTerm := pendingTerm(keyspace.FamilyReturn, 1)
	_, err := source.Build(source.Input{
		Name:     name,
		Families: pendingSourceSpans(name, counts),
		Bodies:   []source.BodySource{{Body: pendingTerm(keyspace.FamilyBody, 1), Terms: []keyspace.Term{returnTerm, returnTerm}}},
	})
	if err == nil {
		t.Fatal("Source accepted a duplicate direct root that would make SealPending's root order ambiguous")
	}
}

func TestSealPendingProductionRejectsCyclicAuthoredParentBeforeSeal(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyValues: 1,
		keyspace.FamilyUnary: 1, keyspace.FamilyReturn: 1,
	}
	body := pendingTerm(keyspace.FamilyBody, 1)
	unary := pendingTerm(keyspace.FamilyUnary, 1)
	values := pendingTerm(keyspace.FamilyValues, 1)
	draft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{unary},
		},
		Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: unary}}},
		Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	})
	if err != nil {
		t.Fatalf("authored.Build rejected the cycle before the discovery gate: %v", err)
	}
	finalize, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = finalize.Abort() })
	walker, err := New(finalize.View())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	builder := &pendingBuilder{view: finalize.View(), discover: true}
	for index, family := range pendingAncestorFamilyKeys {
		builder.parents[index] = make([]keyspace.Term, counts[family]+1)
	}
	for index, family := range pendingClaimFamilyKeys {
		builder.claimed[index] = make([]bool, counts[family]+1)
	}
	if err := discoverPendingParents(walker, builder, countsToInt(counts)); err == nil {
		t.Fatal("SealPending discovery accepted a cyclic authored parent")
	}
}

// TestProductionOwnerChainRejectsDuplicatePendingOccurrences proves the
// malformed cases at the earliest genuine owner boundary. A real SealPending
// call requires a containment/executable/candidate proof quartet; fabricating
// those proofs after this rejection would make the test dishonest.
func TestProductionOwnerChainRejectsDuplicatePendingOccurrences(t *testing.T) {
	for _, row := range []pendingDuplicateSpec{pendingDuplicateLeafSpec(), pendingDuplicateCompositeSpec()} {
		t.Run(row.name, func(t *testing.T) {
			assertProductionContainmentRejectsPendingDuplicate(t, row.name+".lua", row.counts, row.flow)
		})
	}
}

type pendingDuplicateSpec struct {
	name   string
	counts [keyspace.FamilyCount]uint32
	flow   authored.Input
}

func pendingDuplicateLeafSpec() pendingDuplicateSpec {
	term := pendingTerm
	bodyTerm := term(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyBool] = 1
	counts[keyspace.FamilyValues] = 1
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyBinary] = 1
	return pendingDuplicateSpec{
		name: "duplicate-leaf", counts: counts,
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: bodyTerm, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{term(keyspace.FamilyBinary, 1)}},
			Operators: authored.OperatorsInput{Binaries: []authored.Binary{{Owner: bodyTerm, Op: kind.BinaryAdd, Left: term(keyspace.FamilyBool, 1), Right: term(keyspace.FamilyBool, 1)}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: bodyTerm, Values: term(keyspace.FamilyValues, 1)}}},
		},
	}
}

func pendingDuplicateCompositeSpec() pendingDuplicateSpec {
	term := pendingTerm
	bodyTerm := term(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyBool] = 1
	counts[keyspace.FamilyValues] = 1
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyUnary] = 1
	return pendingDuplicateSpec{
		name: "duplicate-composite", counts: counts,
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: bodyTerm, Fixed: authored.Range{End: 2}}}, Terms: []keyspace.Term{term(keyspace.FamilyUnary, 1), term(keyspace.FamilyUnary, 1)}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: bodyTerm, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: bodyTerm, Values: term(keyspace.FamilyValues, 1)}}},
		},
	}
}

func assertProductionContainmentRejectsPendingDuplicate(t *testing.T, name string, counts [keyspace.FamilyCount]uint32, flowInput authored.Input) {
	t.Helper()
	bodyTerm := pendingTerm(keyspace.FamilyBody, 1)
	sourceDraft, err := source.Build(pendingSourceInput(name, counts,
		[][]keyspace.Term{{pendingTerm(keyspace.FamilyReturn, 1)}}, nil, nil, nil,
		pendingSourceExtras{boolOwners: []keyspace.Term{bodyTerm}}))
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = sourceFinalize.Abort() })

	staticCounts := [keyspace.FamilyCount]uint32{}
	staticCounts[keyspace.FamilyBody] = 1
	staticDraft, err := static.Build(static.Input{Counts: staticCounts})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = staticFinalize.Abort() })

	flowInput.Counts = counts
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		t.Fatalf("authored.Build rejected before the canonical containment owner: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = flowFinalize.Abort() })

	bodies, err := body.Seal(sourceFinalize.Preimage(), flowFinalize.View(), staticFinalize.View(), bodyTerm)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	bindings, err := binding.Seal(sourceFinalize.Preimage(), flowFinalize.View(), bodies, bodyTerm)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = moduleFinalize.Abort() })
	if _, _, err := containment.Prove(sourceFinalize.Preimage(), staticFinalize.View(), flowFinalize.View(), bodies, bindings, moduleFinalize.View(), bodyTerm); err == nil {
		t.Fatal("canonical containment owner accepted a duplicate Pending occurrence")
	}
}

func countsToInt(counts [keyspace.FamilyCount]uint32) [keyspace.FamilyCount]int {
	var result [keyspace.FamilyCount]int
	for family, count := range counts {
		result[family] = int(count)
	}
	return result
}
