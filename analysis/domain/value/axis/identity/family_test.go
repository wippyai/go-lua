package identity

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

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
	if got := first.RecursiveFamilyKey(); got != key {
		t.Fatalf("RecursiveFamilyKey() = %#v, want %#v", got, key)
	}
	got, ok := FamilyKeyOf(first)
	if !ok || got != key {
		t.Fatalf("FamilyKeyOf() = (%#v, %v), want (%#v, true)", got, ok, key)
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

	body := typ.NewRecord().Field("value", typ.String).Build()
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
	unmanaged := typ.NewRecursiveFamilyPlaceholder(key)
	plain := typ.NewRecursivePlaceholder("Plain")
	body := typ.NewRecord().Field("value", typ.Number).Build()

	interner.Widen(unmanaged, body, nil)
	if unmanaged.Body != nil {
		t.Fatal("unmanaged family placeholder should not be mutated")
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
	key := FamilyKey{Namespace: "ns", Owner: "owner"}
	family := typ.NewRecursiveFamilyPlaceholder(key)
	got, ok := FamilyKeyOf(family)
	if !ok || got != key {
		t.Fatalf("FamilyKeyOf(family) = (%#v, %v), want (%#v, true)", got, ok, key)
	}

	plain := typ.NewRecursivePlaceholder("Plain")
	if got, ok := FamilyKeyOf(plain); ok || !got.IsZero() {
		t.Fatalf("FamilyKeyOf(plain) = (%#v, %v), want zero/false", got, ok)
	}
}

func TestRecursiveFamilyIdentityStableAcrossWiden(t *testing.T) {
	interner := NewRecursiveFamilyInterner()
	key := FamilyKey{Namespace: "store", Owner: "sym:stable"}
	family := interner.Intern(key)

	initialFingerprint := RecursiveFamilyFingerprint(family)
	initialHash := family.Hash()
	if initialFingerprint == 0 || initialHash == 0 {
		t.Fatalf("initial identity values should be non-zero: fp=%x hash=%x", initialFingerprint, initialHash)
	}

	firstBody := typ.NewRecord().Field("value", typ.String).Build()
	interner.Widen(family, firstBody, nil)
	if got := RecursiveFamilyFingerprint(family); got != initialFingerprint {
		t.Fatalf("fingerprint after first widen = %x, want %x", got, initialFingerprint)
	}
	if got := family.Hash(); got != initialHash {
		t.Fatalf("hash after first widen = %x, want %x", got, initialHash)
	}

	secondBody := typ.NewRecord().
		Field("value", typ.String).
		Field("extra", typ.Number).
		Build()
	interner.Widen(family, secondBody, func(existing, candidate typ.Type) typ.Type {
		return candidate
	})
	if got := RecursiveFamilyFingerprint(family); got != initialFingerprint {
		t.Fatalf("fingerprint after second widen = %x, want %x", got, initialFingerprint)
	}
	if got := family.Hash(); got != initialHash {
		t.Fatalf("hash after second widen = %x, want %x", got, initialHash)
	}
}

func TestRecursiveFamilyFingerprintScansSurfaceWithoutUnfoldingBodies(t *testing.T) {
	hidden := typ.NewRecursive("Hidden", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", self).Build()
	})
	outer := typ.NewRecursivePlaceholder("Outer")
	outer.SetBody(typ.NewRecord().Field("hidden", hidden).Build())
	visible := typ.NewRecursive("Visible", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", self).Build()
	})

	outerOnly := RecursiveFamilyFingerprint(outer)
	if outerOnly == 0 {
		t.Fatal("outer recursive family fingerprint should be non-zero")
	}

	duplicateOuter := typ.NewRecord().
		Field("first", outer).
		Field("second", outer).
		Build()
	if got := RecursiveFamilyFingerprint(duplicateOuter); got != outerOnly {
		t.Fatalf("duplicate outer fingerprint = %x, want %x", got, outerOnly)
	}

	if got := RecursiveFamilyFingerprint(typ.NewRecord().Field("outer", outer).Build()); got != outerOnly {
		t.Fatalf("record containing outer fingerprint = %x, want %x", got, outerOnly)
	}

	explicitHidden := RecursiveFamilyFingerprint(typ.NewRecord().
		Field("outer", outer).
		Field("hidden", hidden).
		Build())
	if explicitHidden == outerOnly {
		t.Fatal("explicit hidden family should affect fingerprint")
	}

	surface := typ.NewRecord().
		Field("handler", typ.Func().Param("self", outer).Returns(typ.NewArray(visible)).Build()).
		Field("duplicate", outer).
		MapComponent(typ.String, visible).
		Build()
	surfaceFP := RecursiveFamilyFingerprint(surface)
	if surfaceFP == 0 || surfaceFP == outerOnly {
		t.Fatalf("surface fingerprint = %x, want visible family included with outer", surfaceFP)
	}

	repeatedSurface := typ.NewRecord().
		Field("a", surface).
		Field("b", surface).
		Build()
	if got := RecursiveFamilyFingerprint(repeatedSurface); got != surfaceFP {
		t.Fatalf("repeated surface fingerprint = %x, want %x", got, surfaceFP)
	}
}

func TestRecursiveFamilyFingerprintWithinReportsBudgetExhaustion(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", self).Build()
	})
	surface := typ.Type(node)
	for i := 0; i < 8; i++ {
		surface = typ.NewRecord().
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

func TestProductFamilyHashTerminatesOnRecursiveMapTower(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewMap(typ.String, typ.NewOptional(self))
	})
	var tower typ.Type = node
	for i := 0; i < 2048; i++ {
		tower = typ.NewMap(typ.String, typ.NewOptional(typ.NewUnion(tower, typ.Nil)))
	}

	if got := ProductFamilyHash(tower); got == 0 {
		t.Fatal("recursive map tower family hash should be non-zero")
	}
}

func TestSameProductFamilyWithPrecision(t *testing.T) {
	left := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			OptField("next", self).
			Build()
	})
	right := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			OptField("next", self).
			Build()
	})
	richer := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("path", typ.String).
			OptField("next", self).
			Build()
	})

	if !SameProductFamilyWithPrecision(left, right, testProductFamilyPrecisionCompare) {
		t.Fatal("same recursive product family should compare equal")
	}
	if SameProductFamilyWithPrecision(left, richer, testProductFamilyPrecisionCompare) {
		t.Fatal("same product family must not collapse strictly richer evidence")
	}
}

func testProductFamilyPrecisionCompare(candidate, baseline typ.Type) (bool, bool) {
	return testProductFamilyPrecisionCompareSeen(candidate, baseline, make(map[[2]uintptr]bool))
}

func testProductFamilyPrecisionCompareSeen(candidate, baseline typ.Type, seen map[[2]uintptr]bool) (bool, bool) {
	if typ.SameNodeOrAcyclicEqual(candidate, baseline) {
		return false, true
	}
	if candidate == nil || baseline == nil {
		return false, false
	}
	cp, bp := typ.TypePointer(candidate), typ.TypePointer(baseline)
	if cp != 0 || bp != 0 {
		key := [2]uintptr{cp, bp}
		if seen[key] {
			return false, true
		}
		seen[key] = true
	}
	if c, ok := candidate.(*typ.Alias); ok {
		return testProductFamilyPrecisionCompareSeen(c.UnaliasedTarget(), baseline, seen)
	}
	if b, ok := baseline.(*typ.Alias); ok {
		return testProductFamilyPrecisionCompareSeen(candidate, b.UnaliasedTarget(), seen)
	}
	if c, ok := candidate.(*typ.Recursive); ok {
		b, ok := baseline.(*typ.Recursive)
		if !ok || c.Name != b.Name || c.Body == nil || b.Body == nil {
			return false, false
		}
		return testProductFamilyPrecisionCompareSeen(c.Body, b.Body, seen)
	}
	if c, ok := candidate.(*typ.Record); ok {
		b, ok := baseline.(*typ.Record)
		if !ok {
			return false, false
		}
		strict := false
		for _, baselineField := range b.Fields {
			candidateField := c.GetField(baselineField.Name)
			if candidateField == nil {
				return false, false
			}
			fieldStrict, comparable := testProductFamilyPrecisionCompareSeen(candidateField.Type, baselineField.Type, seen)
			if !comparable {
				return false, false
			}
			strict = strict || fieldStrict
		}
		return strict || len(c.Fields) > len(b.Fields), true
	}
	if typ.SameNodeOrAcyclicEqual(candidate, baseline) {
		return false, true
	}
	return false, false
}
