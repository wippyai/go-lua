package composite

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
func denominatorSpecs(axes []*axisTemplate, roles vocabulary.Roles) ([]denominator.Spec, bool) {
	specs := make([]denominator.Spec, 0, len(axes))
	for _, entry := range axes {
		semantic, ok := roles.Key(entry.Semantic())
		if !ok {
			return nil, false
		}
		specs = append(specs, denominator.Spec{
			Key:      schema.Key("coordinates/" + string(entry.Key())),
			Owner:    denominator.Owner{Surface: schema.SurfaceKindAxis, Entry: entry.Key()},
			Universe: identity.ContentID(semantic.Digest()),
			Phase:    denominator.PhasePublication,
		})
	}
	return specs, true
}

// denominatorEntries admits the authored inventory. A rejected row leaves the
// table unavailable rather than half declared.
func denominatorEntries(axes []*axisTemplate, roles vocabulary.Roles) ([]*denominator.Entry, bool) {
	specs, specsOK := denominatorSpecs(axes, roles)
	if !specsOK {
		return nil, false
	}
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
