package claim

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestValueClaimRulePreservesRuntimeIdentityAndStaticTargetAuthority(t *testing.T) {
	schema, source := claimSchema(t)
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, claimKey(1), claimKey(900_001), schema)
	if !ok {
		t.Fatal("Value owner")
	}
	rule, ok := Declare(composition, claimKey(2), claimKey(3), claimKey(4), owner)
	if !ok || rule == nil {
		t.Fatal("ValueClaim Rule")
	}
	if evidence, admitted := rule.check(engine.RuleDerivation[value.Value, value.ValueClaim]{}); admitted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged ValueClaim derivation produced evidence")
	}
	if !declareClaimQuery(composition, owner) || !composition.Seal() {
		t.Fatal("claim composition seal")
	}
	withTarget, withoutTarget := 0, 0
	for index, raw := range valueClaimTerms(source) {
		claim, ok := schema.ValueClaim(raw.shard, raw.term)
		if !ok {
			t.Fatalf("ValueClaim(%d)", index)
		}
		if instance, ok := rule.Instance(claim); !ok || instance == nil {
			t.Fatalf("ValueClaim instance %d", index)
		}
		kind, ok := claim.Kind()
		if !ok {
			t.Fatal("claim kind")
		}
		_, static := claim.StaticTarget()
		if static != (kind == flowkind.ValueClaimTypeAs || kind == flowkind.ValueClaimTypeColonColon) {
			t.Fatalf("claim %d static target preservation", index)
		}
		if static {
			withTarget++
		} else {
			withoutTarget++
		}
	}
	if withTarget != 2 || withoutTarget != 1 {
		t.Fatalf("claim denominator target split = %d/%d", withTarget, withoutTarget)
	}
}

type valueClaimTerm struct {
	shard linkproject.Shard
	term  keyspace.Term
}

func valueClaimTerms(source *link.Link) []valueClaimTerm {
	var terms []valueClaimTerm
	for index := 0; index < source.Project().Mounts().Count(); index++ {
		shard, _ := source.Project().Mounts().At(index)
		p, _ := source.Project().Mounts().Program(shard)
		claims := p.Flow().Authored().Claims()
		for at := 0; at < claims.Count(); at++ {
			term, _ := claims.At(at)
			if p.Flow().Executable().Contains(term) {
				terms = append(terms, valueClaimTerm{shard: shard, term: term})
			}
		}
	}
	return terms
}

func claimSchema(t testing.TB) (*value.Schema, *link.Link) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "claim_value.lua", Text: []byte("local value = 'x'\nlocal a = value as string\nlocal b = value :: string\nlocal c = value!\nreturn a, b, c\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(source)
	schema, ok := value.Seal(source, heaps)
	if !heapsOK || !ok {
		t.Fatal("Value schema")
	}
	return schema, source
}

func declareClaimQuery(composition *engine.Composition, owner *valueowner.Owner) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: claimKey(90), Project: func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{Semantic: claimKey(91), Freeze: func(v bool) bool { return v }, Clone: func(v bool) bool { return v }, Equal: func(a, b bool) bool { return a == b }, Fingerprint: func(v bool) uint64 {
			if v {
				return 1
			}
			return 0
		}},
	}, func(query *engine.Query[bool]) bool {
		_, ok := engine.QueryReadFrom(query, owner.ExactRead())
		return ok
	})
	return ok && query != nil
}

func claimKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("claim semantic key")
	}
	return key
}
