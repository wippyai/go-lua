package semantic

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

func newUnderDomain(t testing.TB) (semanticFixture, *Domain[semanticFactor, semanticKey, uint8]) {
	t.Helper()
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:     5,
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join:        max,
		Widen:       max,
		LessOrEq:    func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("semantic domain")
	}
	return fixture, domain
}

func underPlane(t testing.TB, domain *Domain[semanticFactor, semanticKey, uint8], root diagram.Root[semanticFactor, semanticKey, uint8]) Plane[semanticFactor, semanticKey, uint8] {
	t.Helper()
	plane, ok := domain.Plane(root)
	if !ok {
		t.Fatal("semantic plane")
	}
	return plane
}

func TestUnderTreatsAbsentColumnsAsDefault(t *testing.T) {
	fixture, domain := newUnderDomain(t)
	scratch := diagram.NewSoleScratch[semanticKey, uint8]()
	empty := underPlane(t, domain, fixture.diagram.Empty())
	explicitDefault := underPlane(t, domain, fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{when: fixture.all, value: 5}))
	greater := underPlane(t, domain, fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{when: fixture.all, value: 7}))
	if !domain.EqualUnder(empty, explicitDefault, fixture.all, scratch) || !domain.EqualUnder(explicitDefault, empty, fixture.all, scratch) {
		t.Fatal("absent and explicit Default must be equal")
	}
	if !domain.LessOrEqUnder(empty, greater, fixture.all, scratch) {
		t.Fatal("Default must order below the greater explicit column")
	}
	if domain.LessOrEqUnder(greater, empty, fixture.all, scratch) {
		t.Fatal("greater explicit column ordered below Default")
	}
}

func TestUnderUsesReflexivityOnlyAtTheSemanticBoundary(t *testing.T) {
	fixture, domain := newUnderDomain(t)
	scratch := diagram.NewSoleScratch[semanticKey, uint8]()
	plane := underPlane(t, domain, fixture.root(t,
		struct {
			when  support.Mask
			value uint8
		}{when: fixture.notAtom, value: 10},
		struct {
			when  support.Mask
			value uint8
		}{when: fixture.atom, value: 20},
	))
	if !domain.EqualUnder(plane, plane, fixture.all, scratch) {
		t.Fatal("identical Plane must be equal")
	}
	if !domain.LessOrEqUnder(plane, plane, fixture.all, scratch) {
		t.Fatal("identical Plane must be ordered below itself")
	}
}

func TestUnderMasksBranchesOutsideSharedSupport(t *testing.T) {
	fixture, domain := newUnderDomain(t)
	scratch := diagram.NewSoleScratch[semanticKey, uint8]()
	left := underPlane(t, domain, fixture.root(t,
		struct {
			when  support.Mask
			value uint8
		}{when: fixture.notAtom, value: 10},
		struct {
			when  support.Mask
			value uint8
		}{when: fixture.atom, value: 20},
	))
	right := underPlane(t, domain, fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{when: fixture.all, value: 20}))
	if !domain.EqualUnder(left, right, fixture.atom, scratch) {
		t.Fatal("difference outside shared support affected equality")
	}
	if domain.EqualUnder(left, right, fixture.all, scratch) {
		t.Fatal("full support hid unequal low branch")
	}
}

func underWideRoot(t testing.TB, fixture semanticFixture, width int, first uint8) diagram.Root[semanticFactor, semanticKey, uint8] {
	t.Helper()
	builder := fixture.diagram.Begin()
	if builder == nil {
		t.Fatal("diagram builder")
	}
	root := fixture.diagram.Empty()
	for index := 0; index < width; index++ {
		value := uint8(10)
		if index == 0 {
			value = first
		}
		var ok bool
		root, ok = builder.Set(root, semanticColumn, semanticKey(index), fixture.all, fixture.ids[value])
		if !ok {
			t.Fatalf("set key %d", index)
		}
	}
	root, ok := builder.Seal(root)
	if !ok {
		t.Fatal("root seal")
	}
	return root
}

func TestUnderFirstKeyMismatchRejectsAcrossWidths(t *testing.T) {
	for _, width := range []int{1, 64, 4096} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			fixture, domain := newUnderDomain(t)
			scratch := diagram.NewSoleScratch[semanticKey, uint8]()
			left := underPlane(t, domain, underWideRoot(t, fixture, width, 20))
			right := underPlane(t, domain, underWideRoot(t, fixture, width, 10))
			if domain.EqualUnder(left, right, fixture.all, scratch) || domain.LessOrEqUnder(left, right, fixture.all, scratch) {
				t.Fatal("first key must reject equality and order")
			}
		})
	}
}

func BenchmarkUnderFirstKeyMismatch(b *testing.B) {
	for _, width := range []int{1, 64, 4096} {
		b.Run(fmt.Sprintf("width-%d", width), func(b *testing.B) {
			fixture, domain := newUnderDomain(b)
			scratch := diagram.NewSoleScratch[semanticKey, uint8]()
			left := underPlane(b, domain, underWideRoot(b, fixture, width, 20))
			right := underPlane(b, domain, underWideRoot(b, fixture, width, 10))
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				if domain.EqualUnder(left, right, fixture.all, scratch) {
					b.Fatal("equal first-key mismatch")
				}
			}
		})
	}
}
