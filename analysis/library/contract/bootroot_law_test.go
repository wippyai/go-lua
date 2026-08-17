package contract

import (
	"encoding/hex"
	"testing"
)

// The boot-root payload states what one root of the initial environment is: the
// aggregate it boots as, and the mutability its whole object is published with.
// Its identity is the member address it is published at, so these laws hold the
// two facts it does carry apart from every other payload this package writes and
// refuse a root that states neither.

func bootRootCorpus() []BootRoot {
	return []BootRoot{
		{Aggregate: BootAggregateTable, Mutability: MutabilityMutable},
		{Aggregate: BootAggregateTable, Mutability: MutabilitySealed},
		{Aggregate: BootAggregateMetatable, Mutability: MutabilityMutable},
		{Aggregate: BootAggregateMetatable, Mutability: MutabilitySealed},
	}
}

// TestBootRootRoundTripsThroughItsOwnBytes states the format from both ends:
// what it writes it reads back as the same root, for every shape it carries.
func TestBootRootRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, root := range bootRootCorpus() {
		data, err := EncodeBootRoot(root)
		if err != nil {
			t.Fatalf("%+v: the boot root did not encode: %v", root, err)
		}
		got, err := DecodeBootRoot(data)
		if err != nil {
			t.Fatalf("%+v: the boot root did not decode: %v", root, err)
		}
		if got != root {
			t.Fatalf("the payload reads back as %+v, want %+v", got, root)
		}
	}
}

// TestBootRootDistinguishesEveryShapeItCarries states that no two authored roots
// share a payload. A frozen root and a mutable one are different environments,
// so they are different bytes.
func TestBootRootDistinguishesEveryShapeItCarries(t *testing.T) {
	written := make(map[string]BootRoot)
	for _, root := range bootRootCorpus() {
		data, err := EncodeBootRoot(root)
		if err != nil {
			t.Fatalf("%+v: the boot root did not encode: %v", root, err)
		}
		key := string(data)
		if prior, collided := written[key]; collided {
			t.Fatalf("%+v and %+v write one payload", prior, root)
		}
		written[key] = root
	}
}

// TestBootRootRejectsWhatItCannotCarry is the boundary law. A root that declares
// no aggregate is not a root, one published under no mutability states no policy,
// and another payload's stream is not this one.
func TestBootRootRejectsWhatItCannotCarry(t *testing.T) {
	rejected := []BootRoot{
		{},
		{Aggregate: BootAggregateTable},
		{Mutability: MutabilityMutable},
		{Aggregate: bootAggregateLimit, Mutability: MutabilityMutable},
		{Aggregate: BootAggregateTable, Mutability: mutabilityLimit},
	}
	for _, root := range rejected {
		if root.Available() {
			t.Fatalf("%+v is admitted as a boot root", root)
		}
		if _, err := EncodeBootRoot(root); err == nil {
			t.Fatalf("%+v encoded as a payload", root)
		}
	}
	if _, err := DecodeBootRoot(nil); err == nil {
		t.Fatal("an empty payload decoded as a boot root")
	}
	value, err := EncodeExportValue(Aggregate(MutabilityMutable))
	if err != nil {
		t.Fatalf("the export value did not encode: %v", err)
	}
	if _, err := DecodeBootRoot(value); err == nil {
		t.Fatal("an export value payload decoded as a boot root")
	}
	root, err := EncodeBootRoot(BootRoot{Aggregate: BootAggregateTable, Mutability: MutabilityMutable})
	if err != nil {
		t.Fatalf("the boot root did not encode: %v", err)
	}
	if _, err := DecodeExportValue(root); err == nil {
		t.Fatal("a boot root payload decoded as an export value")
	}
	if _, err := DecodeBootRoot(root[:len(root)-1]); err == nil {
		t.Fatal("a truncated boot root decoded as a published root")
	}
	if _, err := DecodeBootRoot(append(append([]byte(nil), root...), 0)); err == nil {
		t.Fatal("a boot root with trailing bytes decoded as a published root")
	}
}

// TestBootRootPayloadWireIsPinned holds the payload bytes still. A boot root
// published in a shipped contract is read by another build, so its encoding is a
// compatibility surface and not an implementation detail.
func TestBootRootPayloadWireIsPinned(t *testing.T) {
	const pinnedMutableTable = "0123616e616c797369732f6c6962726172792f636f6e74726163742f626f6f742d726f6f74020101050101050101"
	const pinnedSealedMetatable = "0123616e616c797369732f6c6962726172792f636f6e74726163742f626f6f742d726f6f74020101050102050102"
	table, err := EncodeBootRoot(BootRoot{Aggregate: BootAggregateTable, Mutability: MutabilityMutable})
	if err != nil {
		t.Fatalf("the mutable table root did not encode: %v", err)
	}
	if got := hex.EncodeToString(table); got != pinnedMutableTable {
		t.Errorf("mutable table payload wire is %s, pinned %s", got, pinnedMutableTable)
	}
	metatable, err := EncodeBootRoot(BootRoot{Aggregate: BootAggregateMetatable, Mutability: MutabilitySealed})
	if err != nil {
		t.Fatalf("the sealed metatable root did not encode: %v", err)
	}
	if got := hex.EncodeToString(metatable); got != pinnedSealedMetatable {
		t.Errorf("sealed metatable payload wire is %s, pinned %s", got, pinnedSealedMetatable)
	}
}
