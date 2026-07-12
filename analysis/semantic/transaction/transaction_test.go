package transaction

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type valueSlot struct{ value int }
type evidenceSlot struct{ proven bool }

func TestRejectsForgedAndForeignHandles(t *testing.T) {
	policy := mustPolicy(t, Commit, Rollback, Rollback, Rollback)
	left := mustBuilder(t, []Capability{{ID: "values", Kind: SlotLane}}, nil)
	right := mustBuilder(t, []Capability{{ID: "values", Kind: SlotLane}}, nil)
	overlay, err := left.BeginOverlay("effects", policy)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := Bind[valueSlot](right, "values", "result")
	if err != nil {
		t.Fatal(err)
	}
	if err := Append(overlay, foreign, "replace", nil); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "outside the verified scope") {
		t.Fatalf("foreign handle error = %v", err)
	}
	if err := Append(overlay, Handle[valueSlot]{}, "replace", nil); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "forged") {
		t.Fatalf("zero handle error = %v", err)
	}
	if _, err := Bind[valueSlot](left, "missing", "result"); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "outside the verified scope") {
		t.Fatalf("out-of-capability bind error = %v", err)
	}
}

func TestRejectsCrossTypeRebind(t *testing.T) {
	builder := mustBuilder(t, []Capability{{ID: "values", Kind: SlotLane}}, nil)
	if _, err := Bind[valueSlot](builder, "values", "result"); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind[evidenceSlot](builder, "values", "result"); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "cannot rebind") {
		t.Fatalf("cross-type rebind error = %v", err)
	}
}

func TestReducerCapabilityClosureIsNormalizedAndSeedPreserving(t *testing.T) {
	called := 0
	closure := func(seed []Capability) ([]Capability, error) {
		called++
		if len(seed) != 1 || seed[0].ID != "typewitness" {
			t.Fatalf("closure seed = %#v", seed)
		}
		return []Capability{
			{ID: "typewitness", Kind: SlotAxis},
			{ID: "runtimekind", Kind: SlotAxis},
		}, nil
	}
	builder := mustBuilder(t, []Capability{{ID: "typewitness", Kind: SlotAxis}}, closure)
	if called != 1 {
		t.Fatalf("closure called %d times, want once", called)
	}
	if _, err := Bind[evidenceSlot](builder, "runtimekind", "value-axis"); err != nil {
		t.Fatalf("bind reducer-expanded capability: %v", err)
	}
	transaction, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	want := []Capability{{ID: "runtimekind", Kind: SlotAxis}, {ID: "typewitness", Kind: SlotAxis}}
	if !reflect.DeepEqual(transaction.Capabilities(), want) {
		t.Fatalf("capabilities = %#v, want %#v", transaction.Capabilities(), want)
	}

	_, err = NewBuilder([]Capability{{ID: "typewitness", Kind: SlotAxis}}, func([]Capability) ([]Capability, error) {
		return []Capability{{ID: "runtimekind", Kind: SlotAxis}}, nil
	})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "dropped seed capability") {
		t.Fatalf("dropped seed error = %v", err)
	}
}

func TestOverlayOperationAndDecisionOrder(t *testing.T) {
	builder := mustBuilder(t, []Capability{{ID: "values", Kind: SlotLane}}, nil)
	value, err := Bind[valueSlot](builder, "values", "result")
	if err != nil {
		t.Fatal(err)
	}
	first, err := builder.BeginOverlay("before-call", mustPolicy(t, Commit, Commit, Commit, Rollback))
	if err != nil {
		t.Fatal(err)
	}
	if err := Append(first, value, "write-first", []byte{1}); err != nil {
		t.Fatal(err)
	}
	if err := Append(first, value, "write-second", []byte{2}); err != nil {
		t.Fatal(err)
	}
	second, err := builder.BeginOverlay("call-local", mustPolicy(t, Commit, Rollback, Rollback, Rollback))
	if err != nil {
		t.Fatal(err)
	}
	if err := Append(second, value, "write-third", []byte{3}); err != nil {
		t.Fatal(err)
	}
	transaction, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	overlays := transaction.Overlays()
	if got := []string{overlays[0].Operations()[0].Opcode(), overlays[0].Operations()[1].Opcode(), overlays[1].Operations()[0].Opcode()}; !slices.Equal(got, []string{"write-first", "write-second", "write-third"}) {
		t.Fatalf("operation order = %v", got)
	}
	decisions, err := transaction.Decisions(OutcomeRaised)
	if err != nil {
		t.Fatal(err)
	}
	want := []OverlayDecision{{OverlayID: "before-call", Disposition: Commit}, {OverlayID: "call-local", Disposition: Rollback}}
	if !reflect.DeepEqual(decisions, want) {
		t.Fatalf("raised decisions = %#v, want %#v", decisions, want)
	}
	target, err := transaction.Target(overlays[0].Operations()[0])
	if err != nil {
		t.Fatal(err)
	}
	if target != (Slot{Capability: "values", ID: "result", Kind: SlotLane}) {
		t.Fatalf("resolved target = %#v", target)
	}
	other := mustBuilder(t, []Capability{{ID: "values", Kind: SlotLane}}, nil)
	otherHandle, err := Bind[valueSlot](other, "values", "other")
	if err != nil {
		t.Fatal(err)
	}
	otherOverlay, err := other.BeginOverlay("effects", mustPolicy(t, Commit, Rollback, Rollback, Rollback))
	if err != nil {
		t.Fatal(err)
	}
	if err := Append(otherOverlay, otherHandle, "replace", nil); err != nil {
		t.Fatal(err)
	}
	otherTransaction, err := other.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherTransaction.Target(overlays[0].Operations()[0]); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "belongs to another") {
		t.Fatalf("foreign operation target error = %v", err)
	}
}

func TestExplicitPoliciesCoverRollbackAndSurvivalForEveryOutcome(t *testing.T) {
	policy := mustPolicy(t, Commit, Rollback, Commit, Rollback)
	want := map[Outcome]OverlayDisposition{
		OutcomeNormal:       Commit,
		OutcomeRaised:       Rollback,
		OutcomeSuspended:    Commit,
		OutcomeNonreturning: Rollback,
	}
	for outcome, expected := range want {
		got, err := policy.For(outcome)
		if err != nil || got != expected {
			t.Fatalf("policy.For(%d) = %d, %v; want %d", outcome, got, err, expected)
		}
	}
	builder := mustBuilder(t, nil, nil)
	if _, err := builder.BeginOverlay("implicit", OutcomePolicy{}); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "explicit policy") {
		t.Fatalf("implicit policy error = %v", err)
	}
}

func TestFreezeIsImmutableAndSingleUse(t *testing.T) {
	builder := mustBuilder(t, []Capability{{ID: "values", Kind: SlotLane}}, nil)
	handle, err := Bind[valueSlot](builder, "values", "result")
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := builder.BeginOverlay("effects", mustPolicy(t, Commit, Rollback, Rollback, Rollback))
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{1, 2, 3}
	if err := Append(overlay, handle, "replace", payload); err != nil {
		t.Fatal(err)
	}
	payload[0] = 9
	transaction, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	operation := transaction.Overlays()[0].Operations()[0]
	if got := operation.Payload(); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("frozen payload = %v", got)
	}
	mutated := operation.Payload()
	mutated[0] = 8
	canonical := transaction.CanonicalBytes()
	canonical[0] ^= 0xff
	if got := transaction.Overlays()[0].Operations()[0].Payload(); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("operation accessor aliases frozen payload: %v", got)
	}
	if _, err := builder.Freeze(); !errors.Is(err, ErrSealed) {
		t.Fatalf("second Freeze error = %v, want ErrSealed", err)
	}
	if _, err := Bind[valueSlot](builder, "values", "later"); !errors.Is(err, ErrSealed) {
		t.Fatalf("post-freeze Bind error = %v, want ErrSealed", err)
	}
}

func TestCanonicalEncodingIgnoresCapabilityAndBindingConstructionOrder(t *testing.T) {
	build := func(reverse bool) FrozenTransaction {
		t.Helper()
		capabilities := []Capability{{ID: "values", Kind: SlotLane}, {ID: "evidence", Kind: SlotAxis}}
		if reverse {
			slices.Reverse(capabilities)
		}
		builder := mustBuilder(t, capabilities, nil)
		var value Handle[valueSlot]
		var evidence Handle[evidenceSlot]
		var err error
		if reverse {
			evidence, err = Bind[evidenceSlot](builder, "evidence", "proof")
			if err == nil {
				value, err = Bind[valueSlot](builder, "values", "result")
			}
		} else {
			value, err = Bind[valueSlot](builder, "values", "result")
			if err == nil {
				evidence, err = Bind[evidenceSlot](builder, "evidence", "proof")
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		overlay, err := builder.BeginOverlay("effects", mustPolicy(t, Commit, Rollback, Rollback, Rollback))
		if err != nil {
			t.Fatal(err)
		}
		if err := Append(overlay, value, "replace", []byte("value")); err != nil {
			t.Fatal(err)
		}
		if err := Append(overlay, evidence, "prove", []byte("evidence")); err != nil {
			t.Fatal(err)
		}
		transaction, err := builder.Freeze()
		if err != nil {
			t.Fatal(err)
		}
		return transaction
	}
	left, right := build(false), build(true)
	if !bytes.Equal(left.CanonicalBytes(), right.CanonicalBytes()) || left.Digest() != right.Digest() {
		t.Fatalf("construction order changed canonical transaction:\n%x\n%x", left.CanonicalBytes(), right.CanonicalBytes())
	}
}

func mustBuilder(t testing.TB, capabilities []Capability, closure ClosureHook) *Builder {
	t.Helper()
	builder, err := NewBuilder(capabilities, closure)
	if err != nil {
		t.Fatal(err)
	}
	return builder
}

func mustPolicy(t testing.TB, normal, raised, suspended, nonreturning OverlayDisposition) OutcomePolicy {
	t.Helper()
	policy, err := NewOutcomePolicy(normal, raised, suspended, nonreturning)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}
