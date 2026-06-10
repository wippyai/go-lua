package identity

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

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
