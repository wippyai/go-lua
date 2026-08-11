package owner

import (
	"reflect"
	"testing"

	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestEffectOwnerBodyRootFactorLaws(t *testing.T) {
	algebra, foreign, source := effectOwnerFixture(t)
	if algebra.RootCount() == 0 || foreign.RootCount() != algebra.RootCount() || algebra.Link() != foreign.Link() || algebra.Link() != source {
		t.Fatal("body-root fixture did not preserve same-Link root vocabulary")
	}

	composition := engine.NewComposition()
	owner, ok := Declare(composition, effectOwnerSemantic(t, source), effectOwnerSummarySemantic(t, source), algebra)
	if !ok || owner == nil || owner.Algebra() != algebra || owner.Link() != source || owner.factor == nil {
		t.Fatal("declare Effect body-root owner")
	}
	foreignOwner, ok := Declare(composition, effectOwnerKey(t, source, 8), effectOwnerKey(t, source, 9), foreign)
	if !ok || foreignOwner == nil {
		t.Fatal("declare foreign Effect owner")
	}

	assertOwnerSurface(t)
	root, rootOK := algebra.RootAt(0)
	foreignRoot, foreignRootOK := foreign.RootAt(0)
	if !rootOK || !foreignRootOK {
		t.Fatal("body-root fixture roots")
	}
	if _, located := owner.Locate(root); located {
		t.Fatal("Locate issued a Ref before Composition sealing")
	}
	if _, located := owner.Locate(foreignRoot); located {
		t.Fatal("Locate accepted a foreign root from the same Link")
	}
	defaultValue := algebra.Default()
	if !owner.admits(coordinate(0), defaultValue) || owner.admits(coordinate(0), foreign.Default()) {
		t.Fatal("default admission crossed the algebra owner fence")
	}
	wantRank := algebra.WidenRank(root, defaultValue, 0)
	if wantRank == 0 || owner.widenRank(coordinate(0), defaultValue, 0) != wantRank || owner.widenRank(coordinate(0), defaultValue, 1) != 0 {
		t.Fatal("Effect rank is not one-component and root-specific")
	}

	read := owner.ExactRead()
	write := owner.ExactWrite()
	rule, ruleOK := engine.DeclareRule(composition, engine.RuleSpec[effectfactor.Value, effectOwnerOperand]{
		Semantic:      effectOwnerKey(t, source, 2),
		OperandFamily: effectOwnerKey(t, source, 3),
		OperandContent: func(value effectOwnerOperand) (effectOwnerOperand, [32]byte, bool) {
			var digest [32]byte
			digest[0] = 1
			return value, digest, true
		},
		Output:    owner.Output(),
		Inputs:    0,
		Admission: engine.AdmitRuleByTrustedTheorem[effectfactor.Value, effectOwnerOperand](effectOwnerKey(t, source, 4)),
		Transfer:  func(engine.Access[effectfactor.Value, effectOwnerOperand]) bool { return true },
	}, func(rule *engine.Rule[effectfactor.Value, effectOwnerOperand]) bool {
		_, ok := engine.WriteTo(rule, write)
		return ok
	})
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[uint64]{
		Semantic: effectOwnerKey(t, source, 5),
		Project:  func(engine.Observation) uint64 { return 0 },
		Result: engine.FrozenResult[uint64]{
			Semantic:    effectOwnerKey(t, source, 6),
			Freeze:      func(value uint64) uint64 { return value },
			Clone:       func(value uint64) uint64 { return value },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *engine.Query[uint64]) bool {
		_, ok := engine.QueryReadFrom(query, read)
		return ok
	})
	if !ruleOK || rule == nil || !queryOK || query == nil {
		t.Fatal("close minimal Rule/Query composition around Effect factor")
	}

	if !composition.Seal() {
		t.Fatal("seal Effect owner composition")
	}
	for index := 0; index < algebra.RootCount(); index++ {
		root, ok := algebra.RootAt(index)
		if !ok {
			t.Fatalf("root %d", index)
		}
		if _, ok := owner.Locate(root); !ok {
			t.Fatalf("Locate rejected sealed root %d", index)
		}
	}
	if _, ok := owner.Locate(foreignRoot); ok {
		t.Fatal("sealed Locate accepted a foreign same-Link root")
	}

	if reflect.ValueOf(owner.Output()).IsZero() || reflect.ValueOf(owner.ExactRead()).IsZero() || reflect.ValueOf(owner.ExactWrite()).IsZero() || reflect.ValueOf(owner.Carry()).IsZero() {
		t.Fatal("declaration callback did not close all Effect forms")
	}
}

type effectOwnerOperand struct{}

func assertOwnerSurface(t testing.TB) {
	t.Helper()
	typ := reflect.TypeOf((*Owner)(nil))
	if typ.NumMethod() > 16 {
		t.Fatalf("Effect owner exported method set grew to %d", typ.NumMethod())
	}
	allowed := map[string]bool{
		"Algebra": true, "Link": true, "Output": true, "ExactRead": true,
		"ExactWrite": true, "Carry": true, "Locate": true,
	}
	for index := 0; index < typ.NumMethod(); index++ {
		method := typ.Method(index)
		if !allowed[method.Name] {
			t.Fatalf("unexpected promoted/public owner method %q", method.Name)
		}
	}
	valueType := reflect.TypeOf(Owner{})
	if _, found := valueType.FieldByName("locate"); found {
		t.Fatal("Effect owner retained a root lookup map")
	}
	for index := 0; index < valueType.NumField(); index++ {
		field := valueType.Field(index)
		if field.Anonymous || field.Type.Kind() == reflect.Map {
			t.Fatalf("Effect owner has a promoted or copied relation field %q", field.Name)
		}
	}
}

func effectOwnerFixture(t testing.TB) (*effectfactor.Algebra, *effectfactor.Algebra, *link.Link) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "effect_owner.lua", Text: []byte(`
local function first() return 1 end
local function second() return 2 end
first()
second()
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "effect_owner", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(source)
	if !ok {
		t.Fatal("seal type authority")
	}
	statics, _, err := staticdomain.Seal(source, types)
	if err != nil {
		t.Fatal(err)
	}
	packs, ok := pack.Seal(source, statics)
	if !ok {
		t.Fatal("seal pack")
	}
	algebra, ok := effectfactor.New(source, packs, contract)
	if !ok {
		t.Fatal("seal Effect factor")
	}
	foreign, ok := effectfactor.New(source, packs, contract)
	if !ok {
		t.Fatal("seal foreign same-Link Effect factor")
	}
	return algebra, foreign, source
}

func effectOwnerSemantic(t testing.TB, source *link.Link) engine.SemanticKey {
	return effectOwnerKey(t, source, 1)
}

func effectOwnerSummarySemantic(t testing.TB, source *link.Link) engine.SemanticKey {
	return effectOwnerKey(t, source, 7)
}

func effectOwnerKey(t testing.TB, source *link.Link, version byte) engine.SemanticKey {
	t.Helper()
	id := source.ContentID()
	var digest [32]byte
	copy(digest[:], id[:])
	digest[0] ^= version
	key, ok := engine.NewSemanticKey(digest, uint64(version))
	if !ok {
		t.Fatal("Effect owner semantic key")
	}
	return key
}
