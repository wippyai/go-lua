package typ

import "testing"

func TestRecursiveFamilyFingerprintScansSurfaceWithoutUnfoldingBodies(t *testing.T) {
	hidden := NewRecursive("Hidden", func(self Type) Type {
		return NewRecord().Field("next", self).Build()
	})
	outer := NewRecursivePlaceholder("Outer")
	outer.SetBody(NewRecord().Field("hidden", hidden).Build())
	visible := NewRecursive("Visible", func(self Type) Type {
		return NewRecord().Field("next", self).Build()
	})

	outerOnly := RecursiveFamilyFingerprint(outer)
	if outerOnly == 0 {
		t.Fatal("outer recursive family fingerprint should be non-zero")
	}

	duplicateOuter := NewRecord().
		Field("first", outer).
		Field("second", outer).
		Build()
	if got := RecursiveFamilyFingerprint(duplicateOuter); got != outerOnly {
		t.Fatalf("duplicate outer fingerprint = %x, want %x", got, outerOnly)
	}

	if got := RecursiveFamilyFingerprint(NewRecord().Field("outer", outer).Build()); got != outerOnly {
		t.Fatalf("record containing outer fingerprint = %x, want %x", got, outerOnly)
	}

	explicitHidden := RecursiveFamilyFingerprint(NewRecord().
		Field("outer", outer).
		Field("hidden", hidden).
		Build())
	if explicitHidden == outerOnly {
		t.Fatal("explicit hidden family should affect fingerprint")
	}

	surface := NewRecord().
		Field("handler", Func().Param("self", outer).Returns(NewArray(visible)).Build()).
		Field("duplicate", outer).
		MapComponent(String, visible).
		Build()
	surfaceFP := RecursiveFamilyFingerprint(surface)
	if surfaceFP == 0 || surfaceFP == outerOnly {
		t.Fatalf("surface fingerprint = %x, want visible family included with outer", surfaceFP)
	}

	repeatedSurface := NewRecord().
		Field("a", surface).
		Field("b", surface).
		Build()
	if got := RecursiveFamilyFingerprint(repeatedSurface); got != surfaceFP {
		t.Fatalf("repeated surface fingerprint = %x, want %x", got, surfaceFP)
	}
}

func TestRecursiveFamilyFingerprintWithinReportsBudgetExhaustion(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("next", self).Build()
	})
	surface := Type(node)
	for i := 0; i < 8; i++ {
		surface = NewRecord().
			Field("left", surface).
			Field("right", NewArray(surface)).
			Build()
	}

	if _, ok := RecursiveFamilyFingerprintWithin(surface, 4); ok {
		t.Fatal("small budget should report exhaustion")
	}
	if fp, ok := RecursiveFamilyFingerprintWithin(surface, 0); !ok || fp == 0 {
		t.Fatalf("unbounded fingerprint = %x/%v, want non-zero/true", fp, ok)
	}
}
