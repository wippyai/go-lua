package recursivefamily

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// FamilyKey is the stable producer identity of a recursive family.
type FamilyKey struct {
	Namespace string
	Owner     string
}

// String renders the key for a recursion-variable name.
func (k FamilyKey) String() string {
	if k.Namespace == "" {
		return k.Owner
	}
	return k.Namespace + ":" + k.Owner
}

// IsZero reports whether no family key is present.
func (k FamilyKey) IsZero() bool {
	return k == FamilyKey{}
}

// Hash folds the key into a stable bucket value.
func (k FamilyKey) Hash() uint64 {
	h := hash.FnvString(k.Namespace)
	return hash.MixHash(h, hash.FnvString(k.Owner))
}

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

// Widen widens the body slot of a family handle by joining the candidate
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
func (i *RecursiveFamilyInterner) Widen(family *typ.Recursive, candidateBody typ.Type, join func(existing, candidate typ.Type) typ.Type) *typ.Recursive {
	if family == nil || candidateBody == nil {
		return family
	}
	if !i.owns(family) {
		return family
	}
	candidateBody = i.rebindRecursiveSelf(candidateBody, family)
	if family.Body == nil {
		family.SetBody(candidateBody)
		return family
	}
	if join == nil {
		return family
	}
	widened := join(family.Body, candidateBody)
	if widened == nil || typ.SameNode(widened, family.Body) {
		return family
	}
	family.SetBody(i.rebindRecursiveSelf(widened, family))
	return family
}

// rebindRecursiveSelf rewrites every recursive reference inside body that this
// interner knows as the same family to family itself, so the widened body keeps
// a single recursion variable.
func (i *RecursiveFamilyInterner) rebindRecursiveSelf(body typ.Type, family *typ.Recursive) typ.Type {
	if body == nil || family == nil {
		return body
	}
	return typ.Rewrite(body, func(node typ.Type) (typ.Type, bool) {
		rec, ok := node.(*typ.Recursive)
		if !ok || rec == family {
			return nil, false
		}
		if i.SameFamily(rec, family) {
			return family, true
		}
		return nil, false
	})
}

// RebindRecursiveRef rewrites every reference to the recursion variable from
// inside body to to, so a body sealed under one recursion variable is re-rooted
// onto another (a fresh fold placeholder onto its family handle). The
// from node itself is left untouched when it appears as a non-reference value.
func RebindRecursiveRef(body typ.Type, from, to *typ.Recursive) typ.Type {
	if body == nil || from == nil || to == nil || from == to {
		return body
	}
	return typ.Rewrite(body, func(node typ.Type) (typ.Type, bool) {
		if typ.IsRecursiveRef(node, from) {
			return to, true
		}
		return nil, false
	})
}

// ContainsRecursiveRef reports whether body contains any reference to the
// recursion variable rec (the same node, the same ID, or a reference with
// rec by structural recursive identity.
func ContainsRecursiveRef(body typ.Type, rec *typ.Recursive) bool {
	if body == nil || rec == nil {
		return false
	}
	return typ.Contains(body, func(t typ.Type) bool {
		other, ok := t.(*typ.Recursive)
		if !ok || other == nil {
			return false
		}
		if typ.IsRecursiveRef(t, rec) {
			return true
		}
		return false
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
func CollapseUnfoldingToFamily(slot typ.Type, family *typ.Recursive) typ.Type {
	if slot == nil || family == nil {
		return slot
	}
	if typ.IsRecursiveRef(slot, family) {
		return family
	}
	return collapseChildren(slot, family)
}

// collapseChildren rebuilds slot replacing each immediate child that embeds a
// reference to family with the family handle, leaving children that do not embed
// family unchanged. Only the one-level structure is preserved; deeper unfoldings
// fold into the single recursion variable.
func collapseChildren(slot typ.Type, family *typ.Recursive) typ.Type {
	child := func(t typ.Type) typ.Type {
		if t == nil {
			return nil
		}
		if ContainsRecursiveRef(t, family) {
			return family
		}
		return t
	}
	switch v := slot.(type) {
	case *typ.Record:
		builder := typetable.NewRecord().SetOpen(v.Open)
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
			builder.MapComponent(typetable.NormalizeKey(child(v.MapKey)), child(v.MapValue))
		}
		return builder.Build()
	case *typ.Map:
		return typetable.NewMap(child(v.Key), child(v.Value))
	case *typ.ReadonlyMap:
		return typetable.NewReadonlyMap(child(v.Key), child(v.Value))
	case *typ.Array:
		return typ.NewArray(child(v.Element))
	case *typ.Optional:
		return typ.NewOptional(child(v.Inner))
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		for i, m := range v.Members {
			members[i] = child(m)
		}
		return typ.NewUnion(members...)
	default:
		return slot
	}
}

// RecursiveFamilyFingerprint folds structural recursive names reachable from t
// into a stable, order-independent hash. Interner-owned family keys are not
// global typ metadata; callers that need keyed family identity must use
// RecursiveFamilyInterner.RecursiveFamilyFingerprint.
//
// The fingerprint is the discriminator a structural product-family relation
// lacks when recursive-containing terminals collapse to one family placeholder.
// Combining a relation-level same-family check with an equal fingerprint keeps
// equivalent unfoldings shared while keeping distinct recursive names or
// interner-owned families apart.
func RecursiveFamilyFingerprint(t typ.Type) uint64 {
	fp, _ := RecursiveFamilyFingerprintWithin(t, 0)
	return fp
}

// RecursiveFamilyFingerprint folds this interner's recursive-family identities
// reachable from t into a stable, order-independent hash.
func (i *RecursiveFamilyInterner) RecursiveFamilyFingerprint(t typ.Type) uint64 {
	fp, _ := i.RecursiveFamilyFingerprintWithin(t, 0)
	return fp
}

// RecursiveFamilyFingerprintWithin is the bounded form of
// RecursiveFamilyFingerprint. When maxNodes is positive and the scan would exceed
// it, the bool is false. Callers that use the fingerprint only to share
// optimization caches can then decline sharing instead of walking enormous
// recursive product surfaces.
func RecursiveFamilyFingerprintWithin(t typ.Type, maxNodes int) (uint64, bool) {
	if t == nil {
		return 0, true
	}
	scan := recursiveFamilyFingerprintScan{
		seenFamilies: make(map[uint64]bool),
		seenNodes:    make(map[uintptr]bool),
		maxNodes:     maxNodes,
	}
	scan.scan(t)
	return scan.fp, !scan.exceeded
}

// RecursiveFamilyFingerprintWithin is the bounded form of
// RecursiveFamilyFingerprint for this interner's family identities.
func (i *RecursiveFamilyInterner) RecursiveFamilyFingerprintWithin(t typ.Type, maxNodes int) (uint64, bool) {
	if t == nil {
		return 0, true
	}
	scan := recursiveFamilyFingerprintScan{
		interner:     i,
		seenFamilies: make(map[uint64]bool),
		seenNodes:    make(map[uintptr]bool),
		maxNodes:     maxNodes,
	}
	scan.scan(t)
	return scan.fp, !scan.exceeded
}

const (
	recursiveFamilyKeyedSalt uint64 = 0x9e3779b97f4a7c15
	recursiveFamilyNamedSalt uint64 = 0xc2b2ae3d27d4eb4f
)

type recursiveFamilyFingerprintScan struct {
	interner     *RecursiveFamilyInterner
	fp           uint64
	seenFamilies map[uint64]bool
	seenNodes    map[uintptr]bool
	maxNodes     int
	nodes        int
	exceeded     bool
}

func (s *recursiveFamilyFingerprintScan) scan(t typ.Type) {
	if t == nil || s == nil || s.exceeded {
		return
	}
	if s.maxNodes > 0 {
		s.nodes++
		if s.nodes > s.maxNodes {
			s.exceeded = true
			return
		}
	}
	if rec, ok := t.(*typ.Recursive); ok {
		s.add(rec)
		return
	}
	t = typ.UnwrapAnnotations(t)
	if rec, ok := t.(*typ.Recursive); ok {
		s.add(rec)
		return
	}
	if ptr := nodeid.StructuralPointer(t); ptr != 0 {
		if s.seenNodes[ptr] {
			return
		}
		s.seenNodes[ptr] = true
	}
	switch v := t.(type) {
	case *typ.Optional:
		s.scan(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			s.scan(member)
		}
	case *typ.Intersection:
		for _, member := range v.Members {
			s.scan(member)
		}
	case *typ.Array:
		s.scan(v.Element)
	case *typ.Map:
		s.scan(v.Key)
		s.scan(v.Value)
	case *typ.ReadonlyMap:
		s.scan(v.Key)
		s.scan(v.Value)
	case *typ.Tuple:
		for _, elem := range v.Elements {
			s.scan(elem)
		}
	case *typ.Function:
		for _, param := range v.Params {
			s.scan(param.Type)
		}
		s.scan(v.Variadic)
		for _, ret := range v.Returns {
			s.scan(ret)
		}
	case *typ.Record:
		for _, field := range v.Fields {
			s.scan(field.Type)
		}
		for _, member := range v.StaticMembers {
			s.scan(member.Type)
		}
		s.scan(v.Metatable)
		if v.HasMapComponent() {
			s.scan(v.MapKey)
			s.scan(v.MapValue)
		}
	case *typ.Alias:
		s.scan(v.Target)
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			s.scan(arg)
		}
	case *typ.Interface:
		for _, method := range v.Methods {
			s.scan(method.Type)
		}
	}
}

func (s *recursiveFamilyFingerprintScan) add(rec *typ.Recursive) {
	if rec == nil || s == nil {
		return
	}
	var id uint64
	if s.interner != nil {
		if identityHash, ok := s.interner.FamilyIdentityHash(rec); ok {
			id = identityHash
		}
	}
	if id == 0 {
		id = hash.MixHash(recursiveFamilyNamedSalt, hash.FnvString(rec.Name))
	}
	if s.seenFamilies[id] {
		return
	}
	s.seenFamilies[id] = true
	// XOR folds per-family identities order-independently so the traversal order
	// does not perturb the fingerprint.
	s.fp ^= id
}
