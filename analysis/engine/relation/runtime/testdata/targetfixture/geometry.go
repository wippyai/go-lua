package targetfixture

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber/lower"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// GeometryFactory is the neutral physical-scope factory accepted by owner
// admission tests. It exposes no guard or support representation to domain
// fixtures; those engine internals remain behind this generic test API.
type GeometryFactory interface {
	Bind(witness.Mounted) (geometry.Geometry, bool)
}

// NewGeometryFactory creates a physical lowering for a declared neutral
// region. The region identity is supplied by the owner fixture; its atom
// extents are taken from the mounted declaration and lowered through the
// generic engine path.
func NewGeometryFactory(region identity.ContentID) (GeometryFactory, bool) {
	if !region.Available() {
		return nil, false
	}
	return declaredGeometryFactory{region: region}, true
}

type declaredGeometryFactory struct {
	region identity.ContentID
}

func (factory declaredGeometryFactory) Bind(mounted witness.Mounted) (geometry.Geometry, bool) {
	if !mounted.Available() || !factory.region.Available() {
		return geometry.Geometry{}, false
	}

	// The mounted scope rows are the owner-issued neutral vocabulary. Gather
	// only their atom identities; do not materialize a table for every region
	// or conjunction. The factory's requested region is checked as a member of
	// that vocabulary so a foreign region cannot silently bind.
	atomIDs := make([]identity.ContentID, 0, 4)
	seen := make(map[identity.ContentID]struct{}, 4)
	foundRegion := false
	for _, scopeID := range mounted.Scopes() {
		scope, scopeOK := mounted.Scope(scopeID)
		if !scopeOK {
			return geometry.Geometry{}, false
		}
		value, valueOK := mounted.RegionForScope(scope)
		if !valueOK || !value.Available() {
			return geometry.Geometry{}, false
		}
		if value.Identity() == factory.region {
			foundRegion = true
		}
		for _, row := range value.Nodes() {
			id := row.Atom.ID()
			if !id.Available() {
				return geometry.Geometry{}, false
			}
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			atomIDs = append(atomIDs, id)
		}
	}
	if !foundRegion {
		return geometry.Geometry{}, false
	}
	sort.Slice(atomIDs, func(left, right int) bool {
		return bytes.Compare(atomIDs[left][:], atomIDs[right][:]) < 0
	})
	physical := make([]guard.Atom, len(atomIDs))
	if len(physical) == 0 {
		// A True-only declaration has no neutral atom. Keep a non-empty
		// physical universe, while Lowering evaluates True directly.
		physical = append(physical, guard.Atom(1))
	}
	manager, err := guard.New(physical)
	if err != nil {
		return geometry.Geometry{}, false
	}
	work := support.New(manager)
	if work == nil {
		return geometry.Geometry{}, false
	}
	extents := make(map[identity.ContentID]support.Mask, len(atomIDs))
	for index, id := range atomIDs {
		mask, maskOK := work.Literal(guard.Atom(index+1), true)
		if !maskOK {
			return geometry.Geometry{}, false
		}
		extents[id] = mask
	}
	if !work.Seal() {
		work.Discard()
		return geometry.Geometry{}, false
	}
	lowering, ok := lower.New(manager, extents)
	if !ok {
		return geometry.Geometry{}, false
	}
	capability, ok := lower.NewFactory(mounted.RuntimeFence().Mount(), lowering)
	if !ok {
		return geometry.Geometry{}, false
	}
	view, ok := capability.Bind(mounted)
	if !ok {
		return geometry.Geometry{}, false
	}
	return view, view.Available()
}
