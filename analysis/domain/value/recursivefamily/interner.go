package recursivefamily

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// RecursiveFamilyInterner owns one canonical *typ.Recursive handle per FamilyKey
// for a single compilation. It is the ownership boundary that makes
// cross-compilation type-state corruption impossible: a compilation may mutate
// only the *Recursive body slots its own interner minted; stdlib, manifest, DB,
// and cache type graphs are immutable inputs that the interner may reference but
// never mutate.
//
// The handle is the recursive IDENTITY of the family: its ID is fixed when the
// key is first interned and never changes, so TypeEquals and IsRecursiveRef treat
// every observation of the family as the same node. The handle's Body is a
// separate monotone lattice slot widened in place (Widen): body refinement (field
// accretion, precision drift) mutates the slot under the stable identity without
// minting a fresh handle.
//
// Owner keys (a function symbol, an allocation site) are unique only within one
// compilation, so each compilation owns its own interner instance; two
// compilations that reuse the same symbol numbers never share a family body.
type RecursiveFamilyInterner struct {
	mu       sync.Mutex
	families map[FamilyKey]*typ.Recursive
	keys     map[*typ.Recursive]FamilyKey
}

// NewRecursiveFamilyInterner creates a compilation-scoped recursive-family
// interner.
func NewRecursiveFamilyInterner() *RecursiveFamilyInterner {
	return &RecursiveFamilyInterner{
		families: make(map[FamilyKey]*typ.Recursive),
		keys:     make(map[*typ.Recursive]FamilyKey),
	}
}

// Reset clears the interner so a reused compilation context starts with no
// inherited family bodies.
func (i *RecursiveFamilyInterner) Reset() {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.families = make(map[FamilyKey]*typ.Recursive)
	i.keys = make(map[*typ.Recursive]FamilyKey)
	i.mu.Unlock()
}

// Intern returns the one canonical *typ.Recursive handle for key.
//
// The first observation of a key mints a placeholder handle with a fixed ID, the
// subsequent observations return that same handle. Producers seal the body with
// Widen. The returned handle is the family's stable recursive identity: two
// observations of one family are literally the same pointer.
func (i *RecursiveFamilyInterner) Intern(key FamilyKey) *typ.Recursive {
	if i == nil {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	if rec, ok := i.families[key]; ok {
		return rec
	}
	rec := typ.NewRecursivePlaceholder(key.String())
	i.families[key] = rec
	i.keys[rec] = key
	return rec
}

// FamilyKeyOf returns the interner-owned family key for t, when present.
func (i *RecursiveFamilyInterner) FamilyKeyOf(t typ.Type) (FamilyKey, bool) {
	rec, ok := t.(*typ.Recursive)
	if i == nil || !ok || rec == nil {
		return FamilyKey{}, false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	key, ok := i.keys[rec]
	if !ok || key.IsZero() {
		return FamilyKey{}, false
	}
	return key, true
}

// SameFamily reports whether a and b are handles owned by this interner for the
// same family key.
func (i *RecursiveFamilyInterner) SameFamily(a, b *typ.Recursive) bool {
	if a == nil || b == nil {
		return false
	}
	if a == b {
		return true
	}
	aKey, aOK := i.FamilyKeyOf(a)
	bKey, bOK := i.FamilyKeyOf(b)
	return aOK && bOK && aKey == bKey
}

// FamilyIdentityHash returns the interner-owned identity hash for rec.
func (i *RecursiveFamilyInterner) FamilyIdentityHash(rec *typ.Recursive) (uint64, bool) {
	key, ok := i.FamilyKeyOf(rec)
	if !ok {
		return 0, false
	}
	return hash.MixHash(recursiveFamilyKeyedSalt, key.Hash()), true
}

// owns reports whether family is a handle minted by this interner. Only owned
// handles may have their body slot mutated; a shared or foreign recursive node
// (stdlib/manifest/DB/cache, or one minted by another compilation) is immutable.
func (i *RecursiveFamilyInterner) owns(family *typ.Recursive) bool {
	if i == nil || family == nil {
		return false
	}
	key, ok := i.FamilyKeyOf(family)
	if !ok {
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.families[key] == family
}
