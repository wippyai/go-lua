package flow

import "github.com/wippyai/go-lua/types/constraint"

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
	var refs FunctionRefs
	if tree.HasRoot {
		refs = WithFunctionRefPath(refs, target, tree.Root)
	}
	for _, entry := range tree.Entries {
		refs = WithFunctionRefPath(refs, appendReferenceTreePath(target, entry.Segments), entry.Set)
	}
	return refs
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
	var refs ClosureRefs
	if tree.HasRoot {
		refs = withClosureRefStructuredPath(refs, target, tree.Root)
	}
	for _, entry := range tree.Entries {
		refs = withClosureRefStructuredPath(refs, appendReferenceTreePath(target, entry.Segments), entry.Set)
	}
	return refs
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
