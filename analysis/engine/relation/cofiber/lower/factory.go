package lower

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// Factory binds one mounted execution to the physical scope authority its own
// declared scopes fold to.
//
// It is the capability form the admission boundary accepts. The mount already
// carries the region each of its scopes stands on, so nothing here selects a
// scope or restates a plan: cofiber runs its cold proof over those regions and
// this fold answers each one, including the conjunctions the proof forms.
type Factory struct {
	lowering Lowering
}

// NewFactory adopts one sealed lowering as a mount-facing capability. A
// lowering without a universe is refused here rather than at the first bind.
func NewFactory(lowering Lowering) (Factory, bool) {
	if !lowering.Available() {
		return Factory{}, false
	}
	return Factory{lowering: lowering}, true
}

// Available reports whether this factory holds a sealed lowering.
func (factory Factory) Available() bool { return factory.lowering.Available() }

// Bind seals the physical scope authority for one mounted execution and
// returns the geometry view over it. A mount whose scopes name an atom the
// lowering was not given refuses, so a geometry never covers a scope whose
// extent nobody declared.
func (factory Factory) Bind(mounted witness.Mounted) (geometry.Geometry, bool) {
	if !factory.Available() || !mounted.Available() {
		return geometry.Geometry{}, false
	}
	authority, ok := cofiber.New(mounted, factory.lowering.Manager(), factory.lowering.Translate)
	if !ok || !authority.Available() {
		return geometry.Geometry{}, false
	}
	return geometry.New(mounted, authority)
}
