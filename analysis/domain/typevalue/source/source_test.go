package source

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	typevaluedomain "github.com/wippyai/go-lua/analysis/domain/typevalue"
	typevalueowner "github.com/wippyai/go-lua/analysis/domain/typevalue/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestSeedResultUsesOnlyTheBinderAuthorizedAuthoritySeed proves the source
// reduction preserves Link's exact marked call-base identity.  In particular,
// a same-content Authority's seed cannot be used to construct this owner's
// source fact.
func TestSeedResultUsesOnlyTheBinderAuthorizedAuthoritySeed(t *testing.T) {
	authority := sourceAuthority(t)
	seed, ok := authority.SeedAt(0)
	if !ok {
		t.Fatal("binder-authorized seed")
	}
	root, got, ok := sourceResult(authority, seed)
	if !ok {
		t.Fatal("source result")
	}
	wantRoot, want, ok := authority.SeedValue(seed)
	if !ok || root != wantRoot || !authority.Equal(got, want) {
		t.Fatal("source reduction changed the sealed TypeValue interpretation")
	}
	seedID, idOK := authority.SeedID(seed)
	_, digest, ok := func(seed typevaluedomain.Seed) (typevaluedomain.Seed, [32]byte, bool) {
		id, ok := authority.SeedID(seed)
		return seed, [32]byte(id), ok && [32]byte(id) != [32]byte{}
	}(seed)
	if !ok || !idOK || digest != [32]byte(seedID) {
		t.Fatal("source operand identity did not preserve the Link Value identity")
	}

	foreignAuthority := sourceAuthority(t)
	foreign, ok := foreignAuthority.SeedAt(0)
	if !ok {
		t.Fatal("foreign seed")
	}
	if _, _, ok := sourceResult(authority, foreign); ok {
		t.Fatal("same-content foreign Authority seed crossed the owner fence")
	}
	if _, _, ok := sourceResult(authority, typevaluedomain.Seed{}); ok {
		t.Fatal("zero TypeValue seed acquired source content")
	}
}

func TestDeclareBindsOnlyAuthorityIssuedSeedAndRejectsForgedEvidence(t *testing.T) {
	authority := sourceAuthority(t)
	composition := engine.NewComposition()
	owner, ok := typevalueowner.Declare(composition, typevalueKey(1), authority)
	if !ok {
		t.Fatal("TypeValue owner")
	}
	rule, ok := Declare(composition, typevalueKey(2), typevalueKey(3), typevalueKey(4), owner)
	if !ok || rule == nil {
		t.Fatal("TypeValue source Rule")
	}
	if !declareSourceQuery(composition, owner) || !composition.Seal() {
		t.Fatal("TypeValue source composition seal")
	}
	inventory, ok := composition.RuleAdmissionInventory()
	if !ok || len(inventory.Rules) != 1 || inventory.Rules[0] != (engine.RuleAdmissionRecord{
		Rule: typevalueKey(2), Basis: engine.RuleAdmissionBasisDerivation, Identity: typevalueKey(4),
	}) {
		t.Fatal("TypeValue source admission")
	}
	report, ok := composition.SemanticReport()
	if !ok || len(report.Incidences) != 0 {
		t.Fatal("zero-read TypeValue source acquired a predecessor")
	}

	for index := 0; index < authority.SeedCount(); index++ {
		seed, ok := authority.SeedAt(index)
		if !ok {
			t.Fatalf("SeedAt(%d)", index)
		}
		frozen, digest, frozenOK := seedContent(owner)(seed)
		replayed, replayDigest, replayOK := seedContent(owner)(frozen)
		if !frozenOK || !replayOK || digest == [32]byte{} || digest != replayDigest || replayed != seed {
			t.Fatalf("seed %d operand content is not pure and idempotent", index)
		}
		if instance, ok := rule.Instance(seed); !ok || instance == nil {
			t.Fatalf("seed %d did not bind its exact TypeValue root", index)
		}
	}
	foreignAuthority := sourceAuthority(t)
	foreign, ok := foreignAuthority.SeedAt(0)
	if !ok {
		t.Fatal("foreign seed")
	}
	if instance, ok := rule.Instance(foreign); ok || instance != nil {
		t.Fatal("foreign same-content seed bound a local TypeValue Rule")
	}
	if evidence, accepted := checker(owner, typevalueKey(2))(engine.RuleDerivation[typevaluedomain.Value, typevaluedomain.Seed]{}); accepted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged derivation minted TypeValue source evidence")
	}
}

func declareSourceQuery(composition *engine.Composition, owner *typevalueowner.Owner) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: typevalueKey(5),
		Project:  func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: typevalueKey(6), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value },
			Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		_, declared := engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	return ok && query != nil
}

func sourceAuthority(t testing.TB) *typevaluedomain.Authority {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "typevalue_source_rule.lua", Text: []byte("type Box = { value: number }\nlocal value = Box('payload')\nreturn value\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "typevalue_source_rule", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, ok := heap.Seal(source)
	if !ok {
		t.Fatal("heap authority")
	}
	types, ok := typeauthority.Seal(source)
	if !ok {
		t.Fatal("type authority")
	}
	statics, _, err := staticdomain.Seal(source, types)
	if err != nil {
		t.Fatal(err)
	}
	authority, ok := typevaluedomain.New(statics, heaps)
	if !ok || authority.SeedCount() != 1 {
		t.Fatal("TypeValue authority")
	}
	return authority
}

func typevalueKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("TypeValue source test key")
	}
	return key
}
