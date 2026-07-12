package concrete

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/semantic/primitive"
	"github.com/wippyai/go-lua/analysis/semantic/transaction"
)

type intValue struct{ value int }

func cloneInt(value intValue) intValue { return value }

func TestExecuteProtectedOutcomeCommitsAndRollsBackOverlaysInOrder(t *testing.T) {
	frozen := protectedTransaction(t)
	registry := primitiveRegistry(t, "protected", []primitive.Step{primitive.TransactionStep(frozen)}, nil)
	cell := mustCell(t, intValue{}, cloneInt)
	builder := NewBuilder(registry)
	bindTransactionCells(t, builder, frozen, map[string]*Cell[intValue]{"value": cell})
	registerAdd(t, builder, "add", "values")
	backend, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Execute(context.Background(), "protected", ExecutionInput{Outcome: transaction.OutcomeRaised}); err != nil {
		t.Fatal(err)
	}
	if got := cell.Load().value; got != 1 {
		t.Fatalf("raised protected outcome = %d, want outer commit 1 and inner rollback", got)
	}
}

func TestReducerCoupledCapabilitiesResolveDistinctTypedBankCells(t *testing.T) {
	frozen := reducerTransaction(t)
	registry := primitiveRegistry(t, "reduce", []primitive.Step{primitive.TransactionStep(frozen)}, nil)
	runtimeKind := mustCell(t, intValue{}, cloneInt)
	typeWitness := mustCell(t, intValue{value: 100}, cloneInt)
	builder := NewBuilder(registry)
	for _, slot := range frozen.Slots() {
		switch slot.Capability {
		case "runtimekind":
			if err := BindCell(builder, slot, "runtimekind.cell.v1", runtimeKind); err != nil {
				t.Fatal(err)
			}
		case "typewitness":
			if err := BindCell(builder, slot, "typewitness.cell.v1", typeWitness); err != nil {
				t.Fatal(err)
			}
		}
	}
	registerAdd(t, builder, "set-kind", "runtimekind")
	registerAdd(t, builder, "reduce-type", "typewitness")
	backend, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Execute(context.Background(), "reduce", ExecutionInput{Outcome: transaction.OutcomeNormal}); err != nil {
		t.Fatal(err)
	}
	if got := runtimeKind.Load().value; got != 2 {
		t.Fatalf("runtimekind = %d, want 2", got)
	}
	if got := typeWitness.Load().value; got != 97 {
		t.Fatalf("typewitness = %d, want reducer update 97", got)
	}
}

func TestErrorAndCancellationPublishNothing(t *testing.T) {
	for name, register := range map[string]func(*Builder, context.CancelFunc){
		"error": func(builder *Builder, _ context.CancelFunc) {
			if err := RegisterOpcode(builder, "fail", "fail.v1", []transaction.Capability{{ID: "values", Kind: transaction.SlotLane}}, func(context.Context, intValue, []byte) (intValue, error) {
				return intValue{value: 999}, errors.New("boom")
			}); err != nil {
				t.Fatal(err)
			}
		},
		"cancel": func(builder *Builder, cancel context.CancelFunc) {
			if err := RegisterOpcode(builder, "fail", "cancel.v1", []transaction.Capability{{ID: "values", Kind: transaction.SlotLane}}, func(context.Context, intValue, []byte) (intValue, error) {
				cancel()
				return intValue{value: 999}, nil
			}); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			frozen := failingTransaction(t)
			registry := primitiveRegistry(t, "failure", []primitive.Step{primitive.TransactionStep(frozen)}, nil)
			cell := mustCell(t, intValue{value: 7}, cloneInt)
			builder := NewBuilder(registry)
			bindTransactionCells(t, builder, frozen, map[string]*Cell[intValue]{"value": cell})
			registerAdd(t, builder, "add", "values")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			register(builder, cancel)
			backend, err := builder.Seal()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := backend.Execute(ctx, "failure", ExecutionInput{Outcome: transaction.OutcomeNormal}); err == nil {
				t.Fatal("Execute succeeded, want failure")
			}
			if got := cell.Load().value; got != 7 {
				t.Fatalf("failed execution published %d, want original 7", got)
			}
		})
	}
}

func TestIntrinsicFailureUsesPrimitiveAuthorityAndRollsBackTransaction(t *testing.T) {
	frozen := singleTransaction(t, "values", "value", "add", 5)
	calls := 0
	call, err := primitive.NewIntrinsicCall("native.check", 1, []byte("sealed-call"))
	if err != nil {
		t.Fatal(err)
	}
	native, err := primitive.NewNativeBinding("native.check", 1, "native.check.v1", func(input primitive.NativeInput) (primitive.NativeOutput, error) {
		calls++
		if string(input.CallPayload) != "sealed-call" {
			t.Fatalf("call payload = %q", input.CallPayload)
		}
		return primitive.NativeOutput{}, errors.New("intrinsic failed")
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := primitiveRegistry(t, "with-native", []primitive.Step{primitive.TransactionStep(frozen), primitive.IntrinsicStep(call)}, func(builder *primitive.Builder) {
		if err := builder.AddIntrinsic(primitive.IntrinsicDescriptor{ID: "native.check", SchemaVersion: 1}); err != nil {
			t.Fatal(err)
		}
		if err := builder.BindIntrinsic(native); err != nil {
			t.Fatal(err)
		}
	})
	cell := mustCell(t, intValue{value: 10}, cloneInt)
	builder := NewBuilder(registry)
	bindTransactionCells(t, builder, frozen, map[string]*Cell[intValue]{"value": cell})
	registerAdd(t, builder, "add", "values")
	backend, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Execute(context.Background(), "with-native", ExecutionInput{Outcome: transaction.OutcomeNormal}); err == nil || calls != 1 {
		t.Fatalf("Execute error/calls = %v/%d", err, calls)
	}
	if got := cell.Load().value; got != 10 {
		t.Fatalf("intrinsic failure published transaction value %d", got)
	}
}

func TestSealRejectsMissingExtraAndCapabilityUnsafeBindings(t *testing.T) {
	frozen := singleTransaction(t, "values", "value", "add", 1)
	registry := primitiveRegistry(t, "simple", []primitive.Step{primitive.TransactionStep(frozen)}, nil)

	missing := NewBuilder(registry)
	if err := RegisterOpcode(missing, "add", "add.v1", []transaction.Capability{{ID: "values", Kind: transaction.SlotLane}}, func(_ context.Context, value intValue, _ []byte) (intValue, error) { return value, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := missing.Seal(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "has no backend cell") {
		t.Fatalf("missing cell error = %v", err)
	}

	unsafe := NewBuilder(registry)
	cell := mustCell(t, intValue{}, cloneInt)
	bindTransactionCells(t, unsafe, frozen, map[string]*Cell[intValue]{"value": cell})
	if err := RegisterOpcode(unsafe, "add", "add.v1", []transaction.Capability{{ID: "evidence", Kind: transaction.SlotLane}}, func(_ context.Context, value intValue, _ []byte) (intValue, error) { return value, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := unsafe.Seal(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("unsafe capability error = %v", err)
	}
}

func protectedTransaction(t testing.TB) transaction.FrozenTransaction {
	t.Helper()
	builder, handle := transactionBuilder(t, []transaction.Capability{{ID: "values", Kind: transaction.SlotLane}}, "values", "value")
	outer := mustPolicy(t, transaction.Commit, transaction.Commit, transaction.Commit, transaction.Rollback)
	inner := mustPolicy(t, transaction.Commit, transaction.Rollback, transaction.Rollback, transaction.Rollback)
	first, err := builder.BeginOverlay("outer", outer)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(first, handle, "add", encodeInt(1)); err != nil {
		t.Fatal(err)
	}
	second, err := builder.BeginOverlay("protected", inner)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(second, handle, "add", encodeInt(10)); err != nil {
		t.Fatal(err)
	}
	return freeze(t, builder)
}

func reducerTransaction(t testing.TB) transaction.FrozenTransaction {
	t.Helper()
	closure := func(seed []transaction.Capability) ([]transaction.Capability, error) {
		return append(seed, transaction.Capability{ID: "runtimekind", Kind: transaction.SlotAxis}), nil
	}
	builder, err := transaction.NewBuilder([]transaction.Capability{{ID: "typewitness", Kind: transaction.SlotAxis}}, closure)
	if err != nil {
		t.Fatal(err)
	}
	kind, err := transaction.Bind[intValue](builder, "runtimekind", "value")
	if err != nil {
		t.Fatal(err)
	}
	witness, err := transaction.Bind[intValue](builder, "typewitness", "value")
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := builder.BeginOverlay("reducer", mustPolicy(t, transaction.Commit, transaction.Rollback, transaction.Rollback, transaction.Rollback))
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(overlay, kind, "set-kind", encodeInt(2)); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(overlay, witness, "reduce-type", encodeInt(-3)); err != nil {
		t.Fatal(err)
	}
	return freeze(t, builder)
}

func failingTransaction(t testing.TB) transaction.FrozenTransaction {
	t.Helper()
	builder, handle := transactionBuilder(t, []transaction.Capability{{ID: "values", Kind: transaction.SlotLane}}, "values", "value")
	overlay, err := builder.BeginOverlay("effects", mustPolicy(t, transaction.Commit, transaction.Rollback, transaction.Rollback, transaction.Rollback))
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(overlay, handle, "add", encodeInt(5)); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(overlay, handle, "fail", nil); err != nil {
		t.Fatal(err)
	}
	return freeze(t, builder)
}

func singleTransaction(t testing.TB, capability, slot, opcode string, delta int) transaction.FrozenTransaction {
	t.Helper()
	builder, handle := transactionBuilder(t, []transaction.Capability{{ID: capability, Kind: transaction.SlotLane}}, capability, slot)
	overlay, err := builder.BeginOverlay("effects", mustPolicy(t, transaction.Commit, transaction.Rollback, transaction.Rollback, transaction.Rollback))
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(overlay, handle, opcode, encodeInt(delta)); err != nil {
		t.Fatal(err)
	}
	return freeze(t, builder)
}

func transactionBuilder(t testing.TB, capabilities []transaction.Capability, capability, slot string) (*transaction.Builder, transaction.Handle[intValue]) {
	t.Helper()
	builder, err := transaction.NewBuilder(capabilities, nil)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := transaction.Bind[intValue](builder, capability, slot)
	if err != nil {
		t.Fatal(err)
	}
	return builder, handle
}

func primitiveRegistry(t testing.TB, id string, steps []primitive.Step, configure func(*primitive.Builder)) primitive.Registry {
	t.Helper()
	builder := primitive.NewBuilder()
	if configure != nil {
		configure(builder)
	}
	program := primitive.ProgramDescriptor{ID: id, SchemaVersion: 1, Steps: steps}
	if err := builder.AddProgram(program); err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		frozen, ok := step.Transaction()
		if !ok {
			continue
		}
		for _, slot := range frozen.Slots() {
			leaf := fmt.Sprintf("%d:%s:%s", slot.Kind, slot.Capability, slot.ID)
			for _, role := range []primitive.CoverageRole{primitive.CoveragePrimitive, primitive.CoverageEffect, primitive.CoverageOutput, primitive.CoverageObserver} {
				if err := builder.AddCoverage(primitive.Coverage{ProgramID: id, LeafID: leaf, Role: role}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	registry, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func bindTransactionCells(t testing.TB, builder *Builder, frozen transaction.FrozenTransaction, cells map[string]*Cell[intValue]) {
	t.Helper()
	for _, slot := range frozen.Slots() {
		cell := cells[slot.ID]
		if cell == nil {
			t.Fatalf("missing synthetic cell for slot %#v", slot)
		}
		if err := BindCell(builder, slot, slot.Capability+".cell.v1", cell); err != nil {
			t.Fatal(err)
		}
	}
}

func registerAdd(t testing.TB, builder *Builder, opcode, capability string) {
	t.Helper()
	kind := transaction.SlotLane
	if capability == "runtimekind" || capability == "typewitness" {
		kind = transaction.SlotAxis
	}
	if err := RegisterOpcode(builder, opcode, opcode+".v1", []transaction.Capability{{ID: capability, Kind: kind}}, func(_ context.Context, value intValue, payload []byte) (intValue, error) {
		value.value += decodeInt(payload)
		return value, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func mustCell[T any](t testing.TB, value T, clone func(T) T) *Cell[T] {
	t.Helper()
	cell, err := NewCell(value, clone)
	if err != nil {
		t.Fatal(err)
	}
	return cell
}

func mustPolicy(t testing.TB, normal, raised, suspended, nonreturning transaction.OverlayDisposition) transaction.OutcomePolicy {
	t.Helper()
	policy, err := transaction.NewOutcomePolicy(normal, raised, suspended, nonreturning)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func freeze(t testing.TB, builder *transaction.Builder) transaction.FrozenTransaction {
	t.Helper()
	frozen, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func encodeInt(value int) []byte { return []byte(fmt.Sprintf("%d", value)) }
func decodeInt(value []byte) int {
	var out int
	_, _ = fmt.Sscanf(string(value), "%d", &out)
	return out
}
