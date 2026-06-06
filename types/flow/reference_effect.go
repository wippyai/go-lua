package flow

import "github.com/wippyai/go-lua/types/constraint"

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

func JoinFunctionRefs(out *PointState, refs FunctionRefs) bool {
	if out == nil {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = FunctionRefsDomain.Join(out.FunctionRefs, refs)
	return !FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

func JoinClosureRefs(out *PointState, refs ClosureRefs) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = ClosureRefsDomain.Join(out.ClosureRefs, refs)
	return !ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

func SetFunctionRef(out *PointState, addr StableAddress, set FunctionRefSet) bool {
	if out == nil {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = WithFunctionRefAddress(out.FunctionRefs, addr, set)
	return !FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

// SetFunctionRefPath strongly updates the identity set for a structured path.
func SetFunctionRefPath(out *PointState, path constraint.Path, set FunctionRefSet) bool {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return false
	}
	return SetFunctionRef(out, addr, set)
}

func SetClosureRef(out *PointState, addr StableAddress, set ClosureRefSet) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = WithClosureRefAddress(out.ClosureRefs, addr, set)
	return !ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

// SetClosureRefPath strongly updates the closure set for a structured path.
func SetClosureRefPath(out *PointState, path constraint.Path, set ClosureRefSet) bool {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return false
	}
	return SetClosureRef(out, addr, set)
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
