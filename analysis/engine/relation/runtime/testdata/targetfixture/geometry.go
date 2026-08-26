package targetfixture

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
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

// NewGeometryFactory creates a one-scope physical authority for a declared
// neutral region. The region identity is supplied by the owner fixture, while
// guard construction and cofiber translation remain generic engine work.
func NewGeometryFactory(region identity.ContentID) (GeometryFactory, bool) {
	if !region.Available() {
		return nil, false
	}
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		return nil, false
	}
	mask, ok := support.True(manager)
	if !ok {
		return nil, false
	}
	return declaredGeometryFactory{region: region, manager: manager, mask: mask}, true
}

type declaredGeometryFactory struct {
	region  identity.ContentID
	manager *guard.Manager
	mask    support.Mask
}

func (factory declaredGeometryFactory) Bind(mounted witness.Mounted) (geometry.Geometry, bool) {
	if !mounted.Available() || factory.manager == nil || !factory.mask.Valid() {
		return geometry.Geometry{}, false
	}
	authority, ok := cofiber.NewDeclared(mounted, factory.manager, map[identity.ContentID]support.Mask{
		factory.region: factory.mask,
	})
	if !ok {
		return geometry.Geometry{}, false
	}
	return geometry.New(mounted, authority)
}
