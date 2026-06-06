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

// EntryClosureRefArgResolver resolves closure identity carried by a runtime
// argument expression when it is not represented by rebasing caller ClosureRefs.
type EntryClosureRefArgResolver func(runtimeIdx int, arg ast.Expr, in *flow.PointState) (flow.ClosureRefSet, bool)

// EntryReferenceTreesArgResolver resolves whole reference subtrees carried by
// one runtime argument whose value is not represented by a caller storage path,
// for example a call expression returning a record with callable fields.
// Returned paths are rooted at placeholder 0 and are rebased to the callee
// parameter path.
type EntryReferenceTreesArgResolver func(runtimeIdx int, arg ast.Expr, in *flow.PointState) (flow.ReferenceContext, bool)

// EntryReferenceArgSources resolves callable-reference evidence carried by a
// runtime argument when simple caller-path rebasing is not enough.
type EntryReferenceArgSources struct {
	FunctionRefs EntryFunctionRefArgResolver
	RefTrees     EntryReferenceTreesArgResolver
	ClosureRefs  EntryClosureRefArgResolver
}

// directCallEntryReferenceInput is the call-boundary projection for reference
// axes. It is the reference-axis counterpart of direct entry-value projection:
// both consume normalized runtime arguments, the callee's ParamSlots mapping,
// and the callee's parameter paths.
type directCallEntryReferenceInput struct {
	Call   *ast.FuncCallExpr
	Callee FuncRef

	ParamSlot EntryValueParamSlot
	ParamPath EntryReferenceParamPath
	ArgPath   EntryReferenceArgPath

	References          flow.ReferenceContext
	ReferenceProjection flow.ReferencePathProjection
	LimitReferencePaths bool
	State               *flow.PointState
	ArgSources          EntryReferenceArgSources
}

// directCallEntryReferences projects both callable-reference axes for one direct
// call-boundary context. Function and closure identities share the same runtime
// argument layout, source paths, and callee parameter paths, so they are projected
// together to preserve a single boundary traversal.
func directCallEntryReferences(in directCallEntryReferenceInput) flow.ReferenceContext {
	out := flow.ReferenceContextBottom()
	references := in.References
	ok := forEachEntryReferenceArg(in, func(runtimeIdx int, arg ast.Expr, target constraint.Path) {
		if in.ArgPath != nil {
			if source, ok := in.ArgPath(runtimeIdx, arg); ok {
				out = out.Join(references.RebaseCallablePaths(source, target))
			}
		}
		if in.ArgSources.RefTrees != nil {
			tree, ok := in.ArgSources.RefTrees(runtimeIdx, arg, in.State)
			if ok {
				out = out.Join(tree.RebaseCallablePaths(constraint.NewPlaceholder(0), target))
			}
		}
		if in.ArgSources.FunctionRefs != nil {
			if set, ok := in.ArgSources.FunctionRefs(runtimeIdx, arg, in.State); ok && !set.IsBottom() {
				out = out.JoinFunctionRefAt(target.Key(), set)
			}
		}
		if in.ArgSources.ClosureRefs != nil {
			if set, ok := in.ArgSources.ClosureRefs(runtimeIdx, arg, in.State); ok && !set.IsBottom() {
				out = out.JoinClosureRefAt(target.Key(), set)
			}
		}
	})
	if !ok {
		return flow.ReferenceContextBottom()
	}
	if in.LimitReferencePaths {
		out = out.ProjectPaths(in.ReferenceProjection)
	}
	return out
}

func forEachEntryReferenceArg(in directCallEntryReferenceInput, visit func(runtimeIdx int, arg ast.Expr, target constraint.Path)) bool {
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

func entryReferenceTargetPath(in directCallEntryReferenceInput, slot int) (constraint.Path, bool) {
	target, ok := in.ParamPath(in.Callee, slot)
	if !ok || target.IsEmpty() || target.Symbol == 0 {
		return constraint.Path{}, false
	}
	return target, true
}
