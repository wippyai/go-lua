package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// EntryReferenceParamPath maps a callee runtime parameter slot to the path that
// represents that parameter at function entry.
type EntryReferenceParamPath func(callee FuncRef, slot int) (constraint.Path, bool)

// EntryReferenceArgPath maps one normalized runtime argument expression to its
// caller-side storage path, when it has one.
type EntryReferenceArgPath func(runtimeIdx int, arg ast.Expr) (constraint.Path, bool)

// EntryFunctionRefArgResolver resolves function identity carried by a runtime
// argument expression when the identity is not represented by a caller storage
// path, for example a direct function literal.
type EntryFunctionRefArgResolver func(runtimeIdx int, arg ast.Expr, in *flow.PointState) (flow.FunctionRefSet, bool)

// EntryFunctionRefsArgResolver resolves a whole function-identity subtree for a
// runtime argument expression whose value is not represented by a caller storage
// path, for example a call expression returning a record with function fields.
// Returned paths are rooted at placeholder 0 and are rebased to the callee
// parameter path.
type EntryFunctionRefsArgResolver func(runtimeIdx int, arg ast.Expr, in *flow.PointState) (flow.FunctionRefs, bool)

// EntryClosureRefArgResolver resolves closure identity carried by a runtime
// argument expression when it is not represented by rebasing caller ClosureRefs.
type EntryClosureRefArgResolver func(runtimeIdx int, arg ast.Expr, in *flow.PointState) (flow.ClosureRefSet, bool)

// EntryClosureRefsArgResolver is the closure-value counterpart to
// EntryFunctionRefsArgResolver.
type EntryClosureRefsArgResolver func(runtimeIdx int, arg ast.Expr, in *flow.PointState) (flow.ClosureRefs, bool)

// EntryReferenceArgSources resolves callable-reference evidence carried by a
// runtime argument when simple caller-path rebasing is not enough.
type EntryReferenceArgSources struct {
	FunctionRefs    EntryFunctionRefArgResolver
	FunctionRefTree EntryFunctionRefsArgResolver
	ClosureRefs     EntryClosureRefArgResolver
	ClosureRefTree  EntryClosureRefsArgResolver
}

// DirectCallEntryReferenceInput is the call-boundary projection for reference
// axes. It is the reference-axis counterpart of direct entry-value projection:
// both consume normalized runtime arguments, the callee's ParamSlots mapping,
// and the callee's parameter paths.
type DirectCallEntryReferenceInput struct {
	Call   *ast.FuncCallExpr
	Callee FuncRef

	ParamSlot EntryValueParamSlot
	ParamPath EntryReferenceParamPath
	ArgPath   EntryReferenceArgPath

	FunctionRefs        flow.FunctionRefs
	ClosureRefs         flow.ClosureRefs
	ReferenceProjection flow.ReferencePathProjection
	LimitReferencePaths bool
	State               *flow.PointState
	ArgSources          EntryReferenceArgSources
}

// DirectCallEntryReferences projects both callable-reference axes for one direct
// call-boundary context. Function and closure identities share the same runtime
// argument layout, source paths, and callee parameter paths, so they are projected
// together to preserve a single boundary traversal.
func DirectCallEntryReferences(in DirectCallEntryReferenceInput) (flow.FunctionRefs, flow.ClosureRefs) {
	functionRefs := flow.FunctionRefsDomain.Bottom()
	closureRefs := flow.ClosureRefsDomain.Bottom()
	ok := forEachEntryReferenceArg(in, func(runtimeIdx int, arg ast.Expr, target constraint.Path) {
		if in.ArgPath != nil {
			if source, ok := in.ArgPath(runtimeIdx, arg); ok {
				functionRefs = flow.FunctionRefsDomain.Join(functionRefs, rebaseEntryFunctionRefs(in.FunctionRefs, source, target))
				closureRefs = flow.ClosureRefsDomain.Join(closureRefs, rebaseEntryClosureRefs(in.ClosureRefs, source, target))
			}
		}
		if in.ArgSources.FunctionRefTree != nil {
			if refs, ok := in.ArgSources.FunctionRefTree(runtimeIdx, arg, in.State); ok &&
				!flow.FunctionRefsDomain.Equal(refs, flow.FunctionRefsDomain.Bottom()) {
				functionRefs = flow.FunctionRefsDomain.Join(functionRefs, rebaseEntryFunctionRefs(refs, constraint.NewPlaceholder(0), target))
			}
		}
		if in.ArgSources.FunctionRefs != nil {
			if set, ok := in.ArgSources.FunctionRefs(runtimeIdx, arg, in.State); ok && !set.IsBottom() {
				functionRefs = joinFunctionRefAt(functionRefs, target.Key(), set)
			}
		}
		if in.ArgSources.ClosureRefTree != nil {
			if refs, ok := in.ArgSources.ClosureRefTree(runtimeIdx, arg, in.State); ok &&
				!flow.ClosureRefsDomain.Equal(refs, flow.ClosureRefsDomain.Bottom()) {
				closureRefs = flow.ClosureRefsDomain.Join(closureRefs, rebaseEntryClosureRefs(refs, constraint.NewPlaceholder(0), target))
			}
		}
		if in.ArgSources.ClosureRefs != nil {
			if set, ok := in.ArgSources.ClosureRefs(runtimeIdx, arg, in.State); ok && !set.IsBottom() {
				closureRefs = joinClosureRefAt(closureRefs, target.Key(), set)
			}
		}
	})
	if !ok {
		return flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()
	}
	functionRefs = flow.FunctionRefsDomain.Join(functionRefs, nil)
	closureRefs = flow.ClosureRefsDomain.Join(closureRefs, nil)
	if !in.LimitReferencePaths {
		return functionRefs, closureRefs
	}
	return flow.ProjectFunctionRefsByReferencePaths(functionRefs, in.ReferenceProjection),
		flow.ProjectClosureRefsByReferencePaths(closureRefs, in.ReferenceProjection)
}

func forEachEntryReferenceArg(in DirectCallEntryReferenceInput, visit func(runtimeIdx int, arg ast.Expr, target constraint.Path)) bool {
	if in.Call == nil || in.ParamSlot == nil || in.ParamPath == nil {
		return false
	}
	for _, arg := range entryRuntimeArgs(in.Callee, in.Call, in.ParamSlot) {
		target, ok := entryReferenceTargetPath(in, arg.Slot)
		if !ok {
			continue
		}
		visit(arg.RuntimeIdx, arg.Expr, target)
	}
	return true
}

func entryReferenceTargetPath(in DirectCallEntryReferenceInput, slot int) (constraint.Path, bool) {
	target, ok := in.ParamPath(in.Callee, slot)
	if !ok || target.IsEmpty() || target.Symbol == 0 {
		return constraint.Path{}, false
	}
	return target, true
}

func rebaseEntryFunctionRefs(refs flow.FunctionRefs, source, target constraint.Path) flow.FunctionRefs {
	if source.IsEmpty() || target.IsEmpty() {
		return flow.FunctionRefsDomain.Bottom()
	}
	return flow.RebaseFunctionRefsPath(refs, source, target)
}

func rebaseEntryClosureRefs(refs flow.ClosureRefs, source, target constraint.Path) flow.ClosureRefs {
	if source.IsEmpty() || target.IsEmpty() {
		return flow.ClosureRefsDomain.Bottom()
	}
	return flow.RebaseClosureRefsPath(refs, source, target)
}

func joinFunctionRefAt(refs flow.FunctionRefs, path constraint.PathKey, set flow.FunctionRefSet) flow.FunctionRefs {
	if set.IsBottom() {
		return refs
	}
	if prev, ok := flow.FunctionRefAt(refs, path); ok {
		set = flow.FunctionRefSetDomain.Join(prev, set)
	}
	return flow.WithFunctionRef(refs, path, set)
}

func joinClosureRefAt(refs flow.ClosureRefs, path constraint.PathKey, set flow.ClosureRefSet) flow.ClosureRefs {
	if set.IsBottom() {
		return refs
	}
	if prev, ok := flow.ClosureRefAt(refs, path); ok {
		set = flow.ClosureRefSetDomain.Join(prev, set)
	}
	return flow.WithClosureRef(refs, path, set)
}
