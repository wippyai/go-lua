package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIndexWriteAdmissionFactsDomain_Laws(t *testing.T) {
	lattice.LawSuite[IndexWriteAdmissionFacts]{
		Name:   "IndexWriteAdmissionFacts",
		Domain: IndexWriteAdmissionFactsDomain,
		Sample: indexWriteAdmissionFactsSample(),
		Format: IndexWriteAdmissionFacts.Format,
	}.Run(t)
}

func TestIndexWriteAdmissionFactsJoinKeepsOnlyCommonProofs(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(1), "m")
	key := constraint.NewPath(cfg.SymbolID(2), "k")
	valuePath := constraint.NewPath(cfg.SymbolID(3), "v")
	otherValue := constraint.NewPath(cfg.SymbolID(4), "w")
	common := IndexWriteAdmissionFact{
		Target:    IndexWriteAdmissionPathKey(target),
		KeyPath:   IndexWriteAdmissionPathKey(key),
		Key:       product.FromType(typ.String),
		ValuePath: IndexWriteAdmissionPathKey(valuePath),
		Value:     product.FromType(typ.String),
	}
	left := IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{
		common,
		{
			Target:    IndexWriteAdmissionPathKey(target.Field("items")),
			KeyPath:   IndexWriteAdmissionPathKey(key),
			Key:       product.FromType(typ.String),
			ValuePath: IndexWriteAdmissionPathKey(otherValue),
			Value:     product.FromType(typ.Boolean),
		},
	})
	right := IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{
		{
			Target:    common.Target,
			KeyPath:   common.KeyPath,
			Key:       product.FromType(typ.String),
			ValuePath: common.ValuePath,
			Value:     product.FromType(typ.Number),
		},
	})

	joined := IndexWriteAdmissionFactsDomain.Join(left, right)
	got, ok := joined.Admission(IndexWriteQuery{
		Target:    target,
		KeyPath:   key,
		KeySymbol: key.Symbol,
		KeyType:   typ.String,
		ValuePath: valuePath,
	})
	if !ok {
		t.Fatal("join dropped common admission proof")
	}
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("joined admission value = %v, want %v", got.ProjectValue(), want)
	}
	if _, ok := joined.Admission(IndexWriteQuery{
		Target:    target.Field("items"),
		KeySymbol: key.Symbol,
		KeyType:   typ.String,
		ValuePath: otherValue,
	}); ok {
		t.Fatalf("join kept one-branch admission proof: %s", joined.Format())
	}
}

func TestIndexWriteAdmissionFactsKillAffectedByWrite(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(12), "store").Field("items")
	key := constraint.NewPath(cfg.SymbolID(13), "id")
	valuePath := constraint.NewPath(cfg.SymbolID(14), "entry")
	facts := IndexWriteAdmissionFacts{}.With(IndexWriteAdmissionFact{
		Target:    IndexWriteAdmissionPathKey(target),
		KeyPath:   IndexWriteAdmissionPathKey(key),
		Key:       product.FromType(typ.String),
		ValuePath: IndexWriteAdmissionPathKey(valuePath),
		Value:     product.FromType(typ.Number),
	})

	if got := facts.KillAffectedByWrite(KeyPresencePathKey(constraint.NewPath(cfg.SymbolID(15), "other"))); !IndexWriteAdmissionFactsDomain.Equal(got, facts) {
		t.Fatalf("unrelated write killed admission: got %s want %s", got.Format(), facts.Format())
	}
	if got := facts.KillAffectedByWrite(KeyPresencePathKey(target.Field("name"))); len(got.Entries()) != 0 {
		t.Fatalf("target write kept admission: %s", got.Format())
	}
	if got := facts.KillAffectedByWrite(KeyPresencePathKey(key)); len(got.Entries()) != 0 {
		t.Fatalf("key write kept admission: %s", got.Format())
	}
	if got := facts.KillAffectedByWrite(KeyPresencePathKey(valuePath.Field("name"))); len(got.Entries()) != 0 {
		t.Fatalf("value write kept admission: %s", got.Format())
	}
}

func TestIndexWriteAdmissionFactsPreservePresentElementWriteWeakensSameTableProof(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(16), "store").Field("items")
	key := constraint.NewPath(cfg.SymbolID(17), "last_id")
	other := constraint.NewPath(cfg.SymbolID(18), "store").Field("edges")
	oldValue := product.FromType(typ.NewRecord().Field("config", typ.NewRecord().Field("func_id", typ.String).Build()).Build())
	written := product.FromType(typ.NewRecord().Field("config", typ.NewRecord().Field("agent", typ.String).Build()).Build())
	edgeValue := product.FromType(typ.NewRecord().Field("targets", typ.NewArray(typ.String)).Build())
	facts := IndexWriteAdmissionFacts{}.
		With(IndexWriteAdmissionFact{
			Target:  IndexWriteAdmissionPathKey(target),
			KeyPath: IndexWriteAdmissionPathKey(key),
			Key:     product.FromType(typ.Any),
			Value:   oldValue,
		}).
		With(IndexWriteAdmissionFact{
			Target:  IndexWriteAdmissionPathKey(other),
			KeyPath: IndexWriteAdmissionPathKey(key),
			Key:     product.FromType(typ.Any),
			Value:   edgeValue,
		})

	got := facts.PreservePresentElementWrite(IndexWriteAdmissionPathKey(target), written)
	admitted, ok := got.Admission(IndexWriteQuery{
		Target:  target,
		KeyPath: key,
		KeyType: typ.Any,
	})
	if !ok {
		t.Fatalf("present element write dropped same-table admission: %s", got.Format())
	}
	want := product.ProjectValueOrUnknown(product.Domain.Join(oldValue, written))
	if !typ.TypeEquals(admitted.ProjectValue(), want) {
		t.Fatalf("same-table admission value = %v, want %v", admitted.ProjectValue(), want)
	}
	if admitted, ok := got.Admission(IndexWriteQuery{
		Target:  other,
		KeyPath: key,
		KeyType: typ.Any,
	}); !ok || !typ.TypeEquals(admitted.ProjectValue(), edgeValue.ProjectValue()) {
		t.Fatalf("unrelated table admission = %v/%v, want edge value/true; facts=%s", admitted.ProjectValue(), ok, got.Format())
	}
}

func TestIndexWriteAdmissionFactsMatchesByKeyValueWhenKeyPathAbsent(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(10), "m")
	facts := IndexWriteAdmissionFacts{}.With(IndexWriteAdmissionFact{
		Target: IndexWriteAdmissionPathKey(target),
		Key:    product.FromType(typ.LiteralString("name")),
		Value:  product.FromType(typ.String),
	})

	if _, ok := facts.Admission(IndexWriteQuery{Target: target, KeyType: typ.LiteralString("name")}); !ok {
		t.Fatal("literal-key query did not match literal-key admission proof")
	}
	if _, ok := facts.Admission(IndexWriteQuery{Target: target, KeyType: typ.LiteralString("other")}); ok {
		t.Fatal("literal-key query matched incompatible key proof")
	}
}

func TestIndexWriteAdmissionFactsMatchesExactKeyPathWithUnknownKeyValue(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(20), "m")
	key := constraint.NewPath(cfg.SymbolID(21), "k")
	otherKey := constraint.NewPath(cfg.SymbolID(22), "other")
	facts := IndexWriteAdmissionFacts{}.With(IndexWriteAdmissionFact{
		Target:  IndexWriteAdmissionPathKey(target),
		KeyPath: IndexWriteAdmissionPathKey(key),
		Key:     product.FromType(typ.Unknown),
		Value:   product.FromType(typ.String),
	})

	if got, ok := facts.Admission(IndexWriteQuery{
		Target:  target,
		KeyPath: key,
		KeyType: typ.String,
	}); !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("exact key-path query = %v/%v, want string/true", got.ProjectValue(), ok)
	}
	if _, ok := facts.Admission(IndexWriteQuery{
		Target:  target,
		KeyPath: otherKey,
		KeyType: typ.String,
	}); ok {
		t.Fatal("different key-path query matched unknown-key admission proof")
	}
	if _, ok := facts.Admission(IndexWriteQuery{
		Target:  target,
		KeyType: typ.LiteralString("name"),
	}); ok {
		t.Fatal("pathless literal query matched unknown-key path-backed proof")
	}
}

func TestIndexWriteAdmissionFactsAddressQuerySupportsNamedRoots(t *testing.T) {
	target, _ := StableAddressOfRoot("$0", []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}})
	key, _ := StableAddressOfRoot("$1", nil)
	value := product.FromType(typ.String)
	facts := IndexWriteAdmissionFacts{}.With(IndexWriteAdmissionFact{
		Target:  target.Key(),
		KeyPath: key.Key(),
		Key:     product.FromType(typ.Unknown),
		Value:   value,
	})

	got, ok := facts.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:     target,
		KeyPath:    key,
		HasKeyPath: true,
		KeyValue:   product.FromType(typ.Number),
	})
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("address admission = %v/%v, want string/true", got.ProjectValue(), ok)
	}
}

func TestIndexWriteAdmissionFactsAddressInvalidationUsesStructuredOverlap(t *testing.T) {
	target, _ := StableAddressOfSymbol(cfg.SymbolID(41), []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}})
	key, _ := StableAddressOfSymbol(cfg.SymbolID(42), nil)
	valuePath, _ := StableAddressOfSymbol(cfg.SymbolID(43), []constraint.Segment{{Kind: constraint.SegmentField, Name: "payload"}})
	facts := IndexWriteAdmissionFacts{}.With(IndexWriteAdmissionFact{
		Target:    target.Key(),
		KeyPath:   key.Key(),
		Key:       product.FromType(typ.String),
		ValuePath: valuePath.Key(),
		Value:     product.FromType(typ.Number),
	})

	sibling, _ := StableAddressOfSymbol(cfg.SymbolID(41), []constraint.Segment{{Kind: constraint.SegmentField, Name: "edges"}})
	if got := facts.KillAffectedByWriteAddress(sibling); !IndexWriteAdmissionFactsDomain.Equal(got, facts) {
		t.Fatalf("sibling write killed admission: got %s want %s", got.Format(), facts.Format())
	}

	child, _ := StableAddressOfSymbol(cfg.SymbolID(43), []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "payload"},
		{Kind: constraint.SegmentField, Name: "id"},
	})
	if got := facts.KillAffectedByWriteAddress(child); len(got.Entries()) != 0 {
		t.Fatalf("value child write kept admission: %s", got.Format())
	}
}

func indexWriteAdmissionFactsSample() []IndexWriteAdmissionFacts {
	target := SymbolPathKey(cfg.SymbolID(1), nil)
	key := SymbolPathKey(cfg.SymbolID(2), nil)
	valuePath := SymbolPathKey(cfg.SymbolID(3), nil)
	otherTarget := SymbolPathKey(cfg.SymbolID(4), []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}})
	return []IndexWriteAdmissionFacts{
		IndexWriteAdmissionFactsDomain.Bottom(),
		IndexWriteAdmissionFactsDomain.Top(),
		IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{{
			Target:    target,
			KeyPath:   key,
			Key:       product.FromType(typ.String),
			ValuePath: valuePath,
			Value:     product.FromType(typ.Number),
		}}),
		IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{{
			Target: target,
			Key:    product.FromType(typ.LiteralString("name")),
			Value:  product.FromType(typ.String),
		}}),
		IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{
			{
				Target:    target,
				KeyPath:   key,
				Key:       product.FromType(typ.String),
				ValuePath: valuePath,
				Value:     product.FromType(typ.Integer),
			},
			{
				Target: otherTarget,
				Key:    product.FromType(typ.Number),
				Value:  product.FromType(typ.Boolean),
			},
		}),
	}
}
