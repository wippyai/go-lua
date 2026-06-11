package recursivefamily

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFamilyKeyString(t *testing.T) {
	if got := (FamilyKey{Owner: "node"}).String(); got != "node" {
		t.Fatalf("owner-only key string = %q, want node", got)
	}
	if got := (FamilyKey{Namespace: "fn", Owner: "node"}).String(); got != "fn:node" {
		t.Fatalf("namespaced key string = %q, want fn:node", got)
	}
}

func TestFamilyKeyIsZero(t *testing.T) {
	if !(FamilyKey{}).IsZero() {
		t.Fatal("empty key should be zero")
	}
	if (FamilyKey{Owner: "node"}).IsZero() {
		t.Fatal("owner key should not be zero")
	}
}

func TestFamilyKeyHash(t *testing.T) {
	left := FamilyKey{Namespace: "fn", Owner: "node"}
	right := FamilyKey{Namespace: "fn", Owner: "node"}
	other := FamilyKey{Namespace: "fn", Owner: "edge"}
	if left.Hash() != right.Hash() {
		t.Fatal("equal keys should have equal hashes")
	}
	if left.Hash() == other.Hash() {
		t.Fatal("different keys should not collide in this regression case")
	}
}

func TestRecursiveFamilyInternerInternsSameKey(t *testing.T) {
	interner := NewRecursiveFamilyInterner()
	key := FamilyKey{Namespace: "store", Owner: "sym:1"}

	first := interner.Intern(key)
	second := interner.Intern(key)
	if first == nil || second == nil {
		t.Fatal("interned family should not be nil")
	}
	if first != second {
		t.Fatal("same key should return the same family handle")
	}
	got, ok := interner.FamilyKeyOf(first)
	if !ok || got != key {
		t.Fatalf("FamilyKeyOf() = (%#v, %v), want (%#v, true)", got, ok, key)
	}
	if !interner.SameFamily(first, second) {
		t.Fatal("same interned key should report SameFamily")
	}
}

func TestRecursiveFamilyInternerResetDropsOwnedFamilies(t *testing.T) {
	interner := NewRecursiveFamilyInterner()
	key := FamilyKey{Namespace: "store", Owner: "sym:reset"}
	first := interner.Intern(key)
	body := typetable.NewRecord().Field("value", typ.String).Build()
	interner.Widen(first, body, nil)
	if first.Body == nil {
		t.Fatal("precondition: first family body should be initialized")
	}

	interner.Reset()
	if got, ok := interner.FamilyKeyOf(first); ok || !got.IsZero() {
		t.Fatalf("FamilyKeyOf(old family) after Reset = (%#v, %v), want zero/false", got, ok)
	}
	if got, ok := interner.FamilyIdentityHash(first); ok || got != 0 {
		t.Fatalf("FamilyIdentityHash(old family) after Reset = %x/%v, want 0/false", got, ok)
	}

	second := interner.Intern(key)
	if second == nil {
		t.Fatal("intern after Reset should return a family")
	}
	if second == first {
		t.Fatal("intern after Reset should mint a fresh family handle")
	}
	if second.Body != nil {
		t.Fatal("fresh family after Reset should not inherit the old body")
	}
	if interner.SameFamily(first, second) {
		t.Fatal("old family should no longer be owned after Reset")
	}
}

func TestRecursiveFamilyInternerOwnershipIsolation(t *testing.T) {
	key := FamilyKey{Namespace: "class", Owner: "alloc:7"}
	left := NewRecursiveFamilyInterner()
	right := NewRecursiveFamilyInterner()

	leftFamily := left.Intern(key)
	rightFamily := right.Intern(key)
	if leftFamily == rightFamily {
		t.Fatal("separate interners should not share family handles")
	}

	body := typetable.NewRecord().Field("value", typ.String).Build()
	left.Widen(rightFamily, body, nil)
	if rightFamily.Body != nil {
		t.Fatal("foreign family body should not be mutated")
	}

	left.Widen(leftFamily, body, nil)
	if leftFamily.Body == nil {
		t.Fatal("owned family body should be initialized")
	}
	if rightFamily.Body != nil {
		t.Fatal("foreign family should remain unchanged")
	}
}

func TestRecursiveFamilyInternerWidenRequiresOwnedHandle(t *testing.T) {
	interner := NewRecursiveFamilyInterner()
	key := FamilyKey{Namespace: "fold", Owner: "site:3"}
	foreign := NewRecursiveFamilyInterner().Intern(key)
	plain := typ.NewRecursivePlaceholder("Plain")
	body := typetable.NewRecord().Field("value", typ.Number).Build()

	interner.Widen(foreign, body, nil)
	if foreign.Body != nil {
		t.Fatal("foreign family placeholder should not be mutated")
	}
	interner.Widen(plain, body, nil)
	if plain.Body != nil {
		t.Fatal("plain recursive placeholder should not be mutated")
	}

	owned := interner.Intern(key)
	interner.Widen(owned, body, nil)
	if owned.Body == nil {
		t.Fatal("owned family should accept widen")
	}
}

func TestFamilyKeyOfReadback(t *testing.T) {
	interner := NewRecursiveFamilyInterner()
	key := FamilyKey{Namespace: "ns", Owner: "owner"}
	family := interner.Intern(key)
	got, ok := interner.FamilyKeyOf(family)
	if !ok || got != key {
		t.Fatalf("FamilyKeyOf(family) = (%#v, %v), want (%#v, true)", got, ok, key)
	}
	if got, ok := NewRecursiveFamilyInterner().FamilyKeyOf(family); ok || !got.IsZero() {
		t.Fatalf("foreign FamilyKeyOf(family) = (%#v, %v), want zero/false", got, ok)
	}

	plain := typ.NewRecursivePlaceholder("Plain")
	if got, ok := interner.FamilyKeyOf(plain); ok || !got.IsZero() {
		t.Fatalf("FamilyKeyOf(plain) = (%#v, %v), want zero/false", got, ok)
	}
}

func TestRecursiveFamilyDifferentKeysDoNotAliasEquivalentBodies(t *testing.T) {
	interner := NewRecursiveFamilyInterner()
	left := interner.Intern(FamilyKey{Namespace: "store", Owner: "left"})
	right := interner.Intern(FamilyKey{Namespace: "store", Owner: "right"})
	body := typetable.NewRecord().Field("value", typ.String).Build()
	interner.Widen(left, body, nil)
	interner.Widen(right, body, nil)

	if interner.SameFamily(left, right) {
		t.Fatal("different keys must not alias even with equivalent bodies")
	}
	leftHash, leftOK := interner.FamilyIdentityHash(left)
	rightHash, rightOK := interner.FamilyIdentityHash(right)
	if !leftOK || !rightOK || leftHash == rightHash {
		t.Fatalf("identity hashes = %x/%v and %x/%v, want distinct", leftHash, leftOK, rightHash, rightOK)
	}
}

func TestRecursiveFamilyIdentityStableAcrossWiden(t *testing.T) {
	interner := NewRecursiveFamilyInterner()
	key := FamilyKey{Namespace: "store", Owner: "sym:stable"}
	family := interner.Intern(key)

	initialFingerprint := interner.RecursiveFamilyFingerprint(family)
	initialIdentityHash, ok := interner.FamilyIdentityHash(family)
	initialStructuralHash := family.Hash()
	if initialFingerprint == 0 || !ok || initialIdentityHash == 0 || initialStructuralHash == 0 {
		t.Fatalf("initial identity values should be non-zero: fp=%x id=%x/%v hash=%x", initialFingerprint, initialIdentityHash, ok, initialStructuralHash)
	}

	firstBody := typetable.NewRecord().Field("value", typ.String).Build()
	interner.Widen(family, firstBody, nil)
	if got := interner.RecursiveFamilyFingerprint(family); got != initialFingerprint {
		t.Fatalf("fingerprint after first widen = %x, want %x", got, initialFingerprint)
	}
	if got, ok := interner.FamilyIdentityHash(family); !ok || got != initialIdentityHash {
		t.Fatalf("identity hash after first widen = %x/%v, want %x/true", got, ok, initialIdentityHash)
	}
	if got := family.Hash(); got == initialStructuralHash {
		t.Fatalf("structural hash after first widen = %x, should no longer be family identity hash", got)
	}

	secondBody := typetable.NewRecord().
		Field("value", typ.String).
		Field("extra", typ.Number).
		Build()
	interner.Widen(family, secondBody, func(existing, candidate typ.Type) typ.Type {
		return candidate
	})
	if got := interner.RecursiveFamilyFingerprint(family); got != initialFingerprint {
		t.Fatalf("fingerprint after second widen = %x, want %x", got, initialFingerprint)
	}
	if got, ok := interner.FamilyIdentityHash(family); !ok || got != initialIdentityHash {
		t.Fatalf("identity hash after second widen = %x/%v, want %x/true", got, ok, initialIdentityHash)
	}
}

func TestRebindRecursiveRefRewritesNestedReferences(t *testing.T) {
	from := typ.NewRecursivePlaceholder("Fold")
	to := typ.NewRecursivePlaceholder("Family")
	body := typetable.NewRecord().
		Field("next", typ.NewOptional(from)).
		Field("value", typ.String).
		Build()

	rebuilt := RebindRecursiveRef(body, from, to)
	rec, ok := rebuilt.(*typ.Record)
	if !ok {
		t.Fatalf("RebindRecursiveRef() = %T, want *typ.Record", rebuilt)
	}
	next := rec.GetField("next")
	if next == nil {
		t.Fatal("rebuilt record missing next field")
	}
	nextOpt, ok := next.Type.(*typ.Optional)
	if !ok {
		t.Fatalf("next field = %T, want *typ.Optional", next.Type)
	}
	if nextOpt.Inner != to {
		t.Fatalf("next inner = %v, want target recursive handle", nextOpt.Inner)
	}
	if value := rec.GetField("value"); value == nil || value.Type != typ.String {
		t.Fatalf("value field = %#v, want string field", value)
	}
	if got := RebindRecursiveRef(body, from, from); got != body {
		t.Fatal("rebinding to the same recursive handle should be a no-op")
	}
}

func TestContainsRecursiveRefDetectsTargetOnly(t *testing.T) {
	target := typ.NewRecursivePlaceholder("Target")
	other := typ.NewRecursivePlaceholder("Other")
	body := typetable.NewRecord().
		Field("next", typ.NewOptional(target)).
		Field("other", other).
		Field("value", typ.Number).
		Build()

	if !ContainsRecursiveRef(body, target) {
		t.Fatal("body should contain target recursive reference")
	}
	if ContainsRecursiveRef(typetable.NewRecord().Field("value", typ.Number).Build(), target) {
		t.Fatal("non-recursive body should not contain target recursive reference")
	}
	if ContainsRecursiveRef(body, typ.NewRecursivePlaceholder("Missing")) {
		t.Fatal("body should not contain an unrelated recursive reference")
	}
	if ContainsRecursiveRef(nil, target) || ContainsRecursiveRef(body, nil) {
		t.Fatal("nil inputs should not report recursive references")
	}
}

func TestCollapseUnfoldingToFamilyFoldsRecursiveChildren(t *testing.T) {
	family := typ.NewRecursivePlaceholder("Family")
	deepUnfolding := typetable.NewRecord().
		Field("next", typ.NewOptional(family)).
		Field("payload", typ.Number).
		Build()
	slot := typetable.NewRecord().
		Field("head", typ.String).
		Field("child", deepUnfolding).
		Field("leaf", typ.Boolean).
		Build()

	collapsed := CollapseUnfoldingToFamily(slot, family)
	rec, ok := collapsed.(*typ.Record)
	if !ok {
		t.Fatalf("CollapseUnfoldingToFamily() = %T, want *typ.Record", collapsed)
	}
	if head := rec.GetField("head"); head == nil || head.Type != typ.String {
		t.Fatalf("head field = %#v, want string field", head)
	}
	if child := rec.GetField("child"); child == nil || child.Type != family {
		t.Fatalf("child field = %#v, want family handle", child)
	}
	if leaf := rec.GetField("leaf"); leaf == nil || leaf.Type != typ.Boolean {
		t.Fatalf("leaf field = %#v, want boolean field", leaf)
	}
	if got := CollapseUnfoldingToFamily(family, family); got != family {
		t.Fatal("bare recursive reference should collapse to the family handle")
	}
}

func TestRecursiveFamilyFingerprintScansSurfaceWithoutUnfoldingBodies(t *testing.T) {
	hidden := typ.NewRecursive("Hidden", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", self).Build()
	})
	outer := typ.NewRecursivePlaceholder("Outer")
	outer.SetBody(typetable.NewRecord().Field("hidden", hidden).Build())
	visible := typ.NewRecursive("Visible", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", self).Build()
	})

	outerOnly := RecursiveFamilyFingerprint(outer)
	if outerOnly == 0 {
		t.Fatal("outer recursive family fingerprint should be non-zero")
	}

	duplicateOuter := typetable.NewRecord().
		Field("first", outer).
		Field("second", outer).
		Build()
	if got := RecursiveFamilyFingerprint(duplicateOuter); got != outerOnly {
		t.Fatalf("duplicate outer fingerprint = %x, want %x", got, outerOnly)
	}

	if got := RecursiveFamilyFingerprint(typetable.NewRecord().Field("outer", outer).Build()); got != outerOnly {
		t.Fatalf("record containing outer fingerprint = %x, want %x", got, outerOnly)
	}

	explicitHidden := RecursiveFamilyFingerprint(typetable.NewRecord().
		Field("outer", outer).
		Field("hidden", hidden).
		Build())
	if explicitHidden == outerOnly {
		t.Fatal("explicit hidden family should affect fingerprint")
	}

	surface := typetable.NewRecord().
		Field("handler", typ.Func().Param("self", outer).Returns(typ.NewArray(visible)).Build()).
		Field("duplicate", outer).
		MapComponent(typ.String, visible).
		Build()
	surfaceFP := RecursiveFamilyFingerprint(surface)
	if surfaceFP == 0 || surfaceFP == outerOnly {
		t.Fatalf("surface fingerprint = %x, want visible family included with outer", surfaceFP)
	}

	repeatedSurface := typetable.NewRecord().
		Field("a", surface).
		Field("b", surface).
		Build()
	if got := RecursiveFamilyFingerprint(repeatedSurface); got != surfaceFP {
		t.Fatalf("repeated surface fingerprint = %x, want %x", got, surfaceFP)
	}
}

func TestRecursiveFamilyFingerprintWithinReportsBudgetExhaustion(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", self).Build()
	})
	surface := typ.Type(node)
	for i := 0; i < 8; i++ {
		surface = typetable.NewRecord().
			Field("left", surface).
			Field("right", typ.NewArray(surface)).
			Build()
	}

	if _, ok := RecursiveFamilyFingerprintWithin(surface, 4); ok {
		t.Fatal("small budget should report exhaustion")
	}
	if fp, ok := RecursiveFamilyFingerprintWithin(surface, 0); !ok || fp == 0 {
		t.Fatalf("unbounded fingerprint = %x/%v, want non-zero/true", fp, ok)
	}
}
