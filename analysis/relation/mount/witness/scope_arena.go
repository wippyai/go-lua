package witness

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// scopeArena is the mounted owner of the neutral formula associated with each
// authenticated ScopeToken. It is append-only: a token can be inserted once,
// and a later insertion must carry the same immutable formula identity. The
// lock protects the map. Region is a sealed concrete value, so no mutable
// implementation can be swapped underneath an entry.
type scopeArena struct {
	mu      sync.RWMutex
	entries map[binding.ScopeToken]scopeEntry
}

type scopeEntry struct {
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
		return existing.region, true
	}
	arena.entries[token] = scopeEntry{region: value}
	return value, true
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
	_, ok := arena.resolve(token)
	return ok
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
