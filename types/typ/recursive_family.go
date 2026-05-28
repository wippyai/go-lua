package typ

import (
	"sync"

	"github.com/wippyai/go-lua/internal"
)

// FamilyKey is the stable owner identity of a recursive family. It is supplied
// by the producer of a recursive type (a source declaration, a class
// allocation, a value-binding store path, a self-embedding equation site), not
// derived from the body, so every observation of one recursive family resolves
// to the same interned handle regardless of how deeply its body has been
// discovered.
//
// Namespace distinguishes co-existing producer namespaces (class allocations,
// store paths, fold sites) so two producers cannot collide on a shared Owner
// string; Owner is the producer-local stable identity within that namespace.
type FamilyKey struct {
	Namespace string
	Owner     string
}

// String renders the family key for the recursion-variable name.
func (k FamilyKey) String() string {
	if k.Namespace == "" {
		return k.Owner
	}
	return k.Namespace + ":" + k.Owner
}

// hash folds the key into a stable bucket value.
func (k FamilyKey) hash() uint64 {
	h := internal.FnvString(k.Namespace)
	return internal.HashCombine(h, internal.FnvString(k.Owner))
}

// RecursiveFamilyInterner owns one canonical *Recursive handle per FamilyKey for
// a single compilation. It is the ownership boundary that makes cross-compilation
// type-state corruption impossible: a compilation may mutate only the *Recursive
// body slots its own interner minted; stdlib, manifest, DB, and cache type graphs
// are immutable inputs that the interner may reference but never mutate.
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
	families map[FamilyKey]*Recursive
}

// NewRecursiveFamilyInterner creates a compilation-scoped recursive-family
// interner.
func NewRecursiveFamilyInterner() *RecursiveFamilyInterner {
	return &RecursiveFamilyInterner{families: make(map[FamilyKey]*Recursive)}
}

// Reset clears the interner so a reused compilation context starts with no
// inherited family bodies.
func (i *RecursiveFamilyInterner) Reset() {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.families = make(map[FamilyKey]*Recursive)
	i.mu.Unlock()
}

// Intern returns the one canonical *Recursive handle for key.
//
// The first observation of a key mints a placeholder handle with a fixed ID, the
// key recorded as its family, and this interner recorded as its owner; subsequent
// observations return that same handle. Producers seal the body with Widen. The
// returned handle is the family's stable recursive identity: two observations of
// one family are literally the same pointer, so Equal is identity and Hash is the
// key hash, stable across every body refinement.
func (i *RecursiveFamilyInterner) Intern(key FamilyKey) *Recursive {
	if i == nil {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	if rec, ok := i.families[key]; ok {
		return rec
	}
	rec := NewRecursivePlaceholder(key.String())
	rec.familyKey = key
	rec.keyed = true
	rec.owner = i
	i.families[key] = rec
	return rec
}

// owns reports whether family is a handle minted by this interner. Only owned
// handles may have their body slot mutated; a shared or foreign recursive node
// (stdlib/manifest/DB/cache, or one minted by another compilation) is immutable.
func (i *RecursiveFamilyInterner) owns(family *Recursive) bool {
	return i != nil && family != nil && family.owner == i
}

// Widen widens the body slot of a keyed family handle by joining the candidate
// body into the current body, rebinding the candidate's self-occurrences to the
// family handle so the cycle stays closed on one node. It mutates the handle in
// place under its stable identity and returns the handle. join is the value-domain
// body lattice join supplied by the producer; it must be monotone and
// finite-height so the body converges.
//
// The first widen seals the handle; later widens monotonically refine the same
// slot. Identity never changes, so the handle is Equal to itself across every
// refinement and the inter-procedural fixpoint detects a fixed point on the family
// while the body slot still settles.
//
// Ownership guard: Widen mutates only a family this interner minted. A shared or
// foreign recursive node is never SetBody'd; it is returned unchanged so an
// immutable input (stdlib/manifest/DB/cache) cannot be corrupted by one
// compilation's convergence seed.
func (i *RecursiveFamilyInterner) Widen(family *Recursive, candidateBody Type, join func(existing, candidate Type) Type) *Recursive {
	if family == nil || candidateBody == nil {
		return family
	}
	if !i.owns(family) {
		return family
	}
	candidateBody = rebindRecursiveSelf(candidateBody, family)
	if family.Body == nil {
		family.SetBody(candidateBody)
		return family
	}
	if join == nil {
		return family
	}
	widened := join(family.Body, candidateBody)
	if widened == nil || SameNode(widened, family.Body) {
		return family
	}
	family.SetBody(rebindRecursiveSelf(widened, family))
	return family
}

// rebindRecursiveSelf rewrites every recursive reference inside body that
// denotes the same family (a keyed reference to family's key, or a structurally
// self-embedding occurrence of family) to family itself, so the widened body
// keeps a single recursion variable.
func rebindRecursiveSelf(body Type, family *Recursive) Type {
	if body == nil || family == nil {
		return body
	}
	return Rewrite(body, func(node Type) (Type, bool) {
		rec, ok := node.(*Recursive)
		if !ok || rec == family {
			return nil, false
		}
		if rec.keyed && rec.familyKey == family.familyKey {
			return family, true
		}
		return nil, false
	})
}

// RebindRecursiveRef rewrites every reference to the recursion variable from
// inside body to to, so a body sealed under one recursion variable is re-rooted
// onto another (a fresh fold placeholder onto its keyed family handle). The
// from node itself is left untouched when it appears as a non-reference value.
func RebindRecursiveRef(body Type, from, to *Recursive) Type {
	if body == nil || from == nil || to == nil || from == to {
		return body
	}
	return Rewrite(body, func(node Type) (Type, bool) {
		if IsRecursiveRef(node, from) {
			return to, true
		}
		return nil, false
	})
}

// ContainsRecursiveRef reports whether body contains any reference to the
// recursion variable rec (the same node, the same ID, or a keyed reference with
// rec's family key).
func ContainsRecursiveRef(body Type, rec *Recursive) bool {
	if body == nil || rec == nil {
		return false
	}
	return Contains(body, func(t Type) bool {
		other, ok := t.(*Recursive)
		if !ok || other == nil {
			return false
		}
		if other == rec || other.ID == rec.ID {
			return true
		}
		return rec.keyed && other.keyed && other.familyKey == rec.familyKey
	})
}

// CollapseUnfoldingToFamily collapses a slot that is an unfolding of family into
// the family body. Every proper descendant subtree that embeds a reference to
// family is itself a deeper unfolding; rewriting each such immediate child to
// family closes a multi-level unfolding tower onto one recursion variable while
// keeping the root's own shape (its non-recursive fields and leaves) as evidence.
//
// A slot that is itself a bare reference to family has no body and is returned as
// family. The result references only the family handle at its recursion edges.
func CollapseUnfoldingToFamily(slot Type, family *Recursive) Type {
	if slot == nil || family == nil {
		return slot
	}
	if IsRecursiveRef(slot, family) {
		return family
	}
	return collapseChildren(slot, family)
}

// collapseChildren rebuilds slot replacing each immediate child that embeds a
// reference to family with the family handle, leaving children that do not embed
// family unchanged. Only the one-level structure is preserved; deeper unfoldings
// fold into the single recursion variable.
func collapseChildren(slot Type, family *Recursive) Type {
	child := func(t Type) Type {
		if t == nil {
			return nil
		}
		if ContainsRecursiveRef(t, family) {
			return family
		}
		return t
	}
	switch v := slot.(type) {
	case *Record:
		builder := NewRecord().SetOpen(v.Open)
		for _, f := range v.Fields {
			ft := child(f.Type)
			switch {
			case f.Optional && f.Readonly:
				builder.OptReadonlyField(f.Name, ft)
			case f.Optional:
				builder.OptField(f.Name, ft)
			case f.Readonly:
				builder.ReadonlyField(f.Name, ft)
			default:
				builder.Field(f.Name, ft)
			}
		}
		if v.Metatable != nil {
			builder.Metatable(child(v.Metatable))
		}
		if v.HasMapComponent() {
			builder.MapComponent(v.MapKey, child(v.MapValue))
		}
		return builder.Build()
	case *Map:
		return NewMap(v.Key, child(v.Value))
	case *Array:
		return NewArray(child(v.Element))
	case *Optional:
		return NewOptional(child(v.Inner))
	case *Union:
		members := make([]Type, len(v.Members))
		for i, m := range v.Members {
			members[i] = child(m)
		}
		return NewUnion(members...)
	default:
		return slot
	}
}

// FamilyKeyOf returns the owner key of a keyed recursive family and whether t is
// a keyed family handle.
func FamilyKeyOf(t Type) (FamilyKey, bool) {
	rec, ok := t.(*Recursive)
	if !ok || rec == nil || !rec.keyed {
		return FamilyKey{}, false
	}
	return rec.familyKey, true
}

// RecursiveFamilyFingerprint folds the recursive-family identities reachable
// from t into a stable, order-independent hash.
//
// A recursive family is identified by its keyed owner FamilyKey when it carries
// one, otherwise by the recursion-variable name the source declaration gave it.
// Both identities are stable across body refinement and unfolding depth, so two
// equivalent unfoldings of one family reference the same handles and produce one
// fingerprint, while two distinct families (a class allocation per module, say)
// carry different identities and differ.
//
// The fingerprint is the discriminator the product-family precision relation
// lacks: SameProductFamily and precisionFamilyHash bottom out at a constant for
// any recursive-containing terminal, so they conflate distinct families that
// share structural precision. That conflation is unsound when a memoized result
// must reflect a specific family. Combining SameProductFamily with an equal
// fingerprint keeps equivalent unfoldings shared while keeping distinct families
// apart.
func RecursiveFamilyFingerprint(t Type) uint64 {
	if t == nil {
		return 0
	}
	var fp uint64
	seen := make(map[uint64]bool)
	Rewrite(t, func(node Type) (Type, bool) {
		rec, ok := node.(*Recursive)
		if !ok {
			return nil, false
		}
		var id uint64
		if rec.keyed {
			id = internal.HashCombine(recursiveFamilyKeyedSalt, rec.familyKey.hash())
		} else {
			id = internal.HashCombine(recursiveFamilyNamedSalt, internal.FnvString(rec.Name))
		}
		if !seen[id] {
			seen[id] = true
			// XOR folds per-family identities order-independently so the
			// traversal order does not perturb the fingerprint.
			fp ^= id
		}
		return nil, false
	})
	return fp
}

const (
	recursiveFamilyKeyedSalt uint64 = 0x9e3779b97f4a7c15
	recursiveFamilyNamedSalt uint64 = 0xc2b2ae3d27d4eb4f
)
