package flow

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

func ClearFunctionRefSubtree(out *PointState, addr StableAddress) bool {
	if out == nil {
		return false
	}
	before := out.FunctionRefs
	out.FunctionRefs = WithoutFunctionRefSubtreeAddress(out.FunctionRefs, addr)
	return !FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

func ClearClosureRefSubtree(out *PointState, addr StableAddress) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = WithoutClosureRefSubtreeAddress(out.ClosureRefs, addr)
	return !ClosureRefsDomain.Equal(before, out.ClosureRefs)
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

func SetClosureRef(out *PointState, addr StableAddress, set ClosureRefSet) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = WithClosureRefAddress(out.ClosureRefs, addr, set)
	return !ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

func ApplyClosureCellEffectsToRefs(out *PointState, addr StableAddress, effects CaptureEffects) bool {
	if out == nil {
		return false
	}
	before := out.ClosureRefs
	out.ClosureRefs = ApplyClosureRefCellEffectsAddress(out.ClosureRefs, addr, effects)
	return !ClosureRefsDomain.Equal(before, out.ClosureRefs)
}
