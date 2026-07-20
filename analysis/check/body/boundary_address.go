package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// boundaryAddressContext owns the body package's point-boundary path/address
// vocabulary. High-level result queries ask this helper for the key form they
// need instead of choosing visible vs. root-or-visible spelling inline.
type boundaryAddressContext struct {
	result   *Result
	point    cfg.Point
	state    state.State
	hasState bool
}

func (r *Result) boundaryAddressContext(point cfg.Point) (boundaryAddressContext, bool) {
	if r == nil || r.visibility == nil {
		return boundaryAddressContext{}, false
	}
	in, hasState := r.boundaryStateAt(point)
	return boundaryAddressContext{result: r, point: point, state: in, hasState: hasState}, true
}

func (r *Result) beforeBoundaryAddressContext(point cfg.Point) (boundaryAddressContext, bool) {
	if r == nil || r.visibility == nil {
		return boundaryAddressContext{}, false
	}
	in, hasState := r.solvedStateAt(point)
	return boundaryAddressContext{result: r, point: point, state: in, hasState: hasState}, true
}

func (r *Result) callEntryAddressContext(point cfg.Point) (boundaryAddressContext, bool) {
	if r == nil || r.visibility == nil {
		return boundaryAddressContext{}, false
	}
	in, hasState := r.solvedStateAt(point)
	return boundaryAddressContext{result: r, point: point, state: in, hasState: hasState}, true
}

func (c boundaryAddressContext) address(p pathdom.Path) (visibility.Address, bool) {
	if c.result == nil || c.result.visibility == nil || p.IsEmpty() {
		return visibility.Address{}, false
	}
	return visibility.AddressAt(c.result.visibility, c.point, p), true
}

func (c boundaryAddressContext) visibleStateKey(p pathdom.Path) (pathaddr.StateKey, bool) {
	address, ok := c.address(p)
	if !ok {
		return "", false
	}
	return address.VisibleStateKey()
}

func (c boundaryAddressContext) visiblePathKey(p pathdom.Path) (pathdom.PathKey, bool) {
	key, ok := c.visibleStateKey(p)
	if !ok {
		return "", false
	}
	return key.PathKey(), true
}

func (c boundaryAddressContext) rootOrVisibleStateKey(p pathdom.Path) (pathaddr.StateKey, bool) {
	address, ok := c.address(p)
	if !ok {
		return "", false
	}
	return address.RootOrVisibleStateKey()
}

func (c boundaryAddressContext) relationOperand(p pathdom.Path, length bool) (state.RelOperand, bool) {
	stateKey, ok := c.rootOrVisibleStateKey(p)
	if !ok {
		return state.RelOperand{}, false
	}
	if length {
		return state.RelLengthOperand(stateKey), true
	}
	return state.RelValueOperand(stateKey), true
}

func (c boundaryAddressContext) typestateResourceKey(p pathdom.Path) (pathaddr.StateKey, bool) {
	stateKey, ok := c.visibleStateKey(p)
	if !ok {
		return "", false
	}
	if !c.hasState {
		return stateKey, true
	}
	return c.state.CanonicalTypestateResourceKey(c.result.visibility.KeySpace(), stateKey), true
}

func (c boundaryAddressContext) callEntryTypestateResourceKey(p pathdom.Path) (pathaddr.StateKey, bool) {
	var (
		stateKey pathaddr.StateKey
		ok       bool
	)
	if p.Root == "" && p.Symbol != 0 && len(p.Segments) == 0 {
		stateKey, ok = c.rootOrVisibleStateKey(p)
	} else {
		stateKey, ok = c.visibleStateKey(p)
	}
	if !ok {
		return "", false
	}
	if !c.hasState {
		return stateKey, true
	}
	return c.state.CanonicalTypestateResourceKey(c.result.visibility.KeySpace(), stateKey), true
}

func (c boundaryAddressContext) typestateResource(p pathdom.Path, protocol typestate.Protocol) (typestate.Resource, bool) {
	key, ok := c.typestateResourceKey(p)
	if !ok {
		return typestate.Resource{}, false
	}
	return state.TypestateResourceFromCanonicalKey(key, protocol), true
}

func (c boundaryAddressContext) callEntryTypestateResource(p pathdom.Path, protocol typestate.Protocol) (typestate.Resource, bool) {
	key, ok := c.callEntryTypestateResourceKey(p)
	if !ok {
		return typestate.Resource{}, false
	}
	return state.TypestateResourceFromCanonicalKey(key, protocol), true
}

func (c boundaryAddressContext) pathsEquivalent(left, right pathdom.Path) bool {
	leftKey, leftOK := c.visibleStateKey(left)
	rightKey, rightOK := c.visibleStateKey(right)
	if !leftOK || !rightOK {
		return false
	}
	if leftKey == rightKey {
		return true
	}
	if !c.hasState {
		return false
	}
	ks := c.result.visibility.KeySpace()
	leftTerm, leftOK := ks.InternStateKey(leftKey)
	rightTerm, rightOK := ks.InternStateKey(rightKey)
	return leftOK && rightOK && c.state.HasEquivalentKeyspaceKey(ks, leftTerm, rightTerm)
}

func (c boundaryAddressContext) forEachStateKey(p pathdom.Path, visit func(pathaddr.StateKey) bool, forms ...visibility.StateKeyForm) bool {
	address, ok := c.address(p)
	if !ok || visit == nil {
		return false
	}
	visited := false
	address.ForEachStateKey(func(stateKey pathaddr.StateKey) bool {
		visited = true
		return visit(stateKey)
	}, forms...)
	return visited
}

func (c boundaryAddressContext) forEachPresenceProofStateKey(p pathdom.Path, visit func(pathaddr.StateKey) bool) bool {
	return c.forEachStateKey(p, visit, visibility.StateKeyVisible, visibility.StateKeyRootOrVisible, visibility.StateKeyStructural)
}

// StateKeyAtBoundary returns the typed point-visible state key for p at point.
// It is the canonical boundary vocabulary for state lanes that use visible
// source paths; PathKeyAtBoundary exists only for compatibility with lanes that
// still expose a path-key string carrier.
func (r *Result) StateKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	addresses, ok := r.boundaryAddressContext(point)
	if !ok {
		return "", false
	}
	return addresses.visibleStateKey(p)
}

// PathKeyAtBoundary returns the canonical path key used by fact application at
// point. It is exposed for diagnostics that need to match solved state lanes
// back to call-boundary facts without re-deriving visibility policy.
func (r *Result) PathKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathdom.PathKey, bool) {
	addresses, ok := r.boundaryAddressContext(point)
	if !ok {
		return "", false
	}
	return addresses.visiblePathKey(p)
}

func (r *Result) rootOrVisibleStateKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	addresses, ok := r.boundaryAddressContext(point)
	if !ok {
		return "", false
	}
	return addresses.rootOrVisibleStateKey(p)
}

func (r *Result) relationGraphKeyAtBoundary(point cfg.Point, p pathdom.Path, length bool) (state.RelOperand, bool) {
	addresses, ok := r.boundaryAddressContext(point)
	if !ok {
		return state.RelOperand{}, false
	}
	return addresses.relationOperand(p, length)
}

// TypestateResourceKeyAtBoundary returns the canonical resource key used by the
// typestate lane at point. It folds proven path equality, matching the
// call-boundary application semantics.
func (r *Result) TypestateResourceKeyAtBoundary(point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	addresses, ok := r.boundaryAddressContext(point)
	if !ok {
		return "", false
	}
	return addresses.typestateResourceKey(p)
}

// TypestateResourceAtBoundary returns the canonical typestate resource for a
// protocol target at point. This keeps the conversion from state keys to
// typestate resource IDs inside the analysis boundary instead of diagnostics.
func (r *Result) TypestateResourceAtBoundary(point cfg.Point, p pathdom.Path, protocol typestate.Protocol) (typestate.Resource, bool) {
	addresses, ok := r.boundaryAddressContext(point)
	if !ok {
		return typestate.Resource{}, false
	}
	return addresses.typestateResource(p, protocol)
}

// TypestateResourceAtCallEntry returns the typestate resource identity used by
// call-boundary lifecycle effect application. It uses the solved call-entry
// state, not the post-call boundary state, so evidence sites join the same
// resource that the effect lane mutates.
func (r *Result) TypestateResourceAtCallEntry(point cfg.Point, p pathdom.Path, protocol typestate.Protocol) (typestate.Resource, bool) {
	addresses, ok := r.callEntryAddressContext(point)
	if !ok {
		return typestate.Resource{}, false
	}
	return addresses.callEntryTypestateResource(p, protocol)
}

// PathsEquivalentAtBoundary reports whether the solved boundary state proves
// left and right are equivalent access paths at point.
func (r *Result) PathsEquivalentAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if r == nil || left.IsEmpty() || right.IsEmpty() {
		return false
	}
	addresses, ok := r.boundaryAddressContext(point)
	if !ok {
		return false
	}
	return addresses.pathsEquivalent(left, right)
}
