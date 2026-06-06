package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestStaticMemberFactsDomain_Laws(t *testing.T) {
	lattice.LawSuite[StaticMemberFacts]{
		Name:   "StaticMemberFacts",
		Domain: StaticMemberFactsDomain,
		Sample: staticMemberFactsSample(),
		Format: StaticMemberFacts.Format,
	}.Run(t)
}

func TestStaticMemberFactsJoinKeepsCommonProvenPaths(t *testing.T) {
	pField := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentField, Name: "kind"}})
	pIndex := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: ""}})
	pInt := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}})

	left := StaticMemberFactsOf([]StaticMemberFact{
		{Path: pField, Value: product.FromType(typ.String)},
		{Path: pIndex, Value: product.FromType(typ.Number)},
	})
	right := StaticMemberFactsOf([]StaticMemberFact{
		{Path: pField, Value: product.FromType(typ.Integer)},
		{Path: pInt, Value: product.FromType(typ.Boolean)},
	})

	joined := StaticMemberFactsDomain.Join(left, right)
	if _, ok := joined.ValueAtAddress(testStableAddressKey(t, pIndex)); ok {
		t.Fatal("join kept string-index fact not proven by both predecessors")
	}
	if _, ok := joined.ValueAtAddress(testStableAddressKey(t, pInt)); ok {
		t.Fatal("join kept int-index fact not proven by both predecessors")
	}
	got, ok := joined.ValueAtAddress(testStableAddressKey(t, pField))
	if !ok {
		t.Fatal("join dropped common field fact")
	}
	want := product.Domain.Join(product.FromType(typ.String), product.FromType(typ.Integer))
	if !product.Domain.Equal(got, want) {
		t.Fatalf("joined field value = %s, want %s", got.ProjectValue(), want.ProjectValue())
	}
}

func TestStaticMemberFactsKillSubtreeUsesStructuralPathPrefix(t *testing.T) {
	root := SymbolPathKey(cfg.SymbolID(1), nil)
	pField := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentField, Name: "foo"}})
	pIndex := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "foo"}})
	other := SymbolPathKey(cfg.SymbolID(2), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "foo"}})

	facts := StaticMemberFactsOf([]StaticMemberFact{
		{Path: pField, Value: product.FromType(typ.String)},
		{Path: pIndex, Value: product.FromType(typ.Number)},
		{Path: other, Value: product.FromType(typ.Boolean)},
	})

	killed := facts.KillSubtreeAddress(testStableAddressKey(t, root))
	if _, ok := killed.ValueAtAddress(testStableAddressKey(t, pField)); ok {
		t.Fatal("root kill kept dot-field fact")
	}
	if _, ok := killed.ValueAtAddress(testStableAddressKey(t, pIndex)); ok {
		t.Fatal("root kill kept string-index fact")
	}
	if _, ok := killed.ValueAtAddress(testStableAddressKey(t, other)); !ok {
		t.Fatal("root kill removed unrelated symbol fact")
	}
}

func TestStaticMemberFactsAddressAPIIsCanonicalSurface(t *testing.T) {
	root, _ := SymbolPathRoot(cfg.SymbolID(8))
	field := PathSuffixOfSegments([]constraint.Segment{{Kind: constraint.SegmentField, Name: "field"}})
	child, ok := StableAddressOfRootAndSuffix(root, field)
	if !ok {
		t.Fatal("child address did not build")
	}
	parent, ok := StableAddressOfRootAndSuffix(root, PathSuffix{})
	if !ok {
		t.Fatal("parent address did not build")
	}

	facts := StaticMemberFactsDomain.Top().WithAddress(child, product.FromType(typ.String))
	got, ok := facts.ValueAtAddress(child)
	if !ok {
		t.Fatal("address value lookup missed fact")
	}
	if !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("address value = %s, want string", got.ProjectValue())
	}
	killed := facts.KillSubtreeAddress(parent)
	if _, ok := killed.ValueAtAddress(child); ok {
		t.Fatal("address subtree kill kept child fact")
	}
}

func TestStaticMemberFactsDirectChildAddressesUnder(t *testing.T) {
	const sym = cfg.SymbolID(14)
	root := constraint.NewPath(sym, "entry")
	meta := root.Field("meta")
	id := meta.Field("id")
	name := meta.Field("name")
	other := root.Field("other").Field("id")
	rootAddr := testStableAddressPath(t, root)
	metaAddr := testStableAddressPath(t, meta)
	idAddr := testStableAddressPath(t, id)
	nameAddr := testStableAddressPath(t, name)
	otherAddr := testStableAddressPath(t, root.Field("other"))

	facts := StaticMemberFactsDomain.Top().
		WithAddress(testStableAddressPath(t, id), product.FromType(typ.String)).
		WithAddress(testStableAddressPath(t, name), product.FromType(typ.Number)).
		WithAddress(testStableAddressPath(t, other), product.FromType(typ.Boolean))

	children := facts.DirectChildAddressesUnder(rootAddr)
	if len(children) != 2 || !children[0].Equal(metaAddr) || !children[1].Equal(otherAddr) {
		t.Fatalf("root children = %v, want meta and other", children)
	}

	metaChildren := facts.DirectChildAddressesUnder(metaAddr)
	if len(metaChildren) != 2 || !metaChildren[0].Equal(idAddr) || !metaChildren[1].Equal(nameAddr) {
		t.Fatalf("meta children = %v, want id and name", metaChildren)
	}
}

func TestStaticMemberFactsAddressEntriesUnder(t *testing.T) {
	const sym = cfg.SymbolID(15)
	root := constraint.NewPath(sym, "entry")
	meta := root.Field("meta")
	id := meta.Field("id")
	other := root.Field("other")
	metaAddr := testStableAddressPath(t, meta)
	idAddr := testStableAddressPath(t, id)

	facts := StaticMemberFactsDomain.Top().
		WithAddress(metaAddr, product.FromType(typ.NewRecord().Build())).
		WithAddress(idAddr, product.FromType(typ.String)).
		WithAddress(testStableAddressPath(t, other), product.FromType(typ.Boolean))

	entries := facts.AddressEntriesUnder(metaAddr)
	if len(entries) != 2 || !entries[0].Address.Equal(metaAddr) || !entries[1].Address.Equal(idAddr) {
		t.Fatalf("entries under meta = %v, want meta and id", entries)
	}
	if !typ.TypeEquals(entries[1].Value.ProjectValue(), typ.String) {
		t.Fatalf("id entry value = %s, want string", entries[1].Value.ProjectValue())
	}
}

func TestInvalidateStaticMemberWritePathBottomsAncestorsAndKillsSubtree(t *testing.T) {
	root := cfg.SymbolID(9)
	parentPath := constraint.NewPath(root, "root").Field("cfg")
	childPath := constraint.NewPath(root, "root").Field("cfg").Field("value")
	otherPath := constraint.NewPath(root, "root").Field("other")
	parent, ok := StableAddressOfPath(parentPath)
	if !ok {
		t.Fatal("parent address did not build")
	}
	child, ok := StableAddressOfPath(childPath)
	if !ok {
		t.Fatal("child address did not build")
	}
	other, ok := StableAddressOfPath(otherPath)
	if !ok {
		t.Fatal("other address did not build")
	}
	out := PointState{StaticMembers: StaticMemberFactsDomain.Top()}
	SetStaticMemberFact(&out, parent, product.FromType(typ.NewRecord().Build()))
	SetStaticMemberFact(&out, child, product.FromType(typ.String))
	SetStaticMemberFact(&out, other, product.FromType(typ.Number))

	if !InvalidateStaticMemberWritePath(&out, childPath) {
		t.Fatal("InvalidateStaticMemberWritePath reported no change")
	}
	if _, ok := out.StaticMembers.ValueAtAddress(child); ok {
		t.Fatalf("write invalidation kept written subtree: %s", out.StaticMembers.Format())
	}
	if _, ok := out.StaticMembers.ValueAtAddress(parent); ok {
		t.Fatalf("write invalidation kept stale ancestor fact: %s", out.StaticMembers.Format())
	}
	if got, ok := out.StaticMembers.ValueAtAddress(other); !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("write invalidation touched unrelated fact: %s", out.StaticMembers.Format())
	}
}

func TestKillStaticMemberSubtreePathNormalizesPath(t *testing.T) {
	root := cfg.SymbolID(10)
	parentPath := constraint.NewPath(root, "root").Field("cfg")
	childPath := constraint.NewPath(root, "root").Field("cfg").Field("value")
	parent := testStableAddressPath(t, parentPath)
	child := testStableAddressPath(t, childPath)
	out := PointState{StaticMembers: StaticMemberFactsDomain.Top()}
	SetStaticMemberFact(&out, parent, product.FromType(typ.NewRecord().Build()))
	SetStaticMemberFact(&out, child, product.FromType(typ.String))

	if !KillStaticMemberSubtreePath(&out, parentPath) {
		t.Fatal("KillStaticMemberSubtreePath reported no change")
	}
	if _, ok := out.StaticMembers.ValueAtAddress(parent); ok {
		t.Fatalf("subtree kill kept parent fact: %s", out.StaticMembers.Format())
	}
	if _, ok := out.StaticMembers.ValueAtAddress(child); ok {
		t.Fatalf("subtree kill kept child fact: %s", out.StaticMembers.Format())
	}
}

func staticMemberFactsSample() []StaticMemberFacts {
	pField := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentField, Name: "foo"}})
	pIndex := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "foo"}})
	pEmpty := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: ""}})
	pInt := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}})
	pOther := SymbolPathKey(cfg.SymbolID(2), []constraint.Segment{{Kind: constraint.SegmentField, Name: "bar"}})

	return []StaticMemberFacts{
		StaticMemberFactsDomain.Bottom(),
		StaticMemberFactsDomain.Top(),
		StaticMemberFactsOf([]StaticMemberFact{{Path: pField, Value: product.FromType(typ.String)}}),
		StaticMemberFactsOf([]StaticMemberFact{{Path: pIndex, Value: product.FromType(typ.Number)}}),
		StaticMemberFactsOf([]StaticMemberFact{{Path: pEmpty, Value: product.FromType(typ.Boolean)}}),
		StaticMemberFactsOf([]StaticMemberFact{{Path: pInt, Value: product.FromType(typ.Integer)}}),
		StaticMemberFactsOf([]StaticMemberFact{
			{Path: pField, Value: product.FromType(typ.String)},
			{Path: pOther, Value: product.FromType(typ.Boolean)},
		}),
		StaticMemberFactsOf([]StaticMemberFact{
			{Path: pField, Value: product.FromType(typ.Number)},
			{Path: pIndex, Value: product.FromType(typ.String)},
			{Path: pInt, Value: product.FromType(typ.Boolean)},
		}),
	}
}
