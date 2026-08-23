// Package structuretest seals one structural vocabulary projection for a law
// that needs a table rather than a composition.
//
// The structural surface's population law makes a table total: every declared
// category carries at least one densely numbered member. A law about one
// vocabulary therefore cannot project that vocabulary alone, and reproducing
// the analyzer's whole contributed inventory would pull the composition into
// packages the composition is built out of. This seals the contributions it is
// handed as themselves and populates every category they leave empty with one
// synthetic member, so the categories under test are the authored declarations
// and nothing else has to be.
package structuretest

import (
	"strconv"

	seal "github.com/wippyai/go-lua/analysis/schema/seal"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// Table seals the given contributions into one complete vocabulary
// projection. The contributions are numbered first and in order, exactly as
// the composition numbers them, so a category declared here reaches the same
// ordinals and the same identities it reaches in the analyzer.
func Table(contributions ...[]structure.Spec) (structure.Table, bool) {
	declared := map[structure.Category]bool{}
	for _, contribution := range contributions {
		for _, spec := range contribution {
			declared[spec.Category] = true
		}
	}
	var filler []structure.Spec
	for category := structure.CategoryInvalid + 1; category.Available(); category++ {
		if declared[category] {
			continue
		}
		spelling := "structuretest/" + strconv.Itoa(int(category))
		filler = append(filler, structure.Spec{
			Key: schema.Key(spelling), Category: category, Ordinal: 1, Spelling: spelling, Accepted: true,
		})
	}
	entries, collected := structure.Collect(append(contributions, filler)...)
	if !collected {
		return structure.Table{}, false
	}
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		return structure.Table{}, false
	}
	for kind := schema.SurfaceKindStructure + 1; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(emptySurface{kind: kind}) {
			return structure.Table{}, false
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		return structure.Table{}, false
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		return structure.Table{}, false
	}
	return structure.NewTable(view)
}

// emptySurface stands in for a surface this projection declares nothing on.
type emptySurface struct{ kind schema.SurfaceKind }

func (surface emptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface emptySurface) Entries() []schema.Entry  { return nil }
func (surface emptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
