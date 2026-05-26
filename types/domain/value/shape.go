package value

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Equivalent reports structural equality or mutual subtyping.
func Equivalent(a, b typ.Type) bool {
	return typ.TypeEquals(a, b) || (subtype.IsSubtype(a, b) && subtype.IsSubtype(b, a))
}

// ElidesOptional reports whether candidate is inside baseline after nil is
// removed from baseline.
func ElidesOptional(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil {
		return false
	}
	nonNil, nilable := SplitNilable(baseline)
	if !nilable || nonNil == nil || typ.TypeEquals(nonNil, baseline) {
		return false
	}
	if optionalEvidenceFamilyCovers(candidate, nonNil) {
		return true
	}
	if typ.ContainsRecursive(candidate) || typ.ContainsRecursive(nonNil) {
		return false
	}
	return subtype.IsSubtype(candidate, nonNil)
}

func optionalEvidenceFamilyCovers(candidate, nonNil typ.Type) bool {
	if candidate == nil || nonNil == nil {
		return false
	}
	if typ.TypeEquals(candidate, nonNil) {
		return true
	}
	if typ.ContainsRecursive(candidate) || typ.ContainsRecursive(nonNil) {
		if _, comparable := typ.ComparePrecision(candidate, nonNil); comparable {
			return true
		}
		if refines, _ := RefinesSoftContainer(candidate, nonNil); refines {
			return true
		}
	}
	return false
}

// SplitNilable separates the non-nil component from an optional/nilable type.
func SplitNilable(t typ.Type) (typ.Type, bool) {
	t = unwrap.Alias(t)
	switch v := t.(type) {
	case nil:
		return nil, false
	case *typ.Optional:
		return v.Inner, true
	case *typ.Union:
		nilable := false
		for _, member := range v.Members {
			member = unwrap.Alias(member)
			if unwrap.IsNilType(member) {
				nilable = true
				break
			}
		}
		if !nilable {
			return t, false
		}
		return typ.UnionWithoutNil(v), true
	default:
		if unwrap.IsNilType(t) {
			return nil, true
		}
		return t, false
	}
}

// IsTruthyRefinement reports whether candidate equals or subtypes the truthy
// refinement of baseline.
func IsTruthyRefinement(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil || typ.TypeEquals(candidate, baseline) {
		return false
	}
	refined := narrow.ToTruthy(baseline)
	if refined == nil || refined.Kind().IsNever() || typ.TypeEquals(refined, baseline) {
		return false
	}
	return typ.TypeEquals(candidate, refined) || subtype.IsSubtype(candidate, refined)
}

// PreferConcreteOverSoft selects a concrete observation over a soft placeholder
// while preserving explicit nilability.
func PreferConcreteOverSoft(a, b typ.Type) (typ.Type, bool) {
	aSoft := typ.IsSoft(a, typ.SoftPlaceholderPolicy)
	bSoft := typ.IsSoft(b, typ.SoftPlaceholderPolicy)
	switch {
	case aSoft && !bSoft && !unwrap.IsNilType(b):
		return b, true
	case bSoft && !aSoft && !unwrap.IsNilType(a):
		return a, true
	}
	if preferred, ok := preferConcreteOverNilableSoft(a, b); ok {
		return preferred, true
	}
	return nil, false
}

func preferConcreteOverNilableSoft(a, b typ.Type) (typ.Type, bool) {
	if preferred, ok := preferConcreteOverNilableSoftDirected(a, b); ok {
		return preferred, true
	}
	return preferConcreteOverNilableSoftDirected(b, a)
}

func preferConcreteOverNilableSoftDirected(softMaybeNil, concrete typ.Type) (typ.Type, bool) {
	inner, nilable := SplitNilable(softMaybeNil)
	if !nilable || inner == nil || !typ.IsSoft(inner, typ.SoftPlaceholderPolicy) {
		return nil, false
	}
	if concrete == nil || unwrap.IsNilType(concrete) {
		return nil, false
	}
	concreteInner, concreteNilable := SplitNilable(concrete)
	if concreteInner == nil {
		return nil, false
	}
	if typ.IsSoft(concreteInner, typ.SoftPlaceholderPolicy) {
		return nil, false
	}
	if concreteNilable {
		return concrete, true
	}
	return typ.NewOptional(concrete), true
}

// CanSelfEmbed reports whether t is a structural type that can recursively
// carry another type value below itself.
func CanSelfEmbed(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch v := t.(type) {
	case *typ.Annotated:
		return CanSelfEmbed(v.Inner)
	case *typ.Alias:
		return CanSelfEmbed(v.Target)
	case *typ.Optional:
		return CanSelfEmbed(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if CanSelfEmbed(member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range v.Members {
			if CanSelfEmbed(member) {
				return true
			}
		}
		return false
	case *typ.Array, *typ.Map, *typ.Tuple, *typ.Record, *typ.Function, *typ.Recursive:
		return true
	default:
		return false
	}
}

// ContainsEquivalent reports whether haystack contains a node equivalent to
// needle while walking structural type children.
func ContainsEquivalent(haystack, needle typ.Type) bool {
	if haystack == nil || needle == nil {
		return false
	}
	if sameEmbeddingNode(haystack, needle) {
		return true
	}
	scanner := structuralScanner{
		visit:  nil,
		seen:   make(structuralTypeSeen),
		hashes: make(map[typ.Type]uint64),
	}
	needleHash := scanner.hash(needle)
	scanner.visit = func(node typ.Type) (bool, bool) {
		if scanner.hash(node) == needleHash && sameEmbeddingNode(node, needle) {
			return true, false
		}
		return false, true
	}
	return scanner.scan(haystack, typ.NewGuard())
}

func sameEmbeddingNode(a, b typ.Type) bool {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if typ.ContainsRecursive(a) || typ.ContainsRecursive(b) {
		return SameEvidenceFamily(a, b)
	}
	return false
}

// ContainsUnion reports whether t contains any union node.
func ContainsUnion(t typ.Type) bool {
	if t == nil {
		return false
	}
	return Scan(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if _, ok := node.(*typ.Union); ok {
			return true, false
		}
		return false, true
	})
}

// SelfEmbeddingUpperBound reports the finite upper bound for observations where
// one structural value embeds the other below itself. Shape-specific joins must
// not recursively descend into those pairs, or convergence can build unbounded
// tuple/record towers such as T, {T}, {{T}}, ...
func SelfEmbeddingUpperBound(a, b typ.Type) (typ.Type, bool) {
	if typ.SameNode(a, b) {
		return a, true
	}
	if upper, ok := existingRecursiveUpperBound(a, b); ok {
		return upper, true
	}
	if upper, ok := topSequenceSelfEmbeddingUpperBound(a, b); ok {
		return upper, true
	}
	if folded, ok := FoldSelfEmbedding(a, b); ok {
		return folded, true
	}
	if folded, ok := FoldSelfEmbedding(b, a); ok {
		return folded, true
	}
	return nil, false
}

// DirectSelfEmbeddingUpperBound reports the finite upper bound for a one-step
// recursive observation. It is the cheap admission guard for callers that are
// about to recursively descend through two structural products: if either side
// already appears as an immediate child of the other, descent would only build
// another copy of the same product, so the value lattice must fold it first.
func DirectSelfEmbeddingUpperBound(a, b typ.Type) (typ.Type, bool) {
	if !canUseDirectSelfEmbedding(a) && !canUseDirectSelfEmbedding(b) {
		return nil, false
	}
	if canUseSelfEmbeddingAnchor(a) && canUseDirectSelfEmbedding(b) && directlyEmbedsAnchor(b, a) {
		if upper, ok := SelfEmbeddingUpperBound(a, b); ok {
			return upper, true
		}
	}
	if canUseSelfEmbeddingAnchor(b) && canUseDirectSelfEmbedding(a) && directlyEmbedsAnchor(a, b) {
		if upper, ok := SelfEmbeddingUpperBound(b, a); ok {
			return upper, true
		}
	}
	return nil, false
}

func canUseDirectSelfEmbedding(t typ.Type) bool {
	switch v := UnwrapStructuralShape(t).(type) {
	case *typ.Optional:
		return canUseDirectSelfEmbedding(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if canUseDirectSelfEmbedding(member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range v.Members {
			if canUseDirectSelfEmbedding(member) {
				return true
			}
		}
		return false
	case *typ.Array, *typ.Map, *typ.Tuple, *typ.Record:
		return true
	default:
		return false
	}
}

func directlyEmbedsAnchor(observation, anchor typ.Type) bool {
	if observation == nil || anchor == nil {
		return false
	}
	observation = UnwrapStructuralShape(observation)
	anchor = UnwrapStructuralShape(anchor)
	if observation == nil || anchor == nil {
		return false
	}
	if !CanSelfEmbed(anchor) {
		return false
	}
	if typ.SameNode(observation, anchor) {
		return false
	}
	scan := directEmbeddingScan{
		anchor:     anchor,
		anchorKind: anchor.Kind(),
		anchorHash: typ.EqualityHash(anchor),
		seen:       make(structuralTypeSeen),
	}
	return scan.immediateChildMatchesAnchor(observation)
}

type directEmbeddingScan struct {
	anchor     typ.Type
	anchorKind kind.Kind
	anchorHash uint64
	seen       structuralTypeSeen
}

func (s *directEmbeddingScan) alreadySeen(t typ.Type) bool {
	hash := structuralSeenHash(t)
	if s.seen.contains(hash, t) {
		return true
	}
	s.seen.remember(hash, t)
	return false
}

func (s *directEmbeddingScan) immediateChildMatchesAnchor(t typ.Type) bool {
	switch v := UnwrapStructuralShape(t).(type) {
	case *typ.Optional:
		return s.immediateChildMatchesAnchor(v.Inner)
	case *typ.Union:
		if s.alreadySeen(v) {
			return false
		}
		for _, member := range v.Members {
			if s.immediateChildMatchesAnchor(member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		if s.alreadySeen(v) {
			return false
		}
		for _, member := range v.Members {
			if s.immediateChildMatchesAnchor(member) {
				return true
			}
		}
		return false
	case *typ.Array:
		return s.childDirectlyMatchesAnchor(v.Element)
	case *typ.Map:
		return s.childDirectlyMatchesAnchor(v.Key) ||
			s.childDirectlyMatchesAnchor(v.Value)
	case *typ.Tuple:
		if s.alreadySeen(v) {
			return false
		}
		for _, elem := range v.Elements {
			if s.childDirectlyMatchesAnchor(elem) {
				return true
			}
		}
		return false
	case *typ.Record:
		if s.alreadySeen(v) {
			return false
		}
		if s.childDirectlyMatchesAnchor(v.MapKey) ||
			s.childDirectlyMatchesAnchor(v.MapValue) ||
			s.childDirectlyMatchesAnchor(v.Metatable) {
			return true
		}
		for _, field := range v.Fields {
			if s.childDirectlyMatchesAnchor(field.Type) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (s *directEmbeddingScan) childDirectlyMatchesAnchor(child typ.Type) bool {
	child = UnwrapStructuralShape(child)
	if child == nil {
		return false
	}
	if s.directSelfEmbeddingAnchorMatch(child) {
		return true
	}
	switch v := child.(type) {
	case *typ.Optional:
		if s.alreadySeen(v) {
			return false
		}
		return s.childDirectlyMatchesAnchor(v.Inner)
	case *typ.Union:
		if s.alreadySeen(v) {
			return false
		}
		for _, member := range v.Members {
			if s.childDirectlyMatchesAnchor(member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		if s.alreadySeen(v) {
			return false
		}
		for _, member := range v.Members {
			if s.childDirectlyMatchesAnchor(member) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (s *directEmbeddingScan) directSelfEmbeddingAnchorMatch(child typ.Type) bool {
	if child == nil || s == nil || s.anchor == nil {
		return child == nil && (s == nil || s.anchor == nil)
	}
	child = UnwrapStructuralShape(child)
	if child == nil {
		return false
	}
	if typ.SameNode(child, s.anchor) {
		return true
	}
	if child.Kind() != s.anchorKind {
		return false
	}
	if typ.EqualityHash(child) != s.anchorHash {
		return false
	}
	if typ.TypeEquals(child, s.anchor) {
		return true
	}
	return false
}

func topSequenceSelfEmbeddingUpperBound(a, b typ.Type) (typ.Type, bool) {
	if topArrayCoversSelfEmbedding(a, b) {
		return a, true
	}
	if topArrayCoversSelfEmbedding(b, a) {
		return b, true
	}
	return nil, false
}

func topArrayCoversSelfEmbedding(anchor, observation typ.Type) bool {
	array, ok := UnwrapStructuralShape(anchor).(*typ.Array)
	if !ok || array == nil || (!typ.IsUnknown(array.Element) && !typ.IsAny(array.Element)) {
		return false
	}
	_, ok = UnwrapStructuralShape(observation).(*typ.Array)
	return ok && ContainsEquivalent(observation, anchor)
}

// FoldSelfEmbedding converts an observation that embeds a previous structural
// observation into an explicit recursive type. This is the convergence-domain
// representation of recursive value growth: the root shape stays precise, and
// the repeated nested edge becomes a mu-type reference instead of another tree
// level.
func FoldSelfEmbedding(anchor, observation typ.Type) (typ.Type, bool) {
	if foldDbg {
		println("FOLDDBG enter FoldSelfEmbedding anchor=", DbgString(anchor), "obs=", DbgString(observation))
	}
	if !canUseSelfEmbeddingAnchor(anchor) || observation == nil {
		if foldDbg {
			println("FOLDDBG gate canUseAnchor=false anchor=", DbgString(anchor), "obs=", DbgString(observation))
		}
		return nil, false
	}
	if !selfEmbeddingObservationPreservesAnchor(anchor, observation) {
		if foldDbg {
			println("FOLDDBG gate preservesAnchor=false anchor=", DbgString(anchor), "obs=", DbgString(observation))
		}
		return nil, false
	}
	if !containsSelfEmbeddingAnchorBelowRoot(anchor, observation) {
		if foldDbg {
			println("FOLDDBG gate belowRoot=false anchor=", DbgString(anchor), "obs=", DbgString(observation))
		}
		return nil, false
	}

	rec := typ.NewRecursivePlaceholder("Inferred")
	root := true
	replaced := false
	body := typ.Rewrite(observation, func(node typ.Type) (typ.Type, bool) {
		if root {
			root = false
			return nil, false
		}
		if sameSelfEmbeddingAnchor(node, anchor) {
			replaced = true
			return rec, true
		}
		return nil, false
	})
	if !replaced {
		return nil, false
	}
	if !productiveSelfEmbeddingBody(body, rec) {
		return nil, false
	}
	rec.SetBody(body)
	return rec, true
}

func productiveSelfEmbeddingBody(body typ.Type, self *typ.Recursive) bool {
	if body == nil || self == nil {
		return false
	}
	switch v := unwrap.Alias(body).(type) {
	case *typ.Optional:
		return productiveSelfEmbeddingBody(v.Inner, self)
	case *typ.Union:
		for _, member := range v.Members {
			if productiveSelfEmbeddingBody(member, self) {
				return true
			}
		}
		return false
	case *typ.Recursive:
		return !typ.IsRecursiveRef(v, self)
	case *typ.Record, *typ.Array, *typ.Map, *typ.Tuple, *typ.Function:
		return true
	default:
		return false
	}
}

func containsSelfEmbeddingAnchorBelowRoot(anchor, observation typ.Type) bool {
	seen := make(map[typ.Type]struct{})
	return containsSelfEmbeddingAnchor(anchor, observation, true, typ.NewGuard(), seen)
}

func containsSelfEmbeddingAnchor(
	anchor typ.Type,
	observation typ.Type,
	root bool,
	guard internal.RecursionGuard,
	seen map[typ.Type]struct{},
) bool {
	if observation == nil {
		return false
	}
	node := typ.UnwrapAnnotated(observation)
	if !root && sameSelfEmbeddingAnchor(node, anchor) {
		return true
	}
	if _, ok := seen[node]; ok {
		return false
	}
	seen[node] = struct{}{}
	next, ok := guard.Enter(node)
	if !ok {
		return false
	}

	switch n := node.(type) {
	case *typ.Optional:
		return containsSelfEmbeddingAnchor(anchor, n.Inner, false, next, seen)
	case *typ.Union:
		for _, member := range n.Members {
			if containsSelfEmbeddingAnchor(anchor, member, false, next, seen) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range n.Members {
			if containsSelfEmbeddingAnchor(anchor, member, false, next, seen) {
				return true
			}
		}
		return false
	case *typ.Array:
		return containsSelfEmbeddingAnchor(anchor, n.Element, false, next, seen)
	case *typ.Map:
		return containsSelfEmbeddingAnchor(anchor, n.Key, false, next, seen) ||
			containsSelfEmbeddingAnchor(anchor, n.Value, false, next, seen)
	case *typ.Tuple:
		for _, elem := range n.Elements {
			if containsSelfEmbeddingAnchor(anchor, elem, false, next, seen) {
				return true
			}
		}
		return false
	case *typ.Function:
		for _, param := range n.Params {
			if containsSelfEmbeddingAnchor(anchor, param.Type, false, next, seen) {
				return true
			}
		}
		for _, ret := range n.Returns {
			if containsSelfEmbeddingAnchor(anchor, ret, false, next, seen) {
				return true
			}
		}
		return n.Variadic != nil && containsSelfEmbeddingAnchor(anchor, n.Variadic, false, next, seen)
	case *typ.Record:
		for _, field := range n.Fields {
			if containsSelfEmbeddingAnchor(anchor, field.Type, false, next, seen) {
				return true
			}
		}
		if n.Metatable != nil && containsSelfEmbeddingAnchor(anchor, n.Metatable, false, next, seen) {
			return true
		}
		if n.HasMapComponent() {
			return containsSelfEmbeddingAnchor(anchor, n.MapKey, false, next, seen) ||
				containsSelfEmbeddingAnchor(anchor, n.MapValue, false, next, seen)
		}
		return false
	case *typ.Alias:
		return containsSelfEmbeddingAnchor(anchor, n.UnaliasedTarget(), false, next, seen)
	case *typ.Instantiated:
		for _, arg := range n.TypeArgs {
			if containsSelfEmbeddingAnchor(anchor, arg, false, next, seen) {
				return true
			}
		}
		return false
	case *typ.Interface:
		for _, method := range n.Methods {
			if method.Type != nil && containsSelfEmbeddingAnchor(anchor, method.Type, false, next, seen) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func sameSelfEmbeddingAnchor(node, anchor typ.Type) bool {
	if node == nil || anchor == nil {
		return node == anchor
	}
	if typ.SameNode(node, anchor) {
		return true
	}
	node = UnwrapStructuralShape(node)
	anchor = UnwrapStructuralShape(anchor)
	if node == nil || anchor == nil {
		return node == anchor
	}
	if typ.SameNode(node, anchor) {
		return true
	}
	if node.Kind() != anchor.Kind() {
		return false
	}
	// Genuine recursive self-embedding: the node recurses to the same fixed-point
	// product family as the anchor, a real back-edge that bounding must fold.
	if typ.ContainsRecursive(node) || typ.ContainsRecursive(anchor) {
		return typ.SameProductFamily(node, anchor)
	}
	if typ.TypeEquals(node, anchor) {
		return true
	}
	// Finite self-embedding tower: fold only when the node is genuinely
	// self-similar to the anchor, i.e. it reconstructs the anchor's product when
	// anchor-shaped descendants act as the shared recursion point. A bare
	// shallow-shape match would fold distinct finite records that only share a
	// field layout with an ancestor (the F1 over-fire), so descend structurally
	// and require every recursive position to re-embed the anchor on both sides.
	if !ShallowStructuralShapeEquals(node, anchor) {
		return false
	}
	return selfSimilarToAnchor(node, anchor, anchor, true, make(map[selfSimilarPair]bool))
}

type selfSimilarPair struct {
	node   typ.Type
	anchor typ.Type
}

// selfEmbeddingGrowthSeed reports whether t is a placeholder the recursion grows
// out of: an unknown/any escape hatch or a soft inference placeholder. These are
// the seeds a self-returning chain bottoms out at before later observations fill
// them with the recursive product, so a one-sided recursion edge against a seed
// is still genuine self-embedding.
func selfEmbeddingGrowthSeed(t typ.Type) bool {
	t = UnwrapStructuralShape(t)
	if t == nil {
		return true
	}
	return typ.IsAny(t) || typ.IsUnknown(t) || typ.IsSoft(t, typ.SoftPlaceholderPolicy)
}

// selfSimilarToAnchor reports whether node reconstructs anchor's product when
// anchor-shaped descendants act as the shared recursion point. Below the top
// comparison, a position where both sides re-embed the root anchor is the shared
// recursion edge and matches without descending further (which is what lets
// growing towers of differing depth still fold). A position where only one side
// re-embeds the anchor breaks self-similarity, separating a genuine tower from
// an incidental shallow-shape collision.
func selfSimilarToAnchor(node, anchor, root typ.Type, top bool, seen map[selfSimilarPair]bool) bool {
	node = UnwrapStructuralShape(node)
	anchor = UnwrapStructuralShape(anchor)
	if node == nil || anchor == nil {
		return node == anchor
	}
	if typ.SameNode(node, anchor) {
		return true
	}

	if !top {
		nodeSeed := selfEmbeddingGrowthSeed(node)
		anchorSeed := selfEmbeddingGrowthSeed(anchor)
		nodeReembeds := ShallowStructuralShapeEquals(node, root)
		anchorReembeds := ShallowStructuralShapeEquals(anchor, root)
		// A placeholder seed is the position the recursion grows out of. The anchor
		// is the established (possibly under-grown) fixed point and the node is a
		// subtree of the observation being checked against it.
		//
		// An anchor-side seed is the recursion bottoming out: the node is a later,
		// more concrete observation of the position the recursion will fill, so any
		// concrete node refines it and the position is self-similar.
		//
		// A node-side seed is the opposite - the observation is less refined than
		// the concrete anchor here. That is genuine self-embedding only when the
		// concrete anchor side itself re-embeds the root (the recursion would fill
		// the node seed with the root product). A node seed sitting opposite a
		// structurally divergent concrete product is not a recursion edge but a
		// distinct sibling shape that merely happens to be soft - e.g. a
		// {[string]: any} field map at the value slot of a {[string]: T[]} anchor -
		// so it must not fold.
		if anchorSeed {
			return true
		}
		if nodeSeed {
			return anchorReembeds
		}
		if nodeReembeds || anchorReembeds {
			if nodeReembeds && anchorReembeds {
				// Both re-embed the root: the shared recursion edge.
				return true
			}
			// Only one side re-embeds the root and the node is not a seed, so it
			// is a concrete divergent structure that merely shares a field layout
			// with an ancestor (the F1 over-fire), not genuine self-embedding.
			return false
		}
	}

	if node.Kind() != anchor.Kind() {
		return false
	}
	pair := selfSimilarPair{node: node, anchor: anchor}
	if seen[pair] {
		return true
	}
	seen[pair] = true

	switch n := node.(type) {
	case *typ.Record:
		// The node is the earlier, under-grown observation, so it may carry a
		// subset of the anchor's fields. Every field it does carry must align with
		// the anchor; a field the anchor lacks is divergence, not growth.
		a, ok := anchor.(*typ.Record)
		if !ok || n.Open != a.Open {
			return false
		}
		for _, field := range n.Fields {
			anchorField := a.GetField(field.Name)
			if anchorField == nil {
				return false
			}
			if field.Optional != anchorField.Optional || field.Readonly != anchorField.Readonly {
				return false
			}
			if !selfSimilarToAnchor(field.Type, anchorField.Type, root, false, seen) {
				return false
			}
		}
		if n.HasMapComponent() {
			if !a.HasMapComponent() || !selfSimilarToAnchor(n.MapValue, a.MapValue, root, false, seen) {
				return false
			}
		}
		return true
	case *typ.Array:
		a, ok := anchor.(*typ.Array)
		return ok && selfSimilarToAnchor(n.Element, a.Element, root, false, seen)
	case *typ.Map:
		a, ok := anchor.(*typ.Map)
		return ok && selfSimilarToAnchor(n.Value, a.Value, root, false, seen)
	case *typ.Tuple:
		a, ok := anchor.(*typ.Tuple)
		if !ok || len(n.Elements) != len(a.Elements) {
			return false
		}
		for i, elem := range n.Elements {
			if !selfSimilarToAnchor(elem, a.Elements[i], root, false, seen) {
				return false
			}
		}
		return true
	case *typ.Function:
		// Function signatures grow as later observations add parameters, so the
		// under-grown node may carry fewer params/returns. Common positions must
		// stay self-similar.
		a, ok := anchor.(*typ.Function)
		if !ok || len(n.Params) > len(a.Params) || len(n.Returns) > len(a.Returns) {
			return false
		}
		for i := range n.Params {
			if !selfSimilarToAnchor(n.Params[i].Type, a.Params[i].Type, root, false, seen) {
				return false
			}
		}
		for i := range n.Returns {
			if !selfSimilarToAnchor(n.Returns[i], a.Returns[i], root, false, seen) {
				return false
			}
		}
		if n.Variadic != nil && a.Variadic != nil && !selfSimilarToAnchor(n.Variadic, a.Variadic, root, false, seen) {
			return false
		}
		return true
	default:
		return typ.TypeEquals(node, anchor)
	}
}

func canUseSelfEmbeddingAnchor(t typ.Type) bool {
	t = UnwrapStructuralShape(t)
	switch t.(type) {
	case *typ.Union, *typ.Intersection, *typ.Recursive:
		return false
	default:
		return CanSelfEmbed(t) && !typ.IsAbsentOrUnknown(t)
	}
}

func selfEmbeddingObservationPreservesAnchor(anchor, observation typ.Type) bool {
	anchor = UnwrapStructuralShape(anchor)
	observation = UnwrapStructuralShape(observation)
	if anchor == nil || observation == nil {
		return false
	}
	if typ.SameNodeOrAcyclicEqual(anchor, observation) {
		return true
	}
	switch a := anchor.(type) {
	case *typ.Record:
		o, ok := observation.(*typ.Record)
		return ok && (ExtendsRecord(o, a) || shallowRecordShapeEquals(o, a))
	case *typ.Map:
		switch o := observation.(type) {
		case *typ.Map:
			return shallowMapKeyShapeEquals(o.Key, a.Key)
		case *typ.Record:
			return o.HasMapComponent() && shallowMapKeyShapeEquals(o.MapKey, a.Key)
		default:
			return false
		}
	case *typ.Array:
		_, ok := observation.(*typ.Array)
		return ok
	case *typ.Tuple:
		o, ok := observation.(*typ.Tuple)
		return ok && tupleObservationPreservesAnchor(a, o)
	case *typ.Function:
		o, ok := observation.(*typ.Function)
		return ok && len(o.Params) == len(a.Params) && len(o.Returns) == len(a.Returns)
	default:
		return ShallowStructuralShapeEquals(observation, anchor)
	}
}

func tupleObservationPreservesAnchor(anchor, observation *typ.Tuple) bool {
	if anchor == nil || observation == nil || len(anchor.Elements) != len(observation.Elements) {
		return false
	}
	for i, anchorElem := range anchor.Elements {
		observationElem := observation.Elements[i]
		if sameSelfEmbeddingAnchor(observationElem, anchor) {
			continue
		}
		if !selfEmbeddingObservationPreservesAnchor(anchorElem, observationElem) {
			return false
		}
	}
	return true
}

func existingRecursiveUpperBound(a, b typ.Type) (typ.Type, bool) {
	if rec, ok := unwrap.Alias(a).(*typ.Recursive); ok && recursiveUpperBoundCovers(rec, b) {
		return a, true
	}
	if rec, ok := unwrap.Alias(b).(*typ.Recursive); ok && recursiveUpperBoundCovers(rec, a) {
		return b, true
	}
	if rec, ok := unwrap.Alias(b).(*typ.Recursive); ok && containsRecursiveMember(a, rec) {
		return a, true
	}
	if rec, ok := unwrap.Alias(a).(*typ.Recursive); ok && containsRecursiveMember(b, rec) {
		return b, true
	}
	if recursiveStructuralUpperBoundCovers(a, b) {
		return a, true
	}
	if recursiveStructuralUpperBoundCovers(b, a) {
		return b, true
	}
	return nil, false
}

// RecursiveUnionUpperBound reports whether an existing union already contains a
// recursive product member that covers a later observation from the same family.
// This keeps convergence finite: once a recursive product is in the abstract
// value, later unfolded observations must be admitted by that product instead of
// appended as another union member through generic union construction.
func RecursiveUnionUpperBound(a, b typ.Type) (typ.Type, bool) {
	if recursiveUnionCovers(a, b) {
		return a, true
	}
	if recursiveUnionCovers(b, a) {
		return b, true
	}
	return nil, false
}

// RecursiveEvidenceCovers reports whether upper is a recursive product-domain
// upper bound for observation. It is the recursive-safe coverage relation used by
// domains before falling back to generic subtype checks.
func RecursiveEvidenceCovers(upper, observation typ.Type) bool {
	if upper == nil || observation == nil {
		return false
	}
	if !typ.ContainsRecursive(upper) && !typ.ContainsRecursive(observation) {
		return false
	}
	if recursiveUnionCovers(upper, observation) {
		return true
	}
	if recursiveStructuralUpperBoundCovers(upper, observation) {
		return true
	}
	if rec, ok := unwrap.Alias(upper).(*typ.Recursive); ok {
		return recursiveUpperBoundCovers(rec, observation)
	}
	return recursiveEvidenceUpperBoundCovers(upper, observation)
}

func recursiveUnionCovers(upper, observation typ.Type) bool {
	if upper == nil || observation == nil {
		return false
	}
	switch u := unwrap.Alias(upper).(type) {
	case *typ.Optional:
		inner, _ := SplitNilable(observation)
		return recursiveUnionCovers(u.Inner, inner)
	case *typ.Union:
		for _, member := range u.Members {
			if recursiveMemberCoversObservation(member, observation) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func recursiveMemberCoversObservation(member, observation typ.Type) bool {
	if member == nil || observation == nil {
		return false
	}
	if rec, ok := unwrap.Alias(member).(*typ.Recursive); ok {
		return recursiveUpperBoundCovers(rec, observation)
	}
	return recursiveStructuralUpperBoundCovers(member, observation)
}

func recursiveStructuralUpperBoundCovers(upper, observation typ.Type) bool {
	if upper == nil || observation == nil {
		return false
	}
	if _, ok := unwrap.Alias(upper).(*typ.Union); ok {
		return false
	}
	if !selfEmbeddingObservationPreservesAnchor(observation, upper) || !ContainsRecursiveType(upper) {
		return false
	}
	if recursiveEvidenceUpperBoundCovers(upper, observation) {
		return true
	}
	refines, _ := RefinesSoftContainer(upper, observation)
	return refines
}

func recursiveUpperBoundCovers(rec *typ.Recursive, observation typ.Type) bool {
	if rec == nil || rec.Body == nil || observation == nil {
		return false
	}
	if typ.IsRecursiveRef(observation, rec) {
		return true
	}
	if !recursiveEvidenceUpperBoundCovers(rec, observation) {
		refines, _ := RefinesSoftContainer(rec, observation)
		if !refines {
			return false
		}
	}
	if selfEmbeddingObservationPreservesAnchor(rec.Body, observation) {
		return true
	}
	return ShallowStructuralShapeEquals(observation, rec.Body) &&
		ContainsRecursiveRef(observation, rec)
}

func recursiveEvidenceUpperBoundCovers(upper, observation typ.Type) bool {
	cover := recursiveEvidenceCover{
		seen: make(recursiveCoverSeen),
	}
	return cover.covers(upper, observation)
}

type recursiveEvidenceCover struct {
	seen recursiveCoverSeen
}

type recursiveCoverSeenKey struct {
	upper       uint64
	observation uint64
	family      bool
}

type recursiveCoverSeenEntry struct {
	upper       typ.Type
	observation typ.Type
}

type recursiveCoverSeen map[recursiveCoverSeenKey][]recursiveCoverSeenEntry

func (s recursiveCoverSeen) contains(upper, observation typ.Type) bool {
	if upper == nil || observation == nil || s == nil {
		return false
	}
	key := recursiveCoverKey(upper, observation)
	if key.family {
		_, ok := s[key]
		return ok
	}
	for _, entry := range s[key] {
		if (typ.SameNode(entry.upper, upper) || typ.TypeEquals(entry.upper, upper)) &&
			(typ.SameNode(entry.observation, observation) || typ.TypeEquals(entry.observation, observation)) {
			return true
		}
	}
	return false
}

func (s recursiveCoverSeen) remember(upper, observation typ.Type) {
	if upper == nil || observation == nil || s == nil {
		return
	}
	key := recursiveCoverKey(upper, observation)
	s[key] = append(s[key], recursiveCoverSeenEntry{
		upper:       upper,
		observation: observation,
	})
}

func recursiveCoverKey(upper, observation typ.Type) recursiveCoverSeenKey {
	if typ.ContainsRecursive(upper) || typ.ContainsRecursive(observation) {
		return recursiveCoverSeenKey{
			upper:       typ.ProductFamilyHash(upper),
			observation: typ.ProductFamilyHash(observation),
			family:      true,
		}
	}
	return recursiveCoverSeenKey{
		upper:       typ.EqualityHash(upper),
		observation: typ.EqualityHash(observation),
	}
}

func (c *recursiveEvidenceCover) covers(upper, observation typ.Type) bool {
	upper = unwrapRecursiveCoverageTransparent(upper)
	observation = unwrapRecursiveCoverageTransparent(observation)
	if upper == nil || observation == nil {
		return upper == observation
	}
	if typ.SameNode(upper, observation) {
		return true
	}
	recursive := typ.ContainsRecursive(upper) || typ.ContainsRecursive(observation)
	if typ.IsAny(upper) || typ.IsUnknown(upper) {
		return true
	}
	if typ.IsNever(observation) {
		return true
	}
	if typ.IsAny(observation) || typ.IsUnknown(observation) {
		// An any/unknown observation at a recursive self-edge is the growth seed the
		// recursion bottoms at: a finite self-embedding tower stops unfolding with an
		// unknown self-edge, and the recursive family is the upper bound of that
		// tower. This applies only when the upper is the recursive node itself (the
		// back-edge); a broader observation matched against a non-self-edge upper
		// that merely contains recursion is still not covered.
		if _, ok := upper.(*typ.Recursive); ok {
			return true
		}
		return false
	}
	if c.seen.contains(upper, observation) {
		return true
	}
	c.seen.remember(upper, observation)

	if o, ok := upper.(*typ.Optional); ok {
		if unwrap.IsNilType(observation) {
			return true
		}
		if observed, ok := observation.(*typ.Optional); ok {
			return c.covers(o.Inner, observed.Inner)
		}
		return c.covers(o.Inner, observation)
	}
	if o, ok := observation.(*typ.Optional); ok {
		return c.covers(upper, typ.Nil) && c.covers(upper, o.Inner)
	}

	if rec, ok := upper.(*typ.Recursive); ok {
		if typ.IsRecursiveRef(observation, rec) {
			return true
		}
		if rec.Body == nil || rec.Body == rec {
			return false
		}
		return c.covers(rec.Body, observation)
	}
	if rec, ok := observation.(*typ.Recursive); ok {
		if rec.Body == nil || rec.Body == rec {
			return false
		}
		return c.covers(upper, rec.Body)
	}

	if u, ok := upper.(*typ.Union); ok {
		if observed, ok := observation.(*typ.Union); ok {
			if len(observed.Members) == 0 {
				return false
			}
			for _, observedMember := range observed.Members {
				matched := false
				for _, upperMember := range u.Members {
					if c.covers(upperMember, observedMember) {
						matched = true
						break
					}
				}
				if !matched {
					return false
				}
			}
			return true
		}
		for _, member := range u.Members {
			if c.covers(member, observation) {
				return true
			}
		}
		return false
	}
	if u, ok := observation.(*typ.Union); ok {
		for _, member := range u.Members {
			if !c.covers(upper, member) {
				return false
			}
		}
		return len(u.Members) > 0
	}
	if i, ok := upper.(*typ.Intersection); ok {
		for _, member := range i.Members {
			if !c.covers(member, observation) {
				return false
			}
		}
		return len(i.Members) > 0
	}
	if i, ok := observation.(*typ.Intersection); ok {
		for _, member := range i.Members {
			if c.covers(upper, member) {
				return true
			}
		}
		return false
	}

	switch u := upper.(type) {
	case *typ.Array:
		return c.arrayCovers(u, observation)
	case *typ.Map:
		return c.mapCovers(u, observation)
	case *typ.Tuple:
		return c.tupleCovers(u, observation)
	case *typ.Record:
		return c.recordCovers(u, observation)
	case *typ.Function:
		return c.functionCovers(u, observation)
	default:
		if !recursive {
			return subtype.IsSubtype(observation, upper)
		}
		return false
	}
}

func unwrapRecursiveCoverageTransparent(t typ.Type) typ.Type {
	for t != nil {
		switch v := t.(type) {
		case *typ.Annotated:
			if v.Inner == nil || v.Inner == t {
				return t
			}
			t = v.Inner
		case *typ.Alias:
			target := v.UnaliasedTarget()
			if target == nil || target == t {
				return t
			}
			t = target
		default:
			return t
		}
	}
	return nil
}

func (c *recursiveEvidenceCover) arrayCovers(upper *typ.Array, observation typ.Type) bool {
	switch observed := observation.(type) {
	case *typ.Array:
		return c.covers(upper.Element, observed.Element)
	case *typ.Tuple:
		for _, elem := range observed.Elements {
			if !c.covers(upper.Element, elem) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (c *recursiveEvidenceCover) mapCovers(upper *typ.Map, observation typ.Type) bool {
	switch observed := observation.(type) {
	case *typ.Map:
		return c.covers(upper.Key, observed.Key) && c.covers(upper.Value, observed.Value)
	case *typ.Array:
		return c.covers(upper.Key, typ.Integer) && c.covers(upper.Value, observed.Element)
	case *typ.Tuple:
		if !c.covers(upper.Key, typ.Integer) {
			return false
		}
		for _, elem := range observed.Elements {
			if !c.covers(upper.Value, elem) {
				return false
			}
		}
		return true
	case *typ.Record:
		if !observed.HasMapComponent() {
			return false
		}
		return c.covers(upper.Key, observed.MapKey) && c.covers(upper.Value, observed.MapValue)
	default:
		return false
	}
}

func (c *recursiveEvidenceCover) tupleCovers(upper *typ.Tuple, observation typ.Type) bool {
	observed, ok := observation.(*typ.Tuple)
	if !ok || len(upper.Elements) != len(observed.Elements) {
		return false
	}
	for i, elem := range upper.Elements {
		if !c.covers(elem, observed.Elements[i]) {
			return false
		}
	}
	return true
}

func (c *recursiveEvidenceCover) recordCovers(upper *typ.Record, observation typ.Type) bool {
	observed, ok := observation.(*typ.Record)
	if !ok || upper == nil || observed == nil {
		return false
	}
	if upper.Metatable != nil {
		if observed.Metatable == nil || !c.covers(upper.Metatable, observed.Metatable) {
			return false
		}
	}
	if upper.HasMapComponent() {
		if !observed.HasMapComponent() ||
			!c.covers(upper.MapKey, observed.MapKey) ||
			!c.covers(upper.MapValue, observed.MapValue) {
			return false
		}
	}
	for _, upperField := range upper.Fields {
		observedField := observed.GetField(upperField.Name)
		if observedField == nil {
			if upperField.Optional || unwrap.IsOptionalLike(upperField.Type) {
				continue
			}
			return false
		}
		if !upperField.Readonly && observedField.Readonly {
			return false
		}
		if !upperField.Optional && !unwrap.IsOptionalLike(upperField.Type) && observedField.Optional {
			return false
		}
		if !c.covers(upperField.Type, observedField.Type) {
			return false
		}
	}
	return true
}

func (c *recursiveEvidenceCover) functionCovers(upper *typ.Function, observation typ.Type) bool {
	observed, ok := observation.(*typ.Function)
	if !ok || upper == nil || observed == nil {
		return false
	}
	observedReq := typ.MinRequiredArgs(observed)
	upperReq := typ.MinRequiredArgs(upper)
	if observedReq > upperReq || (upper.Variadic == nil && observedReq > len(upper.Params)) {
		return false
	}
	if observed.Variadic == nil && len(upper.Params) > len(observed.Params) {
		return false
	}
	maxParams := len(observed.Params)
	if len(upper.Params) > maxParams {
		maxParams = len(upper.Params)
	}
	for i := 0; i < maxParams; i++ {
		var observedParam, upperParam typ.Type
		if i < len(observed.Params) {
			observedParam = observed.Params[i].Type
		} else if observed.Variadic != nil {
			observedParam = observed.Variadic
		}
		if i < len(upper.Params) {
			upperParam = upper.Params[i].Type
		} else if upper.Variadic != nil {
			upperParam = upper.Variadic
		}
		if observedParam == nil || upperParam == nil {
			continue
		}
		if !c.covers(observedParam, upperParam) {
			return false
		}
	}
	if observed.Variadic != nil && upper.Variadic != nil && !c.covers(observed.Variadic, upper.Variadic) {
		return false
	}
	for i := 0; i < len(upper.Returns); i++ {
		observedReturn := typ.Nil
		if i < len(observed.Returns) {
			observedReturn = observed.Returns[i]
		}
		if !c.covers(upper.Returns[i], observedReturn) {
			return false
		}
	}
	return true
}

// ContainsRecursiveRef reports whether haystack contains rec by recursive ID
// without structurally hashing or comparing the recursive body.
func ContainsRecursiveRef(haystack typ.Type, rec *typ.Recursive) bool {
	if haystack == nil || rec == nil {
		return false
	}
	return Scan(haystack, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if typ.IsRecursiveRef(node, rec) {
			return true, false
		}
		return false, true
	})
}

// ContainsRecursiveType reports whether haystack contains any explicit
// recursive-type reference. The recursive body is not expanded here; the
// presence of the ref is the finite convergence witness.
func ContainsRecursiveType(haystack typ.Type) bool {
	return typ.ContainsRecursive(haystack)
}

func containsRecursiveMember(t typ.Type, rec *typ.Recursive) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Recursive:
		return typ.IsRecursiveRef(v, rec)
	case *typ.Optional:
		return containsRecursiveMember(v.Inner, rec)
	case *typ.Union:
		for _, member := range v.Members {
			if containsRecursiveMember(member, rec) {
				return true
			}
		}
	}
	return false
}

// JoinStructuralUnionShape folds a union whose members all belong to one
// compatible structural table family with another observation from that family.
// It deliberately refuses discriminated/recursive alternatives because those
// are semantic variants, not partial observations of one table shape.
func JoinStructuralUnionShape(a, b typ.Type, join func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
	if join == nil {
		join = typ.JoinPreferNonSoft
	}
	if joined, ok := joinStructuralUnionShapeDirected(a, b, join); ok {
		return joined, true
	}
	if joined, ok := joinStructuralUnionShapeDirected(b, a, join); ok {
		return joined, true
	}
	return joinStructuralUnionShapePair(a, b, join)
}

func joinStructuralUnionShapeDirected(unionType, other typ.Type, join func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
	u, ok := unwrap.Alias(unionType).(*typ.Union)
	if !ok || u == nil || len(u.Members) == 0 || other == nil {
		return nil, false
	}
	acc := other
	for _, member := range u.Members {
		if member == nil {
			continue
		}
		if SameConvergedFact(acc, member) {
			continue
		}
		joined, ok := JoinStructuralShape(acc, member, join)
		if !ok {
			return nil, false
		}
		acc = joined
	}
	return acc, true
}

func joinStructuralUnionShapePair(a, b typ.Type, join func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
	au, okA := unwrap.Alias(a).(*typ.Union)
	bu, okB := unwrap.Alias(b).(*typ.Union)
	if !okA || !okB || au == nil || bu == nil {
		return nil, false
	}
	members := make([]typ.Type, 0, len(au.Members)+len(bu.Members))
	members = append(members, au.Members...)
	members = append(members, bu.Members...)
	union := typ.NewUnion(members...)
	collapsed := CollapseStructuralUnionShape(union, join)
	if collapsed == nil || typ.SameNode(collapsed, union) {
		return nil, false
	}
	return collapsed, true
}

// JoinStructuralShape joins one compatible table-shaped pair.
func JoinStructuralShape(a, b typ.Type, join func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
	if seq, ok := JoinSequenceShape(a, b, join); ok {
		return seq, true
	}
	if joined, ok := JoinMapShape(a, b, join); ok {
		return joined, true
	}
	if joined, ok := JoinRecordShape(a, b, join); ok {
		return joined, true
	}
	if joined, ok := JoinMapRecordShape(a, b, join); ok {
		return joined, true
	}
	return nil, false
}

// CollapseStructuralUnionShape folds compatible structural members inside a
// union into one canonical table shape. Non-structural members, discriminated
// variants, and recursive alternatives remain separate union members.
func CollapseStructuralUnionShape(t typ.Type, join func(typ.Type, typ.Type) typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if join == nil {
		join = typ.JoinPreferNonSoft
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Optional:
		inner := CollapseStructuralUnionShape(v.Inner, join)
		if inner == nil || typ.SameNode(inner, v.Inner) {
			return t
		}
		return typ.NewOptional(inner)
	case *typ.Union:
		return collapseStructuralUnionMembers(v, join)
	default:
		return t
	}
}

func collapseStructuralUnionMembers(u *typ.Union, join func(typ.Type, typ.Type) typ.Type) typ.Type {
	var structural typ.Type
	residual := make([]typ.Type, 0, len(u.Members))
	changed := false
	for _, member := range u.Members {
		collapsed := CollapseStructuralUnionShape(member, join)
		if !typ.SameNode(collapsed, member) {
			changed = true
		}
		if structural == nil {
			if IsStructuredTableShape(collapsed) {
				structural = collapsed
				continue
			}
			residual = append(residual, collapsed)
			continue
		}
		if upper, ok := SelfEmbeddingUpperBound(structural, collapsed); ok {
			structural = upper
			changed = true
			continue
		}
		if joined, ok := JoinStructuralShape(structural, collapsed, join); ok {
			structural = joined
			changed = true
			continue
		}
		residual = append(residual, collapsed)
	}
	if structural == nil {
		if changed {
			return typ.NewUnion(residual...)
		}
		return u
	}
	if !changed {
		return u
	}
	residual = append(residual, structural)
	if len(residual) == 1 {
		return structural
	}
	return typ.NewUnion(residual...)
}

// Scan walks structural type children until visit stops traversal.
func Scan(
	t typ.Type,
	guard internal.RecursionGuard,
	visit func(node typ.Type) (stop bool, descend bool),
) bool {
	scanner := structuralScanner{
		visit: visit,
		seen:  make(structuralTypeSeen),
	}
	return scanner.scan(t, guard)
}

type structuralScanner struct {
	visit  func(node typ.Type) (stop bool, descend bool)
	seen   structuralTypeSeen
	hashes map[typ.Type]uint64
}

type structuralTypeSeen map[uint64][]typ.Type

func (s structuralTypeSeen) contains(hash uint64, node typ.Type) bool {
	if node == nil || s == nil {
		return false
	}
	for _, existing := range s[hash] {
		if typ.SameNodeOrAcyclicEqual(existing, node) {
			return true
		}
	}
	return false
}

func (s structuralTypeSeen) remember(hash uint64, node typ.Type) {
	if node == nil || s == nil {
		return
	}
	s[hash] = append(s[hash], node)
}

func structuralSeenHash(node typ.Type) uint64 {
	if typ.ContainsRecursive(node) {
		return typ.ProductFamilyHash(node)
	}
	return typ.EqualityHash(node)
}

func (s *structuralScanner) scan(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	node := t
	for {
		ann, ok := node.(*typ.Annotated)
		if !ok || ann.Inner == nil || ann.Inner == node {
			break
		}
		node = ann.Inner
	}

	if s.seenEquivalent(node) {
		return false
	}
	s.remember(node)

	if stop, descend := s.visit(node); stop {
		return true
	} else if !descend {
		return false
	}

	switch n := node.(type) {
	case *typ.Optional:
		return s.scan(n.Inner, next)
	case *typ.Union:
		for _, m := range n.Members {
			if s.scan(m, next) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, m := range n.Members {
			if s.scan(m, next) {
				return true
			}
		}
		return false
	case *typ.Array:
		return s.scan(n.Element, next)
	case *typ.Map:
		return s.scan(n.Key, next) || s.scan(n.Value, next)
	case *typ.Tuple:
		for _, e := range n.Elements {
			if s.scan(e, next) {
				return true
			}
		}
		return false
	case *typ.Function:
		for _, p := range n.Params {
			if s.scan(p.Type, next) {
				return true
			}
		}
		for _, r := range n.Returns {
			if s.scan(r, next) {
				return true
			}
		}
		return n.Variadic != nil && s.scan(n.Variadic, next)
	case *typ.Record:
		for _, f := range n.Fields {
			if s.scan(f.Type, next) {
				return true
			}
		}
		if n.Metatable != nil && s.scan(n.Metatable, next) {
			return true
		}
		if n.HasMapComponent() {
			return s.scan(n.MapKey, next) || s.scan(n.MapValue, next)
		}
		return false
	case *typ.Alias:
		return s.scan(n.Target, next)
	case *typ.Instantiated:
		for _, a := range n.TypeArgs {
			if s.scan(a, next) {
				return true
			}
		}
		return false
	case *typ.Interface:
		for _, m := range n.Methods {
			if m.Type != nil && s.scan(m.Type, next) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (s *structuralScanner) seenEquivalent(node typ.Type) bool {
	return s.seen.contains(s.hash(node), node)
}

func (s *structuralScanner) remember(node typ.Type) {
	s.seen.remember(s.hash(node), node)
}

func (s *structuralScanner) hash(node typ.Type) uint64 {
	if node == nil {
		return 0
	}
	if s.hashes != nil {
		if h, ok := s.hashes[node]; ok {
			return h
		}
	}
	h := structuralSeenHash(node)
	if s.hashes == nil {
		s.hashes = make(map[typ.Type]uint64)
	}
	s.hashes[node] = h
	return h
}

// ExtendsRecord reports whether a extends b by adding record fields. This
// treats record field supersets as refinements when b is a record or union of
// records.
func ExtendsRecord(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	ar, ok := a.(*typ.Record)
	if !ok {
		return false
	}
	switch br := b.(type) {
	case *typ.Record:
		return RecordSuperset(ar, br)
	case *typ.Union:
		return recordSupersetUnion(ar, br)
	default:
		return false
	}
}

// RecordSuperset reports whether newRec preserves oldRec and may add fields.
func RecordSuperset(newRec, oldRec *typ.Record) bool {
	if newRec == nil || oldRec == nil {
		return false
	}
	if oldRec.Metatable != nil {
		if newRec.Metatable == nil || !recordSlotPreserves(newRec.Metatable, oldRec.Metatable) {
			return false
		}
	}
	if oldRec.HasMapComponent() {
		if !newRec.HasMapComponent() {
			return false
		}
		if !recordSlotPreserves(newRec.MapKey, oldRec.MapKey) || !recordSlotPreserves(newRec.MapValue, oldRec.MapValue) {
			return false
		}
	}
	oldFields := make(map[string]typ.Field, len(oldRec.Fields))
	for _, f := range oldRec.Fields {
		oldFields[f.Name] = f
	}
	for _, nf := range newRec.Fields {
		if of, ok := oldFields[nf.Name]; ok {
			if of.Optional && !nf.Optional {
				// ok: stronger requirement
			} else if !of.Optional && nf.Optional {
				return false
			}
			if of.Readonly && !nf.Readonly {
				return false
			}
			if of.Type != nil {
				if IsOpenTopRecord(nf.Type) && IsStructuredTableShape(of.Type) {
					return false
				}
				if nf.Type == nil || !recordSlotPreserves(nf.Type, of.Type) {
					return false
				}
			}
			delete(oldFields, nf.Name)
		}
	}
	return len(oldFields) == 0
}

func recordSlotPreserves(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil {
		return candidate == baseline
	}
	if typ.TypeEquals(candidate, baseline) {
		return true
	}
	if typ.ContainsRecursive(candidate) || typ.ContainsRecursive(baseline) {
		if _, comparable := typ.ComparePrecision(candidate, baseline); comparable {
			return true
		}
		return ShallowStructuralShapeEquals(candidate, baseline)
	}
	_, comparable := typ.ComparePrecision(candidate, baseline)
	return comparable
}

func recordSupersetUnion(newRec *typ.Record, oldUnion *typ.Union) bool {
	if newRec == nil || oldUnion == nil {
		return false
	}
	if len(oldUnion.Members) == 0 {
		return false
	}
	for _, member := range oldUnion.Members {
		oldRec, ok := member.(*typ.Record)
		if !ok {
			return false
		}
		if !RecordSuperset(newRec, oldRec) {
			return false
		}
	}
	return true
}

// IsOpenTopRecord reports whether t is an open record with no concrete fields
// or map component.
func IsOpenTopRecord(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil {
		return false
	}
	return rec.Open && len(rec.Fields) == 0 && !rec.HasMapComponent()
}

// IsStructuredTableShape reports whether t carries table structure beyond an
// open-top placeholder.
func IsStructuredTableShape(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Array:
		return true
	case *typ.Tuple:
		return len(v.Elements) > 0
	case *typ.Map:
		return true
	case *typ.Record:
		return v.HasMapComponent() || len(v.Fields) > 0
	default:
		return false
	}
}

// RefinesSoftContainer reports whether candidate preserves the same table shape
// while replacing a soft placeholder element/value with concrete evidence.
func RefinesSoftContainer(candidate, baseline typ.Type) (bool, bool) {
	return refinesSoftContainer(candidate, baseline, make(softContainerSeen))
}

type softContainerSeenKey struct {
	candidate uint64
	baseline  uint64
	family    bool
}

type softContainerSeenEntry struct {
	candidate typ.Type
	baseline  typ.Type
}

type softContainerSeen map[softContainerSeenKey][]softContainerSeenEntry

func (s softContainerSeen) contains(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil || s == nil {
		return false
	}
	key := softContainerSeenKeyFor(candidate, baseline)
	if key.family {
		_, ok := s[key]
		return ok
	}
	for _, entry := range s[key] {
		if sameValueNodeOrAcyclicEqual(entry.candidate, candidate) &&
			sameValueNodeOrAcyclicEqual(entry.baseline, baseline) {
			return true
		}
	}
	return false
}

func (s softContainerSeen) remember(candidate, baseline typ.Type) {
	if candidate == nil || baseline == nil || s == nil {
		return
	}
	key := softContainerSeenKeyFor(candidate, baseline)
	s[key] = append(s[key], softContainerSeenEntry{candidate: candidate, baseline: baseline})
}

func softContainerSeenKeyFor(candidate, baseline typ.Type) softContainerSeenKey {
	if typ.ContainsRecursive(candidate) || typ.ContainsRecursive(baseline) {
		return softContainerSeenKey{
			candidate: typ.ProductFamilyHash(candidate),
			baseline:  typ.ProductFamilyHash(baseline),
			family:    true,
		}
	}
	return softContainerSeenKey{
		candidate: typ.EqualityHash(candidate),
		baseline:  typ.EqualityHash(baseline),
	}
}

func refinesSoftContainer(candidate, baseline typ.Type, seen softContainerSeen) (bool, bool) {
	candidate = UnwrapStructuralShape(candidate)
	baseline = UnwrapStructuralShape(baseline)
	if candidate == nil || baseline == nil {
		return candidate == baseline, false
	}
	if sameValueNodeOrAcyclicEqual(candidate, baseline) {
		return true, false
	}
	if seen.contains(candidate, baseline) {
		return true, false
	}
	seen.remember(candidate, baseline)
	if c, ok := candidate.(*typ.Union); ok {
		strict := false
		for _, member := range c.Members {
			refines, changed := refinesSoftContainer(member, baseline, seen)
			if !refines {
				return false, false
			}
			if changed {
				strict = true
			}
		}
		return len(c.Members) > 0, strict
	}
	if b, ok := baseline.(*typ.Union); ok {
		matched := false
		strict := false
		for _, member := range b.Members {
			refines, changed := refinesSoftContainer(candidate, member, seen)
			if refines {
				matched = true
				strict = strict || changed
				continue
			}
			if typ.IsSoft(member, typ.SoftPlaceholderPolicy) {
				strict = true
				continue
			}
			return false, false
		}
		return matched, strict
	}
	if c, ok := candidate.(*typ.Recursive); ok {
		b, ok := baseline.(*typ.Recursive)
		if !ok || c.Name != b.Name || c.Body == nil || b.Body == nil {
			return false, false
		}
		return refinesSoftContainer(c.Body, b.Body, seen)
	}
	if _, ok := baseline.(*typ.Recursive); ok {
		return false, false
	}

	switch b := baseline.(type) {
	case *typ.Array:
		c, ok := candidate.(*typ.Array)
		if !ok {
			return false, false
		}
		return refinesSoftContainerSlot(c.Element, b.Element, seen)
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		if !ok || !Equivalent(c.Key, b.Key) {
			return false, false
		}
		return refinesSoftContainerSlot(c.Value, b.Value, seen)
	case *typ.Record:
		c, ok := candidate.(*typ.Record)
		if !ok || !sameRecordLayout(c, b) {
			return false, false
		}
		strict := false
		for i, field := range c.Fields {
			refines, changed := refinesSoftContainerSlot(field.Type, b.Fields[i].Type, seen)
			if !refines {
				return false, false
			}
			if changed {
				strict = true
			}
		}
		if !c.HasMapComponent() && !b.HasMapComponent() {
			return true, strict
		}
		if !c.HasMapComponent() || !b.HasMapComponent() || !Equivalent(c.MapKey, b.MapKey) {
			return false, false
		}
		refines, changed := refinesSoftContainerSlot(c.MapValue, b.MapValue, seen)
		return refines, strict || changed
	default:
		return false, false
	}
}

func refinesSoftContainerSlot(candidate, baseline typ.Type, seen softContainerSeen) (bool, bool) {
	if sameValueNodeOrAcyclicEqual(candidate, baseline) {
		return true, false
	}
	if typ.ContainsRecursive(candidate) || typ.ContainsRecursive(baseline) {
		if refines, changed := refinesSoftContainer(candidate, baseline, seen); refines {
			return true, changed
		}
	}
	if b, ok := UnwrapStructuralShape(baseline).(*typ.Union); ok {
		matched := false
		strict := false
		for _, member := range b.Members {
			refines, changed := refinesSoftContainerSlot(candidate, member, seen)
			if refines {
				matched = true
				strict = strict || changed
				continue
			}
			if typ.IsSoft(member, typ.SoftPlaceholderPolicy) {
				strict = true
				continue
			}
			return false, false
		}
		return matched, strict
	}
	if (typ.IsAny(baseline) || typ.IsUnknown(baseline)) && CanSelfEmbed(candidate) {
		return false, false
	}
	preferred, ok := PreferConcreteOverSoft(baseline, candidate)
	return ok && sameValueNodeOrAcyclicEqual(preferred, candidate), ok
}

func sameRecordFrame(a, b *typ.Record) bool {
	if !sameRecordLayout(a, b) {
		return false
	}
	for i, field := range a.Fields {
		if !typ.TypeEquals(field.Type, b.Fields[i].Type) {
			return false
		}
	}
	return true
}

func sameRecordLayout(a, b *typ.Record) bool {
	if a == nil || b == nil || a.Open != b.Open || len(a.Fields) != len(b.Fields) {
		return false
	}
	if (a.Metatable == nil) != (b.Metatable == nil) {
		return false
	}
	if a.Metatable != nil && !typ.TypeEquals(a.Metatable, b.Metatable) {
		return false
	}
	for i, field := range a.Fields {
		other := b.Fields[i]
		if field.Name != other.Name || field.Optional != other.Optional || field.Readonly != other.Readonly {
			return false
		}
	}
	return true
}

// RefinesFalsyMapKey reports whether candidate is the same table-derived shape
// as baseline after removing stale falsy key members from baseline.
func RefinesFalsyMapKey(candidate, baseline typ.Type) (bool, bool) {
	candidate = UnwrapStructuralShape(candidate)
	baseline = UnwrapStructuralShape(baseline)
	if candidate == nil || baseline == nil {
		return candidate == baseline, false
	}
	if typ.TypeEquals(candidate, baseline) {
		return true, false
	}

	switch b := baseline.(type) {
	case *typ.Array:
		c, ok := candidate.(*typ.Array)
		if !ok {
			return false, false
		}
		return truthyElementRefinement(c.Element, b.Element)
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		if !ok {
			return false, false
		}
		return mapKeyTruthyRefinement(c.Key, c.Value, b.Key, b.Value)
	case *typ.Record:
		if c, ok := candidate.(*typ.Map); ok {
			if len(b.Fields) != 0 || b.Metatable != nil || !b.HasMapComponent() {
				return false, false
			}
			return mapKeyTruthyRefinement(c.Key, c.Value, b.MapKey, b.MapValue)
		}
		c, ok := candidate.(*typ.Record)
		if !ok || !c.HasMapComponent() || !b.HasMapComponent() {
			return false, false
		}
		if c.Open && !b.Open {
			return false, false
		}
		if len(c.Fields) != len(b.Fields) {
			return false, false
		}
		for _, bf := range b.Fields {
			cf := c.GetField(bf.Name)
			if cf == nil || cf.Optional != bf.Optional || cf.Readonly != bf.Readonly || !typ.TypeEquals(cf.Type, bf.Type) {
				return false, false
			}
		}
		if (c.Metatable == nil) != (b.Metatable == nil) || (c.Metatable != nil && !typ.TypeEquals(c.Metatable, b.Metatable)) {
			return false, false
		}
		return mapKeyTruthyRefinement(c.MapKey, c.MapValue, b.MapKey, b.MapValue)
	default:
		return false, false
	}
}

func mapKeyTruthyRefinement(candidateKey, candidateValue, baselineKey, baselineValue typ.Type) (bool, bool) {
	if !typ.TypeEquals(candidateValue, baselineValue) {
		return false, false
	}
	if IsTruthyRefinement(candidateKey, baselineKey) {
		return true, true
	}
	return false, false
}

func truthyElementRefinement(candidate, baseline typ.Type) (bool, bool) {
	if typ.TypeEquals(candidate, baseline) {
		return true, false
	}
	if IsTruthyRefinement(candidate, baseline) {
		return true, true
	}
	return false, false
}

// NestedNilOnlyRegression reports whether candidate's apparent refinement only
// adds nested nil facts over a more useful baseline shape.
func NestedNilOnlyRegression(candidate, baseline typ.Type) bool {
	candidate = UnwrapStructuralShape(candidate)
	baseline = UnwrapStructuralShape(baseline)
	if candidate == nil || baseline == nil || typ.TypeEquals(candidate, baseline) {
		return false
	}
	if unwrap.IsNilType(candidate) {
		return typ.IsAny(baseline) || typ.IsUnknown(baseline) || unwrap.IsOptionalLike(baseline)
	}

	switch c := candidate.(type) {
	case *typ.Record:
		b, ok := baseline.(*typ.Record)
		if !ok {
			return false
		}
		for _, cf := range c.Fields {
			bf := b.GetField(cf.Name)
			if bf == nil {
				continue
			}
			if unwrap.IsNilType(cf.Type) && (bf.Optional || typ.IsAny(bf.Type) || typ.IsUnknown(bf.Type) || unwrap.IsOptionalLike(bf.Type)) {
				return true
			}
			if NestedNilOnlyRegression(cf.Type, bf.Type) {
				return true
			}
		}
		if c.HasMapComponent() && b.HasMapComponent() {
			return NestedNilOnlyRegression(c.MapValue, b.MapValue)
		}
	case *typ.Array:
		if b, ok := baseline.(*typ.Array); ok {
			return NestedNilOnlyRegression(c.Element, b.Element)
		}
	case *typ.Map:
		if b, ok := baseline.(*typ.Map); ok {
			return NestedNilOnlyRegression(c.Value, b.Value)
		}
	case *typ.Tuple:
		b, ok := baseline.(*typ.Tuple)
		if !ok || len(c.Elements) != len(b.Elements) {
			return false
		}
		for i := range c.Elements {
			if NestedNilOnlyRegression(c.Elements[i], b.Elements[i]) {
				return true
			}
		}
	case *typ.Function:
		b, ok := baseline.(*typ.Function)
		if !ok || len(c.Returns) != len(b.Returns) {
			return false
		}
		for i := range c.Returns {
			if NestedNilOnlyRegression(c.Returns[i], b.Returns[i]) {
				return true
			}
		}
	}
	return false
}

// ContainsNestedStructuralShape reports whether haystack embeds the same
// shallow structural shape as needle below the root.
func ContainsNestedStructuralShape(haystack, needle typ.Type) bool {
	scan := nestedStructuralShapeScan{
		needle: needle,
		seen:   make(map[nestedStructuralShapeSeenKey]bool),
	}
	return scan.contains(haystack, false)
}

type nestedStructuralShapeSeenKey struct {
	node           typ.Type
	belowContainer bool
}

type nestedStructuralShapeScan struct {
	needle typ.Type
	seen   map[nestedStructuralShapeSeenKey]bool
}

func (s *nestedStructuralShapeScan) contains(haystack typ.Type, belowContainer bool) bool {
	if haystack == nil || s == nil || s.needle == nil {
		return false
	}

	node := UnwrapStructuralShape(haystack)
	if node == nil {
		return false
	}
	if belowContainer && ShallowStructuralShapeEquals(node, s.needle) {
		return true
	}
	key := nestedStructuralShapeSeenKey{node: node, belowContainer: belowContainer}
	if s.seen[key] {
		return false
	}
	s.seen[key] = true

	descend := func(child typ.Type, childBelowContainer bool) bool {
		return s.contains(child, childBelowContainer)
	}

	switch n := node.(type) {
	case *typ.Optional:
		return descend(n.Inner, belowContainer)
	case *typ.Union:
		for _, member := range n.Members {
			if descend(member, belowContainer) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range n.Members {
			if descend(member, belowContainer) {
				return true
			}
		}
		return false
	case *typ.Array:
		return descend(n.Element, true)
	case *typ.Map:
		return descend(n.Key, true) || descend(n.Value, true)
	case *typ.Tuple:
		for _, elem := range n.Elements {
			if descend(elem, true) {
				return true
			}
		}
		return false
	case *typ.Record:
		for _, field := range n.Fields {
			if descend(field.Type, true) {
				return true
			}
		}
		if n.Metatable != nil && descend(n.Metatable, true) {
			return true
		}
		if n.HasMapComponent() {
			return descend(n.MapKey, true) || descend(n.MapValue, true)
		}
		return false
	case *typ.Function:
		for _, param := range n.Params {
			if descend(param.Type, true) {
				return true
			}
		}
		if n.Variadic != nil && descend(n.Variadic, true) {
			return true
		}
		for _, ret := range n.Returns {
			if descend(ret, true) {
				return true
			}
		}
		return false
	case *typ.Instantiated:
		for _, arg := range n.TypeArgs {
			if descend(arg, belowContainer) {
				return true
			}
		}
		return false
	case *typ.Interface:
		for _, method := range n.Methods {
			if method.Type != nil && descend(method.Type, true) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// ShallowStructuralShapeEquals reports whether a and b have the same root
// structural container shape.
func ShallowStructuralShapeEquals(a, b typ.Type) bool {
	a = UnwrapStructuralShape(a)
	b = UnwrapStructuralShape(b)
	if a == nil || b == nil {
		return a == b
	}

	switch av := a.(type) {
	case *typ.Union:
		for _, member := range av.Members {
			if ShallowStructuralShapeEquals(member, b) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range av.Members {
			if ShallowStructuralShapeEquals(member, b) {
				return true
			}
		}
		return false
	}
	switch bv := b.(type) {
	case *typ.Union:
		for _, member := range bv.Members {
			if ShallowStructuralShapeEquals(a, member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range bv.Members {
			if ShallowStructuralShapeEquals(a, member) {
				return true
			}
		}
		return false
	}

	switch av := a.(type) {
	case *typ.Array:
		_, ok := b.(*typ.Array)
		return ok
	case *typ.Map:
		bv, ok := b.(*typ.Map)
		return ok && shallowMapKeyShapeEquals(av.Key, bv.Key)
	case *typ.Tuple:
		bv, ok := b.(*typ.Tuple)
		return ok && len(av.Elements) == len(bv.Elements)
	case *typ.Record:
		bv, ok := b.(*typ.Record)
		return ok && shallowRecordShapeEquals(av, bv)
	default:
		return typ.SameNodeOrAcyclicEqual(a, b)
	}
}

// SameEvidenceFamily reports whether two types are the same shallow evidence
// variant. It is intentionally weaker than equality/subtyping and stronger than
// table-shape equality: recursive products are compared coinductively by root
// shape, while literal discriminants must agree.
func SameEvidenceFamily(a, b typ.Type) bool {
	return sameEvidenceFamily(a, b, make(map[typeFamilyPair]bool))
}

type typeFamilyPair struct {
	a typ.Type
	b typ.Type
}

func sameEvidenceFamily(a, b typ.Type, seen map[typeFamilyPair]bool) bool {
	a = unwrapEvidenceFamily(a)
	b = unwrapEvidenceFamily(b)
	if a == nil || b == nil {
		return a == b
	}
	if a == b {
		return true
	}
	pair := typeFamilyPair{a: a, b: b}
	if seen[pair] {
		return true
	}
	seen[pair] = true

	if al, ok := a.(*typ.Literal); ok {
		bl, ok := b.(*typ.Literal)
		return ok && typ.LiteralEquals(al, bl)
	}
	if _, ok := b.(*typ.Literal); ok {
		return false
	}

	if ar, ok := a.(*typ.Recursive); ok {
		br, ok := b.(*typ.Recursive)
		if !ok || ar.Name != br.Name {
			return false
		}
		return sameEvidenceFamily(ar.Body, br.Body, seen)
	}
	if _, ok := b.(*typ.Recursive); ok {
		return false
	}

	if a.Kind() == kind.Integer && b.Kind() == kind.Number {
		return true
	}
	if a.Kind() == kind.Number && b.Kind() == kind.Integer {
		return true
	}
	if a.Kind() != b.Kind() {
		return false
	}

	switch av := a.(type) {
	case *typ.Optional:
		bv, ok := b.(*typ.Optional)
		return ok && sameEvidenceFamily(av.Inner, bv.Inner, seen)
	case *typ.Union:
		bv, ok := b.(*typ.Union)
		if !ok || len(av.Members) != len(bv.Members) {
			return false
		}
		for _, am := range av.Members {
			if !containsEvidenceFamilyMember(bv.Members, am, seen) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		bv, ok := b.(*typ.Intersection)
		if !ok || len(av.Members) != len(bv.Members) {
			return false
		}
		for _, am := range av.Members {
			if !containsEvidenceFamilyMember(bv.Members, am, seen) {
				return false
			}
		}
		return true
	case *typ.Array:
		_, ok := b.(*typ.Array)
		return ok
	case *typ.Map:
		bv, ok := b.(*typ.Map)
		return ok && sameMapKeyEvidenceFamily(av.Key, bv.Key, seen)
	case *typ.Tuple:
		bv, ok := b.(*typ.Tuple)
		return ok && len(av.Elements) == len(bv.Elements)
	case *typ.Record:
		bv, ok := b.(*typ.Record)
		return ok && sameRecordEvidenceFamily(av, bv, seen)
	case *typ.Function:
		bv, ok := b.(*typ.Function)
		return ok && len(av.Params) == len(bv.Params) && len(av.Returns) == len(bv.Returns)
	case *typ.Interface:
		bv, ok := b.(*typ.Interface)
		return ok && sameInterfaceEvidenceFamily(av, bv, seen)
	default:
		return a.Kind() == b.Kind()
	}
}

func unwrapEvidenceFamily(t typ.Type) typ.Type {
	for t != nil {
		switch v := t.(type) {
		case *typ.Annotated:
			t = v.Inner
		case *typ.Alias:
			t = v.Target
		default:
			return t
		}
	}
	return nil
}

func containsEvidenceFamilyMember(members []typ.Type, target typ.Type, seen map[typeFamilyPair]bool) bool {
	for _, member := range members {
		if sameEvidenceFamily(member, target, seen) {
			return true
		}
	}
	return false
}

func sameMapKeyEvidenceFamily(a, b typ.Type, seen map[typeFamilyPair]bool) bool {
	if typ.IsAny(a) || typ.IsAny(b) || typ.IsUnknown(a) || typ.IsUnknown(b) {
		return true
	}
	return sameEvidenceFamily(a, b, seen)
}

func sameRecordEvidenceFamily(a, b *typ.Record, seen map[typeFamilyPair]bool) bool {
	if a == nil || b == nil || a.HasMapComponent() != b.HasMapComponent() {
		return a == b
	}
	if a.HasMapComponent() && !sameMapKeyEvidenceFamily(a.MapKey, b.MapKey, seen) {
		return false
	}
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	for _, af := range a.Fields {
		bf := b.GetField(af.Name)
		if bf == nil {
			return false
		}
		if !sameFieldDiscriminantFamily(af.Type, bf.Type) {
			return false
		}
	}
	return true
}

func sameFieldDiscriminantFamily(a, b typ.Type) bool {
	a = unwrapEvidenceFamily(a)
	b = unwrapEvidenceFamily(b)
	al, aLit := a.(*typ.Literal)
	bl, bLit := b.(*typ.Literal)
	if aLit || bLit {
		return aLit && bLit && typ.LiteralEquals(al, bl)
	}
	return true
}

func sameInterfaceEvidenceFamily(a, b *typ.Interface, seen map[typeFamilyPair]bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Name != "" || b.Name != "" {
		return a.Name == b.Name
	}
	if len(a.Methods) != len(b.Methods) {
		return false
	}
	for _, am := range a.Methods {
		matched := false
		for _, bm := range b.Methods {
			if am.Name == bm.Name && sameEvidenceFamily(am.Type, bm.Type, seen) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// UnwrapStructuralShape strips transparent wrappers for structural comparison.
func UnwrapStructuralShape(t typ.Type) typ.Type {
	for t != nil {
		switch v := t.(type) {
		case *typ.Annotated:
			if v.Inner == nil || v.Inner == t {
				return t
			}
			t = v.Inner
		case *typ.Alias:
			if v.Target == nil || v.Target == t {
				return t
			}
			t = v.Target
		case *typ.Optional:
			if v.Inner == nil || v.Inner == t {
				return t
			}
			t = v.Inner
		default:
			return t
		}
	}
	return nil
}

func shallowMapKeyShapeEquals(a, b typ.Type) bool {
	if a == nil || b == nil {
		return a == b
	}
	if typ.TypeEquals(a, b) {
		return true
	}
	return typ.IsAny(a) || typ.IsAny(b) || typ.IsUnknown(a) || typ.IsUnknown(b)
}

func shallowRecordShapeEquals(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.HasMapComponent() != b.HasMapComponent() {
		return false
	}
	if a.HasMapComponent() && !shallowMapKeyShapeEquals(a.MapKey, b.MapKey) {
		return false
	}
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	for _, field := range a.Fields {
		if b.GetField(field.Name) == nil {
			return false
		}
	}
	return true
}

// UnionMembers returns explicit union members after structural unwrapping.
func UnionMembers(t typ.Type) []typ.Type {
	switch v := UnwrapStructuralShape(t).(type) {
	case *typ.Union:
		return v.Members
	case *typ.Optional:
		return append([]typ.Type{typ.Nil}, UnionMembers(v.Inner)...)
	default:
		return nil
	}
}
