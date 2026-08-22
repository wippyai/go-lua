package contract

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// TestContentIDIsDerivedOnceAtSeal states the compute-once law. A sealed
// Contract is immutable, so its identity is derived by the constructor and
// afterwards only read. Reading it allocates nothing: an answer that had to be
// re-derived would have to hash the whole contract again, and hashing
// allocates.
func TestContentIDIsDerivedOnceAtSeal(t *testing.T) {
	contract := encodingContract(t)
	first := contract.ContentID()
	if !first.Available() {
		t.Fatal("sealed contract has no identity")
	}
	allocations := testing.AllocsPerRun(200, func() {
		if contract.ContentID() != first {
			t.Fatal("identity changed between reads")
		}
	})
	if allocations != 0 {
		t.Fatalf("reading a sealed identity allocated %.1f objects per call; the identity is being re-derived", allocations)
	}
}

// TestContentIDIsStableAcrossReads states that repeated reads answer one
// value. Re-derivation and a sealed column agree only while the derivation is
// deterministic, so this law keeps the column honest about what it replaced.
func TestContentIDIsStableAcrossReads(t *testing.T) {
	contract := encodingContract(t)
	first := contract.ContentID()
	for index := 0; index < 1000; index++ {
		if contract.ContentID() != first {
			t.Fatalf("read %d answered a different identity", index)
		}
	}
}

// TestContentIDIsContentIdentityNotPointerIdentity states that the identity
// belongs to the content. Two independently sealed contracts over equal
// declarations are distinct Go objects and must answer one identity.
func TestContentIDIsContentIdentityNotPointerIdentity(t *testing.T) {
	first, second := encodingContract(t), encodingContract(t)
	if first == second {
		t.Fatal("the fixture handed back one contract; the law needs two")
	}
	if first.ContentID() != second.ContentID() {
		t.Fatal("equal declarations sealed different identities")
	}
}

// TestContentIDSeparatesDifferentDeclarations states the no-aliasing law.
// Contracts whose declared content differs must never share an identity, or a
// consumer keyed by that identity would read one contract's answers for
// another.
func TestContentIDSeparatesDifferentDeclarations(t *testing.T) {
	open := encodingContractWithEffectTail(t, vocabulary.RowUnknownOpen)
	closed := encodingContractWithEffectTail(t, vocabulary.RowClosed)
	if !open.ContentID().Available() || !closed.ContentID().Available() {
		t.Fatal("a sealed contract has no identity")
	}
	if open.ContentID() == closed.ContentID() {
		t.Fatal("contracts with different declared effect rows aliased one identity")
	}
}

// TestContentIDIsSharedByConcurrentReaders states that concurrent consumers
// observe one sealed answer. The identity is written before the contract
// escapes its constructor, so no reader can race a derivation.
func TestContentIDIsSharedByConcurrentReaders(t *testing.T) {
	contract := encodingContract(t)
	expected := contract.ContentID()
	const readers = 64
	var group sync.WaitGroup
	answers := make([]bool, readers)
	group.Add(readers)
	for index := 0; index < readers; index++ {
		go func(slot int) {
			defer group.Done()
			for round := 0; round < 100; round++ {
				if contract.ContentID() != expected {
					return
				}
			}
			answers[slot] = true
		}(index)
	}
	group.Wait()
	for slot, agreed := range answers {
		if !agreed {
			t.Fatalf("concurrent reader %d observed a different identity", slot)
		}
	}
}

// TestUnavailableContractHasNoIdentity keeps the boundary failing closed for a
// value that was never sealed.
func TestUnavailableContractHasNoIdentity(t *testing.T) {
	var absent *Contract
	if absent.ContentID().Available() {
		t.Fatal("an unsealed contract answered an identity")
	}
}

// TestSealRefusesAContractWithoutIdentity states that a failed derivation is
// loud. A contract whose identity cannot be derived is malformed, and New must
// refuse it rather than publish a contract that answers a zero identity.
func TestSealRefusesAContractWithoutIdentity(t *testing.T) {
	contract := encodingContract(t)
	id, err := contract.sealContentID()
	if err != nil || !id.Available() {
		t.Fatalf("a sealed contract failed its own derivation: id=%t err=%v", id.Available(), err)
	}
	// An unsealed value is exactly the state New refuses to publish.
	unsealed := &Contract{Operations: contract.Operations, exactKeys: contract.exactKeys}
	if _, err := unsealed.sealContentID(); err == nil {
		t.Fatal("deriving an identity for an unsealed contract succeeded silently")
	}
}
