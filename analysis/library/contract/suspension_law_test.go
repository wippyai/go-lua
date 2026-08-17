package contract

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// The suspension payload relates two outcome cases of one callable member: the
// case control leaves at, and the case it re-enters at. Both are named, never
// minted: the payload carries the sealed structural outcome members' own keys,
// so a reader resolves them against the declared outcome vocabulary instead of
// interpreting an ordinal this format invented. What the payload owns outright
// is the policy half - which authority restores the suspension, and whether one
// live suspension survives its first restoration.

func suspension() Suspension {
	return Suspension{
		Yield:        "outcome/yield",
		Reentry:      "outcome/normal",
		Source:       ReentryByCall,
		Multiplicity: ReentryOnce,
	}
}

// TestSuspensionRoundTripsThroughItsOwnBytes states the format from both ends:
// what it writes it reads back as the same relation.
func TestSuspensionRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, row := range []Suspension{
		suspension(),
		{Yield: "outcome/yield", Reentry: "outcome/normal", Source: ReentryByProvider, Multiplicity: ReentryOnce},
		{Yield: "outcome/cancel", Reentry: "outcome/return", Source: ReentryByCall, Multiplicity: ReentryMany},
	} {
		data, err := EncodeSuspension(row)
		if err != nil {
			t.Fatalf("%+v: the suspension did not encode: %v", row, err)
		}
		got, err := DecodeSuspension(data)
		if err != nil {
			t.Fatalf("%+v: the suspension did not decode: %v", row, err)
		}
		if got != row {
			t.Fatalf("the suspension reads back as %+v, want %+v", got, row)
		}
	}
}

// TestSuspensionNamesTheStructureSurface states which surface an outcome
// reference resolves against. A suspension names two members of the declared
// outcome vocabulary, and the entry identity it derives is that surface's own,
// so a reader never has to guess which table to look in.
func TestSuspensionNamesTheStructureSurface(t *testing.T) {
	const outcome schema.Key = "outcome/yield"
	if SuspensionOutcomeEntryID(outcome) != schema.NewEntryID(schema.SurfaceKindStructure, outcome) {
		t.Fatal("a suspension derives an identity that is not the structure surface's own")
	}
	if SuspensionOutcomeEntryID("").Available() {
		t.Fatal("a suspension that names no outcome derived an entry identity")
	}
}

// TestSuspensionRejectsWhatItCannotCarry is the boundary law. A relation with
// one end unnamed relates nothing, a relation whose two ends are one outcome
// leaves and re-enters at the same case, and a policy outside the closed
// vocabulary is an authority nothing declares.
func TestSuspensionRejectsWhatItCannotCarry(t *testing.T) {
	for name, row := range map[string]Suspension{
		"no yield outcome":   {Reentry: "outcome/normal", Source: ReentryByCall, Multiplicity: ReentryOnce},
		"no reentry outcome": {Yield: "outcome/yield", Source: ReentryByCall, Multiplicity: ReentryOnce},
		"one outcome twice":  {Yield: "outcome/yield", Reentry: "outcome/yield", Source: ReentryByCall, Multiplicity: ReentryOnce},
		"no source":          {Yield: "outcome/yield", Reentry: "outcome/normal", Multiplicity: ReentryOnce},
		"no multiplicity":    {Yield: "outcome/yield", Reentry: "outcome/normal", Source: ReentryByCall},
		"undeclared source":  {Yield: "outcome/yield", Reentry: "outcome/normal", Source: ReentrySource(9), Multiplicity: ReentryOnce},
	} {
		if row.Available() {
			t.Fatalf("%s: an unavailable suspension claims to be one", name)
		}
		if _, err := EncodeSuspension(row); err == nil {
			t.Fatalf("%s: encoded as a payload", name)
		}
	}
	if _, err := DecodeSuspension(nil); err == nil {
		t.Fatal("an empty payload decoded as a suspension")
	}
	delegation, err := EncodeRuleDelegation("call-dispatch")
	if err != nil {
		t.Fatalf("the delegation did not encode: %v", err)
	}
	if _, err := DecodeSuspension(delegation); err == nil {
		t.Fatal("a rule delegation payload decoded as a suspension")
	}
	data, err := EncodeSuspension(suspension())
	if err != nil {
		t.Fatalf("the suspension did not encode: %v", err)
	}
	if _, err := DecodeRuleDelegation(data); err == nil {
		t.Fatal("a suspension payload decoded as a rule delegation")
	}
	if _, err := DecodeSuspension(data[:len(data)-1]); err == nil {
		t.Fatal("a truncated suspension decoded as a relation")
	}
	if _, err := DecodeSuspension(append(append([]byte(nil), data...), 0)); err == nil {
		t.Fatal("a suspension with trailing bytes decoded as a relation")
	}
}

// TestSuspensionPayloadWireIsPinned holds the payload bytes still. A suspension
// published in a shipped contract is read by another build, so its encoding is a
// compatibility surface and not an implementation detail.
func TestSuspensionPayloadWireIsPinned(t *testing.T) {
	const pinned = "0124616e616c797369732f6c6962726172792f636f6e74726163742f73757370656e73696f6e020101080d6f7574636f6d652f7969656c64080e6f7574636f6d652f6e6f726d616c050101050101"
	data, err := EncodeSuspension(suspension())
	if err != nil {
		t.Fatalf("the suspension did not encode: %v", err)
	}
	if got := hex.EncodeToString(data); got != pinned {
		t.Errorf("payload wire is %s, pinned %s", got, pinned)
	}
}
