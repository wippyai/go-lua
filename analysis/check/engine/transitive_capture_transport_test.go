package engine

import (
	"encoding/json"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

func transportPartition(t *testing.T, facts ...equation.Fact) equation.Partition {
	t.Helper()
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: facts})
	if err != nil {
		t.Fatalf("building partition: %v", err)
	}
	return partition
}

func materializationEquation(name string, prototype string, captures ...string) equation.Equation {
	operands := []equation.Operand{
		{Role: "prototype", Term: equation.ClosedTerm([]byte("prototype/" + prototype))},
		{Role: "result", Term: equation.ClosedTerm([]byte("path/sym9"))},
	}
	for index, capture := range captures {
		operands = append(operands, equation.Operand{Role: equation.IndexedRole(equation.RoleFamilyCapture, index), Term: equation.ClosedTerm([]byte(capture))})
	}
	return equation.Equation{
		Target:     equation.Coordinate{Name: name},
		Occurrence: equation.Occurrence{Kind: "object-materialization"},
		Operands:   operands,
	}
}

func writeEquation(name, target string) equation.Equation {
	return equation.Equation{
		Target:     equation.Coordinate{Name: name},
		Occurrence: equation.Occurrence{Kind: "environment-write"},
		Operands:   []equation.Operand{{Role: "target", Term: equation.ClosedTerm([]byte(target))}},
	}
}

func capabilityPartition(t *testing.T, term string, handle closureHandle) equation.Partition {
	t.Helper()
	encoded, err := json.Marshal(handle)
	if err != nil {
		t.Fatalf("encoding handle: %v", err)
	}
	return transportPartition(t,
		equation.Fact{Key: "value/" + term + "/op-00000001", Value: []byte("scalar/function")},
		equation.Fact{Key: "closure/" + term + "/op-00000001", Value: encoded},
	)
}

// TestTransitiveCaptureTransportsSymbolOnlyDescendantsRead pins the transport a
// depth-2 dispatch depends on: the intermediate body constructs a closure over a
// symbol it never reads itself, so the symbol is bound nowhere in its entry and
// the descendant would resolve nothing without this row.
func TestTransitiveCaptureTransportsSymbolOnlyDescendantsRead(t *testing.T) {
	handle := closureHandle{Prototype: "chunk.helper"}
	lexical := &lexicalEvaluator{byPrototype: map[string]front.Compilation{"chunk.helper": {PrototypeName: "chunk.helper"}}}
	child := front.Compilation{PrototypeName: "chunk.middle", Artifact: equation.Artifact{Equations: []equation.Equation{
		materializationEquation("op-00000001", "chunk.inner", "path/sym2"),
	}}}
	seeds := l1Seeds()
	transported := lexical.transitiveCaptureCapabilities(child, seeds, nil, capabilityPartition(t, "path/sym2", handle))
	if len(transported) != 1 || transported[0].Term != "path/sym2" || transported[0].Handle.Prototype != "chunk.helper" {
		t.Fatalf("transported = %#v, want the capability the nested closure reads", transported)
	}
}

// TestTransitiveCaptureTransportsEnvironmentOfACrossingCapability pins the
// second shape: the body binds the callable it received but nothing that
// callable's own body reads, so the capability's captures travel with it.
func TestTransitiveCaptureTransportsEnvironmentOfACrossingCapability(t *testing.T) {
	handle := closureHandle{Prototype: "chunk.helper"}
	lexical := &lexicalEvaluator{byPrototype: map[string]front.Compilation{"chunk.helper": {PrototypeName: "chunk.helper"}}}
	child := front.Compilation{PrototypeName: "chunk.outer"}
	crossing := []entryClosureSeed{{Term: "path/sym5", Handle: closureHandle{Prototype: "chunk.forwarding", Captures: []string{"path/sym2"}}}}
	transported := lexical.transitiveCaptureCapabilities(child, l1Seeds(), crossing, capabilityPartition(t, "path/sym2", handle))
	if len(transported) != 1 || transported[0].Term != "path/sym2" {
		t.Fatalf("transported = %#v, want the crossing capability's own capture", transported)
	}
}

// TestTransitiveCaptureWithholdsBoundAndUncapableTerms keeps the transport
// fail-closed: a term the entry already binds is not restated, and a term
// holding no published capability leaves the descendant's dispatch opaque.
func TestTransitiveCaptureWithholdsBoundAndUncapableTerms(t *testing.T) {
	handle := closureHandle{Prototype: "chunk.helper"}
	lexical := &lexicalEvaluator{byPrototype: map[string]front.Compilation{"chunk.helper": {PrototypeName: "chunk.helper"}}}
	child := front.Compilation{PrototypeName: "chunk.middle", Artifact: equation.Artifact{Equations: []equation.Equation{
		materializationEquation("op-00000001", "chunk.inner", "path/sym2"),
	}}}
	bound := []entrySeed{{Term: "path/sym2", Value: []byte("scalar/function")}}
	if transported := lexical.transitiveCaptureCapabilities(child, bound, nil, capabilityPartition(t, "path/sym2", handle)); len(transported) != 0 {
		t.Fatalf("transported = %#v, want nothing for a term the entry already binds", transported)
	}
	valueOnly := transportPartition(t, equation.Fact{Key: "value/path/sym2/op-00000001", Value: []byte("scalar/function")})
	if transported := lexical.transitiveCaptureCapabilities(child, l1Seeds(), nil, valueOnly); len(transported) != 0 {
		t.Fatalf("transported = %#v, want nothing without a published capability", transported)
	}
	unadmitted := &lexicalEvaluator{byPrototype: map[string]front.Compilation{}}
	if transported := unadmitted.transitiveCaptureCapabilities(child, l1Seeds(), nil, capabilityPartition(t, "path/sym2", handle)); len(transported) != 0 {
		t.Fatalf("transported = %#v, want nothing for an unadmitted prototype", transported)
	}
}

// TestTransitiveCaptureWithholdsRebindableCaptures pins the stability the
// transported value rests on: a body this call can run writes the term, so the
// value carried at entry is the one held before that write.
func TestTransitiveCaptureWithholdsRebindableCaptures(t *testing.T) {
	handle := closureHandle{Prototype: "chunk.helper"}
	lexical := &lexicalEvaluator{byPrototype: map[string]front.Compilation{
		"chunk.helper": {PrototypeName: "chunk.helper"},
		"chunk.inner":  {PrototypeName: "chunk.inner", Artifact: equation.Artifact{Equations: []equation.Equation{writeEquation("op-00000001", "path/sym2")}}},
	}}
	child := front.Compilation{PrototypeName: "chunk.middle", Artifact: equation.Artifact{Equations: []equation.Equation{
		materializationEquation("op-00000001", "chunk.inner", "path/sym2"),
	}}}
	if transported := lexical.transitiveCaptureCapabilities(child, l1Seeds(), nil, capabilityPartition(t, "path/sym2", handle)); len(transported) != 0 {
		t.Fatalf("transported = %#v, want nothing for a term a reachable body rebinds", transported)
	}
}

// TestAppendNestedCaptureCapabilitiesKeepsEntrySeedsUnique pins the entry
// contract: a transported term joins the seed and capability lanes exactly
// once, and a term already seeded keeps its existing row.
func TestAppendNestedCaptureCapabilitiesKeepsEntrySeedsUnique(t *testing.T) {
	seeds := []entrySeed{{Term: "path/sym1", Value: []byte("scalar/top")}}
	nested := []nestedCaptureSeed{
		{Term: "path/sym1", Value: []byte("scalar/function"), Handle: closureHandle{Prototype: "chunk.a"}},
		{Term: "path/sym2", Value: []byte("scalar/function"), Handle: closureHandle{Prototype: "chunk.b"}},
		{Term: "path/sym2", Value: []byte("scalar/function"), Handle: closureHandle{Prototype: "chunk.b"}},
	}
	seeds, closures := appendNestedCaptureCapabilities(seeds, nil, nested)
	if len(seeds) != 2 || seeds[0].Term != "path/sym1" || string(seeds[0].Value) != "scalar/top" || seeds[1].Term != "path/sym2" {
		t.Fatalf("seeds = %#v, want the existing row kept and one new term", seeds)
	}
	if len(closures) != 1 || closures[0].Term != "path/sym2" {
		t.Fatalf("capabilities = %#v, want one row for the newly seeded term", closures)
	}
}

// TestChildCallDiagnosticRecognizesAlreadyTransportedSubjects pins the boundary
// rule for a diagnostic proven two bodies down: its transported spelling names
// the same subject, so it crosses the next boundary as well.
func TestChildCallDiagnosticRecognizesAlreadyTransportedSubjects(t *testing.T) {
	direct := equation.Fact{Key: "type.call.direct.argument_type/op-00000001/argument-00000000"}
	nested := equation.Fact{Key: "child/aabb/type.call.direct.argument_type/op-00000001/argument-00000000"}
	deeper := equation.Fact{Key: "child/ccdd/child/aabb/type.assignment/op-00000002"}
	for _, fact := range []equation.Fact{direct, nested, deeper} {
		if !childCallDiagnostic(fact) {
			t.Fatalf("%q must cross the call boundary", fact.Key)
		}
	}
	for _, fact := range []equation.Fact{{Key: "effect.freeze.mutation/op-00000001"}, {Key: "child/aabb/effect.freeze.mutation/op-00000001"}, {Key: "child/aabb"}} {
		if childCallDiagnostic(fact) {
			t.Fatalf("%q is not a call-boundary subject", fact.Key)
		}
	}
}

func l1Seeds() []entrySeed {
	return []entrySeed{{Term: "path/sym1", Value: []byte("scalar/top")}}
}

// TestExternalCallbackReceiverMayMutateReadsTheReceiversOwnCell pins the
// method-resolution rule the any-result rests on: an opaque callback holds the
// receiver's cell, so a member this call reads may have been installed there.
// The conclusion belongs to that cell, not to whichever boundary the call site
// resolved, so it holds with or without a provider term and never for a
// standard-library one.
func TestExternalCallbackReceiverMayMutateReadsTheReceiversOwnCell(t *testing.T) {
	identity := []byte("sealed-table/receiver/op-00000001")
	reachable := transportPartition(t,
		equation.Fact{Key: factkey.Epoch.Key().String() + "path/sym1/op-00000001", Value: []byte("op-00000001")},
		heapIdentityFact("path/sym1", "op-00000001", identity),
		heapExternalCallbackFact(identity, "op-00000002"),
	)
	if !externalCallbackReceiverMayMutate([]byte("path/sym1"), nil, reachable) {
		t.Fatalf("a receiver an opaque callback holds may be mutated without any provider term")
	}
	if !externalCallbackReceiverMayMutate([]byte("path/sym1"), []byte(`provider/global/"opaque_host"`), reachable) {
		t.Fatalf("an unresolved provider does not change the receiver's own reachability")
	}
	if externalCallbackReceiverMayMutate([]byte("path/sym1"), []byte(`provider/global/"string.format"`), reachable) {
		t.Fatalf("a standard-library provider owns its own result contract")
	}
	unreachable := transportPartition(t,
		equation.Fact{Key: factkey.Epoch.Key().String() + "path/sym1/op-00000001", Value: []byte("op-00000001")},
		heapIdentityFact("path/sym1", "op-00000001", identity),
	)
	if externalCallbackReceiverMayMutate([]byte("path/sym1"), nil, unreachable) {
		t.Fatalf("a receiver no callback holds keeps its proven member surface")
	}
}
