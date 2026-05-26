package value

import (
	"sync"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/typ"
)

// recursiveFamilyInterner hash-conses recursive product families to one
// canonical representative per verified equivalence class.
//
// It buckets candidates by typ.ProductFamilyHash (a coinductive structural
// fold) and probes each bucket with factTypeMetadataEqual, the metadata-
// sensitive coinductive verifier. The fingerprint is only a bucket key: raw
// uint64 hash collisions are resolved by the verifier, so distinct families
// that happen to collide never merge. A separate alias bucket keeps each
// (alias name, canonical target) wrapper canonical so an aliased family is a
// stable distinct carrier from the bare family. The store is process-global and
// append-only, matching the deeply-immutable value contract.
type recursiveFamilyInterner struct {
	mu      sync.Mutex
	buckets map[uint64][]typ.Type
	aliases map[uint64][]*typ.Alias
}

var recursiveFamilies = &recursiveFamilyInterner{
	buckets: make(map[uint64][]typ.Type),
	aliases: make(map[uint64][]*typ.Alias),
}

// ZZResetCanonicalRecursiveFamilies clears the process-global canonical
// recursive-family interner. Diagnostic A/B for the cross-compilation leak.
func ZZResetCanonicalRecursiveFamilies() {
	recursiveFamilies.mu.Lock()
	recursiveFamilies.buckets = make(map[uint64][]typ.Type)
	recursiveFamilies.aliases = make(map[uint64][]*typ.Alias)
	recursiveFamilies.mu.Unlock()
}

// CanonicalRecursiveFamily returns the single canonical representative for the
// recursive product family of t, hash-consed across all observations.
//
// Two observations of the same recursive family carry distinct type nodes (fresh
// *typ.Recursive IDs and fresh child nodes every fixpoint iteration) yet denote
// one family. Canonicalization returns the same node pointer for every member of
// a verified equivalence class, so the value-domain shape and identity axes can
// reduce recursive Equal to canonical-rep identity, making Equal the kernel of
// Hash for recursive products by construction.
//
// Non-recursive, dynamic, and placeholder shapes are returned unchanged:
//
//   - P3.1: any, unknown, dynamic, and unsealed recursive placeholders keep their
//     separate paths and are never canonicalized (they are not concrete families).
//   - P3.2: a top-level alias wrapper is preserved; only its recursive Target is
//     canonicalized, so the alias name survives and an aliased family stays a
//     distinct carrier from the bare family.
func CanonicalRecursiveFamily(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if alias, ok := t.(*typ.Alias); ok && alias != nil {
		canonicalTarget := CanonicalRecursiveFamily(alias.Target)
		if !typ.ContainsRecursive(canonicalTarget) {
			return t
		}
		return recursiveFamilies.canonicalAlias(alias.Name, canonicalTarget)
	}
	if !canonicalizableRecursiveFamily(t) {
		return t
	}
	return recursiveFamilies.canonical(t)
}

// canonicalizableRecursiveFamily reports whether t is a concrete recursive
// product family eligible for canonicalization. Gradual placeholders (any,
// unknown) and unsealed recursive holes are excluded: they keep their own
// admission paths so the placeholder/dynamic distinction is not collapsed.
func canonicalizableRecursiveFamily(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return false
	}
	if rec, ok := t.(*typ.Recursive); ok && rec != nil && rec.Body == nil {
		return false
	}
	return typ.ContainsRecursive(t)
}

// canonical returns the canonical representative equal to t, installing t as the
// representative when its equivalence class is seen for the first time.
func (i *recursiveFamilyInterner) canonical(t typ.Type) typ.Type {
	h := typ.ProductFamilyHash(t)

	i.mu.Lock()
	defer i.mu.Unlock()

	for _, rep := range i.buckets[h] {
		if factTypeMetadataEqual(rep, t, nil) {
			return rep
		}
	}
	i.buckets[h] = append(i.buckets[h], t)
	return t
}

// canonicalAlias returns the canonical alias wrapper for (name, canonicalTarget),
// installing a fresh wrapper the first time the pair is seen. Two builds of the
// same aliased recursive family therefore share one alias node, so the aliased
// shape is a stable distinct carrier (same name over the same canonical target)
// that compares Equal by pointer like the bare family.
func (i *recursiveFamilyInterner) canonicalAlias(name string, canonicalTarget typ.Type) *typ.Alias {
	h := internal.HashCombine(internal.FnvString(name), typ.ProductFamilyHash(canonicalTarget))

	i.mu.Lock()
	defer i.mu.Unlock()

	for _, rep := range i.aliases[h] {
		if rep.Name == name && rep.Target == canonicalTarget {
			return rep
		}
	}
	wrapper := typ.NewAlias(name, canonicalTarget)
	i.aliases[h] = append(i.aliases[h], wrapper)
	return wrapper
}
