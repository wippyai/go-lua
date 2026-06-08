package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ExpressionObserver reads solved expression types without re-entering
// synthesis.
type ExpressionObserver interface {
	TypeOf(ast.Expr, cfg.Point) typ.Type
}

// ArgumentObservation refines a call argument at a call boundary using solved
// pre-state/path observations.
type ArgumentObservation func(point cfg.Point, arg ast.Expr, current typ.Type) typ.Type

// CallEntryConfig supplies the syntax and solved-state services needed to
// reduce one call site into parameter evidence.
type CallEntryConfig struct {
	Result              *api.FuncResult
	Graph               *cfg.Graph
	Bindings            *bind.BindingTable
	ModuleBindings      *bind.BindingTable
	PreAssignTargets    checkcallsite.PreAssignmentTargets
	HasFunctionRef      func(cfg.SymbolID) bool
	EvidenceAllowed     func(cfg.SymbolID, int) bool
	Observer            ExpressionObserver
	ArgumentObservation ArgumentObservation
}

// CallEntryProjector owns call-site to EntryParams evidence reduction.
type CallEntryProjector struct {
	cfg CallEntryConfig
}

// NewCallEntryProjector creates a parameter-evidence projector for call sites.
func NewCallEntryProjector(cfg CallEntryConfig) CallEntryProjector {
	if cfg.Graph == nil && cfg.Result != nil {
		cfg.Graph = cfg.Result.Graph
	}
	if cfg.Bindings == nil && cfg.Graph != nil {
		cfg.Bindings = cfg.Graph.Bindings()
	}
	return CallEntryProjector{cfg: cfg}
}

// CalleeSymbol selects the canonical function symbol that should receive
// call-entry evidence.
func (p CallEntryProjector) CalleeSymbol(info *cfg.CallInfo) cfg.SymbolID {
	if info == nil {
		return 0
	}
	return checkcallsite.SelectPreferredSymbol(
		checkcallsite.CallableCalleeSymbolCandidates(info, p.cfg.Graph, p.cfg.Bindings, p.cfg.ModuleBindings),
		p.cfg.HasFunctionRef,
	)
}

// EntryEvidence returns callee and callback EntryParams evidence for one call.
func (p CallEntryProjector) EntryEvidence(point cfg.Point, evidence api.CallEvidence, calleeSym cfg.SymbolID) map[cfg.SymbolID][]typ.Type {
	info := evidence.Info
	if info == nil || calleeSym == 0 {
		return nil
	}
	argTypes := p.callArgumentTypes(point, info)
	out := make(map[cfg.SymbolID][]typ.Type, 1)
	if entry := FilterEmptyBodyVector(p.entryParameterEvidence(point, info, calleeSym, argTypes)); len(entry) > 0 {
		out[calleeSym] = entry
	}
	for sym, entry := range p.callbackEntryEvidence(info, evidence) {
		if len(entry) > 0 {
			out[sym] = entry
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p CallEntryProjector) callArgumentTypes(point cfg.Point, info *cfg.CallInfo) []typ.Type {
	argTypes := make([]typ.Type, len(info.Args))
	for i, arg := range info.Args {
		if arg == nil {
			continue
		}
		argType := p.typeOf(arg, point)
		argSym := p.callArgumentSymbol(info, i, arg)
		if preType := p.preAssignmentType(point, argSym); preType != nil {
			if p.cfg.PreAssignTargets.Contains(info, argSym) {
				argType = preType
			} else {
				argType = typ.JoinPreferNonSoft(argType, preType)
			}
		}
		if p.cfg.ArgumentObservation != nil {
			argType = p.cfg.ArgumentObservation(point, arg, argType)
		}
		argTypes[i] = argType
	}
	return argTypes
}

func (p CallEntryProjector) typeOf(expr ast.Expr, point cfg.Point) typ.Type {
	if p.cfg.Observer == nil {
		return nil
	}
	return p.cfg.Observer.TypeOf(expr, point)
}

func (p CallEntryProjector) callArgumentSymbol(info *cfg.CallInfo, idx int, arg ast.Expr) cfg.SymbolID {
	if info == nil {
		return 0
	}
	if idx >= 0 && idx < len(info.ArgSymbols) && info.ArgSymbols[idx] != 0 {
		return info.ArgSymbols[idx]
	}
	if p.cfg.Bindings == nil {
		return 0
	}
	return checkcallsite.SymbolFromExpr(arg, p.cfg.Bindings)
}

func (p CallEntryProjector) preAssignmentType(point cfg.Point, sym cfg.SymbolID) typ.Type {
	if sym == 0 || p.cfg.Result == nil {
		return nil
	}
	return checkcallsite.PreAssignmentTypeAtJoin(p.cfg.Graph, point, sym, func(point cfg.Point, id cfg.SymbolID) (typ.Type, bool) {
		tv := p.cfg.Result.EffectiveTypeAt(point, id)
		if tv.State != flow.StateResolved || tv.Type == nil {
			return nil, false
		}
		return tv.Type, true
	})
}

func (p CallEntryProjector) entryParameterEvidence(point cfg.Point, info *cfg.CallInfo, calleeSym cfg.SymbolID, argTypes []typ.Type) []typ.Type {
	runtimeArgCount := checkcallsite.RuntimeArgCount(info)
	evidence := EnsureCapacity(nil, runtimeArgCount)
	for runtimeIdx := 0; runtimeIdx < runtimeArgCount; runtimeIdx++ {
		if !p.evidenceAllowed(calleeSym, runtimeIdx) {
			continue
		}
		arg := checkcallsite.RuntimeArgAt(info, runtimeIdx)
		if arg == nil {
			continue
		}
		argType := p.runtimeArgumentType(point, info, runtimeIdx, arg, argTypes)
		evidence, _ = MergeBodyCallArgAt(evidence, runtimeIdx, argType, typ.JoinPreferNonSoft, true)
	}
	return evidence
}

func (p CallEntryProjector) runtimeArgumentType(point cfg.Point, info *cfg.CallInfo, runtimeIdx int, arg ast.Expr, argTypes []typ.Type) typ.Type {
	if checkcallsite.IsMethodCallInfo(info) && runtimeIdx == 0 {
		return p.typeOf(info.Receiver, point)
	}
	argIdx := runtimeIdx
	if checkcallsite.IsMethodCallInfo(info) {
		argIdx--
	}
	if argIdx >= 0 && argIdx < len(argTypes) && argTypes[argIdx] != nil {
		return argTypes[argIdx]
	}
	return p.typeOf(arg, point)
}

func (p CallEntryProjector) callbackEntryEvidence(info *cfg.CallInfo, evidence api.CallEvidence) map[cfg.SymbolID][]typ.Type {
	var out map[cfg.SymbolID][]typ.Type
	for i, arg := range info.Args {
		if arg == nil {
			continue
		}
		expectedFn := unwrap.Function(evidence.ExpectedArgType(i))
		if expectedFn == nil {
			continue
		}
		argSym := checkcallsite.CanonicalSymbolFromExprWithAliases(
			arg,
			0,
			p.cfg.Graph,
			p.cfg.Bindings,
			p.cfg.ModuleBindings,
			p.cfg.HasFunctionRef,
		)
		if argSym == 0 || !p.hasFunctionRef(argSym) {
			continue
		}
		evidence := out[argSym]
		for j, param := range expectedFn.Params {
			if !p.evidenceAllowed(argSym, j) {
				continue
			}
			evidence, _ = MergeBodyCallArgAt(evidence, j, param.Type, typ.JoinPreferNonSoft, true)
		}
		if len(evidence) == 0 {
			continue
		}
		if out == nil {
			out = make(map[cfg.SymbolID][]typ.Type)
		}
		out[argSym] = evidence
	}
	return out
}

func (p CallEntryProjector) hasFunctionRef(sym cfg.SymbolID) bool {
	if p.cfg.HasFunctionRef == nil {
		return sym != 0
	}
	return p.cfg.HasFunctionRef(sym)
}

func (p CallEntryProjector) evidenceAllowed(sym cfg.SymbolID, idx int) bool {
	if p.cfg.EvidenceAllowed == nil {
		return true
	}
	return p.cfg.EvidenceAllowed(sym, idx)
}
