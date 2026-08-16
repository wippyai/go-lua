package link

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/lua/semantics/exactkey"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func TestLinkValuesAreCanonicalShardScopedAndComplete(t *testing.T) {
	left := source(t, `return 1, "same"`)
	right := source(t, `return 2, "same"`)
	l := linked(t, contract(t), linkproject.Module{Name: "left", Program: left}, linkproject.Module{Name: "right", Program: right})
	leftShard, rightShard := onlyShard(t, l, left), onlyShard(t, l, right)
	leftInteger, _, _, _ := left.Source().Literals().Integers().At(0)
	rightInteger, _, _, _ := right.Source().Literals().Integers().At(0)
	if leftInteger != rightInteger {
		t.Fatal("fixture must reuse raw Program terms")
	}
	values := l.Boundary().Values()
	leftProjectShard := leftShard
	rightProjectShard := rightShard
	_, leftShardOK := l.Project().Mounts().Index(leftProjectShard)
	_, rightShardOK := l.Project().Mounts().Index(rightProjectShard)
	leftValue, ok := values.Of(leftProjectShard, leftInteger)
	if !leftShardOK || !ok {
		t.Fatal("left integer missing Link Value")
	}
	rightValue, ok := values.Of(rightProjectShard, rightInteger)
	if !rightShardOK || !ok || leftValue == rightValue {
		t.Fatal("same raw term crossed a shard boundary")
	}
	if shard, term, ok := values.Origin(leftValue); !ok || shard != leftProjectShard || term != leftInteger {
		t.Fatalf("left occurrence = %v/%d/%t", shard, term, ok)
	}
	seen := make(map[linkboundary.Value]struct{}, values.Count())
	for index := 0; index < values.Count(); index++ {
		value, ok := values.At(index)
		if !ok {
			t.Fatalf("ValueAt(%d)=%v/%t", index, value, ok)
		}
		if _, duplicate := seen[value]; duplicate {
			t.Fatalf("duplicate Value %v", value)
		}
		seen[value] = struct{}{}
		shard, term, ok := values.Origin(value)
		if !ok {
			t.Fatalf("ValueOrigin(%v) absent", value)
		}
		if got, ok := values.Of(shard, term); !ok || got != value {
			t.Fatalf("Value roundtrip=%v/%t, want %v", got, ok, value)
		}
	}
	if values.Count() == 0 {
		t.Fatal("Link omitted all source value occurrences")
	}
	if _, ok := values.Of(leftProjectShard, 0); ok {
		t.Fatal("zero Term became a Value")
	}
}

func TestLinkValueRejectsForeignSameOrdinal(t *testing.T) {
	p := source(t, `return 1`)
	left := linked(t, contract(t), linkproject.Module{Name: "main", Program: p})
	right := linked(t, contract(t), linkproject.Module{Name: "main", Program: p})
	local, ok := left.Boundary().Values().At(0)
	if !ok {
		t.Fatal("left first Value absent")
	}
	foreign, ok := right.Boundary().Values().At(0)
	if !ok {
		t.Fatal("right first Value absent")
	}
	if local == foreign {
		t.Fatal("same ordinal Values from independent Links compare equal")
	}
	if _, _, ok := left.Boundary().Values().Origin(foreign); ok {
		t.Fatal("foreign same-ordinal Value passed origin validation")
	}
}

func TestLinkValueFamilyOrderIsThePublishedLaw(t *testing.T) {
	p := source(t, `local x = -1; return x, true, "s"`)
	l := linked(t, contract(t), linkproject.Module{Name: "main", Program: p})
	shard := onlyShard(t, l, p)
	want := canonicalValueTerms(t, p)
	values := l.Boundary().Values()
	projectShard := shard
	_, shardOK := l.Project().Mounts().Index(projectShard)
	if !shardOK || values.Count() != len(want) {
		t.Fatalf("ValueCount=%d want %d", values.Count(), len(want))
	}
	for index, term := range want {
		value, ok := values.At(index)
		if !ok {
			t.Fatalf("ValueAt(%d) absent", index)
		}
		gotShard, got, ok := values.Origin(value)
		if !ok || gotShard != projectShard || got != term {
			t.Fatalf("ValueAt(%d) occurrence=%v/%d/%t, want %v/%d", index, gotShard, got, ok, projectShard, term)
		}
	}
}

func TestLinkKeyDeduplicatesExactLuaKeysAndTargetPaths(t *testing.T) {
	c := contract(t, target.BindingSpec{Namespace: target.BindingModule, Owner: []string{"math"}, Member: []string{"abs"}})
	left := source(t, `local x = { [1] = true, math = 1 }; return math.abs(x)`)
	right := source(t, `local x = { [1] = false, math = 2 }; return math.abs(x)`)
	l := linked(t, c, linkproject.Module{Name: "left", Program: left}, linkproject.Module{Name: "right", Program: right})
	keys := l.Project().Keys()
	leftShard, rightShard := onlyShard(t, l, left), onlyShard(t, l, right)
	leftMath := exactStringKey(t, left, "math")
	rightMath := exactStringKey(t, right, "math")
	leftProjectShard := leftShard
	rightProjectShard := rightShard
	_, leftProjectOK := l.Project().Mounts().Index(leftProjectShard)
	_, rightProjectOK := l.Project().Mounts().Index(rightProjectShard)
	first, ok := keys.ForProgram(leftProjectShard, left, leftMath)
	if !leftProjectOK || !rightProjectOK || !ok {
		t.Fatal("left exact key missing")
	}
	second, ok := keys.ForProgram(rightProjectShard, right, rightMath)
	if !ok || first != second {
		t.Fatalf("equal string keys=%v/%v", first, second)
	}
	var targetMath target.ExactKey
	for index := 0; index < c.ExactKeyCount(); index++ {
		key, ok := c.ExactKeyAt(index)
		if !ok {
			t.Fatal("malformed Target exact key table")
		}
		value, ok := c.ExactKeyValue(key)
		if ok && value.Kind == keyspace.LiteralString && value.String == "math" {
			targetMath = key
			break
		}
	}
	fromTargetMath, ok := keys.ForTarget(c, targetMath)
	if targetMath == 0 || !ok || fromTargetMath != first {
		t.Fatalf("Target math key=%v/%t, want Program canonical key %v", fromTargetMath, ok, first)
	}
	firstIndex, firstIndexOK := keys.Index(first)
	decoded := artifactAssertProjectionRoundTrip(t, l, c, left, right)
	got, ok := decoded.Project().Keys().ForTarget(c, targetMath)
	gotIndex, gotIndexOK := decoded.Project().Keys().Index(got)
	if !ok || !firstIndexOK || !gotIndexOK || gotIndex != firstIndex {
		t.Fatalf("artifact Target math key=%v/%t, want %v", got, ok, first)
	}
	fromTarget, ok := findProjectKey(keys, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "abs"})
	if !ok {
		t.Fatal("target path key missing")
	}
	if value, ok := keys.Exact(fromTarget); !ok || value.Kind != keyspace.LiteralString || value.String != "abs" {
		t.Fatal("target path Key did not roundtrip")
	}
	if _, ok := findProjectKey(keys, keyspace.LiteralValue{}); ok {
		t.Fatal("nil key acquired identity")
	}
	if _, ok := findProjectKey(keys, keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.NaN())}); ok {
		t.Fatal("NaN key acquired identity")
	}
	seen := make(map[keyspace.LiteralValue]struct{}, keys.Count())
	for index := 0; index < keys.Count(); index++ {
		key, ok := keys.At(index)
		if !ok {
			t.Fatalf("KeyAt(%d) absent", index)
		}
		value, ok := keys.Exact(key)
		if !ok {
			t.Fatalf("ExactKey(%v) absent", key)
		}
		if _, duplicate := seen[value]; duplicate {
			t.Fatalf("duplicate exact payload %#v", value)
		}
		seen[value] = struct{}{}
		got, ok := findProjectKey(keys, value)
		if !ok || got != key {
			t.Fatalf("Key roundtrip=%v/%t, want %v", got, ok, key)
		}
	}
}

func findProjectKey(keys linkproject.Keys, literal keyspace.LiteralValue) (linkproject.Key, bool) {
	normalized, ok := exactkey.Normalize(literal)
	if !ok {
		return linkproject.Key{}, false
	}
	for index := 0; index < keys.Count(); index++ {
		key, present := keys.At(index)
		value, exact := keys.Exact(key)
		if !present || !exact {
			return linkproject.Key{}, false
		}
		order, comparable := exactkey.Compare(normalized, value)
		if comparable && order == 0 {
			return key, true
		}
	}
	return linkproject.Key{}, false
}

func TestLinkKeyNormalizesLuaNumericEquality(t *testing.T) {
	integer, ok := exactkey.Normalize(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1})
	if !ok {
		t.Fatal("integer key rejected")
	}
	floating, ok := exactkey.Normalize(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1.0)})
	order, ordered := exactkey.Compare(integer, floating)
	if !ok || !ordered || order != 0 {
		t.Fatal("1 and 1.0 did not normalize together")
	}
	zero, ok := exactkey.Normalize(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 0})
	if !ok {
		t.Fatal("zero key rejected")
	}
	negativeZero, ok := exactkey.Normalize(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Copysign(0, -1))})
	order, ordered = exactkey.Compare(zero, negativeZero)
	if !ok || !ordered || order != 0 {
		t.Fatal("-0 and 0 did not normalize together")
	}
	infinite, ok := exactkey.Normalize(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Inf(1))})
	if !ok || infinite.Kind != keyspace.LiteralFloat {
		t.Fatal("infinite float key lost its exact identity")
	}
	if _, ok := exactkey.Normalize(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.NaN())}); ok {
		t.Fatal("NaN key normalized")
	}
}

func canonicalValueTerms(t testing.TB, p *program.Program) []keyspace.Term {
	t.Helper()
	result := make([]keyspace.Term, 0)
	appendFamily := func(count int, at func(int) (keyspace.Term, bool)) {
		for index := 0; index < count; index++ {
			term, ok := at(index)
			if !ok {
				t.Fatalf("malformed Value family at %d", index)
			}
			result = append(result, term)
		}
	}
	authored := p.Flow().Authored()
	literals := p.Source().Literals()
	storage := authored.Storage()
	outcomes := p.Flow().Outcomes()
	for _, family := range []struct {
		count int
		at    func(int) (keyspace.Term, bool)
	}{
		{literals.Nils().Count(), func(index int) (keyspace.Term, bool) { term, _, ok := literals.Nils().At(index); return term, ok }},
		{literals.Bools().Count(), func(index int) (keyspace.Term, bool) { term, _, _, ok := literals.Bools().At(index); return term, ok }},
		{literals.Integers().Count(), func(index int) (keyspace.Term, bool) {
			term, _, _, ok := literals.Integers().At(index)
			return term, ok
		}},
		{literals.Floats().Count(), func(index int) (keyspace.Term, bool) { term, _, _, ok := literals.Floats().At(index); return term, ok }},
		{literals.Strings().Count(), func(index int) (keyspace.Term, bool) { term, _, _, ok := literals.Strings().At(index); return term, ok }},
		{storage.Reads().Count(), storage.Reads().At}, {storage.Varargs().Count(), storage.Varargs().At},
		{authored.Operators().Unaries().Count(), authored.Operators().Unaries().At}, {authored.Operators().Binaries().Count(), authored.Operators().Binaries().At},
		{authored.Operators().Selects().Count(), authored.Operators().Selects().At}, {authored.Functions().Count(), authored.Functions().At},
		{authored.Calls().Count(), authored.Calls().At}, {authored.Tables().Count(), authored.Tables().At},
		{authored.TypeValues().Count(), authored.TypeValues().At}, {authored.Claims().Count(), authored.Claims().At},
		{authored.Values().Count(), authored.Values().At}, {storage.Cells().Count(), storage.Cells().At},
	} {
		appendFamily(family.count, family.at)
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		for index := 0; index < outcomes.Count(); index++ {
			term, ok := outcomes.At(index)
			if !ok {
				t.Fatal("malformed Outcome family")
			}
			outcome, ok := outcomes.Get(term)
			if !ok {
				t.Fatal("malformed Outcome")
			}
			if outcome.Kind == kind {
				result = append(result, term)
			}
		}
	}
	return result
}
