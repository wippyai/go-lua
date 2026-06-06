package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/callsite"
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

// DirectCallEntryReferenceInput is the call-boundary projection for reference
// axes. It is the reference-axis counterpart of DirectCallEntryProductValues:
// both consume normalized runtime arguments, the callee's ParamSlots mapping,
// and the callee's parameter paths.
type DirectCallEntryReferenceInput struct {
	Call   *ast.FuncCallExpr
	Callee FuncRef

	ParamSlot EntryValueParamSlot
	ParamPath EntryReferenceParamPath
	ArgPath   EntryReferenceArgPath

	FunctionRefs           flow.FunctionRefs
	ClosureRefs            flow.ClosureRefs
	ReferenceProjection    flow.ReferencePathProjection
	LimitReferencePaths    bool
	State                  *flow.PointState
	ResolveFunctionArg     EntryFunctionRefArgResolver
	ResolveFunctionArgRefs EntryFunctionRefsArgResolver
	ResolveClosureArg      EntryClosureRefArgResolver
	ResolveClosureArgRefs  EntryClosureRefsArgResolver
}

// DirectCallEntryFunctionRefs projects caller function identities into callee
// parameter paths. A path-backed argument rebases its whole FunctionRefs subtree;
// a direct function literal or other non-path callable seeds the parameter root.
func DirectCallEntryFunctionRefs(in DirectCallEntryReferenceInput) flow.FunctionRefs {
	out := flow.FunctionRefsDomain.Bottom()
	ok := forEachEntryReferenceArg(in, func(runtimeIdx int, arg ast.Expr, target constraint.Path) {
		if in.ArgPath != nil {
			if source, ok := in.ArgPath(runtimeIdx, arg); ok {
				out = flow.FunctionRefsDomain.Join(out, rebaseEntryFunctionRefs(in.FunctionRefs, source, target))
			}
		}
		if in.ResolveFunctionArgRefs != nil {
			if refs, ok := in.ResolveFunctionArgRefs(runtimeIdx, arg, in.State); ok &&
				!flow.FunctionRefsDomain.Equal(refs, flow.FunctionRefsDomain.Bottom()) {
				out = flow.FunctionRefsDomain.Join(out, rebaseEntryFunctionRefs(refs, constraint.NewPlaceholder(0), target))
			}
		}
		if in.ResolveFunctionArg == nil {
			return
		}
		set, ok := in.ResolveFunctionArg(runtimeIdx, arg, in.State)
		if !ok || set.IsBottom() {
			return
		}
		out = joinFunctionRefAt(out, target.Key(), set)
	})
	if !ok {
		return flow.FunctionRefsDomain.Bottom()
	}
	out = flow.FunctionRefsDomain.Join(out, nil)
	if !in.LimitReferencePaths {
		return out
	}
	return flow.ProjectFunctionRefsByReferencePaths(out, in.ReferenceProjection)
}

// DirectCallEntryClosureRefs is the closure-value counterpart to
// DirectCallEntryFunctionRefs. It preserves closure entry environments when the
// caller has them, while still allowing a direct function literal to seed a
// parameter root through ResolveClosureArg.
func DirectCallEntryClosureRefs(in DirectCallEntryReferenceInput) flow.ClosureRefs {
	out := flow.ClosureRefsDomain.Bottom()
	ok := forEachEntryReferenceArg(in, func(runtimeIdx int, arg ast.Expr, target constraint.Path) {
		if in.ArgPath != nil {
			if source, ok := in.ArgPath(runtimeIdx, arg); ok {
				out = flow.ClosureRefsDomain.Join(out, rebaseEntryClosureRefs(in.ClosureRefs, source, target))
			}
		}
		if in.ResolveClosureArgRefs != nil {
			if refs, ok := in.ResolveClosureArgRefs(runtimeIdx, arg, in.State); ok &&
				!flow.ClosureRefsDomain.Equal(refs, flow.ClosureRefsDomain.Bottom()) {
				out = flow.ClosureRefsDomain.Join(out, rebaseEntryClosureRefs(refs, constraint.NewPlaceholder(0), target))
			}
		}
		if in.ResolveClosureArg == nil {
			return
		}
		set, ok := in.ResolveClosureArg(runtimeIdx, arg, in.State)
		if !ok || set.IsBottom() {
			return
		}
		out = joinClosureRefAt(out, target.Key(), set)
	})
	if !ok {
		return flow.ClosureRefsDomain.Bottom()
	}
	out = flow.ClosureRefsDomain.Join(out, nil)
	if !in.LimitReferencePaths {
		return out
	}
	return flow.ProjectClosureRefsByReferencePaths(out, in.ReferenceProjection)
}

func forEachEntryReferenceArg(in DirectCallEntryReferenceInput, visit func(runtimeIdx int, arg ast.Expr, target constraint.Path)) bool {
	if in.Call == nil || in.ParamSlot == nil || in.ParamPath == nil {
		return false
	}
	for runtimeIdx := 0; runtimeIdx < callsite.RuntimeArgExprCount(in.Call); runtimeIdx++ {
		arg := callsite.RuntimeArgExprAt(in.Call, runtimeIdx)
		target, ok := entryReferenceTargetPath(in, runtimeIdx)
		if !ok {
			continue
		}
		visit(runtimeIdx, arg, target)
	}
	return true
}

func entryReferenceTargetPath(in DirectCallEntryReferenceInput, runtimeIdx int) (constraint.Path, bool) {
	_, slot, ok := in.ParamSlot(in.Callee, in.Call, runtimeIdx)
	if !ok {
		return constraint.Path{}, false
	}
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
	sourceAddr, sourceOK := flow.StableAddressOfPath(source)
	targetAddr, targetOK := flow.StableAddressOfPath(target)
	if !sourceOK || !targetOK {
		return flow.FunctionRefsDomain.Bottom()
	}
	if flow.FunctionRefsDomain.Equal(refs, flow.FunctionRefsDomain.Top()) {
		return flow.WithFunctionRefAddress(nil, targetAddr, flow.FunctionRefSetTop())
	}
	return flow.RebaseFunctionRefsAddress(refs, sourceAddr, targetAddr)
}

func rebaseEntryClosureRefs(refs flow.ClosureRefs, source, target constraint.Path) flow.ClosureRefs {
	if source.IsEmpty() || target.IsEmpty() {
		return flow.ClosureRefsDomain.Bottom()
	}
	sourceAddr, sourceOK := flow.StableAddressOfPath(source)
	targetAddr, targetOK := flow.StableAddressOfPath(target)
	if !sourceOK || !targetOK {
		return flow.ClosureRefsDomain.Bottom()
	}
	if flow.ClosureRefsDomain.Equal(refs, flow.ClosureRefsDomain.Top()) {
		return flow.WithClosureRefAddress(nil, targetAddr, flow.ClosureRefSetTop())
	}
	return flow.RebaseClosureRefsAddress(refs, sourceAddr, targetAddr)
}

func joinFunctionRefAt(refs flow.FunctionRefs, path constraint.PathKey, set flow.FunctionRefSet) flow.FunctionRefs {
	addr, ok := flow.StableAddressFromKey(path)
	if !ok || set.IsBottom() {
		return refs
	}
	if prev, ok := flow.FunctionRefAtAddress(refs, addr); ok {
		set = flow.FunctionRefSetDomain.Join(prev, set)
	}
	return flow.WithFunctionRefAddress(refs, addr, set)
}

func joinClosureRefAt(refs flow.ClosureRefs, path constraint.PathKey, set flow.ClosureRefSet) flow.ClosureRefs {
	addr, ok := flow.StableAddressFromKey(path)
	if !ok || set.IsBottom() {
		return refs
	}
	if prev, ok := flow.ClosureRefAtAddress(refs, addr); ok {
		set = flow.ClosureRefSetDomain.Join(prev, set)
	}
	return flow.WithClosureRefAddress(refs, addr, set)
}
