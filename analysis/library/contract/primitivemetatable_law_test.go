package contract

import (
	"encoding/hex"
	"testing"
)

// The primitive-metatable payload is the environment's statement that a base
// primitive of the language reaches its methods through a metatable another
// contract owns. It is the one cross-contract reference the surface carries, so
// these laws hold what makes such a reference resolvable: the mount selector of
// the contract that owns the metatable, and the metatable-key address inside it.

func primitiveMetatableCorpus() [][]PrimitiveAttachment {
	return [][]PrimitiveAttachment{
		{{Base: ConstantString, Contract: "string", Path: Metatable("__index"), Mutability: MutabilityMutable}},
		{{Base: ConstantString, Contract: "string", Path: Metatable("__index"), Mutability: MutabilitySealed}},
		{{Base: ConstantString, Contract: "text", Path: Metatable("__index"), Mutability: MutabilityMutable}},
		{{Base: ConstantString, Contract: "string", Path: Metatable("__call"), Mutability: MutabilityMutable}},
		{
			{Base: ConstantBoolean, Contract: "boolean", Path: Metatable("__index"), Mutability: MutabilitySealed},
			{Base: ConstantString, Contract: "string", Path: Metatable("__index"), Mutability: MutabilityMutable},
		},
	}
}

// TestPrimitiveMetatableRoundTripsThroughItsOwnBytes states the format from both
// ends: what it writes it reads back as the same attachment set.
func TestPrimitiveMetatableRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, attachments := range primitiveMetatableCorpus() {
		data, err := EncodePrimitiveMetatables(attachments)
		if err != nil {
			t.Fatalf("%+v: the primitive metatables did not encode: %v", attachments, err)
		}
		got, err := DecodePrimitiveMetatables(data)
		if err != nil {
			t.Fatalf("%+v: the primitive metatables did not decode: %v", attachments, err)
		}
		if len(got) != len(attachments) {
			t.Fatalf("the payload reads back %d attachments, want %d", len(got), len(attachments))
		}
		for index, want := range attachments {
			if got[index].Base != want.Base || got[index].Contract != want.Contract ||
				got[index].Mutability != want.Mutability || !got[index].Path.Equal(want.Path) {
				t.Fatalf("attachment %d reads back as %+v, want %+v", index, got[index], want)
			}
		}
	}
}

// TestPrimitiveMetatableDistinguishesEveryShapeItCarries states that no two
// authored attachment sets share a payload. Attaching a metatable another
// contract owns is a different environment from attaching this one's.
func TestPrimitiveMetatableDistinguishesEveryShapeItCarries(t *testing.T) {
	written := make(map[string][]PrimitiveAttachment)
	for _, attachments := range primitiveMetatableCorpus() {
		data, err := EncodePrimitiveMetatables(attachments)
		if err != nil {
			t.Fatalf("%+v: the primitive metatables did not encode: %v", attachments, err)
		}
		key := string(data)
		if prior, collided := written[key]; collided {
			t.Fatalf("%+v and %+v write one payload", prior, attachments)
		}
		written[key] = attachments
	}
}

// TestPrimitiveMetatableRejectsWhatItCannotCarry is the boundary law. An
// attachment that names no owning contract cannot be resolved by a reader; one
// whose address is not a metatable key names an ordinary export and attaches no
// metatable at all; and two attachments over one base are two metatables for one
// primitive, which a reader has no ground to choose between.
func TestPrimitiveMetatableRejectsWhatItCannotCarry(t *testing.T) {
	rejected := [][]PrimitiveAttachment{
		nil,
		{{}},
		{{Base: ConstantString, Path: Metatable("__index"), Mutability: MutabilityMutable}},
		{{Base: ConstantString, Contract: "string", Mutability: MutabilityMutable}},
		{{Base: ConstantString, Contract: "string", Path: Metatable("__index")}},
		{{Base: constantKindLimit, Contract: "string", Path: Metatable("__index"), Mutability: MutabilityMutable}},
		{{Base: ConstantString, Contract: "string", Path: Metatable("__index"), Mutability: mutabilityLimit}},
		// An ordinary export path reaches a value, not the metatable that
		// publishes it.
		{{Base: ConstantString, Contract: "string", Path: Export("upper"), Mutability: MutabilityMutable}},
		{{Base: ConstantString, Contract: "string", Path: Root(), Mutability: MutabilityMutable}},
		// One primitive, two metatables.
		{
			{Base: ConstantString, Contract: "string", Path: Metatable("__index"), Mutability: MutabilityMutable},
			{Base: ConstantString, Contract: "text", Path: Metatable("__index"), Mutability: MutabilityMutable},
		},
		// Base order is content, so an unordered set is not the authored one.
		{
			{Base: ConstantString, Contract: "string", Path: Metatable("__index"), Mutability: MutabilityMutable},
			{Base: ConstantBoolean, Contract: "boolean", Path: Metatable("__index"), Mutability: MutabilityMutable},
		},
	}
	for _, attachments := range rejected {
		if _, err := EncodePrimitiveMetatables(attachments); err == nil {
			t.Fatalf("%+v encoded as a payload", attachments)
		}
	}
	if _, err := DecodePrimitiveMetatables(nil); err == nil {
		t.Fatal("an empty payload decoded as a primitive metatable set")
	}
	path, err := EncodePath(Metatable("__index"))
	if err != nil {
		t.Fatalf("the export path did not encode: %v", err)
	}
	if _, err := DecodePrimitiveMetatables(path); err == nil {
		t.Fatal("an export path payload decoded as a primitive metatable set")
	}
	attached, err := EncodePrimitiveMetatables([]PrimitiveAttachment{
		{Base: ConstantString, Contract: "string", Path: Metatable("__index"), Mutability: MutabilityMutable},
	})
	if err != nil {
		t.Fatalf("the primitive metatables did not encode: %v", err)
	}
	if _, err := DecodePath(attached); err == nil {
		t.Fatal("a primitive metatable payload decoded as an export path")
	}
	if _, err := DecodePrimitiveMetatables(attached[:len(attached)-1]); err == nil {
		t.Fatal("a truncated primitive metatable set decoded as a published set")
	}
	if _, err := DecodePrimitiveMetatables(append(append([]byte(nil), attached...), 0)); err == nil {
		t.Fatal("a primitive metatable set with trailing bytes decoded as a published set")
	}
}

// TestPrimitiveMetatablePayloadWireIsPinned holds the payload bytes still. The
// attachment published in a shipped contract is read by another build, so its
// encoding is a compatibility surface and not an implementation detail.
func TestPrimitiveMetatablePayloadWireIsPinned(t *testing.T) {
	const pinned = "012d616e616c797369732f6c6962726172792f636f6e74726163742f7072696d69746976652d6d6574617461626c650201010401010301050501050501010806737472696e67073c0125616e616c797369732f6c6962726172792f636f6e74726163742f6578706f72742d7061746802010104010103010305010208075f5f696e646578"
	attached, err := EncodePrimitiveMetatables([]PrimitiveAttachment{
		{Base: ConstantString, Contract: "string", Path: Metatable("__index"), Mutability: MutabilityMutable},
	})
	if err != nil {
		t.Fatalf("the primitive metatables did not encode: %v", err)
	}
	if got := hex.EncodeToString(attached); got != pinned {
		t.Errorf("primitive metatable payload wire is %s, pinned %s", got, pinned)
	}
}
