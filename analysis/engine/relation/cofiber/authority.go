package cofiber

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	schemaregion "github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Authority is the one mount-fenced, solve-local cofiber authority.  It owns
// no semantic state and no parallel scope registry: Mounted's append-only
// arena remains the sole issuer of Scope tokens.  Its frozen declarations are
// only the one-time proof translating neutral declared regions into this
// exact physical guard manager.
type Authority struct{ data *authorityData }

type authorityData struct {
	mounted       witness.Mounted
	fence         binding.Fence
	manager       *guard.Manager
	declaredMasks map[witness.Scope]support.Mask
	runtimeMasks  map[witness.Scope]support.Mask

	// normalizations is a physical formula memo, not a second scope
	// registry: Mounted's arena remains the only token issuer and owner of
	// token-to-Region association.  The memo avoids repeatedly traversing a
	// reduced BDD merely to recover the same arena-issued Scope.
	mu        sync.RWMutex
	byHandle  map[guard.Guard]witness.Scope
	byFormula map[guard.FormulaID]witness.Scope
	sealed    bool
}

// NewFromLookup seals one cofiber authority from an owner-issued neutral atom
// lookup. The lookup is the only runtime bridge: it carries the exact mounted
// generation and the existing guard manager, while this package performs the
// one concrete Region-to-Mask lowering during the cold proof.
func NewFromLookup(mounted witness.Mounted, lookup Lookup) (Authority, bool) {
	if !lookup.Available() || !lookup.ValidFor(mounted) {
		return Authority{}, false
	}
	physicalIndex, indexOK := lookup.physicalIndex()
	if !indexOK {
		return Authority{}, false
	}
	authority, ok := New(mounted, lookup.manager(), func(value schemaregion.Region) (support.Mask, bool) {
		return lowerRegion(value, lookup, physicalIndex)
	})
	if !ok || authority.data == nil {
		return Authority{}, false
	}
	return authority, true
}

type declaredScope struct {
	scope  witness.Scope
	region schemaregion.Region
	mask   support.Mask
}

// New consumes the sole cold neutral-to-physical translation boundary.  The
// mapper is invoked only while New is running and is never retained.  Every
// mounted declared scope is mapped twice for determinism, then the complete
// declared vocabulary is proved to preserve conjunction and entailment before
// an Authority exists.
//
// The caller must create this at Bootstrap.  It is deliberately not an
// operator/read callback: later Mask and Normalize calls redeem only the
// sealed table and Mounted's arena.
func New(mounted witness.Mounted, manager *guard.Manager, mapRegion func(schemaregion.Region) (support.Mask, bool)) (Authority, bool) {
	if !mounted.Available() || manager == nil || !manager.Valid(manager.True()) || mapRegion == nil {
		return Authority{}, false
	}
	fence := mounted.RuntimeFence()
	if !fence.Available() {
		return Authority{}, false
	}
	data := &authorityData{
		mounted: mounted, fence: fence, manager: manager,
		declaredMasks: make(map[witness.Scope]support.Mask),
		runtimeMasks:  make(map[witness.Scope]support.Mask),
		byHandle:      make(map[guard.Guard]witness.Scope),
		byFormula:     make(map[guard.FormulaID]witness.Scope),
	}

	declaredIDs := mounted.Scopes()
	if declaredIDs == nil {
		return Authority{}, false
	}
	declared := make([]declaredScope, 0, len(declaredIDs))
	for _, id := range declaredIDs {
		scope, scopeOK := mounted.Scope(id)
		region, regionOK := mounted.RegionForScope(scope)
		if !scopeOK || !scope.ValidFor(fence) || !regionOK || !region.Available() {
			return Authority{}, false
		}
		first, firstOK := mapRegion(region)
		second, secondOK := mapRegion(region)
		if !firstOK || !secondOK || !data.owns(first) || !data.owns(second) || !first.Equal(second) {
			return Authority{}, false
		}
		// A declared scope is already the mounted owner-issued token for
		// this exact physical formula. Seed the normalization directory with
		// that token instead of minting a second runtime scope for the same
		// mask. Derived conjunctions still enter through normalize below.
		root, rootOK := first.Guard()
		formula, formulaOK := first.Identity()
		if !rootOK || !formulaOK || !formula.Available() {
			return Authority{}, false
		}
		if prior, exists := data.byFormula[formula]; exists && !prior.Same(scope) {
			return Authority{}, false
		}
		data.byFormula[formula] = scope
		data.byHandle[root] = scope
		if prior, exists := data.declaredMasks[scope]; exists && !prior.Equal(first) {
			return Authority{}, false
		}
		data.declaredMasks[scope] = first
		declared = append(declared, declaredScope{scope: scope, region: region, mask: first})
	}
	if !data.provesTranslation(declared, mapRegion) {
		return Authority{}, false
	}
	// The neutral proof rows are cold bootstrap evidence only.  Solve retains
	// one frozen Scope-to-Mask table plus the physical formula memo; keeping
	// Regions or a second normalized token vector here would preserve the
	// exact duplicate representation this authority exists to remove.
	declared = nil
	data.sealed = true
	if !data.available() {
		return Authority{}, false
	}
	return Authority{data: data}, true
}

// NewDeclared is the closed bootstrap form of New.  The declaration compiler
// or a test-world builder supplies the already-sealed neutral-region identity
// to physical-mask table; cofiber owns the only translation callback and
// consumes that table during the one cold proof.  The table is copied before
// New runs and is never retained, so this is not a second runtime scope
// authority or a compatibility path.
//
// Entries must include the declared regions and every declared conjunction
// that New's cold translation proof will visit.  Missing or foreign entries
// refuse the whole authority.
func NewDeclared(mounted witness.Mounted, manager *guard.Manager, declared map[identity.ContentID]support.Mask) (Authority, bool) {
	if mounted.Available() == false || manager == nil || len(declared) == 0 {
		return Authority{}, false
	}
	table := make(map[identity.ContentID]support.Mask, len(declared))
	for id, mask := range declared {
		if !id.Available() || !mask.Valid() || mask.Manager() != manager {
			return Authority{}, false
		}
		table[id] = mask
	}
	return New(mounted, manager, func(value schemaregion.Region) (support.Mask, bool) {
		if !value.Available() {
			return support.Mask{}, false
		}
		id := value.Identity()
		if !id.Available() {
			return support.Mask{}, false
		}
		mask, exists := table[id]
		return mask, exists && mask.Valid() && mask.Manager() == manager
	})
}

// Available reports whether this authority still names one exact mounted
// runtime, one exact guard manager, and its complete frozen translation.
func (authority Authority) Available() bool {
	return authority.data != nil && authority.data.available()
}

// Fence returns the exact mounted runtime fence captured at construction.
func (authority Authority) Fence() binding.Fence {
	if !authority.Available() {
		return binding.Fence{}
	}
	return authority.data.fence
}

// ValidFor reports whether this cofiber authority was sealed for the exact
// mounted artifact supplied by a consumer.  Its runtime fence is only the
// semantic token namespace; the retained Mounted identity also covers the
// address book and arrangement that make the physical masks meaningful.
func (authority Authority) ValidFor(mounted witness.Mounted) bool {
	if !authority.Available() || !mounted.Available() || !authority.data.mounted.Same(mounted) {
		return false
	}
	want := authority.data.mounted.Arrangement()
	got := mounted.Arrangement()
	return want.Available() && got.Available() && want.Digest() == got.Digest()
}

// Manager returns the one sealed physical Boolean universe.  It is exposed
// only to lower state owners that build/inspect physical diagrams; logical
// operators should use Scope values and Conjoin instead.
func (authority Authority) Manager() *guard.Manager {
	if !authority.Available() {
		return nil
	}
	return authority.data.manager
}

// Mask redeems one mounted Scope into its exact physical support formula.
// Declared scopes use the translation sealed by New.  Runtime-normalized
// scopes use a private mask-backed Region; arbitrary arena entries do not
// become executable just because they carry a matching runtime fence.
func (authority Authority) Mask(scope witness.Scope) (support.Mask, bool) {
	if !authority.Available() || !scope.ValidFor(authority.data.fence) {
		return support.Mask{}, false
	}
	return authority.data.mask(scope)
}

// Normalize reifies one exact physical support formula as the canonical
// mounted runtime Scope for that formula. The physical formula identity is
// used only as the cofiber memo key; it is never converted into a logical
// Region identity.
func (authority Authority) Normalize(mask support.Mask) (witness.Scope, bool) {
	if !authority.Available() || !authority.data.owns(mask) || support.Empty(mask) {
		return witness.Scope{}, false
	}
	return authority.data.normalize(mask)
}

// Conjoin obtains the exact physical intersection of two authenticated
// scopes, then returns the canonical normalized Scope for that intersection.
// This is the operator-facing scope combination seam; it never asks an
// operator to carry a Mask or to invoke neutral Region algebra directly.
func (authority Authority) Conjoin(left, right witness.Scope) (witness.Scope, bool) {
	if !authority.Available() {
		return witness.Scope{}, false
	}
	leftMask, leftOK := authority.Mask(left)
	rightMask, rightOK := authority.Mask(right)
	if !leftOK || !rightOK {
		return witness.Scope{}, false
	}
	combined, combinedOK := support.Intersect(leftMask, rightMask)
	if !combinedOK {
		return witness.Scope{}, false
	}
	return authority.Normalize(combined)
}

// Entails reports exact physical scope inclusion for two authenticated
// mounted scopes.  It is the selection-facing dual of Conjoin: operators ask
// this authority rather than reopening neutral Region algebra after state has
// partitioned a fiber.
func (authority Authority) Entails(premise, conclusion witness.Scope) bool {
	if !authority.Available() {
		return false
	}
	premiseMask, premiseOK := authority.Mask(premise)
	conclusionMask, conclusionOK := authority.Mask(conclusion)
	return premiseOK && conclusionOK && premiseMask.Entails(conclusionMask)
}

func (data *authorityData) available() bool {
	// This is deliberately constant-time.  New proves every declared entry
	// before setting sealed; runtime reads must not rescan declarations or
	// re-run type resolution on each scope access.
	return data != nil && data.sealed && data.mounted.Available() && data.fence.Available() && data.mounted.RuntimeFence().Same(data.fence) && data.manager != nil && data.manager.Valid(data.manager.True()) && data.declaredMasks != nil && data.runtimeMasks != nil && data.byHandle != nil && data.byFormula != nil
}

func (data *authorityData) owns(mask support.Mask) bool {
	return data != nil && mask.Valid() && mask.Manager() == data.manager
}

func (data *authorityData) mask(scope witness.Scope) (support.Mask, bool) {
	if mask, declared := data.declaredMasks[scope]; declared {
		return data.nonEmpty(mask)
	}
	mask, maskOK := data.runtimeMasks[scope]
	if !maskOK {
		return support.Mask{}, false
	}
	return data.nonEmpty(mask)
}

func (data *authorityData) normalize(mask support.Mask) (witness.Scope, bool) {
	if !data.owns(mask) || support.Empty(mask) {
		return witness.Scope{}, false
	}
	root, rootOK := mask.Guard()
	if !rootOK {
		return witness.Scope{}, false
	}
	data.mu.RLock()
	if scope, exists := data.byHandle[root]; exists {
		data.mu.RUnlock()
		return scope, scope.ValidFor(data.fence)
	}
	data.mu.RUnlock()

	formula, formulaOK := mask.Identity()
	if !formulaOK {
		return witness.Scope{}, false
	}
	data.mu.RLock()
	if scope, exists := data.byFormula[formula]; exists {
		data.mu.RUnlock()
		data.mu.Lock()
		data.byHandle[root] = scope
		data.mu.Unlock()
		return scope, scope.ValidFor(data.fence)
	}
	data.mu.RUnlock()

	// Serialize only first admission of one physical formula.  Scope arena
	// interning is itself concurrent-safe; this narrower memo lock prevents
	// duplicate cold formula-to-token work and makes repeat normalization a
	// constant-time same-handle lookup.
	data.mu.Lock()
	defer data.mu.Unlock()
	if scope, exists := data.byFormula[formula]; exists {
		data.byHandle[root] = scope
		return scope, scope.ValidFor(data.fence)
	}
	// FormulaID and ContentID are the same owner-issued full-width identity
	// representation. This explicit conversion crosses into Mounted's
	// formula-only arena admission; it does not hash, derive, or create a
	// neutral Region.
	scope, scopeOK := data.mounted.AdmitRuntimeFormula(identity.ContentID(formula))
	if !scopeOK || !scope.ValidFor(data.fence) {
		return witness.Scope{}, false
	}
	data.byFormula[formula] = scope
	data.byHandle[root] = scope
	data.runtimeMasks[scope] = mask
	return scope, true
}

// provesTranslation is the cold semantic admission proof for the mapper.  A
// translator is accepted only when it is deterministic and preserves the
// complete declared conjunction/entailment algebra.  The O(n²) work belongs
// at seal/bootstrap; no mapper survives into runtime.
func (data *authorityData) provesTranslation(declared []declaredScope, mapRegion func(schemaregion.Region) (support.Mask, bool)) bool {
	if data == nil || declared == nil || mapRegion == nil {
		return false
	}
	for _, left := range declared {
		for _, right := range declared {
			leftEntailsRight := schemaregion.Entails(left.region, right.region)
			if leftEntailsRight != left.mask.Entails(right.mask) {
				return false
			}
			combinedRegion, combinedOK := schemaregion.Conjoin(left.region, right.region)
			if !combinedOK || !combinedRegion.Available() {
				return false
			}
			first, firstOK := mapRegion(combinedRegion)
			second, secondOK := mapRegion(combinedRegion)
			intersection, intersectionOK := support.Intersect(left.mask, right.mask)
			if !firstOK || !secondOK || !intersectionOK || !data.owns(first) || !data.owns(second) || !first.Equal(second) || !first.Equal(intersection) {
				return false
			}
		}
	}
	return true
}

// nonEmpty is the central executable-scope boundary.  False is a valid
// physical partition result, but it denotes no emitted fiber and must never
// acquire a mounted Scope token.  Treating it as a scope would make an empty
// conjunction executable and would let Apply observe a row where no guard
// valuation exists.
func (data *authorityData) nonEmpty(mask support.Mask) (support.Mask, bool) {
	if !data.owns(mask) || support.Empty(mask) {
		return support.Mask{}, false
	}
	return mask, true
}
