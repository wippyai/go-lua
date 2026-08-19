package semantic

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// referenceClose is the mask-then-erase-Default reading of a closed
// contribution, written as two independent pointwise passes over each column.
// It is the oracle the one tracked traversal must agree with: the law below
// compares the two on a matrix of planes, authored surfaces, and supports.
func referenceClose(t testing.TB, domain *Domain[semanticFactor, semanticKey, uint8], input Plane[semanticFactor, semanticKey, uint8], within support.Mask, surface ContributionRegions[semanticKey]) (Plane[semanticFactor, semanticKey, uint8], bool) {
	t.Helper()
	builder := domain.diagram.Begin()
	if builder == nil {
		t.Fatal("reference builder")
	}
	root, ok := builder.TransformSoleFactor(input.Root(), func(key semanticKey, value diagram.Value[uint8]) (diagram.Value[uint8], bool) {
		region, present := surface(key)
		if !present {
			return builder.Constant(terminal.ID[uint8]{})
		}
		if !region.Valid() || region.Manager() != domain.Guards() || !region.Entails(within) {
			return diagram.Value[uint8]{}, false
		}
		masked, masked0k := builder.Mask(value, region)
		if !masked0k {
			return diagram.Value[uint8]{}, false
		}
		return domain.eraseDefault(builder, masked)
	})
	if !ok {
		builder.Discard()
		return Plane[semanticFactor, semanticKey, uint8]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[semanticFactor, semanticKey, uint8]{}, false
	}
	return domain.Plane(root)
}

type closeCase struct {
	name    string
	plane   func(t testing.TB, fixture semanticFixture) diagram.Root[semanticFactor, semanticKey, uint8]
	within  func(fixture semanticFixture) support.Mask
	surface func(fixture semanticFixture) ContributionRegions[semanticKey]
}

// closeColumns writes one column per key from a per-key cell description.
func closeColumns(t testing.TB, fixture semanticFixture, width int, cell func(key semanticKey) []struct {
	when  support.Mask
	value uint8
}) diagram.Root[semanticFactor, semanticKey, uint8] {
	t.Helper()
	builder := fixture.diagram.Begin()
	if builder == nil {
		t.Fatal("column builder")
	}
	root := fixture.diagram.Empty()
	for index := 0; index < width; index++ {
		key := semanticKey(index)
		for _, write := range cell(key) {
			var ok bool
			root, ok = builder.Set(root, semanticColumn, key, write.when, fixture.ids[write.value])
			if !ok {
				t.Fatalf("write key %d value %d", key, write.value)
			}
		}
	}
	sealed, ok := builder.Seal(root)
	if !ok {
		t.Fatal("column seal")
	}
	return sealed
}

type closeWrite = struct {
	when  support.Mask
	value uint8
}

func closeCases() []closeCase {
	uniform := func(width int, value uint8) func(testing.TB, semanticFixture) diagram.Root[semanticFactor, semanticKey, uint8] {
		return func(t testing.TB, fixture semanticFixture) diagram.Root[semanticFactor, semanticKey, uint8] {
			return closeColumns(t, fixture, width, func(semanticKey) []closeWrite {
				return []closeWrite{{when: fixture.all, value: value}}
			})
		}
	}
	return []closeCase{
		{
			name: "empty-plane",
			plane: func(_ testing.TB, fixture semanticFixture) diagram.Root[semanticFactor, semanticKey, uint8] {
				return fixture.diagram.Empty()
			},
			within: func(fixture semanticFixture) support.Mask { return fixture.all },
			surface: func(fixture semanticFixture) ContributionRegions[semanticKey] {
				return func(semanticKey) (support.Mask, bool) { return fixture.all, true }
			},
		},
		{
			name:   "whole-surface-non-default",
			plane:  uniform(8, 10),
			within: func(fixture semanticFixture) support.Mask { return fixture.all },
			surface: func(fixture semanticFixture) ContributionRegions[semanticKey] {
				return func(semanticKey) (support.Mask, bool) { return fixture.all, true }
			},
		},
		{
			name:   "whole-surface-physical-default",
			plane:  uniform(8, 5),
			within: func(fixture semanticFixture) support.Mask { return fixture.all },
			surface: func(fixture semanticFixture) ContributionRegions[semanticKey] {
				return func(semanticKey) (support.Mask, bool) { return fixture.all, true }
			},
		},
		{
			name:   "narrowed-surface-erases-inside-support",
			plane:  uniform(8, 10),
			within: func(fixture semanticFixture) support.Mask { return fixture.all },
			surface: func(fixture semanticFixture) ContributionRegions[semanticKey] {
				return func(semanticKey) (support.Mask, bool) { return fixture.atom, true }
			},
		},
		{
			name: "split-column-authored-on-one-fiber",
			plane: func(t testing.TB, fixture semanticFixture) diagram.Root[semanticFactor, semanticKey, uint8] {
				return closeColumns(t, fixture, 6, func(semanticKey) []closeWrite {
					return []closeWrite{{when: fixture.atom, value: 20}, {when: fixture.notAtom, value: 5}}
				})
			},
			within: func(fixture semanticFixture) support.Mask { return fixture.all },
			surface: func(fixture semanticFixture) ContributionRegions[semanticKey] {
				return func(semanticKey) (support.Mask, bool) { return fixture.atom, true }
			},
		},
		{
			name: "latent-payload-outside-within",
			plane: func(t testing.TB, fixture semanticFixture) diagram.Root[semanticFactor, semanticKey, uint8] {
				return closeColumns(t, fixture, 6, func(semanticKey) []closeWrite {
					return []closeWrite{{when: fixture.atom, value: 20}, {when: fixture.notAtom, value: 30}}
				})
			},
			within: func(fixture semanticFixture) support.Mask { return fixture.atom },
			surface: func(fixture semanticFixture) ContributionRegions[semanticKey] {
				return func(semanticKey) (support.Mask, bool) { return fixture.atom, true }
			},
		},
		{
			name:   "unauthored-keys-are-absent",
			plane:  uniform(9, 10),
			within: func(fixture semanticFixture) support.Mask { return fixture.all },
			surface: func(fixture semanticFixture) ContributionRegions[semanticKey] {
				return func(key semanticKey) (support.Mask, bool) { return fixture.all, key%3 == 0 }
			},
		},
		{
			name:   "no-authored-surface-at-all",
			plane:  uniform(5, 10),
			within: func(fixture semanticFixture) support.Mask { return fixture.all },
			surface: func(fixture semanticFixture) ContributionRegions[semanticKey] {
				return func(semanticKey) (support.Mask, bool) { return support.Mask{}, false }
			},
		},
		{
			name: "two-atom-columns-under-conjunctive-surface",
			plane: func(t testing.TB, fixture semanticFixture) diagram.Root[semanticFactor, semanticKey, uint8] {
				return closeColumns(t, fixture, 7, func(key semanticKey) []closeWrite {
					if key%2 == 0 {
						return []closeWrite{{when: fixture.atom, value: 40}, {when: fixture.notAtom2, value: 7}}
					}
					return []closeWrite{{when: fixture.all, value: 5}, {when: fixture.atom2, value: 60}}
				})
			},
			within: func(fixture semanticFixture) support.Mask { return fixture.all },
			surface: func(fixture semanticFixture) ContributionRegions[semanticKey] {
				return func(key semanticKey) (support.Mask, bool) {
					if key%2 == 0 {
						return fixture.atom, true
					}
					return fixture.atom2, true
				}
			},
		},
	}
}

// TestCloseContributionAgreesWithMaskThenEraseDefault proves the tracked close
// denotes exactly the two-pass reading it replaces, and that a close which
// moves nothing republishes its input root instead of an equal copy.
func TestCloseContributionAgreesWithMaskThenEraseDefault(t *testing.T) {
	for _, testCase := range closeCases() {
		t.Run(testCase.name, func(t *testing.T) {
			fixture, domain := newUnderDomain(t)
			scratch := diagram.NewSoleScratch[semanticKey, uint8]()
			regions := support.New(fixture.diagram.Guards())
			input := underPlane(t, domain, testCase.plane(t, fixture))
			within := testCase.within(fixture)
			surface := testCase.surface(fixture)

			expected, ok := referenceClose(t, domain, input, within, surface)
			if !ok {
				t.Fatal("reference close")
			}
			closed, _, ok := domain.CloseContribution(input, within, surface, scratch, regions)
			if !ok {
				t.Fatal("tracked close")
			}
			if !domain.Same(closed, expected) {
				t.Fatal("tracked close denotes a different plane than mask-then-erase-Default")
			}
			if domain.Same(input, closed) && closed.Root() != input.Root() {
				t.Fatal("an unmoved close must republish the input root, not an equal copy")
			}
			if !domain.ContributionClosed(closed, within, surface) {
				t.Fatal("closed plane is not physically closed under its own surface")
			}
		})
	}
}

// TestCloseContributionChangedRegionEqualsTheEqualUnderDiff proves the region
// the close reports is the exact disagreement region between its input and
// its output: on every sub-support of within, emptiness of the reported
// region and the independent EqualUnder verdict are the same fact.
func TestCloseContributionChangedRegionEqualsTheEqualUnderDiff(t *testing.T) {
	for _, testCase := range closeCases() {
		t.Run(testCase.name, func(t *testing.T) {
			fixture, domain := newUnderDomain(t)
			scratch := diagram.NewSoleScratch[semanticKey, uint8]()
			regions := support.New(fixture.diagram.Guards())
			input := underPlane(t, domain, testCase.plane(t, fixture))
			within := testCase.within(fixture)
			surface := testCase.surface(fixture)

			// Every route is published before the close runs, so the
			// independent EqualUnder verdict reads exactly the same region the
			// close intersects its candidate report with.
			probes := []struct {
				name string
				mask support.Mask
			}{
				{"all", fixture.all},
				{"atom", fixture.atom},
				{"not-atom", fixture.notAtom},
				{"atom2", fixture.atom2},
				{"not-atom2", fixture.notAtom2},
			}
			routes := make([]support.Mask, len(probes))
			for index, probe := range probes {
				route, ok := support.Intersect(probe.mask, within)
				if !ok {
					t.Fatalf("%s route", probe.name)
				}
				routes[index] = route
			}
			closed, changed, ok := domain.CloseContribution(input, within, surface, scratch, regions)
			if !ok {
				t.Fatal("tracked close")
			}
			for index, probe := range probes {
				overlap, ok := regions.And(routes[index], changed)
				if !ok {
					t.Fatalf("%s overlap", probe.name)
				}
				if regions.Empty(overlap) != domain.EqualUnder(input, closed, routes[index], scratch) {
					t.Fatalf("%s: reported change region disagrees with the EqualUnder verdict on that route", probe.name)
				}
			}
			// Re-closing a closed plane is the same traversal over a plane it
			// already fixes, so it must report no movement at all.
			settled, again, ok := domain.CloseContribution(closed, within, surface, scratch, regions)
			if !ok {
				t.Fatal("second close")
			}
			if !regions.Empty(again) {
				t.Fatal("closing an already closed plane reported movement")
			}
			if settled.Root() != closed.Root() {
				t.Fatal("closing an already closed plane minted a new root")
			}
		})
	}
}

// BenchmarkReindexManyColumns drives the pointwise operations a transported
// plane runs per column: two masks, a totalizing zip, an existential
// discharge, the transport itself, and the Default erasure. It is the
// measurement for transaction-owned memo storage, since one Reindex crosses
// every one of those memos once per key.
func BenchmarkReindexManyColumns(b *testing.B) {
	for _, width := range []int{16, 256, 4096} {
		b.Run(fmt.Sprintf("width-%d", width), func(b *testing.B) {
			fixture, domain := newUnderDomain(b)
			manager := fixture.diagram.Guards()
			target, ok := manager.SealScope(nil)
			if !ok {
				b.Fatal("target scope")
			}
			plan, ok := manager.NewReindex(manager.AllScope(), target)
			if !ok || !plan.Forget(guard.Atom(1)) || !plan.Forget(guard.Atom(2)) {
				b.Fatal("forget builder")
			}
			relation, ok := plan.Seal()
			if !ok {
				b.Fatal("forget plan")
			}
			input := underPlane(b, domain, closeColumns(b, fixture, width, func(key semanticKey) []closeWrite {
				if key%2 == 0 {
					return []closeWrite{{when: fixture.atom, value: 20}, {when: fixture.notAtom, value: 10}}
				}
				return []closeWrite{{when: fixture.atom2, value: 40}, {when: fixture.notAtom2, value: 7}}
			}))
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				if _, ok := domain.Reindex(input, fixture.all, fixture.all, relation); !ok {
					b.Fatal("reindex")
				}
			}
		})
	}
}

// BenchmarkContributionIssuance measures the whole shape a RuleContribution
// issuance runs: close the candidate, prove it agreed with its closed surface
// inside its support, and decide whether the closed plane can republish the
// candidate root. The tracked close answers the agreement question from its
// own traversal, so the comparison is against the close plus the separate
// EqualUnder pass it removes. The converged shape is already closed; the
// default-encoded shape forces the close to erase a physical Default from
// every second column.
func BenchmarkContributionIssuance(b *testing.B) {
	shapes := []struct {
		name string
		cell func(fixture semanticFixture, key semanticKey) []closeWrite
	}{
		{"converged", func(fixture semanticFixture, key semanticKey) []closeWrite {
			if key%2 == 0 {
				return []closeWrite{{when: fixture.atom, value: 20}, {when: fixture.notAtom, value: 30}}
			}
			return []closeWrite{{when: fixture.all, value: 10}}
		}},
		{"default-encoded", func(fixture semanticFixture, key semanticKey) []closeWrite {
			if key%2 == 0 {
				return []closeWrite{{when: fixture.atom, value: 20}, {when: fixture.notAtom, value: 5}}
			}
			return []closeWrite{{when: fixture.all, value: 10}}
		}},
	}
	for _, shape := range shapes {
		for _, width := range []int{16, 256, 4096} {
			fixture, domain := newUnderDomain(b)
			input := underPlane(b, domain, closeColumns(b, fixture, width, func(key semanticKey) []closeWrite {
				return shape.cell(fixture, key)
			}))
			surface := func(semanticKey) (support.Mask, bool) { return fixture.all, true }
			b.Run(fmt.Sprintf("%s/tracked-width-%d", shape.name, width), func(b *testing.B) {
				scratch := diagram.NewSoleScratch[semanticKey, uint8]()
				regions := support.New(fixture.diagram.Guards())
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					closed, moved, ok := domain.CloseContribution(input, fixture.all, surface, scratch, regions)
					if !ok || !regions.Empty(moved) {
						b.Fatal("issuance")
					}
					domain.Same(input, closed)
				}
			})
			b.Run(fmt.Sprintf("%s/mask-then-erase-width-%d", shape.name, width), func(b *testing.B) {
				scratch := diagram.NewSoleScratch[semanticKey, uint8]()
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					closed, ok := referenceClose(b, domain, input, fixture.all, surface)
					if !ok || !domain.EqualUnder(input, closed, fixture.all, scratch) {
						b.Fatal("issuance")
					}
					domain.Same(input, closed)
				}
			})
		}
	}
}
