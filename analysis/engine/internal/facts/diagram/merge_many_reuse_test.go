package diagram

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
)

func TestMergeSoleFactorManyReusesSemanticReferenceAndSparsifiesUndefined(t *testing.T) {
	fixture := newRelationFixture(t)
	// The reference column is a nondefault value over all guards. The two
	// real operands carry it on complementary authored surfaces.
	on := fixture.atom
	regions := support.New(fixture.diagram.guards)
	if regions == nil {
		t.Fatal("region work")
	}
	notOn, ok := regions.Not(on)
	if !ok {
		t.Fatal("off support")
	}
	if !regions.Seal() {
		t.Fatal("region seal")
	}
	reference := fixture.sealed(t, relationWrite{key: 1, when: fixture.all, value: 20})
	left := fixture.sealed(t, relationWrite{key: 1, when: on, value: 20})
	right := fixture.sealed(t, relationWrite{key: 1, when: notOn, value: 20})

	combine := func(_ relationKey, reference terminal.ID[uint8], values []terminal.ID[uint8], present []bool) (terminal.ID[uint8], bool) {
		for index := range values {
			if present[index] {
				return values[index], true
			}
		}
		return reference, true
	}

	t.Run("reference_reuse", func(t *testing.T) {
		builder := fixture.diagram.Begin()
		if builder == nil {
			t.Fatal("builder")
		}
		work := support.New(fixture.diagram.guards)
		if work == nil {
			t.Fatal("work")
		}
		merged, valid := builder.MergeSoleFactorMany(reference, []Root[relationFactor, relationKey, uint8]{left, right}, NewSoleScratch[relationKey, uint8](), work, combine, func(key relationKey, output []support.Mask) bool {
			if key != 1 || len(output) != 2 {
				return false
			}
			output[0], output[1] = on, notOn
			return true
		})
		if !valid {
			builder.Discard()
			t.Fatal("many merge")
		}
		if merged.root != reference.root || merged.count != reference.count {
			builder.Discard()
			t.Fatal("equal reconstruction did not retain reference root")
		}
		if _, valid := builder.Seal(merged); !valid {
			t.Fatal("seal")
		}
	})

	t.Run("undefined_result_removes_reference_column", func(t *testing.T) {
		builder := fixture.diagram.Begin()
		if builder == nil {
			t.Fatal("builder")
		}
		work := support.New(fixture.diagram.guards)
		if work == nil {
			t.Fatal("work")
		}
		merged, valid := builder.MergeSoleFactorMany(reference, []Root[relationFactor, relationKey, uint8]{fixture.diagram.Empty(), fixture.diagram.Empty()}, NewSoleScratch[relationKey, uint8](), work, func(_ relationKey, _ terminal.ID[uint8], _ []terminal.ID[uint8], present []bool) (terminal.ID[uint8], bool) {
			if len(present) != 2 || present[0] == present[1] {
				return terminal.ID[uint8]{}, false
			}
			return terminal.ID[uint8]{}, true
		}, func(key relationKey, output []support.Mask) bool {
			if key != 1 || len(output) != 2 {
				return false
			}
			// Both branches are Present(Default) through complementary coverage.
			// They must choose the one zero terminal pointer, reduce low==high,
			// and delete the stale reference column rather than retain two
			// sibling undefined leaves.
			output[0], output[1] = on, notOn
			return true
		})
		if !valid {
			builder.Discard()
			t.Fatal("many merge")
		}
		if merged.count != 0 || merged.root != nil {
			builder.Discard()
			t.Fatalf("undefined fold retained stale sparse column: root=%p count=%d", merged.root, merged.count)
		}
		sealed, valid := builder.Seal(merged)
		if !valid {
			t.Fatal("seal")
		}
		if count, valid := fixture.diagram.Count(sealed); !valid || count != 0 {
			t.Fatalf("sealed undefined fold count=%d/%t", count, valid)
		}
	})
}
