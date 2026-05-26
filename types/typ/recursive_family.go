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

// recursiveFamilyInterner owns one canonical *Recursive handle per FamilyKey.
//
// The handle is the recursive IDENTITY of the family: its ID is fixed when the
// key is first interned and never changes, so TypeEquals and IsRecursiveRef
// treat every observation of the family as the same node. The handle's Body is
// a separate monotone lattice slot widened in place (WidenRecursiveBody): body
// refinement (field accretion, precision drift) mutates the slot under the
// stable identity without ever minting a fresh handle. The store is
// process-global and append-only.
type recursiveFamilyInternerKeyed struct {
	mu       sync.Mutex
	families map[FamilyKey]*Recursive
}

var keyedRecursiveFamilies = &recursiveFamilyInternerKeyed{
	families: make(map[FamilyKey]*Recursive),
}

// ResetKeyedRecursiveFamilies clears the keyed family interner. Owner keys (a
// function symbol, an allocation site) are unique only within one compilation, so
// the interner is scoped per compilation: a fresh compilation resets it, and two
// compilations that reuse the same symbol numbers never share a stale family body.
func ResetKeyedRecursiveFamilies() {
	keyedRecursiveFamilies.mu.Lock()
	keyedRecursiveFamilies.families = make(map[FamilyKey]*Recursive)
	keyedRecursiveFamilies.mu.Unlock()
}

// InternRecursiveFamily returns the one canonical *Recursive handle for key.
//
// The first observation of a key mints a placeholder handle with a fixed ID and
// the key recorded as its family; subsequent observations return that same
// handle. Producers seal the body with WidenRecursiveBody. The returned handle
// is the family's stable recursive identity: two observations of one family are
// literally the same pointer, so Equal is identity and Hash is the key hash,
// stable across every body refinement.
func InternRecursiveFamily(key FamilyKey) *Recursive {
	keyedRecursiveFamilies.mu.Lock()
	defer keyedRecursiveFamilies.mu.Unlock()

	if rec, ok := keyedRecursiveFamilies.families[key]; ok {
		return rec
	}
	rec := NewRecursivePlaceholder(key.String())
	rec.familyKey = key
	rec.keyed = true
	keyedRecursiveFamilies.families[key] = rec
	return rec
}

// WidenRecursiveBody widens the body slot of a keyed family handle by joining
// the candidate body into the current body, rebinding the candidate's
// self-occurrences to the family handle so the cycle stays closed on one node.
// It mutates the handle in place under its stable identity and returns the
// handle. join is the value-domain body lattice join supplied by the producer;
// it must be monotone and finite-height so the body converges.
//
// The first widen seals the handle; later widens monotonically refine the same
// slot. Identity never changes, so the handle is Equal to itself across every
// refinement and the inter-procedural fixpoint detects a fixed point on the
// family while the body slot still settles.
func WidenRecursiveBody(family *Recursive, candidateBody Type, join func(existing, candidate Type) Type) *Recursive {
	if family == nil || candidateBody == nil {
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
