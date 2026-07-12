// Package readexpr adapts Lua expression paths to check-body state reads.
package readexpr

import (
	"sync"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type Config struct {
	Registry        *axis.Registry
	Facts           factflow.Facts
	Visibility      *visibility.Resolver
	TypeValues      *typevalue.Cache
	Context         *Context
	ProofState      func(cfg.Point) (state.State, bool)
	ProofVisibility *visibility.Resolver
	Cancel          *cancellation.Token

	active *projectionActive
	memo   *projectionMemo
}

// Context owns reusable projection scratch for one stable read model. Callers
// must not share it across states that can answer the same point/path
// differently.
type Context struct {
	Cancel *cancellation.Token
	active projectionActive
	memo   projectionMemo
}

func (c *Context) bind(config Config) Config {
	if c == nil {
		return config
	}
	config.active = &c.active
	config.memo = &c.memo
	if config.Cancel == nil {
		config.Cancel = c.Cancel
	}
	return config
}

type projectionFrame struct {
	point cfg.Point
	path  projectionPathIdentity
}

type projectionFrameMapKey struct {
	point cfg.Point
	path  projectionPathIdentity
}

type projectionMemoKey struct {
	point       cfg.Point
	path        projectionPathIdentity
	overlayRoot bool
}

type projectionMemoMapKey struct {
	point       cfg.Point
	path        projectionPathIdentity
	overlayRoot bool
}

type projectionResult struct {
	value product.Value
	ok    bool
}

type projectionPathIdentity = keyspace.PathIdentity

func newProjectionPathIdentity(config Config, p pathdom.Path) (projectionPathIdentity, bool) {
	var ks *keyspace.KeySpace
	if config.Visibility != nil {
		ks = config.Visibility.KeySpace()
	}
	return keyspace.PathIdentityFromPath(ks, p)
}

const (
	projectionScratchInline       = 8
	projectionScratchRetainMapMax = 64
)

type projectionActive struct {
	small    [projectionScratchInline]projectionFrame
	smallLen int
	entries  map[projectionFrameMapKey]struct{}
}

func (a *projectionActive) contains(frame projectionFrame) bool {
	if a == nil {
		return false
	}
	for i := 0; i < a.smallLen; i++ {
		if a.small[i].equal(frame) {
			return true
		}
	}
	if a.entries == nil {
		return false
	}
	_, ok := a.entries[frame.mapKey()]
	return ok
}

func (a *projectionActive) push(frame projectionFrame) bool {
	if a == nil || a.contains(frame) {
		return false
	}
	if a.entries == nil && a.smallLen < len(a.small) {
		a.small[a.smallLen] = frame
		a.smallLen++
		return true
	}
	if a.entries == nil {
		a.entries = make(map[projectionFrameMapKey]struct{}, len(a.small)+1)
		for i := 0; i < a.smallLen; i++ {
			a.entries[a.small[i].mapKey()] = struct{}{}
			a.small[i] = projectionFrame{}
		}
		a.smallLen = 0
	}
	a.entries[frame.mapKey()] = struct{}{}
	return true
}

func (a *projectionActive) pop(frame projectionFrame) {
	if a == nil {
		return
	}
	for i := 0; i < a.smallLen; i++ {
		if a.small[i].equal(frame) {
			last := a.smallLen - 1
			a.small[i] = a.small[last]
			a.small[last] = projectionFrame{}
			a.smallLen--
			return
		}
	}
	delete(a.entries, frame.mapKey())
}

func (f projectionFrame) equal(other projectionFrame) bool {
	return f.point == other.point && f.path == other.path
}

func (f projectionFrame) mapKey() projectionFrameMapKey {
	return projectionFrameMapKey(f)
}

type projectionMemoEntry struct {
	key    projectionMemoKey
	result projectionResult
}

type projectionMemo struct {
	small    [projectionScratchInline]projectionMemoEntry
	smallLen int
	entries  map[projectionMemoMapKey]projectionResult
}

type projectionScratch struct {
	active projectionActive
	memo   projectionMemo
}

var projectionScratchPool = sync.Pool{
	New: func() any { return new(projectionScratch) },
}

func getProjectionScratch() *projectionScratch {
	return projectionScratchPool.Get().(*projectionScratch)
}

func putProjectionScratch(scratch *projectionScratch) {
	if scratch == nil {
		return
	}
	scratch.active.reset()
	scratch.memo.reset()
	projectionScratchPool.Put(scratch)
}

func (a *projectionActive) reset() {
	if a == nil {
		return
	}
	for i := 0; i < a.smallLen; i++ {
		a.small[i] = projectionFrame{}
	}
	a.smallLen = 0
	if len(a.entries) > projectionScratchRetainMapMax {
		a.entries = nil
		return
	}
	clear(a.entries)
}

func (m *projectionMemo) lookup(key projectionMemoKey) (projectionResult, bool) {
	if m == nil {
		return projectionResult{}, false
	}
	for i := 0; i < m.smallLen; i++ {
		entry := m.small[i]
		if entry.key.equal(key) {
			return entry.result, true
		}
	}
	if m.entries == nil {
		return projectionResult{}, false
	}
	result, ok := m.entries[key.mapKey()]
	return result, ok
}

func (m *projectionMemo) remember(key projectionMemoKey, result projectionResult) {
	if m == nil {
		return
	}
	for i := 0; i < m.smallLen; i++ {
		if m.small[i].key.equal(key) {
			m.small[i].result = result
			return
		}
	}
	if m.entries == nil && m.smallLen < len(m.small) {
		m.small[m.smallLen] = projectionMemoEntry{key: key, result: result}
		m.smallLen++
		return
	}
	if m.entries == nil {
		m.entries = make(map[projectionMemoMapKey]projectionResult, len(m.small)+1)
		for i := 0; i < m.smallLen; i++ {
			entry := m.small[i]
			m.entries[entry.key.mapKey()] = entry.result
			m.small[i] = projectionMemoEntry{}
		}
		m.smallLen = 0
	}
	m.entries[key.mapKey()] = result
}

func (k projectionMemoKey) equal(other projectionMemoKey) bool {
	return k.point == other.point && k.overlayRoot == other.overlayRoot && k.path == other.path
}

func (k projectionMemoKey) mapKey() projectionMemoMapKey {
	return projectionMemoMapKey(k)
}

func (m *projectionMemo) reset() {
	if m == nil {
		return
	}
	for i := 0; i < m.smallLen; i++ {
		m.small[i] = projectionMemoEntry{}
	}
	m.smallLen = 0
	if len(m.entries) > projectionScratchRetainMapMax {
		m.entries = nil
		return
	}
	clear(m.entries)
}

func Provider(config Config) sourcevalue.ExpressionValueProvider {
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	return func(point cfg.Point, expr factflow.ExprRef, _ factflow.ValueSource, in state.State) (product.Value, bool) {
		p, ok := config.Facts.ExpressionPathRef(expr)
		if ok {
			return Project(config, point, p, in)
		}
		dyn, ok := config.Facts.DynamicIndexExpression(expr)
		if !ok {
			return product.Value{}, false
		}
		// Dynamic reads must pass through the shared concrete/symbolic kernel.
		// Recasting an exact scalar key as a static Project path loses the
		// stable-heap proof that a missing member evaluates to explicit nil.
		return dynamicIndexExpressionValue(config, point, dyn, in)
	}
}

// DynamicIndexReadProvenPresent reports whether the solved facts prove that a
// dynamic-index expression reads an existing non-nil slot at point. It exposes
// the same proof used by expression projection so diagnostic readmodels do not
// rebuild in-range or key-membership logic independently.
func DynamicIndexReadProvenPresent(config Config, point cfg.Point, expr factflow.ExprRef, in state.State) bool {
	dyn, ok := config.Facts.DynamicIndexExpression(expr)
	if !ok {
		return false
	}
	if dynamicIndexKeyMembershipProvesRead(config, point, dyn, in) ||
		dynamicIndexInBoundsProvesRead(config, point, dyn, in) {
		return true
	}
	if _, ok := dynamicIndexExpressionProvenMemberValue(config, point, dyn, in); ok {
		return true
	}
	return false
}

func Project(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	config = config.Context.bind(config)
	if config.Cancel != nil && config.Cancel.Canceled() {
		return product.Value{}, false
	}
	var scratch *projectionScratch
	if config.active == nil || config.memo == nil {
		scratch = getProjectionScratch()
		defer putProjectionScratch(scratch)
		if config.active == nil {
			config.active = &scratch.active
		}
		if config.memo == nil {
			config.memo = &scratch.memo
		}
	}
	value, ok := project(config, point, p, in, true)
	if !ok {
		return product.Value{}, false
	}
	return applyPathPresenceProof(config, point, p, in, value), true
}

// ProjectWithoutRootStaticMemberOverlay projects a path without synthesizing a
// root type witness from current static-member facts. It is for callers that can
// first answer from an existing root witness and only need the expensive overlay
// as a fallback.
func ProjectWithoutRootStaticMemberOverlay(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	config = config.Context.bind(config)
	if config.Cancel != nil && config.Cancel.Canceled() {
		return product.Value{}, false
	}
	var scratch *projectionScratch
	if config.active == nil || config.memo == nil {
		scratch = getProjectionScratch()
		defer putProjectionScratch(scratch)
		if config.active == nil {
			config.active = &scratch.active
		}
		if config.memo == nil {
			config.memo = &scratch.memo
		}
	}
	value, ok := project(config, point, p, in, false)
	if !ok {
		return product.Value{}, false
	}
	return applyPathPresenceProof(config, point, p, in, value), true
}

func applyPathPresenceProof(config Config, point cfg.Point, p pathdom.Path, in state.State, value product.Value) product.Value {
	proven, ok := pathPresenceProof(config, point, p, in)
	if !ok {
		return value
	}
	value = product.WithPresence(config.Registry, value, proven)
	if presence.Equal(proven, presence.Present()) {
		value = sourcevalue.WithoutNilRuntimeKind(config.Registry, value)
	}
	return value
}

func pathPresenceProof(config Config, point cfg.Point, p pathdom.Path, in state.State) (presence.Value, bool) {
	if config.Visibility == nil || p.IsEmpty() {
		return presence.Bottom(), false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return presence.Bottom(), false
	}
	var out presence.Value
	found := false
	visibility.AddressAt(config.Visibility, point, p).ForEachStateKey(func(stateKey pathaddr.StateKey) bool {
		key, ok := ks.InternStateKey(stateKey)
		if !ok {
			return true
		}
		proven, ok := in.BranchProofPresence(key)
		if !ok {
			return true
		}
		if found && !presence.Equal(out, proven) {
			found = false
			return false
		}
		out = proven
		found = true
		return true
	}, visibility.StateKeyVisible, visibility.StateKeyRootOrVisible, visibility.StateKeyStructural)
	if !found {
		return presence.Bottom(), false
	}
	return out, true
}

// ProjectStaticMember reads one proven static member for owner without
// synthesizing a whole root static-member witness.
func ProjectStaticMember(config Config, point cfg.Point, owner pathdom.Path, member segment.Segment, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	config = config.Context.bind(config)
	var scratch *projectionScratch
	if config.active == nil {
		scratch = getProjectionScratch()
		defer putProjectionScratch(scratch)
		config.active = &scratch.active
	}
	if config.Visibility == nil || owner.IsEmpty() {
		return product.Value{}, false
	}
	if memberKey, ok := staticMemberLocalKey(config, point, owner, member); ok {
		value, ok := readStaticMemberLocalValue(config, memberKey, in)
		if !ok {
			return product.Value{}, false
		}
		if current, ok := currentLocalPathKeyValue(config, memberKey, in); ok {
			value = mergeCurrentStaticMemberValue(reg, config.TypeValues, value, current, true)
		}
		return value, true
	}
	memberPath := owner.AppendSegments([]segment.Segment{member})
	memberKey, ok := visibility.AddressAt(config.Visibility, point, memberPath).VisiblePathKey()
	if !ok {
		return product.Value{}, false
	}
	value, ok := readStaticMemberValue(config, memberKey, in)
	if !ok {
		return product.Value{}, false
	}
	if current, ok := currentPathKeyValue(config, point, memberPath, in); ok {
		value = mergeCurrentStaticMemberValue(reg, config.TypeValues, value, current, true)
	}
	return value, true
}
