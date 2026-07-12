package program

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestFreezeCanonicalizesProgramAndDetachesInput(t *testing.T) {
	left := validSpec()
	right := validSpec()
	slices.Reverse(right.Members)
	slices.Reverse(right.Blocks)
	slices.Reverse(right.Edges)
	slices.Reverse(right.Observations)
	slices.Reverse(right.CallSCC.Members)
	slices.Reverse(right.Routes[0].Known)

	a := mustFreeze(t, left)
	b := mustFreeze(t, right)
	if a.Digest() != b.Digest() || !bytes.Equal(a.CanonicalBytes(), b.CanonicalBytes()) {
		t.Fatal("permutation changed canonical Program")
	}
	left.Blocks[0].Transactions[0].Digest[0] = 9
	left.Loops[0].Blocks[0] = 99
	left.Routes[0].Known[0].Guard = "mutated"
	if a.Blocks()[0].Transactions[0].Digest[0] != 1 || a.Loops()[0].Blocks[0] != 1 || a.Routes()[0].Known[0].Guard == "mutated" {
		t.Fatal("Program aliases caller storage")
	}
	encoded := a.CanonicalBytes()
	encoded[0] ^= 0xff
	if bytes.Equal(encoded, a.CanonicalBytes()) {
		t.Fatal("CanonicalBytes aliases Program storage")
	}
}

func TestRejectsDanglingAndDuplicateReferences(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Spec)
		want string
	}{
		{"duplicate block", func(s *Spec) { s.Blocks = append(s.Blocks, s.Blocks[0]) }, "duplicate ID"},
		{"dangling edge", func(s *Spec) { s.Edges[0].To = 99 }, "unknown target"},
		{"dangling transaction", func(s *Spec) { s.Blocks[0].Transactions[0].Digest[0] = 8 }, "undeclared transaction"},
		{"duplicate observation", func(s *Spec) { s.Observations = append(s.Observations, s.Observations[0]) }, "duplicate ID"},
		{"unknown observation block", func(s *Spec) { s.Observations[0].At = 99 }, "unknown block"},
		{"duplicate member", func(s *Spec) {
			s.Members = append(s.Members, "alpha")
			s.CallSCC.Members = append(s.CallSCC.Members, "alpha")
		}, "duplicate ID"},
		{"duplicate region", func(s *Spec) { s.Loops[0].ID = s.CallSCC.ID }, "duplicate ID"},
		{"prose edge guard", func(s *Spec) { s.Edges[0].Guard = "arbitrary prose" }, "invalid guard ID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validSpec()
			test.edit(&spec)
			_, err := Freeze(spec)
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRejectsInvalidLoopAndSCCOwnership(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Spec)
	}{
		{"foreign loop block", func(s *Spec) { s.Loops[0].Blocks = append(s.Loops[0].Blocks, 3) }},
		{"entry outside loop", func(s *Spec) { s.Loops[0].Entry = 3 }},
		{"unknown loop owner", func(s *Spec) { s.Loops[0].Owner = "missing" }},
		{"wrong SCC", func(s *Spec) { s.Loops[0].SCC = 99 }},
		{"incomplete SCC members", func(s *Spec) { s.CallSCC.Members = s.CallSCC.Members[:1] }},
		{"invalid parent", func(s *Spec) { s.Loops[0].Parent = 99 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validSpec()
			test.edit(&spec)
			if _, err := Freeze(spec); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestMixedRoutesRequireExplicitResidueAndCompletenessProof(t *testing.T) {
	tests := []struct {
		name string
		edit func(*MixedTargetRoute)
		want string
	}{
		{"complete without proof", func(r *MixedTargetRoute) { r.Proof = [32]byte{} }, "require proof"},
		{"open with proof", func(r *MixedTargetRoute) { r.Completeness = TargetsOpen }, "cannot claim"},
		{"empty route", func(r *MixedTargetRoute) { r.Known = nil; r.Residue = TargetResidue{} }, "no targets"},
		{"unknown known member", func(r *MixedTargetRoute) { r.Known[0].Member = "missing" }, "invalid known"},
		{"duplicate known target", func(r *MixedTargetRoute) { r.Known = append(r.Known, r.Known[0]) }, "duplicate known"},
		{"prose known guard", func(r *MixedTargetRoute) { r.Known[0].Guard = "arbitrary prose" }, "invalid known"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validSpec()
			test.edit(&spec.Routes[0])
			_, err := Freeze(spec)
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCanonicalCodecCommitsNodeKindsAndRouteResidue(t *testing.T) {
	base := mustFreeze(t, validSpec())
	changed := validSpec()
	changed.Routes[0].Residue.Native = true
	withNative := mustFreeze(t, changed)
	if base.Digest() == withNative.Digest() {
		t.Fatal("native residue did not affect digest")
	}
	changed = validSpec()
	changed.Loops = nil
	withoutLoop := mustFreeze(t, changed)
	if base.Digest() == withoutLoop.Digest() {
		t.Fatal("LoopMu did not affect digest")
	}
	if !bytes.Contains(base.CanonicalBytes(), []byte(Schema)) {
		t.Fatal("codec does not commit schema")
	}
}

func validSpec() Spec {
	var tx TransactionDigest
	tx[0] = 1
	var proof [32]byte
	proof[0] = 7
	return Spec{
		Entry:        1,
		Members:      []MemberID{"alpha", "beta"},
		Transactions: []TransactionRef{{Digest: tx}},
		Blocks:       []Block{{ID: 1, Member: "alpha", Transactions: []TransactionRef{{Digest: tx}}}, {ID: 2, Member: "alpha"}, {ID: 3, Member: "beta"}},
		Edges:        []Edge{{From: 1, To: 2, Guard: "always"}, {From: 2, To: 3, Guard: "call"}},
		Observations: []ObservationSlot{{ID: 1, At: 2, Kind: ObserveResult, Schema: "result.v1"}},
		CallSCC:      CallSCCMu{ID: 10, Members: []MemberID{"alpha", "beta"}},
		Loops:        []LoopMu{{ID: 11, SCC: 10, Owner: "alpha", Entry: 1, Blocks: []BlockID{1, 2}}},
		Routes:       []MixedTargetRoute{{At: 2, Known: []KnownTarget{{Guard: "is-beta", Member: "beta"}}, Residue: TargetResidue{Unknown: true}, Completeness: TargetsComplete, Proof: proof}},
	}
}

func mustFreeze(t *testing.T, spec Spec) Program {
	t.Helper()
	got, err := Freeze(spec)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
