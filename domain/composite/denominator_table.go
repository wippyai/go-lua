package composite

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// denominatorEntries is the analyzer's closed-world coordinate inventory: one
// closed world per declared axis, the coordinate population of that axis.
//
// The inventory is derived from the axis table rather than listed beside it. A
// denominator names the universe a totality claim quantifies over, and the
// coordinate universes are exactly the coordinate populations of its spaces;
// writing them out by hand would be a second axis catalog to keep in step, and
// an axis added without its denominator would leave a claim about that axis
// quantifying over nothing.
//
// What this reads out of the axis table is the one thing the denominator
// surface does not derive: the identity of the set description, which is the
// axis's own semantic identity. The identity a verdict carries, the owning
// axis and the closure phase are the denominator surface's own derivation, so
// there is nothing authored here to disagree with them. A rejected row leaves
// the table unavailable rather than half declared.
//
// Relation families are registered directly from the generated denominator
// catalog by the composition root; they are never projected through this list.
func denominatorEntries(axes []*axisTemplate, roles vocabulary.Roles) ([]*denominator.Entry, bool) {
	entries := make([]*denominator.Entry, 0, len(axes))
	for _, entry := range axes {
		semantic, semanticOK := roles.Key(entry.Semantic())
		if !semanticOK {
			return nil, false
		}
		world, worldOK := denominator.Coordinate(entry.Key(), identity.ContentID(semantic.Digest()))
		if !worldOK {
			return nil, false
		}
		entries = append(entries, world)
	}
	return entries, true
}
