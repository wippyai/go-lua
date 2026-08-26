// Package factorycatalog owns the immutable operation-factory directory used
// when a checked relation schema is composed.  It is deliberately a child of
// semantic/binding: the directory selects an already-issued Factory, but it
// does not know anything about a domain or about relation execution.
package factorycatalog

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Entry associates one exact sealed operation contract with the factory that
// owns its typed binding.  Signature is retained as the lookup witness; the
// factory is never selected by trying unrelated contracts.
type Entry struct {
	Signature signature.Signature
	Factory   binding.Factory
}

type entry struct {
	signature signature.Signature
	factory   binding.Factory
}

// Catalog is an immutable directory of operation factories.  The map is
// construction-owned and has no mutating accessor, so callers can only redeem
// a factory through the exact identity and digest checks in Bind.
type Catalog struct {
	entries map[signature.Identity]entry
	sealed  bool
}

var _ binding.Factory = Catalog{}

// NewCatalog validates and freezes an operation-factory directory.  Every
// entry must name an available signature with an available identity, and its
// factory must admit that exact signature.  A nil entry slice denotes the
// valid closed empty directory; nil factories are always refused.
func NewCatalog(entries []Entry) (Catalog, bool) {
	if entries == nil {
		entries = []Entry{}
	}

	indexed := make(map[signature.Identity]entry, len(entries))
	for _, candidate := range entries {
		operation := candidate.Signature
		identity := operation.Identity()
		if !operation.Available() || !identity.Available() || candidate.Factory == nil {
			return Catalog{}, false
		}
		if _, duplicate := indexed[identity]; duplicate {
			return Catalog{}, false
		}
		if _, admitted := admit(candidate.Factory, operation); !admitted {
			return Catalog{}, false
		}
		indexed[identity] = entry{signature: operation, factory: candidate.Factory}
	}

	return Catalog{entries: indexed, sealed: true}, true
}

// EmptyCatalog returns the valid closed empty directory.
func EmptyCatalog() Catalog {
	catalog, _ := NewCatalog(nil)
	return catalog
}

// Available reports whether this value is a sealed factory directory.  The
// empty directory is available: an unknown operation then refuses at Bind.
func (catalog Catalog) Available() bool {
	return catalog.sealed && catalog.entries != nil
}

// Count reports the number of exact operation identities in the directory.
func (catalog Catalog) Count() int {
	if !catalog.Available() {
		return 0
	}
	return len(catalog.entries)
}

// Bind redeems the factory for one exact request.  The identity is the sole
// directory coordinate; after that one lookup, the stored signature digest
// and the factory's re-admitted result must both agree.  There is no
// try-all, identity substitution, or version fallback.
func (catalog Catalog) Bind(request signature.Signature) (binding.Binding, bool) {
	if !catalog.Available() || !request.Available() || !request.Identity().Available() {
		return nil, false
	}
	selected, found := catalog.entries[request.Identity()]
	if !found || selected.signature.Digest() != request.Digest() {
		return nil, false
	}
	return admit(selected.factory, request)
}

// admit keeps hostile factories fail-closed.  Factory is intentionally the
// only callback in this package; a factory panic cannot turn a bad binding
// into a successful composition.
func admit(factory binding.Factory, operation signature.Signature) (result binding.Binding, ok bool) {
	defer func() {
		if recover() != nil {
			result = nil
			ok = false
		}
	}()
	return binding.Admit(factory, operation)
}
