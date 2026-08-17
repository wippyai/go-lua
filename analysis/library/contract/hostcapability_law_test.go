package contract

import (
	"encoding/hex"
	"testing"
)

// The host-capability payload is the grant the environment boots under: the
// audited capability identities the host allows a contract published into it to
// exercise. It carries identities and nothing else - what each capability MEANS
// belongs to the vocabulary that audits it, and restating that here would be a
// second authority over one classification.

func hostCapabilityCorpus() [][]string {
	return [][]string{
		{"iteration.Iterator"},
		{"mutation.Mutate"},
		{"dispatch.ModuleLoad", "iteration.Iterator", "mutation.Mutate"},
		{"ownership.Borrow", "ownership.Retain", "ownership.Store"},
	}
}

// TestHostCapabilitiesRoundTripThroughTheirOwnBytes states the format from both
// ends: what it writes it reads back as the same grant, in the same order.
func TestHostCapabilitiesRoundTripThroughTheirOwnBytes(t *testing.T) {
	for _, granted := range hostCapabilityCorpus() {
		data, err := EncodeHostCapabilities(granted)
		if err != nil {
			t.Fatalf("%v: the host capabilities did not encode: %v", granted, err)
		}
		got, err := DecodeHostCapabilities(data)
		if err != nil {
			t.Fatalf("%v: the host capabilities did not decode: %v", granted, err)
		}
		if len(got) != len(granted) {
			t.Fatalf("the payload reads back %d capabilities, want %d", len(got), len(granted))
		}
		for index, want := range granted {
			if got[index] != want {
				t.Fatalf("capability %d reads back as %q, want %q", index, got[index], want)
			}
		}
	}
}

// TestHostCapabilitiesDistinguishEveryShapeTheyCarry states that no two authored
// grants share a payload. A host that grants one capability more is a different
// host.
func TestHostCapabilitiesDistinguishEveryShapeTheyCarry(t *testing.T) {
	written := make(map[string][]string)
	for _, granted := range hostCapabilityCorpus() {
		data, err := EncodeHostCapabilities(granted)
		if err != nil {
			t.Fatalf("%v: the host capabilities did not encode: %v", granted, err)
		}
		key := string(data)
		if prior, collided := written[key]; collided {
			t.Fatalf("%v and %v write one payload", prior, granted)
		}
		written[key] = granted
	}
}

// TestHostCapabilitiesRejectWhatTheyCannotCarry is the boundary law. A grant of
// nothing grants nothing and is not written as an empty list that looks like one;
// an unnamed capability names no audited identity; a repeated one is granted
// twice under one name; and an unordered grant is not the authored artifact,
// because order is content and a set written two ways would publish two
// identities for one environment.
func TestHostCapabilitiesRejectWhatTheyCannotCarry(t *testing.T) {
	rejected := [][]string{
		nil,
		{},
		{""},
		{"mutation.Mutate", "mutation.Mutate"},
		{"mutation.Mutate", "iteration.Iterator"},
	}
	for _, granted := range rejected {
		if _, err := EncodeHostCapabilities(granted); err == nil {
			t.Fatalf("%v encoded as a payload", granted)
		}
	}
	if _, err := DecodeHostCapabilities(nil); err == nil {
		t.Fatal("an empty payload decoded as a host capability grant")
	}
	delegation, err := EncodeRuleDelegation("pattern")
	if err != nil {
		t.Fatalf("the rule delegation did not encode: %v", err)
	}
	if _, err := DecodeHostCapabilities(delegation); err == nil {
		t.Fatal("a rule delegation payload decoded as a host capability grant")
	}
	granted, err := EncodeHostCapabilities([]string{"iteration.Iterator"})
	if err != nil {
		t.Fatalf("the host capabilities did not encode: %v", err)
	}
	if _, err := DecodeRuleDelegation(granted); err == nil {
		t.Fatal("a host capability payload decoded as a rule delegation")
	}
	if _, err := DecodeHostCapabilities(granted[:len(granted)-1]); err == nil {
		t.Fatal("a truncated host capability grant decoded as a published grant")
	}
	if _, err := DecodeHostCapabilities(append(append([]byte(nil), granted...), 0)); err == nil {
		t.Fatal("a host capability grant with trailing bytes decoded as a published grant")
	}
}

// TestHostCapabilityPayloadWireIsPinned holds the payload bytes still. The grant
// published in a shipped contract is read by another build, so its encoding is a
// compatibility surface and not an implementation detail.
func TestHostCapabilityPayloadWireIsPinned(t *testing.T) {
	const pinned = "0129616e616c797369732f6c6962726172792f636f6e74726163742f686f73742d6361706162696c6974790201010401010301060812697465726174696f6e2e4974657261746f72"
	granted, err := EncodeHostCapabilities([]string{"iteration.Iterator"})
	if err != nil {
		t.Fatalf("the host capabilities did not encode: %v", err)
	}
	if got := hex.EncodeToString(granted); got != pinned {
		t.Errorf("host capability payload wire is %s, pinned %s", got, pinned)
	}
}
