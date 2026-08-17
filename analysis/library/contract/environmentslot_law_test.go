package contract

import (
	"encoding/hex"
	"testing"
)

// The environment-slot payload binds one slot of the initial environment to the
// value that initially occupies it. The binding is the one place a name meets a
// value, so what it carries is the ADDRESS of that value - or, when the value is
// a literal that terminates the path, the literal itself - and the mutability the
// slot is published under. Never the name a reader could have written down.

func environmentSlotCorpus() []EnvironmentSlot {
	return []EnvironmentSlot{
		BindValue(Root(), MutabilityMutable),
		BindValue(Root(), MutabilitySealed),
		BindValue(Export("table"), MutabilityMutable),
		BindValue(Export("print"), MutabilitySealed),
		BindValue(NewPath(Step{Kind: StepExport, Key: "errors"}, Step{Kind: StepExport, Key: "Error"}), MutabilityMutable),
		BindValue(Metatable("__index"), MutabilityMutable),
		BindConstant(Constant{Kind: ConstantString, String: "Lua 5.3"}, MutabilityMutable),
		BindConstant(Constant{Kind: ConstantString, String: "Lua 5.3"}, MutabilitySealed),
		BindConstant(Constant{Kind: ConstantNil}, MutabilityMutable),
		BindConstant(Constant{Kind: ConstantInteger, Integer: -1}, MutabilityMutable),
	}
}

// TestEnvironmentSlotRoundTripsThroughItsOwnBytes states the format from both
// ends: what it writes it reads back as the same binding, for everything a slot
// can hold.
func TestEnvironmentSlotRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, slot := range environmentSlotCorpus() {
		data, err := EncodeEnvironmentSlot(slot)
		if err != nil {
			t.Fatalf("%+v: the environment slot did not encode: %v", slot, err)
		}
		got, err := DecodeEnvironmentSlot(data)
		if err != nil {
			t.Fatalf("%+v: the environment slot did not decode: %v", slot, err)
		}
		if got.Binding != slot.Binding || got.Mutability != slot.Mutability ||
			got.Constant != slot.Constant || !got.Value.Equal(slot.Value) {
			t.Fatalf("the payload reads back as %+v, want %+v", got, slot)
		}
	}
}

// TestEnvironmentSlotDistinguishesEveryShapeItCarries states that no two
// authored bindings share a payload. A slot bound to the environment root and one
// bound to an export of it hold different values, so they are different bytes.
func TestEnvironmentSlotDistinguishesEveryShapeItCarries(t *testing.T) {
	written := make(map[string]EnvironmentSlot)
	for _, slot := range environmentSlotCorpus() {
		data, err := EncodeEnvironmentSlot(slot)
		if err != nil {
			t.Fatalf("%+v: the environment slot did not encode: %v", slot, err)
		}
		key := string(data)
		if prior, collided := written[key]; collided {
			t.Fatalf("%+v and %+v write one payload", prior, slot)
		}
		written[key] = slot
	}
}

// TestEnvironmentSlotRejectsWhatItCannotCarry is the boundary law. A binding to
// an unwalkable address binds nothing, a binding published under no mutability
// states no policy, a binding that selects one field and carries the other says
// two things at once, and another payload's stream is not this one.
//
// A value binding with no address is deliberately absent from the rejected set:
// the root path IS the empty path, `_G` binds the environment root, and a format
// that refused it could not state the one slot every Lua environment has.
func TestEnvironmentSlotRejectsWhatItCannotCarry(t *testing.T) {
	rejected := []EnvironmentSlot{
		{},
		{Binding: SlotBindingValue, Value: Export("print")},
		{Mutability: MutabilityMutable, Value: Export("print")},
		BindValue(Export(""), MutabilityMutable),
		BindValue(NewPath(Step{Key: "print"}), MutabilityMutable),
		{Binding: SlotBindingValue, Value: Export("print"), Mutability: mutabilityLimit},
		{Binding: slotBindingLimit, Value: Export("print"), Mutability: MutabilityMutable},
		// A value binding holds an address, so it carries no constant.
		{Binding: SlotBindingValue, Value: Export("print"), Mutability: MutabilityMutable, Constant: Constant{Kind: ConstantNil}},
		// A constant binding terminates the path, so it carries no address.
		{Binding: SlotBindingConstant, Value: Export("_VERSION"), Mutability: MutabilityMutable, Constant: Constant{Kind: ConstantNil}},
		{Binding: SlotBindingConstant, Mutability: MutabilityMutable},
		BindConstant(Constant{Kind: ConstantInteger, String: "1"}, MutabilityMutable),
	}
	for _, slot := range rejected {
		if slot.Available() {
			t.Fatalf("%+v is admitted as an environment slot", slot)
		}
		if _, err := EncodeEnvironmentSlot(slot); err == nil {
			t.Fatalf("%+v encoded as a payload", slot)
		}
	}
	if _, err := DecodeEnvironmentSlot(nil); err == nil {
		t.Fatal("an empty payload decoded as an environment slot")
	}
	path, err := EncodePath(Export("print"))
	if err != nil {
		t.Fatalf("the export path did not encode: %v", err)
	}
	if _, err := DecodeEnvironmentSlot(path); err == nil {
		t.Fatal("an export path payload decoded as an environment slot")
	}
	slot, err := EncodeEnvironmentSlot(BindValue(Export("print"), MutabilityMutable))
	if err != nil {
		t.Fatalf("the environment slot did not encode: %v", err)
	}
	if _, err := DecodePath(slot); err == nil {
		t.Fatal("an environment slot payload decoded as an export path")
	}
	if _, err := DecodeExportValue(slot); err == nil {
		t.Fatal("an environment slot payload decoded as an export value")
	}
	if _, err := DecodeEnvironmentSlot(slot[:len(slot)-1]); err == nil {
		t.Fatal("a truncated environment slot decoded as a published binding")
	}
	if _, err := DecodeEnvironmentSlot(append(append([]byte(nil), slot...), 0)); err == nil {
		t.Fatal("an environment slot with trailing bytes decoded as a published binding")
	}
}

// TestEnvironmentSlotPayloadWireIsPinned holds the payload bytes still. A slot
// binding published in a shipped contract is read by another build, so its
// encoding is a compatibility surface and not an implementation detail.
func TestEnvironmentSlotPayloadWireIsPinned(t *testing.T) {
	const pinnedValue = "012a616e616c797369732f6c6962726172792f636f6e74726163742f656e7669726f6e6d656e742d736c6f74020101050101050101073a0125616e616c797369732f6c6962726172792f636f6e74726163742f6578706f72742d7061746802010104010103010305010108057072696e74"
	const pinnedConstant = "012a616e616c797369732f6c6962726172792f636f6e74726163742f656e7669726f6e6d656e742d736c6f7402010105010205010103010405010508074c756120352e33"
	value, err := EncodeEnvironmentSlot(BindValue(Export("print"), MutabilityMutable))
	if err != nil {
		t.Fatalf("the value binding did not encode: %v", err)
	}
	if got := hex.EncodeToString(value); got != pinnedValue {
		t.Errorf("value binding payload wire is %s, pinned %s", got, pinnedValue)
	}
	constant, err := EncodeEnvironmentSlot(BindConstant(Constant{Kind: ConstantString, String: "Lua 5.3"}, MutabilityMutable))
	if err != nil {
		t.Fatalf("the constant binding did not encode: %v", err)
	}
	if got := hex.EncodeToString(constant); got != pinnedConstant {
		t.Errorf("constant binding payload wire is %s, pinned %s", got, pinnedConstant)
	}
}
