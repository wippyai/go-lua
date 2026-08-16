package grammar

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// denominatorSpecs is the authored analyzer denominator inventory: one closed
// world per declared axis, the coordinate population of that axis.
//
// The inventory is derived from the axis table rather than listed beside it. A
// denominator names the universe a totality claim quantifies over, and the
// universes the analyzer has today are exactly the coordinate populations of
// its coordinate spaces; writing them out by hand would be a second axis
// catalog to keep in step, and an axis added without its denominator would
// leave a claim about that axis quantifying over nothing.
//
// Each universe is identified by its axis's own semantic identity, which the
// axis surface has already proved unique across the inventory, so two axes
// cannot present one closed world under two names. The phase is publication:
// an axis's coordinates are derived by the solver, so the set is total only
// once the fixpoint that derives it has closed.
func denominatorSpecs(axes []*axisTemplate, bundle vocabulary.Bundle) []denominator.Spec {
	specs := make([]denominator.Spec, 0, len(axes))
	for _, entry := range axes {
		specs = append(specs, denominator.Spec{
			Key:      schema.Key("coordinates/" + string(entry.Key())),
			Owner:    denominator.Owner{Surface: schema.SurfaceKindAxis, Entry: entry.Key()},
			Universe: identity.ContentID(entry.Semantic(bundle).Digest()),
			Phase:    denominator.PhasePublication,
		})
	}
	return specs
}

// denominatorEntries admits the authored inventory. A rejected row leaves the
// table unavailable rather than half declared.
func denominatorEntries(axes []*axisTemplate, bundle vocabulary.Bundle) ([]*denominator.Entry, bool) {
	specs := denominatorSpecs(axes, bundle)
	entries := make([]*denominator.Entry, 0, len(specs))
	for _, spec := range specs {
		entry, ok := denominator.New(spec)
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	return entries, true
}
