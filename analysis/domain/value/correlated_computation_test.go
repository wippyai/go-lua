package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

func TestValueOperandsRebindAndRetainOnlyCanonicalGeometry(t *testing.T) {
	schema, source := correlatedFixture(t, `
local left = false
local right = "right"
local negated = not left
local both = left and right
local either = left or right
local asserted = right as string
local cast = right :: string
local nonnil = right!
return negated, both, either, asserted, cast, nonnil
`, false)
	foreignSchema, foreign := correlatedFixture(t, `return not true, false and "x", "x" as string`, false)

	if len(unaryNotTerms(source)) != 1 || len(selectTerms(source)) != 2 || len(valueClaimTerms(source)) != 3 {
		t.Fatal("fixture missed a direct Value operand denominator")
	}
	for index, raw := range unaryNotTerms(source) {
		operand, ok := schema.UnaryNot(raw.shard, raw.term)
		id, idOK := operand.ID()
		result, input, endpoints := operand.Endpoints()
		if !ok || !idOK || !id.Available() || !endpoints || result == input {
			t.Fatalf("UnaryNot(%d) lost exact Value relation", index)
		}
		if !schema.OwnsUnaryNot(operand) || foreignSchema.OwnsUnaryNot(operand) {
			t.Fatal("foreign Link UnaryNot crossed Value owner")
		}
	}
	for index, raw := range selectTerms(source) {
		for branch := 0; branch < 2; branch++ {
			operand, ok := schema.SelectBranch(raw.shard, raw.term, branch)
			id, idOK := operand.ID()
			result, left, chosen, _, _, endpoints := operand.Endpoints()
			if !ok || !idOK || !id.Available() || !endpoints || result == left || chosen == (Coordinate{}) {
				t.Fatalf("SelectBranch(%d,%d) lost exact geometry", index, branch)
			}
			if !schema.OwnsSelectBranch(operand) || foreignSchema.OwnsSelectBranch(operand) {
				t.Fatal("foreign Link Select crossed Value owner")
			}
		}
	}
	for index, raw := range valueClaimTerms(source) {
		claim, ok := schema.ValueClaim(raw.shard, raw.term)
		id, idOK := claim.ID()
		result, operand, endpoints := claim.Endpoints()
		if !ok || !idOK || !id.Available() || !endpoints || result == operand {
			t.Fatalf("ValueClaim(%d) lost exact identity route", index)
		}
		kind, kindOK := claim.Kind()
		_, staticTarget := claim.StaticTarget()
		if !kindOK || staticTarget != (kind == flowkind.ValueClaimTypeAs || kind == flowkind.ValueClaimTypeColonColon) {
			t.Fatalf("ValueClaim(%d) lost exact Static authority", index)
		}
		if !schema.OwnsValueClaim(claim) || foreignSchema.OwnsValueClaim(claim) {
			t.Fatal("foreign Link ValueClaim crossed Value owner")
		}
	}
	if len(unaryNotTerms(foreign)) == 0 {
		t.Fatal("foreign fixture malformed")
	}
}

type flowTerm struct {
	shard linkproject.Shard
	term  keyspace.Term
}

func unaryNotTerms(source *link.Link) []flowTerm {
	var terms []flowTerm
	for index := 0; index < source.Project().Mounts().Count(); index++ {
		shard, _ := source.Project().Mounts().At(index)
		p, _ := source.Project().Mounts().Program(shard)
		unaries := p.Flow().Authored().Operators().Unaries()
		for at := 0; at < unaries.Count(); at++ {
			term, _ := unaries.At(at)
			_, op, _, ok := unaries.Get(term)
			if ok && p.Flow().Executable().Contains(term) && op == flowkind.UnaryNot {
				terms = append(terms, flowTerm{shard: shard, term: term})
			}
		}
	}
	return terms
}

func selectTerms(source *link.Link) []flowTerm {
	var terms []flowTerm
	for index := 0; index < source.Project().Mounts().Count(); index++ {
		shard, _ := source.Project().Mounts().At(index)
		p, _ := source.Project().Mounts().Program(shard)
		selects := p.Flow().Authored().Operators().Selects()
		for at := 0; at < selects.Count(); at++ {
			term, _ := selects.At(at)
			if p.Flow().Executable().Contains(term) {
				terms = append(terms, flowTerm{shard: shard, term: term})
			}
		}
	}
	return terms
}

func valueClaimTerms(source *link.Link) []flowTerm {
	var terms []flowTerm
	for index := 0; index < source.Project().Mounts().Count(); index++ {
		shard, _ := source.Project().Mounts().At(index)
		p, _ := source.Project().Mounts().Program(shard)
		claims := p.Flow().Authored().Claims()
		for at := 0; at < claims.Count(); at++ {
			term, _ := claims.At(at)
			if p.Flow().Executable().Contains(term) {
				terms = append(terms, flowTerm{shard: shard, term: term})
			}
		}
	}
	return terms
}

func TestTruthTransformsAreExactAndDoNotLeakInputIdentity(t *testing.T) {
	schema, source := correlatedFixture(t, `return nil, false, true, "text"`, false)
	find := func(kind runtimekind.Kind, truth Truth) Value {
		values := source.Boundary().Values()
		for index := 0; index < values.Count(); index++ {
			raw, ok := values.At(index)
			if !ok {
				t.Fatal("ValueAt")
			}
			fact, ok := schema.SourceValue(raw)
			if ok && schema.RuntimeKinds(fact).Contains(kind) && schema.Truthiness(fact) == truth {
				return fact
			}
		}
		t.Fatalf("missing source kind=%d truth=%d", kind, truth)
		return Value{}
	}
	falseValue := find(runtimekind.Boolean, TruthFalse)
	stringValue := find(runtimekind.String, TruthTrue)
	nilValue := find(runtimekind.Nil, TruthFalse)
	joined, ok := schema.Join(falseValue, stringValue)
	if !ok {
		t.Fatal("join")
	}
	not, ok := schema.Not(joined)
	if !ok || schema.RuntimeKinds(not) != runtimekind.Bit(runtimekind.Boolean) || schema.Truthiness(not) != (TruthFalse|TruthTrue) {
		t.Fatal("not lost exact boolean image")
	}
	if filtered, ok := schema.FilterTruth(joined, true); !ok || !schema.Equal(filtered, stringValue) {
		t.Fatal("truthy selection did not retain only the truthy alternative")
	}
	if filtered, ok := schema.FilterTruth(joined, false); !ok || !schema.Equal(filtered, falseValue) {
		t.Fatal("falsy selection did not retain only the falsy alternative")
	}
	if notFalse, ok := schema.Not(falseValue); !ok || schema.RuntimeKinds(notFalse) != runtimekind.Bit(runtimekind.Boolean) || schema.Truthiness(notFalse) != TruthTrue {
		t.Fatal("not false did not produce one truthy boolean")
	}
	if notNil, ok := schema.Not(nilValue); !ok || schema.RuntimeKinds(notNil) != runtimekind.Bit(runtimekind.Boolean) || schema.Truthiness(notNil) != TruthTrue {
		t.Fatal("not nil did not produce one truthy boolean")
	}
	if notTop, ok := schema.Not(schema.Top()); !ok || schema.RuntimeKinds(notTop) != runtimekind.Bit(runtimekind.Boolean) || schema.Truthiness(notTop) != (TruthFalse|TruthTrue) {
		t.Fatal("not top leaked non-boolean alternatives")
	}
}

func TestFilterTruthTopRetainsEveryCapabilityPossibility(t *testing.T) {
	schema, _ := contextualSourceSeedFixture(t)
	if schema.CapabilityCount() == 0 {
		t.Fatal("fixture has no capability denominator")
	}
	for _, truthy := range []bool{false, true} {
		filtered, ok := schema.FilterTruth(schema.Top(), truthy)
		if !ok || schema.Equal(filtered, schema.Bottom()) {
			t.Fatalf("FilterTruth(Top,%t)", truthy)
		}
		atoms, ok := schema.Atoms(filtered)
		if !ok || len(atoms) == 0 {
			t.Fatalf("FilterTruth(Top,%t) atoms", truthy)
		}
		for _, atom := range atoms {
			if (truthy && !atom.Truthiness().MayBeTrue()) || (!truthy && !atom.Truthiness().MayBeFalse()) {
				t.Fatalf("FilterTruth(Top,%t) retained wrong truth atom", truthy)
			}
			for index := 0; index < schema.CapabilityCount(); index++ {
				capability, ok := schema.CapabilityAt(index)
				if !ok || !schema.HasCapability(filtered, atom, capability) {
					t.Fatalf("FilterTruth(Top,%t) dropped capability %d", truthy, index)
				}
			}
		}
	}
}
