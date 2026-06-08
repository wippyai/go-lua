package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
)

// FunctionRefTree is the normalized function-identity value of one reference
// subtree before it is installed at a concrete target path.
type FunctionRefTree struct {
	Root    FunctionRefSet
	HasRoot bool
	Entries []FunctionRefTreeEntry
}

type FunctionRefTreeEntry struct {
	Segments []constraint.Segment
	Set      FunctionRefSet
}

// ClosureRefTree is the closure-identity counterpart of FunctionRefTree.
type ClosureRefTree struct {
	Root    ClosureRefSet
	HasRoot bool
	Entries []ClosureRefTreeEntry
}

type ClosureRefTreeEntry struct {
	Segments []constraint.Segment
	Set      ClosureRefSet
}

// ReplaceFunctionRefSubtree clears all function identities at addr and its
// descendants, then joins the supplied refs into the function-reference axis.
func ReplaceFunctionRefSubtree(out *PointState, addr StableAddress, refs FunctionRefs) bool {
	if out == nil {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = WithoutFunctionRefSubtreeAddress(out.FunctionRefs, addr)
	out.FunctionRefs = FunctionRefsDomain.Join(out.FunctionRefs, refs)
	return !FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

// ReplaceFunctionRefSubtreePath applies a strong subtree update from a
// structured path, keeping address normalization in the reference axis.
func ReplaceFunctionRefSubtreePath(out *PointState, path constraint.Path, refs FunctionRefs) bool {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return false
	}
	return ReplaceFunctionRefSubtree(out, addr, refs)
}

// ReplaceFunctionRefTreePath strongly installs a normalized function-identity
// tree at target, replacing every stale identity in the target subtree.
func ReplaceFunctionRefTreePath(out *PointState, target constraint.Path, tree FunctionRefTree) bool {
	return ReplaceFunctionRefSubtreePath(out, target, functionRefsFromTree(target, tree))
}

// JoinFunctionRefTreePath additively publishes a normalized function-identity
// tree at target without clearing existing identities.
func JoinFunctionRefTreePath(out *PointState, target constraint.Path, tree FunctionRefTree) bool {
	if out == nil {
		return false
	}
	refs := functionRefsFromTree(target, tree)
	if FunctionRefsDomain.Equal(refs, FunctionRefsDomain.Bottom()) {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = FunctionRefsDomain.Join(out.FunctionRefs, refs)
	return !FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

func functionRefsFromTree(target constraint.Path, tree FunctionRefTree) FunctionRefs {
	tree = canonicalFunctionRefTree(tree)
	var refs FunctionRefs
	if tree.HasRoot {
		refs = WithFunctionRefPath(refs, target, tree.Root)
	}
	for _, entry := range tree.Entries {
		refs = WithFunctionRefPath(refs, appendReferenceTreePath(target, entry.Segments), entry.Set)
	}
	return refs
}

// FunctionRefTreeFromSubtreePath projects refs under source into a relative
// tree. Summary return slots use placeholder roots; callers can install the
// resulting tree at a concrete target without owning address rebasing.
func FunctionRefTreeFromSubtreePath(refs FunctionRefs, source constraint.Path) (FunctionRefTree, bool) {
	sourceAddr, ok := StableAddressOfPath(source)
	if !ok {
		return FunctionRefTree{}, false
	}
	if FunctionRefsDomain.Equal(refs, FunctionRefsDomain.Top()) {
		return FunctionRefTree{Root: FunctionRefSetTop(), HasRoot: true}, true
	}
	if len(refs) == 0 {
		return FunctionRefTree{}, false
	}
	var tree FunctionRefTree
	for key, set := range refs {
		if set.IsBottom() {
			continue
		}
		addr, ok := StableAddressFromCanonicalKey(key)
		if !ok {
			continue
		}
		remainder, ok := addr.RemainderAfterPrefix(sourceAddr)
		if !ok {
			continue
		}
		if len(remainder) == 0 {
			if tree.HasRoot {
				set = FunctionRefSetDomain.Join(tree.Root, set)
			}
			tree.Root = set
			tree.HasRoot = true
			continue
		}
		tree.Entries = append(tree.Entries, FunctionRefTreeEntry{
			Segments: append([]constraint.Segment(nil), remainder...),
			Set:      set,
		})
	}
	tree = canonicalFunctionRefTree(tree)
	return tree, tree.HasRoot || len(tree.Entries) > 0
}

// AssignFunctionRefSubtreePath copies all function identities rooted at source
// to target and strongly replaces the target subtree. Flow owns the projection,
// rebase, and replacement as one reference-axis transaction; syntax-facing
// transfer code should only decide which paths participate.
func AssignFunctionRefSubtreePath(out *PointState, source, target constraint.Path) bool {
	if out == nil {
		return false
	}
	refs := ProjectFunctionRefsByReferencePaths(out.FunctionRefs, ReferencePathProjection{
		Subtrees: []constraint.Path{source},
	})
	rebased := RebaseFunctionRefsPath(refs, source, target)
	return ReplaceFunctionRefSubtreePath(out, target, rebased)
}

// ReplaceClosureRefSubtree clears all closure identities at addr and its
// descendants, then joins the supplied refs into the closure-reference axis.
func ReplaceClosureRefSubtree(out *PointState, addr StableAddress, refs ClosureRefs) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = WithoutClosureRefSubtreeAddress(out.ClosureRefs, addr)
	out.ClosureRefs = ClosureRefsDomain.Join(out.ClosureRefs, refs)
	return !ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

// ReplaceClosureRefSubtreePath applies a strong closure-ref subtree update from
// a structured path.
func ReplaceClosureRefSubtreePath(out *PointState, path constraint.Path, refs ClosureRefs) bool {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return false
	}
	return ReplaceClosureRefSubtree(out, addr, refs)
}

// ReplaceClosureRefTreePath strongly installs a normalized closure-identity
// tree at target, replacing every stale identity in the target subtree.
func ReplaceClosureRefTreePath(out *PointState, target constraint.Path, tree ClosureRefTree) bool {
	return ReplaceClosureRefSubtreePath(out, target, closureRefsFromTree(target, tree))
}

func closureRefsFromTree(target constraint.Path, tree ClosureRefTree) ClosureRefs {
	tree = canonicalClosureRefTree(tree)
	var refs ClosureRefs
	if tree.HasRoot {
		refs = withClosureRefStructuredPath(refs, target, tree.Root)
	}
	for _, entry := range tree.Entries {
		refs = withClosureRefStructuredPath(refs, appendReferenceTreePath(target, entry.Segments), entry.Set)
	}
	return refs
}

// ClosureRefTreeFromSubtreePath is the closure-reference counterpart of
// FunctionRefTreeFromSubtreePath.
func ClosureRefTreeFromSubtreePath(refs ClosureRefs, source constraint.Path) (ClosureRefTree, bool) {
	sourceAddr, ok := StableAddressOfPath(source)
	if !ok {
		return ClosureRefTree{}, false
	}
	if ClosureRefsDomain.Equal(refs, ClosureRefsDomain.Top()) {
		return ClosureRefTree{Root: ClosureRefSetTop(), HasRoot: true}, true
	}
	if len(refs) == 0 {
		return ClosureRefTree{}, false
	}
	var tree ClosureRefTree
	for key, set := range refs {
		if set.IsBottom() {
			continue
		}
		addr, ok := StableAddressFromCanonicalKey(key)
		if !ok {
			continue
		}
		remainder, ok := addr.RemainderAfterPrefix(sourceAddr)
		if !ok {
			continue
		}
		if len(remainder) == 0 {
			if tree.HasRoot {
				set = ClosureRefSetDomain.Join(tree.Root, set)
			}
			tree.Root = set
			tree.HasRoot = true
			continue
		}
		tree.Entries = append(tree.Entries, ClosureRefTreeEntry{
			Segments: append([]constraint.Segment(nil), remainder...),
			Set:      set,
		})
	}
	tree = canonicalClosureRefTree(tree)
	return tree, tree.HasRoot || len(tree.Entries) > 0
}

func withClosureRefStructuredPath(refs ClosureRefs, path constraint.Path, set ClosureRefSet) ClosureRefs {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return refs
	}
	return WithClosureRefAddress(refs, addr, set)
}

func appendReferenceTreePath(base constraint.Path, segments []constraint.Segment) constraint.Path {
	if len(segments) == 0 {
		return base
	}
	next := base
	next.Segments = append(append([]constraint.Segment(nil), base.Segments...), segments...)
	return next
}

func canonicalFunctionRefTree(tree FunctionRefTree) FunctionRefTree {
	out := FunctionRefTree{}
	if tree.HasRoot && !tree.Root.IsBottom() {
		out.Root = tree.Root
		out.HasRoot = true
	}
	if len(tree.Entries) == 0 {
		return out
	}
	out.Entries = make([]FunctionRefTreeEntry, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		if len(entry.Segments) == 0 {
			if entry.Set.IsBottom() {
				continue
			}
			if out.HasRoot {
				out.Root = FunctionRefSetDomain.Join(out.Root, entry.Set)
			} else {
				out.Root = entry.Set
				out.HasRoot = true
			}
			continue
		}
		if entry.Set.IsBottom() {
			continue
		}
		out.Entries = append(out.Entries, FunctionRefTreeEntry{
			Segments: append([]constraint.Segment(nil), entry.Segments...),
			Set:      entry.Set,
		})
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		return compareReferenceTreeSegments(out.Entries[i].Segments, out.Entries[j].Segments) < 0
	})
	out.Entries = mergeFunctionRefTreeEntries(out.Entries)
	return out
}

func mergeFunctionRefTreeEntries(entries []FunctionRefTreeEntry) []FunctionRefTreeEntry {
	if len(entries) <= 1 {
		return entries
	}
	out := entries[:0]
	for _, entry := range entries {
		last := len(out) - 1
		if last >= 0 && compareReferenceTreeSegments(out[last].Segments, entry.Segments) == 0 {
			out[last].Set = FunctionRefSetDomain.Join(out[last].Set, entry.Set)
			continue
		}
		out = append(out, entry)
	}
	return out
}

func canonicalClosureRefTree(tree ClosureRefTree) ClosureRefTree {
	out := ClosureRefTree{}
	if tree.HasRoot && !tree.Root.IsBottom() {
		out.Root = tree.Root
		out.HasRoot = true
	}
	if len(tree.Entries) == 0 {
		return out
	}
	out.Entries = make([]ClosureRefTreeEntry, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		if len(entry.Segments) == 0 {
			if entry.Set.IsBottom() {
				continue
			}
			if out.HasRoot {
				out.Root = ClosureRefSetDomain.Join(out.Root, entry.Set)
			} else {
				out.Root = entry.Set
				out.HasRoot = true
			}
			continue
		}
		if entry.Set.IsBottom() {
			continue
		}
		out.Entries = append(out.Entries, ClosureRefTreeEntry{
			Segments: append([]constraint.Segment(nil), entry.Segments...),
			Set:      entry.Set,
		})
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		return compareReferenceTreeSegments(out.Entries[i].Segments, out.Entries[j].Segments) < 0
	})
	out.Entries = mergeClosureRefTreeEntries(out.Entries)
	return out
}

func mergeClosureRefTreeEntries(entries []ClosureRefTreeEntry) []ClosureRefTreeEntry {
	if len(entries) <= 1 {
		return entries
	}
	out := entries[:0]
	for _, entry := range entries {
		last := len(out) - 1
		if last >= 0 && compareReferenceTreeSegments(out[last].Segments, entry.Segments) == 0 {
			out[last].Set = ClosureRefSetDomain.Join(out[last].Set, entry.Set)
			continue
		}
		out = append(out, entry)
	}
	return out
}

func compareReferenceTreeSegments(a, b []constraint.Segment) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if c := compareReferenceTreeSegment(a[i], b[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

func compareReferenceTreeSegment(a, b constraint.Segment) int {
	if a.Kind < b.Kind {
		return -1
	}
	if a.Kind > b.Kind {
		return 1
	}
	if a.Name < b.Name {
		return -1
	}
	if a.Name > b.Name {
		return 1
	}
	if a.Index < b.Index {
		return -1
	}
	if a.Index > b.Index {
		return 1
	}
	return 0
}

// AssignClosureRefSubtreePath is the closure-value counterpart of
// AssignFunctionRefSubtreePath.
func AssignClosureRefSubtreePath(out *PointState, source, target constraint.Path) bool {
	if out == nil {
		return false
	}
	refs := ProjectClosureRefsByReferencePaths(out.ClosureRefs, ReferencePathProjection{
		Subtrees: []constraint.Path{source},
	})
	rebased := RebaseClosureRefsPath(refs, source, target)
	return ReplaceClosureRefSubtreePath(out, target, rebased)
}

func ClearFunctionRefSubtree(out *PointState, addr StableAddress) bool {
	if out == nil {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = WithoutFunctionRefSubtreeAddress(out.FunctionRefs, addr)
	return !FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

// ClearFunctionRefSubtreePath removes function identities below a structured
// path.
func ClearFunctionRefSubtreePath(out *PointState, path constraint.Path) bool {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return false
	}
	return ClearFunctionRefSubtree(out, addr)
}

func ClearClosureRefSubtree(out *PointState, addr StableAddress) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = WithoutClosureRefSubtreeAddress(out.ClosureRefs, addr)
	return !ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

// ClearClosureRefSubtreePath removes closure identities below a structured path.
func ClearClosureRefSubtreePath(out *PointState, path constraint.Path) bool {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return false
	}
	return ClearClosureRefSubtree(out, addr)
}

func ApplyClosureCellEffectsToRefs(out *PointState, addr StableAddress, effects CaptureEffects) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = ApplyClosureRefCellEffectsAddress(out.ClosureRefs, addr, effects)
	return !ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

// ApplyClosureCellEffectsToRefsPath updates a stored closure environment at a
// structured path.
func ApplyClosureCellEffectsToRefsPath(out *PointState, path constraint.Path, effects CaptureEffects) bool {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return false
	}
	return ApplyClosureCellEffectsToRefs(out, addr, effects)
}
