package witness

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// scopeArena is the mounted owner of the neutral formula associated with each
// authenticated ScopeToken. It is append-only: a token can be inserted once,
// and a later insertion must carry the same immutable formula identity. The
// lock protects both the map and the consistency check against a mutable
// Region implementation.
type scopeArena struct {
	mu      sync.RWMutex
	entries map[binding.ScopeToken]scopeEntry
}

type scopeEntry struct {
	region   Region
	identity identity.ContentID
}

func newScopeArena() *scopeArena {
	return &scopeArena{entries: make(map[binding.ScopeToken]scopeEntry)}
}

func (arena *scopeArena) available() bool {
	return arena != nil && arena.entries != nil
}

// intern adopts one exact token/formula pair. Existing token identities are
// never replaced, even when a caller presents a different Region value.
func (arena *scopeArena) intern(token binding.ScopeToken, region Region) (Region, bool) {
	if arena == nil || !token.Available() {
		return nil, false
	}
	regionID, ok := scopeRegionIdentity(region)
	if !ok {
		return nil, false
	}
	arena.mu.Lock()
	defer arena.mu.Unlock()
	if arena.entries == nil {
		return nil, false
	}
	if existing, exists := arena.entries[token]; exists {
		if existing.identity != regionID {
			return nil, false
		}
		if currentID, currentOK := scopeRegionIdentity(existing.region); !currentOK || currentID != existing.identity {
			return nil, false
		}
		return existing.region, true
	}
	arena.entries[token] = scopeEntry{region: region, identity: regionID}
	return region, true
}

// resolve authenticates one token and returns the exact arena-owned Region.
// It never scans declared scopes or reconstructs a formula from token bytes.
func (arena *scopeArena) resolve(token binding.ScopeToken) (Region, bool) {
	if arena == nil || !token.Available() {
		return nil, false
	}
	arena.mu.RLock()
	entry, ok := arena.entries[token]
	arena.mu.RUnlock()
	if !ok {
		return nil, false
	}
	currentID, currentOK := scopeRegionIdentity(entry.region)
	if !currentOK || currentID != entry.identity {
		return nil, false
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
	currentID, currentOK := scopeRegionIdentity(entry.region)
	if !currentOK || currentID != entry.identity {
		return identity.ContentID{}, false
	}
	return entry.identity, true
}

func scopeRegionIdentity(region Region) (identity.ContentID, bool) {
	if region == nil {
		return identity.ContentID{}, false
	}
	value, ok := region.Identity()
	return value, ok && value.Available()
}
