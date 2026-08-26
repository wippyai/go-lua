package witness

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// scopeArena is the mounted owner of authenticated ScopeToken membership and
// the neutral formula associated with Region-bearing entries. It is
// append-only: a token can be inserted once, and a later insertion must carry
// the same immutable formula identity. The lock protects the map. Region is a
// sealed concrete value, so no mutable implementation can be swapped
// underneath an entry.
type scopeArena struct {
	mu      sync.RWMutex
	entries map[binding.ScopeToken]scopeEntry
}

type scopeEntry struct {
	// A zero Region marks a formula-only entry.  The physical formula identity
	// is already carried by the ScopeToken map key, so the arena does not keep a
	// second copy of it.
	region region.Region
}

func newScopeArena() *scopeArena {
	return &scopeArena{entries: make(map[binding.ScopeToken]scopeEntry)}
}

func (arena *scopeArena) available() bool {
	return arena != nil && arena.entries != nil
}

// intern adopts one exact token/formula pair. Existing token identities are
// never replaced, even when a caller presents a different Region value.
func (arena *scopeArena) intern(token binding.ScopeToken, value region.Region) (region.Region, bool) {
	if arena == nil || !token.Available() || !value.Available() {
		return region.Region{}, false
	}
	arena.mu.Lock()
	defer arena.mu.Unlock()
	if arena.entries == nil {
		return region.Region{}, false
	}
	if existing, exists := arena.entries[token]; exists {
		// A formula-only entry is intentionally not upgraded to a Region. The
		// caller cannot replace the arena's immutable membership with a neutral
		// representation after physical admission.
		if !existing.region.Available() {
			return region.Region{}, false
		}
		return existing.region, true
	}
	arena.entries[token] = scopeEntry{region: value}
	return value, true
}

// internToken adopts one already-issued physical formula token into the same
// mounted arena used by neutral Regions.  It deliberately stores an empty
// entry: the formula identity is already carried by token and a physical
// formula may have no neutral Region representation.  Repeated admission of
// the exact token is idempotent, while an existing entry is never replaced or
// upgraded.
func (arena *scopeArena) internToken(token binding.ScopeToken) bool {
	if arena == nil || !token.Available() {
		return false
	}
	arena.mu.Lock()
	defer arena.mu.Unlock()
	if arena.entries == nil {
		return false
	}
	if _, exists := arena.entries[token]; exists {
		return true
	}
	arena.entries[token] = scopeEntry{}
	return true
}

// resolve authenticates one token and returns the exact arena-owned Region.
// It never scans declared scopes or reconstructs a formula from token bytes.
func (arena *scopeArena) resolve(token binding.ScopeToken) (region.Region, bool) {
	if arena == nil || !token.Available() {
		return region.Region{}, false
	}
	arena.mu.RLock()
	entry, ok := arena.entries[token]
	arena.mu.RUnlock()
	if !ok {
		return region.Region{}, false
	}
	if !entry.region.Available() {
		return region.Region{}, false
	}
	return entry.region, true
}

func (arena *scopeArena) contains(token binding.ScopeToken) bool {
	if arena == nil || !token.Available() {
		return false
	}
	arena.mu.RLock()
	_, ok := arena.entries[token]
	arena.mu.RUnlock()
	if !ok {
		return false
	}
	// Membership is broader than Region resolution: physical formula-only
	// entries are valid runtime scope identities but cannot be projected back
	// into neutral schema algebra.
	return true
}

func (arena *scopeArena) identity(token binding.ScopeToken) (identity.ContentID, bool) {
	if arena == nil || !token.Available() {
		return identity.ContentID{}, false
	}
	arena.mu.RLock()
	entry, ok := arena.entries[token]
	arena.mu.RUnlock()
	if !ok {
		return identity.ContentID{}, false
	}
	if !entry.region.Available() {
		return identity.ContentID{}, false
	}
	value := entry.region.Identity()
	return value, value.Available()
}

// scopeRegionIdentity extracts the token identity only after the concrete
// sealed region has passed its availability check. The identity is used to
// issue the runtime-fenced ScopeToken; it is not a substitute for retaining
// or validating the Region itself.
func scopeRegionIdentity(value region.Region) (identity.ContentID, bool) {
	if !value.Available() {
		return identity.ContentID{}, false
	}
	identityValue := value.Identity()
	return identityValue, identityValue.Available()
}
