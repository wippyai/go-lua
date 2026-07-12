package primitive

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/semantic/transaction"
)

type axisValue struct{ value uint8 }

func TestSealReducerCoupledProgramDerivesExpandedCapabilities(t *testing.T) {
	program := reducerProgram(t)
	builder := NewBuilder()
	if err := builder.AddProgram(program); err != nil {
		t.Fatal(err)
	}
	addCompleteCoverage(t, builder, program.ID,
		leafID(transaction.SlotAxis, "runtimekind", "value"),
		leafID(transaction.SlotAxis, "typewitness", "value"))
	registry, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	want := []transaction.Capability{
		{ID: "runtimekind", Kind: transaction.SlotAxis},
		{ID: "typewitness", Kind: transaction.SlotAxis},
	}
	if got := registry.Capabilities(program.ID); !reflect.DeepEqual(got, want) {
		t.Fatalf("derived capabilities = %#v, want %#v", got, want)
	}
	if registry.Digest() == ([32]byte{}) || len(registry.CanonicalBytes()) == 0 {
		t.Fatal("registry was not canonically sealed")
	}
}

func TestSealRejectsMissingAndDuplicateCoverage(t *testing.T) {
	program := reducerProgram(t)

	missing := NewBuilder()
	if err := missing.AddProgram(program); err != nil {
		t.Fatal(err)
	}
	addCompleteCoverage(t, missing, program.ID, leafID(transaction.SlotAxis, "typewitness", "value"))
	if _, err := missing.Seal(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "missing row") || !strings.Contains(err.Error(), "runtimekind") {
		t.Fatalf("missing reducer coverage error = %v", err)
	}

	duplicate := NewBuilder()
	if err := duplicate.AddProgram(program); err != nil {
		t.Fatal(err)
	}
	runtimeLeaf := leafID(transaction.SlotAxis, "runtimekind", "value")
	addCompleteCoverage(t, duplicate, program.ID, runtimeLeaf, leafID(transaction.SlotAxis, "typewitness", "value"))
	if err := duplicate.AddCoverage(Coverage{ProgramID: program.ID, LeafID: runtimeLeaf, Role: CoveragePrimitive}); err != nil {
		t.Fatal(err)
	}
	if _, err := duplicate.Seal(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "duplicate row") {
		t.Fatalf("duplicate coverage error = %v", err)
	}
}

func TestCoverageDistinguishesTwoLeavesUnderOneCapability(t *testing.T) {
	program := twoLeafProgram(t)
	builder := NewBuilder()
	if err := builder.AddProgram(program); err != nil {
		t.Fatal(err)
	}
	addCompleteCoverage(t, builder, program.ID, leafID(transaction.SlotLane, "values", "left"))
	if _, err := builder.Seal(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "values:right") {
		t.Fatalf("missing second leaf coverage error = %v", err)
	}
}

func TestSealRequiresExactIntrinsicBindings(t *testing.T) {
	call, err := NewIntrinsicCall("numeric.floor", 1, []byte("call"))
	if err != nil {
		t.Fatal(err)
	}
	program := ProgramDescriptor{ID: "floor", SchemaVersion: 1, Steps: []Step{IntrinsicStep(call)}}

	missing := NewBuilder()
	if err := missing.AddIntrinsic(IntrinsicDescriptor{ID: "numeric.floor", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if err := missing.AddProgram(program); err != nil {
		t.Fatal(err)
	}
	if _, err := missing.Seal(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "has no native binding") {
		t.Fatalf("missing binding error = %v", err)
	}

	extra := NewBuilder()
	binding := mustBinding(t, "numeric.floor", 1, "floor.v1")
	if err := extra.BindIntrinsic(binding); err != nil {
		t.Fatal(err)
	}
	if _, err := extra.Seal(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "has no intrinsic descriptor") {
		t.Fatalf("extra binding error = %v", err)
	}

	duplicate := NewBuilder()
	if err := duplicate.AddIntrinsic(IntrinsicDescriptor{ID: "numeric.floor", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if err := duplicate.AddIntrinsic(IntrinsicDescriptor{ID: "numeric.floor", SchemaVersion: 1}); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("duplicate descriptor error = %v", err)
	}
	if err := duplicate.BindIntrinsic(binding); err != nil {
		t.Fatal(err)
	}
	if err := duplicate.BindIntrinsic(binding); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("duplicate binding error = %v", err)
	}
}

func TestCircuitReferenceAndConcreteExecutionShareOneNativeAuthority(t *testing.T) {
	calls := 0
	binding, err := NewNativeBinding("numeric.floor", 1, "floor.v1", func(input NativeInput) (NativeOutput, error) {
		calls++
		if !bytes.Equal(input.CallPayload, []byte("circuit-payload")) {
			t.Fatalf("native authority call payload = %q", input.CallPayload)
		}
		return NativeOutput{Payload: append([]byte("floor:"), input.Payload...)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	call, err := NewIntrinsicCall("numeric.floor", 1, []byte("circuit-payload"))
	if err != nil {
		t.Fatal(err)
	}
	builder := NewBuilder()
	if err := builder.AddIntrinsic(IntrinsicDescriptor{ID: "numeric.floor", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if err := builder.BindIntrinsic(binding); err != nil {
		t.Fatal(err)
	}
	if err := builder.AddProgram(ProgramDescriptor{ID: "floor", SchemaVersion: 1, Steps: []Step{IntrinsicStep(call)}}); err != nil {
		t.Fatal(err)
	}
	registry, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := registry.Program("floor")
	if !ok {
		t.Fatal("missing sealed program")
	}
	reference, ok := stored.Steps[0].IntrinsicCall()
	if !ok || reference.ID != "numeric.floor" {
		t.Fatalf("circuit reference = %#v/%v", reference, ok)
	}
	output, err := registry.InvokeIntrinsic(reference, NativeInput{Payload: []byte("value")})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !bytes.Equal(output.Payload, []byte("floor:value")) {
		t.Fatalf("native authority calls/output = %d/%q", calls, output.Payload)
	}
}

func TestRegistryCanonicalBytesIgnoreRegistrationOrder(t *testing.T) {
	build := func(reverse bool) Registry {
		t.Helper()
		builder := NewBuilder()
		programs := []ProgramDescriptor{simpleProgram(t, "alpha", "values"), simpleProgram(t, "beta", "evidence")}
		if reverse {
			programs[0], programs[1] = programs[1], programs[0]
		}
		for _, program := range programs {
			if err := builder.AddProgram(program); err != nil {
				t.Fatal(err)
			}
			slot := program.Steps[0].transaction.Slots()[0]
			addCompleteCoverage(t, builder, program.ID, leafID(slot.Kind, slot.Capability, slot.ID))
		}
		registry, err := builder.Seal()
		if err != nil {
			t.Fatal(err)
		}
		return registry
	}
	left, right := build(false), build(true)
	if !left.Equal(right) || left.Digest() != right.Digest() {
		t.Fatalf("registration order changed registry:\n%x\n%x", left.CanonicalBytes(), right.CanonicalBytes())
	}
}

func reducerProgram(t testing.TB) ProgramDescriptor {
	t.Helper()
	closure := func(seed []transaction.Capability) ([]transaction.Capability, error) {
		return append(seed, transaction.Capability{ID: "runtimekind", Kind: transaction.SlotAxis}), nil
	}
	builder, err := transaction.NewBuilder([]transaction.Capability{{ID: "typewitness", Kind: transaction.SlotAxis}}, closure)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := transaction.Bind[axisValue](builder, "typewitness", "value")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Bind[axisValue](builder, "runtimekind", "value"); err != nil {
		t.Fatal(err)
	}
	policy, err := transaction.NewOutcomePolicy(transaction.Commit, transaction.Rollback, transaction.Rollback, transaction.Rollback)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := builder.BeginOverlay("reduction", policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(overlay, handle, "reduce", []byte("runtimekind")); err != nil {
		t.Fatal(err)
	}
	frozen, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	return ProgramDescriptor{ID: "reduce-types", SchemaVersion: 1, Steps: []Step{TransactionStep(frozen)}}
}

func simpleProgram(t testing.TB, id, capability string) ProgramDescriptor {
	t.Helper()
	builder, err := transaction.NewBuilder([]transaction.Capability{{ID: capability, Kind: transaction.SlotLane}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := transaction.Bind[axisValue](builder, capability, "value")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := transaction.NewOutcomePolicy(transaction.Commit, transaction.Rollback, transaction.Rollback, transaction.Rollback)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := builder.BeginOverlay("effects", policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(overlay, handle, "replace", nil); err != nil {
		t.Fatal(err)
	}
	frozen, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	return ProgramDescriptor{ID: id, SchemaVersion: 1, Steps: []Step{TransactionStep(frozen)}}
}

func twoLeafProgram(t testing.TB) ProgramDescriptor {
	t.Helper()
	builder, err := transaction.NewBuilder([]transaction.Capability{{ID: "values", Kind: transaction.SlotLane}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	left, err := transaction.Bind[axisValue](builder, "values", "left")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Bind[axisValue](builder, "values", "right"); err != nil {
		t.Fatal(err)
	}
	policy, err := transaction.NewOutcomePolicy(transaction.Commit, transaction.Rollback, transaction.Rollback, transaction.Rollback)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := builder.BeginOverlay("effects", policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(overlay, left, "replace", nil); err != nil {
		t.Fatal(err)
	}
	frozen, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	return ProgramDescriptor{ID: "two-leaf", SchemaVersion: 1, Steps: []Step{TransactionStep(frozen)}}
}

func addCompleteCoverage(t testing.TB, builder *Builder, programID string, leaves ...string) {
	t.Helper()
	for _, leaf := range leaves {
		for _, role := range requiredCoverageRoles {
			if err := builder.AddCoverage(Coverage{ProgramID: programID, LeafID: leaf, Role: role}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func leafID(kind transaction.SlotKind, capability, slot string) string {
	return leafForSlot(transaction.Slot{Kind: kind, Capability: capability, ID: slot}).ID
}

func mustBinding(t testing.TB, id string, version uint16, implementation string) NativeBinding {
	t.Helper()
	binding, err := NewNativeBinding(id, version, implementation, func(input NativeInput) (NativeOutput, error) {
		return NativeOutput{Payload: append([]byte(nil), input.Payload...)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}
