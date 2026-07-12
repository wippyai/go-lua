package circuit_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/circuit"
)

func TestUncertifiedNonDistributiveBindingsRemainSeparate(t *testing.T) {
	domain, key := testDomain(t, 4, nil)
	// These encode correlated outcomes: (x=number,y=string) and
	// (x=string,y=number). Merging bindings would invent impossible pairings.
	left := disjunct(t, []string{"call-a"}, []string{"route-left"}, []string{"alias-x"}, "x-number:y-string")
	right := disjunct(t, []string{"call-b"}, []string{"route-right"}, []string{"alias-y"}, "x-string:y-number")
	lcell, _ := domain.Singleton(key, left)
	rcell, _ := domain.Singleton(key, right)
	joined, stats, err := domain.Join(lcell, rcell)
	if err != nil {
		t.Fatal(err)
	}
	if got := joined.Disjuncts(); len(got) != 2 || stats.CertifiedMerges != 0 || joined.PrecisionLost() {
		t.Fatalf("uncertified correlated join = %#v stats=%#v", got, stats)
	}
	if _, _, err := domain.Join(lcell, rcell, circuit.ExactMergeCertificate{}); !errors.Is(err, circuit.ErrCertificate) {
		t.Fatalf("zero/forged certificate accepted: %v", err)
	}
}

func TestExactMergeRequiresAuthorityCertificateAndUnionGuards(t *testing.T) {
	left := disjunct(t, []string{"a"}, []string{"p1"}, []string{"x"}, "left")
	right := disjunct(t, []string{"b"}, []string{"p2"}, []string{"y"}, "right")
	merged := disjunct(t, []string{"b", "a"}, []string{"p2", "p1"}, []string{"y", "x"}, "equivalent")
	authority, err := circuit.NewExactMergeAuthority(func(claim circuit.MergeClaim) bool { return claim.Merged().Binding() == "equivalent" })
	if err != nil {
		t.Fatal(err)
	}
	domain, key := testDomain(t, 1, authority)
	lcell, _ := domain.Singleton(key, left)
	rcell, _ := domain.Singleton(key, right)
	if _, _, err := domain.Join(lcell, rcell); !errors.Is(err, circuit.ErrWidenRequired) {
		t.Fatalf("uncertified bound merge=%v", err)
	}
	certificate, err := domain.CertifyExactMerge(circuit.NewMergeClaim(left, right, merged))
	if err != nil {
		t.Fatal(err)
	}
	joined, stats, err := domain.Join(lcell, rcell, certificate)
	if err != nil {
		t.Fatal(err)
	}
	if len(joined.Disjuncts()) != 1 || joined.Disjuncts()[0].Binding() != "equivalent" || stats.CertifiedMerges != 1 {
		t.Fatalf("certified join=%#v/%#v", joined.Disjuncts(), stats)
	}
	foreign, err := circuit.NewExactMergeAuthority(func(circuit.MergeClaim) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	foreignDomain, _ := testDomain(t, 1, foreign)
	if _, _, err := foreignDomain.Join(lcell, rcell, certificate); !errors.Is(err, circuit.ErrCertificate) {
		t.Fatalf("foreign authority accepted certificate: %v", err)
	}
}

func TestBoundedWideningRetainsUnionGuardsAndMarksLoss(t *testing.T) {
	domain, key := testDomain(t, 2, nil)
	inputs := []circuit.Disjunct{
		disjunct(t, []string{"a"}, []string{"p1"}, []string{"x"}, "one"),
		disjunct(t, []string{"b"}, []string{"p2"}, []string{"y"}, "two"),
		disjunct(t, []string{"c"}, []string{"p3"}, []string{"z"}, "three"),
	}
	first, _ := domain.Singleton(key, inputs[0])
	second, _ := domain.Singleton(key, inputs[1])
	third, _ := domain.Singleton(key, inputs[2])
	pair, _, err := domain.Join(first, second)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := domain.Join(pair, third); !errors.Is(err, circuit.ErrWidenRequired) {
		t.Fatalf("unbounded join=%v", err)
	}
	widened, stats, err := domain.Widen(pair, third)
	if err != nil {
		t.Fatal(err)
	}
	if !widened.PrecisionLost() || len(widened.Disjuncts()) != 1 || !stats.PrecisionLost {
		t.Fatalf("widened=%#v stats=%#v", widened, stats)
	}
	d := widened.Disjuncts()[0]
	if d.Binding() != "binding-top" || !slices.Equal(d.ApplicationGuards().IDs(), guards("a", "b", "c")) || !slices.Equal(d.ProvenanceGuards().IDs(), guards("p1", "p2", "p3")) || !slices.Equal(d.AliasGuards().IDs(), guards("x", "y", "z")) {
		t.Fatalf("widened disjunct=%#v", d)
	}
}

func TestDeterministicOrderCodecPolicyAndFiniteRankVerification(t *testing.T) {
	domain, key := testDomain(t, 2, nil)
	a := disjunct(t, []string{"a"}, []string{"p1"}, []string{"x"}, "one")
	b := disjunct(t, []string{"b"}, []string{"p2"}, []string{"y"}, "two")
	c := disjunct(t, []string{"c"}, []string{"p3"}, []string{"z"}, "three")
	ac, _ := domain.Singleton(key, a)
	bc, _ := domain.Singleton(key, b)
	left, _, _ := domain.Join(ac, bc)
	right, _, _ := domain.Join(bc, ac)
	if !bytes.Equal(left.CanonicalBytes(), right.CanonicalBytes()) {
		t.Fatal("join/codec depends on input order")
	}
	if err := circuit.VerifyFiniteSamples(domain, key, []circuit.Disjunct{a, b, c}); err != nil {
		t.Fatal(err)
	}

	p1, err := circuit.NewBindingPartitionPolicy(7, []circuit.ClassID{"b", "a"}, []circuit.ClassID{"target"}, []circuit.ClassID{"prov"}, []circuit.ClassID{"alias"})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := circuit.NewBindingPartitionPolicy(7, []circuit.ClassID{"a", "b"}, []circuit.ClassID{"target"}, []circuit.ClassID{"prov"}, []circuit.ClassID{"alias"})
	if err != nil {
		t.Fatal(err)
	}
	if p1.CellCount() != 2 || !bytes.Equal(p1.CanonicalBytes(), p2.CanonicalBytes()) {
		t.Fatal("partition policy is not finite/deterministic")
	}
	precision1, err := circuit.NewPrecisionPolicy(5, 2, "binding-top", guards("c", "a", "b"), guards("p3", "p1", "p2"), guards("z", "x", "y"))
	if err != nil {
		t.Fatal(err)
	}
	precision2, err := circuit.NewPrecisionPolicy(5, 2, "binding-top", guards("a", "b", "c"), guards("p1", "p2", "p3"), guards("x", "y", "z"))
	if err != nil {
		t.Fatal(err)
	}
	if precision1.GuardVocabularySize() != 9 || !bytes.Equal(precision1.CanonicalBytes(), precision2.CanonicalBytes()) {
		t.Fatal("precision guard vocabulary is not finite/deterministic")
	}
}

func TestOutOfVocabularyGuardIsRejectedAtCellIngress(t *testing.T) {
	authority, err := circuit.NewExactMergeAuthority(func(circuit.MergeClaim) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	domain, key := testDomain(t, 2, authority)
	outside := disjunct(t, []string{"outside"}, []string{"p1"}, []string{"x"}, "one")
	if _, err := domain.Singleton(key, outside); !errors.Is(err, circuit.ErrInvalidCell) {
		t.Fatalf("outside guard accepted: %v", err)
	}
	inside := disjunct(t, []string{"a"}, []string{"p1"}, []string{"x"}, "two")
	merged := disjunct(t, []string{"a", "outside"}, []string{"p1"}, []string{"x"}, "merged")
	if _, err := domain.CertifyExactMerge(circuit.NewMergeClaim(outside, inside, merged)); !errors.Is(err, circuit.ErrCertificate) {
		t.Fatalf("outside guard certificate accepted: %v", err)
	}
}

func testDomain(t testing.TB, max uint16, authority *circuit.ExactMergeAuthority) (*circuit.Domain, circuit.CellKey) {
	t.Helper()
	policy, err := circuit.NewBindingPartitionPolicy(3, []circuit.ClassID{"apply"}, []circuit.ClassID{"target"}, []circuit.ClassID{"prov"}, []circuit.ClassID{"alias"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := policy.Partition(circuit.PartitionInput{Application: "apply", Target: "target", Provenance: "prov", Alias: "alias"})
	if err != nil {
		t.Fatal(err)
	}
	precision, err := circuit.NewPrecisionPolicy(5, max, "binding-top",
		guards("a", "b", "c", "call-a", "call-b"),
		guards("p1", "p2", "p3", "route-left", "route-right"),
		guards("x", "y", "z", "alias-x", "alias-y"))
	if err != nil {
		t.Fatal(err)
	}
	domain, err := circuit.NewBindingDomain(policy, precision, authority, func(w circuit.BindingID, members []circuit.BindingID) bool {
		return w == "binding-top" && len(members) > 0
	})
	if err != nil {
		t.Fatal(err)
	}
	return domain, key
}
func disjunct(t testing.TB, a, p, l []string, b string) circuit.Disjunct {
	t.Helper()
	app := guardSet(t, a)
	prov := guardSet(t, p)
	alias := guardSet(t, l)
	d, err := circuit.NewDisjunct(app, prov, alias, circuit.BindingID(b))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func guardSet(t testing.TB, ids []string) circuit.GuardSet {
	t.Helper()
	typed := make([]circuit.GuardID, len(ids))
	for i, id := range ids {
		typed[i] = circuit.GuardID(id)
	}
	g, err := circuit.NewGuardSet(typed...)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func guards(ids ...string) []circuit.GuardID {
	out := make([]circuit.GuardID, len(ids))
	for i, id := range ids {
		out[i] = circuit.GuardID(id)
	}
	return out
}
