package operation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func testBinding(member string) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{member}}
}

func testOutcome(anchor string) OutcomeInput {
	return OutcomeInput{ValueSlots: 1, Anchor: []byte(anchor)}
}

func compileTestCore(t *testing.T, input Input) Core {
	t.Helper()
	keys := make([]keyspace.LiteralValue, 0)
	for _, operation := range input.Operations {
		for _, binding := range operation.Bindings {
			for _, segment := range append(append([]string{}, binding.Owner...), binding.Member...) {
				keys = append(keys, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment})
			}
		}
	}
	table, err := exactkey.Compile(keys)
	if err != nil {
		t.Fatalf("exactkey.Compile: %v", err)
	}
	geometry, err := CompileGeometry(input)
	if err != nil {
		t.Fatalf("CompileGeometry: %v", err)
	}
	core, err := CompileAnchors(geometry, table)
	if err != nil {
		t.Fatalf("CompileAnchors: %v", err)
	}
	return core
}

func TestCoreCanonicalSourceMappingIsIndependentOfInputOrder(t *testing.T) {
	root := OperationInput{
		Source:            0,
		Bindings:          []vocabulary.BindingSpec{testBinding("root")},
		OutcomeValueSlots: []OutcomeInput{testOutcome("root-outcome")},
		Produced:          []ProducedInput{{TargetSource: 1, Outcome: 0, Result: 0}},
	}
	child := OperationInput{Source: 1, OutcomeValueSlots: []OutcomeInput{testOutcome("child-outcome")}}

	first := compileTestCore(t, Input{Operations: []OperationInput{root, child}})
	second := compileTestCore(t, Input{Operations: []OperationInput{child, root}})
	if first.OperationCount() != 3 || second.OperationCount() != 3 {
		t.Fatalf("operation counts = %d/%d, want authored rows plus opaque", first.OperationCount(), second.OperationCount())
	}
	for source := 0; source < 2; source++ {
		left, leftOK := first.SourceOperation(source)
		right, rightOK := second.SourceOperation(source)
		if !leftOK || !rightOK || left != right {
			t.Fatalf("source %d mapping = %d/%v and %d/%v", source, left, leftOK, right, rightOK)
		}
	}
	rootAnchor, rootOK := first.Anchor(1)
	childAnchor, childOK := first.Anchor(2)
	opaqueAnchor, opaqueOK := first.Anchor(3)
	if !rootOK || !childOK || !opaqueOK || !rootAnchor.Available() || !childAnchor.Available() || !opaqueAnchor.Available() {
		t.Fatal("bound, produced, and opaque anchors must all be available")
	}
	for _, op := range []vocabulary.Operation{1, 2, 3} {
		left, leftOK := first.Anchor(op)
		right, rightOK := second.Anchor(op)
		if !leftOK || !rightOK || left != right {
			t.Fatalf("operation %d anchor differs across source order", op)
		}
	}
}

func TestCoreRejectsDuplicateBindings(t *testing.T) {
	binding := testBinding("duplicate")
	_, err := CompileGeometry(Input{Operations: []OperationInput{
		{Source: 0, Bindings: []vocabulary.BindingSpec{binding}, OutcomeValueSlots: []OutcomeInput{testOutcome("left")}},
		{Source: 1, Bindings: []vocabulary.BindingSpec{binding}, OutcomeValueSlots: []OutcomeInput{testOutcome("right")}},
	}})
	if err == nil {
		t.Fatal("duplicate binding ownership was accepted")
	}
}

func TestCoreRejectsMalformedProducedGeometry(t *testing.T) {
	tests := []struct {
		name string
		ops  []OperationInput
	}{
		{
			name: "missing parent",
			ops: []OperationInput{
				{Source: 0, Bindings: []vocabulary.BindingSpec{testBinding("root")}, OutcomeValueSlots: []OutcomeInput{testOutcome("root")}},
				{Source: 1, OutcomeValueSlots: []OutcomeInput{testOutcome("child")}},
			},
		},
		{
			name: "cycle",
			ops: []OperationInput{
				{Source: 0, OutcomeValueSlots: []OutcomeInput{testOutcome("left")}, Produced: []ProducedInput{{TargetSource: 1, Outcome: 0, Result: 0}}},
				{Source: 1, OutcomeValueSlots: []OutcomeInput{testOutcome("right")}, Produced: []ProducedInput{{TargetSource: 0, Outcome: 0, Result: 0}}},
			},
		},
		{
			name: "multiple parents",
			ops: []OperationInput{
				{Source: 0, Bindings: []vocabulary.BindingSpec{testBinding("left")}, OutcomeValueSlots: []OutcomeInput{testOutcome("left")}, Produced: []ProducedInput{{TargetSource: 2, Outcome: 0, Result: 0}}},
				{Source: 1, Bindings: []vocabulary.BindingSpec{testBinding("right")}, OutcomeValueSlots: []OutcomeInput{testOutcome("right")}, Produced: []ProducedInput{{TargetSource: 2, Outcome: 0, Result: 0}}},
				{Source: 2, OutcomeValueSlots: []OutcomeInput{testOutcome("child")}},
			},
		},
		{
			name: "duplicate step",
			ops: []OperationInput{
				{Source: 0, Bindings: []vocabulary.BindingSpec{testBinding("root")}, OutcomeValueSlots: []OutcomeInput{testOutcome("step")}, Produced: []ProducedInput{
					{TargetSource: 1, Outcome: 0, Result: 0}, {TargetSource: 1, Outcome: 0, Result: 0},
				}},
				{Source: 1, Bindings: []vocabulary.BindingSpec{testBinding("child")}, OutcomeValueSlots: []OutcomeInput{testOutcome("child")}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CompileGeometry(Input{Operations: test.ops}); err == nil {
				t.Fatal("malformed produced geometry was accepted")
			}
		})
	}
}

func TestCoreOwnsCallbackCoordinatesAndLifecycles(t *testing.T) {
	core := compileTestCore(t, Input{Operations: []OperationInput{{
		Source: 0, Bindings: []vocabulary.BindingSpec{testBinding("callbacks")}, OutcomeValueSlots: []OutcomeInput{testOutcome("outcome")},
		Callbacks: []CallbackInput{
			{Source: 1, Lifecycle: vocabulary.CallbackRetainedOptionalOnce},
			{Source: 0, Lifecycle: vocabulary.CallbackSyncRequiredMany},
		},
	}}})
	first, firstOK := core.CallbackAt(1, 0)
	second, secondOK := core.CallbackAt(1, 1)
	if !firstOK || !secondOK || first != 1 || second != 2 {
		t.Fatalf("callback handles = %d/%v and %d/%v", first, firstOK, second, secondOK)
	}
	source, sourceOK := core.CallbackSource(first)
	lifecycle, lifecycleOK := core.CallbackLifecycle(first)
	if !sourceOK || source != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}) || !lifecycleOK || lifecycle != vocabulary.CallbackRetainedOptionalOnce {
		t.Fatalf("first callback geometry = %#v/%v lifecycle=%d/%v", source, sourceOK, lifecycle, lifecycleOK)
	}
	owner, ownerOK := core.CallbackOwner(second)
	if !ownerOK || owner != 1 {
		t.Fatalf("second callback owner = %d/%v", owner, ownerOK)
	}
}

func TestCoreCopiesConstructionInputBeforePublishingRows(t *testing.T) {
	input := Input{Operations: []OperationInput{{
		Source:            0,
		Bindings:          []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"stable"}}},
		OutcomeValueSlots: []OutcomeInput{{ValueSlots: 1, Anchor: []byte("stable-selector")}},
	}}}
	keys, err := exactkey.Compile([]keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "stable"}})
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := CompileGeometry(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Operations[0].Bindings[0].Member[0] = "mutated"
	input.Operations[0].OutcomeValueSlots[0].Anchor[0] = 'x'
	core, err := CompileAnchors(geometry, keys)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := core.BindingAt(1, 0)
	if !ok || len(binding.Member) != 1 || binding.Member[0] != "stable" {
		t.Fatalf("published binding = %#v/%v", binding, ok)
	}
	if _, ok := core.Anchor(1); !ok {
		t.Fatal("published anchor unavailable after input mutation")
	}
}
