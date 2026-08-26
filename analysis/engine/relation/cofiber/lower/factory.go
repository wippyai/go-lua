package lower

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/identity"
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
	state *factoryState
}

// factoryState is shared by Factory copies. Factory is intentionally a value
// capability, so copying it must not reopen the one-shot bind operation or
// permit two concurrent callers to construct two physical authorities.
type factoryState struct {
	mu       sync.Mutex
	expected identity.MountID
	lowering Lowering

	consumed bool
	cached   bool
	mounted  witness.Mounted
	view     geometry.Geometry
}

// NewFactory adopts one sealed lowering as a mount-facing capability. A
// lowering without a universe is refused here rather than at the first bind.
func NewFactory(expected identity.MountID, lowering Lowering) (Factory, bool) {
	if !expected.Available() || !lowering.Available() {
		return Factory{}, false
	}
	return Factory{state: &factoryState{expected: expected, lowering: lowering}}, true
}

// Available reports whether this factory holds a sealed lowering.
func (factory Factory) Available() bool {
	return factory.state != nil && factory.state.expected.Available() && factory.state.lowering.Available()
}

// Bind seals the physical scope authority for one mounted execution and
// returns the geometry view over it. A mount whose scopes name an atom the
// lowering was not given refuses, so a geometry never covers a scope whose
// extent nobody declared.
func (factory Factory) Bind(mounted witness.Mounted) (geometry.Geometry, bool) {
	if !factory.Available() || !mounted.Available() || mounted.RuntimeFence().Mount() != factory.state.expected {
		return geometry.Geometry{}, false
	}

	state := factory.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.cached {
		if state.mounted.Same(mounted) {
			return state.view, true
		}
		return geometry.Geometry{}, false
	}
	// Consume before construction. A failed first construction is terminal as
	// well: accepting a later mount would make the factory's one-shot contract
	// depend on which caller happened to race or which mount was tried first.
	if state.consumed {
		return geometry.Geometry{}, false
	}
	state.consumed = true

	authority, ok := cofiber.New(mounted, state.lowering.Manager(), state.lowering.Translate)
	if !ok || !authority.Available() {
		return geometry.Geometry{}, false
	}
	view, ok := geometry.New(mounted, authority)
	if !ok || !view.Available() {
		return geometry.Geometry{}, false
	}
	state.mounted = mounted
	state.view = view
	state.cached = true
	return view, true
}
