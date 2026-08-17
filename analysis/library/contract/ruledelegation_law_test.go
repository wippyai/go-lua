package contract

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// The rule-delegation payload names the rule that owns a member's
// caller-dependent result selection. Its whole content is an identity, so these
// laws hold the identity still, hold it apart from every other payload this
// package writes, and refuse a delegation that names nothing.

// TestRuleDelegationRoundTripsThroughItsOwnBytes states the format from both
// ends: what it writes it reads back as the same rule identity.
func TestRuleDelegationRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, rule := range []schema.Key{"value-presence-refinement", "call-dispatch", "raw-get"} {
		data, err := EncodeRuleDelegation(rule)
		if err != nil {
			t.Fatalf("%s: the delegation did not encode: %v", rule, err)
		}
		got, err := DecodeRuleDelegation(data)
		if err != nil {
			t.Fatalf("%s: the delegation did not decode: %v", rule, err)
		}
		if got != rule {
			t.Fatalf("the delegation names %q, want %q", got, rule)
		}
	}
}

// TestRuleDelegationNamesTheRuleSurface states which surface a delegation
// resolves against. A delegation is an entry reference, and the entry identity
// it derives is the rule surface's own, so a reader never has to guess which
// table to look in.
func TestRuleDelegationNamesTheRuleSurface(t *testing.T) {
	const rule schema.Key = "value-presence-refinement"
	if RuleDelegationEntryID(rule) != schema.NewEntryID(schema.SurfaceKindRule, rule) {
		t.Fatal("a delegation derives an identity that is not the rule surface's own")
	}
	if RuleDelegationEntryID("").Available() {
		t.Fatal("a delegation that names no rule derived an entry identity")
	}
}

// TestRuleDelegationRejectsWhatItCannotCarry is the boundary law. A payload is
// decodable on its own, so it must also be refusable on its own: another
// payload's stream, a truncated one, and a delegation that names nothing are all
// refused rather than read as a rule reference.
func TestRuleDelegationRejectsWhatItCannotCarry(t *testing.T) {
	if _, err := EncodeRuleDelegation(""); err == nil {
		t.Fatal("a delegation that names no rule encoded as a payload")
	}
	if _, err := DecodeRuleDelegation(nil); err == nil {
		t.Fatal("an empty payload decoded as a rule delegation")
	}
	path, err := EncodePath(Export("gmatch"))
	if err != nil {
		t.Fatalf("the export path did not encode: %v", err)
	}
	if _, err := DecodeRuleDelegation(path); err == nil {
		t.Fatal("an export path payload decoded as a rule delegation")
	}
	delegation, err := EncodeRuleDelegation("call-dispatch")
	if err != nil {
		t.Fatalf("the delegation did not encode: %v", err)
	}
	if _, err := DecodePath(delegation); err == nil {
		t.Fatal("a rule delegation payload decoded as an export path")
	}
	if _, err := DecodeRuleDelegation(delegation[:len(delegation)-1]); err == nil {
		t.Fatal("a truncated delegation decoded as a rule reference")
	}
	if _, err := DecodeRuleDelegation(append(append([]byte(nil), delegation...), 0)); err == nil {
		t.Fatal("a delegation with trailing bytes decoded as a rule reference")
	}
}

// TestRuleDelegationPayloadWireIsPinned holds the payload bytes still. A
// delegation published in a shipped contract is read by another build, so its
// encoding is a compatibility surface and not an implementation detail.
func TestRuleDelegationPayloadWireIsPinned(t *testing.T) {
	const pinned = "0129616e616c797369732f6c6962726172792f636f6e74726163742f72756c652d64656c65676174696f6e020101080d63616c6c2d6469737061746368"
	data, err := EncodeRuleDelegation("call-dispatch")
	if err != nil {
		t.Fatalf("the delegation did not encode: %v", err)
	}
	if got := hex.EncodeToString(data); got != pinned {
		t.Errorf("payload wire is %s, pinned %s", got, pinned)
	}
}
