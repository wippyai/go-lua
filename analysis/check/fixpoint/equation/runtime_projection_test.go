package equation

import (
	"bytes"
	"testing"
)

func TestRuntimeProjectionIsArtifactBoundAndConservative(t *testing.T) {
	artifact, _, _ := stage3Artifact(t)
	compiled, err := CompileArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	p := compiled.RuntimeProjection()
	if p.ArtifactID() != compiled.ContentID() || p.OperationCount() != len(compiled.Operations()) || p.OperandCount() != len(compiled.Operands()) {
		t.Fatalf("projection identity/counts = id %x ops %d operands %d", p.ArtifactID(), p.OperationCount(), p.OperandCount())
	}
	for index := 0; index < p.OperandCount(); index++ {
		operand, ok := p.OperandAt(index)
		if !ok {
			t.Fatalf("operand %d missing", index)
		}
		if operand.Status != FactProven || operand.Provenance.ArtifactID != compiled.ContentID() || operand.Provenance.ContractID == (ContentID{}) || operand.Shape.Status != FactUnknown {
			t.Fatalf("operand %d facts = %#v", index, operand)
		}
		if operand.Kind == OperandEntryProjection && operand.Storage != RuntimeStorageEntryProjection {
			t.Fatalf("entry operand storage = %d", operand.Storage)
		}
		if operand.Kind == OperandCanonicalConstant && operand.Storage != RuntimeStorageCanonicalArena {
			t.Fatalf("constant operand storage = %d", operand.Storage)
		}
	}
	for index := 0; index < p.OperationCount(); index++ {
		op, _ := p.OperationAt(index)
		if op.Effects.Allocation.Status != FactUnknown || op.Placement.Status != FactUnknown || op.AllocationTemplate.Status != FactUnknown || op.Boundaries.Suspends.Status != FactUnknown || op.Boundaries.InLoop.Status != FactProven || op.Boundaries.InLoop.Value {
			t.Fatalf("operation %d unexpectedly optimistic: %#v", index, op)
		}
		admission := p.JITAdmissionAt(index)
		if !admission.InterpreterSafe || !admission.DeoptOnGuardFailure || admission.StackPlacement || admission.AllocationSinking || admission.Inlining || admission.BoundsSpecialize || admission.TypeSpecialize || admission.RegisterResidency {
			t.Fatalf("operation %d JIT admission = %#v", index, admission)
		}
	}
	if got := p.JITAdmissionAt(-1); !got.InterpreterSafe || !got.DeoptOnGuardFailure {
		t.Fatalf("invalid admission = %#v", got)
	}
}

func TestRuntimeProjectionCodecRoundTripAndRejectsMutation(t *testing.T) {
	artifact, _, _ := stage3Artifact(t)
	compiled, err := CompileArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	encoded := compiled.RuntimeProjection().CanonicalBytes()
	decoded, err := DecodeRuntimeProjection(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Admitted() {
		t.Fatal("wire decode became artifact admission")
	}
	if decoded.ContentID() != compiled.RuntimeProjection().ContentID() || !bytes.Equal(decoded.CanonicalBytes(), encoded) {
		t.Fatal("runtime projection codec changed canonical content")
	}
	admitted, err := AdmitRuntimeProjectionEncoding(compiled, encoded)
	if err != nil || !admitted.Admitted() || admitted.ContentID() != decoded.ContentID() {
		t.Fatalf("codec admission = %#v, %v", admitted, err)
	}
	encoded[0] ^= 0xff
	if bytes.Equal(encoded, compiled.RuntimeProjection().CanonicalBytes()) {
		t.Fatal("canonical bytes accessor leaked retained storage")
	}
	if _, err := DecodeRuntimeProjection(encoded); err == nil {
		t.Fatal("mutated projection was admitted")
	}
}

func TestRuntimePlacementWireOrdinalsFrozen(t *testing.T) {
	escapes := []RuntimeEscape{
		RuntimeEscapeUnknown,
		RuntimeEscapeNone,
		RuntimeEscapeReturn,
		RuntimeEscapeStore,
		RuntimeEscapeShare,
	}
	for wire, value := range escapes {
		gotWire, ok := runtimeEscapeWire(value)
		if !ok || gotWire != byte(wire) {
			t.Fatalf("runtime escape %s wire = %d/%t, want %d", value.Name(), gotWire, ok, wire)
		}
		gotValue, ok := runtimeEscapeFromWire(byte(wire))
		if !ok || gotValue != value {
			t.Fatalf("runtime escape wire %d = %s/%t, want %s", wire, gotValue.Name(), ok, value.Name())
		}
	}

	placements := []RuntimePlacement{
		RuntimePlacementUnknown,
		RuntimePlacementInterpreter,
		RuntimePlacementStack,
		RuntimePlacementRegister,
		RuntimePlacementOwnedHeap,
		RuntimePlacementSharedHeap,
	}
	for wire, value := range placements {
		gotWire, ok := runtimePlacementWire(value)
		if !ok || gotWire != byte(wire) {
			t.Fatalf("runtime placement %s wire = %d/%t, want %d", value, gotWire, ok, wire)
		}
		gotValue, ok := runtimePlacementFromWire(byte(wire))
		if !ok || gotValue != value {
			t.Fatalf("runtime placement wire %d = %s/%t, want %s", wire, gotValue, ok, value)
		}
	}
}

func TestCyclicRuntimeProjectionClearsAcyclicLoopFacts(t *testing.T) {
	artifact, _, _ := cyclicVMFixture(t)
	compiled, err := CompileCyclicArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	p := compiled.Artifact.RuntimeProjection()
	for index := 0; index < p.OperationCount(); index++ {
		op, _ := p.OperationAt(index)
		if op.Boundaries.InLoop.Status != FactUnknown || op.Boundaries.InWTO.Status != FactUnknown {
			t.Fatalf("cyclic operation %d retained acyclic boundary fact: %#v", index, op.Boundaries)
		}
		if got := p.JITAdmissionAt(index); got.Inlining || got.StackPlacement || got.AllocationSinking {
			t.Fatalf("cyclic operation %d admitted a boundary-sensitive optimization: %#v", index, got)
		}
	}
}

func TestRuntimeProjectionJITAdmissionHasNoHotPathAllocations(t *testing.T) {
	artifact, _, _ := stage3Artifact(t)
	compiled, err := CompileArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	p := compiled.RuntimeProjection()
	_ = p.JITAdmissionAt(0)
	if got := testing.AllocsPerRun(1000, func() { _ = p.JITAdmissionAt(0) }); got != 0 {
		t.Fatalf("JIT admission allocations/run = %v, want 0", got)
	}
}

func BenchmarkRuntimeProjectionJITAdmission(b *testing.B) {
	body := testBody(31)
	entry := EntryParameter{Body: body, Name: "entry"}
	artifact := Artifact{Equations: []Equation{
		{Target: Coordinate{Body: body, Name: "identity"}, Entry: entry, Occurrence: Occurrence{Kind: "entry", ContractID: testID(41)}, KernelID: "canonical/identity", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
	}}
	compiled, err := CompileArtifact(artifact)
	if err != nil {
		b.Fatal(err)
	}
	p := compiled.RuntimeProjection()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_ = p.JITAdmissionAt(0)
	}
}

func TestRuntimeProjectionJITAdmissionRequiresEveryProof(t *testing.T) {
	artifact, _, _ := stage3Artifact(t)
	compiled, err := CompileArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	p := compiled.RuntimeProjection()
	op := &p.ops[0]
	provenance := FactProvenance{ArtifactID: p.artifactID, ContractID: op.ContractID, Operation: 0}
	provenFalse := provenBool(false, provenance)
	op.Effects = RuntimeEffectSummary{HeapWrite: provenFalse, GlobalWrite: provenFalse, UpvalueWrite: provenFalse, Allocation: provenFalse, Error: provenFalse, UserYield: provenFalse, SystemYield: provenFalse}
	op.Boundaries.Suspends = provenFalse
	for index := op.OperandStart; index < op.OperandStart+op.OperandCount; index++ {
		p.operands[index].Shape = RuntimeValueFact{Tag: RuntimeValueNumber, Status: FactProven, Provenance: provenance}
	}
	admission := p.JITAdmissionAt(0)
	if !admission.Inlining || !admission.TypeSpecialize || !admission.BoundsSpecialize {
		t.Fatalf("proven effect/shape admission = %#v", admission)
	}
	op.Effects.Error.Status = FactUnknown
	if got := p.JITAdmissionAt(0); got.Inlining {
		t.Fatalf("unknown error fact permitted inlining: %#v", got)
	}
	p.operands[op.OperandStart].Shape.Status = FactUnknown
	if got := p.JITAdmissionAt(0); got.TypeSpecialize || got.BoundsSpecialize {
		t.Fatalf("unknown shape fact permitted specialization: %#v", got)
	}
}
