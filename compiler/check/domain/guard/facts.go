package guard

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// TypeCheckBind is the narrowing established by `local value, err = T:is(x)`.
// On the edge where err is nil, every NarrowSym carries Type.
type TypeCheckBind struct {
	ErrSym     cfg.SymbolID
	NarrowSyms []cfg.SymbolID
	Type       typ.Type
}

// PredicateFunction records a local function whose body returns a builtin
// type(param) == kind predicate on one of its parameters.
type PredicateFunction struct {
	FuncSym    cfg.SymbolID
	ParamIndex int
	Kind       string
}

// PredicateResult records an assigned predicate result `local ok = P(arg)`,
// keyed by the ok symbol inside the graph that owns the assignment.
type PredicateResult struct {
	CondSym   cfg.SymbolID
	NarrowSym cfg.SymbolID
	Kind      string
}

// TypeCheckBinds derives one graph's `T:is` success-edge narrowing facts.
func TypeCheckBinds(g *cfg.Graph, typeByName func(string) typ.Type) []TypeCheckBind {
	if g == nil || typeByName == nil {
		return nil
	}
	bindings := g.Bindings()
	var out []TypeCheckBind
	g.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i := range info.Targets {
			target := info.Targets[i]
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			call, retIdx := info.CallForTarget(i)
			if call == nil || retIdx != 1 || !call.IsTypeCheck || call.Method != "is" || call.Receiver == nil {
				continue
			}
			checked := typeByName(call.TypeCheckName)
			if checked == nil || typ.IsAbsentOrUnknown(checked) {
				continue
			}
			narrowSyms := typeCheckNarrowSyms(info, i, retIdx, call, bindings)
			if len(narrowSyms) == 0 {
				continue
			}
			out = append(out, TypeCheckBind{
				ErrSym:     target.Symbol,
				NarrowSyms: narrowSyms,
				Type:       checked,
			})
		}
	})
	if len(out) == 0 {
		return nil
	}
	for i := range out {
		slices.Sort(out[i].NarrowSyms)
		out[i].NarrowSyms = slices.Compact(out[i].NarrowSyms)
	}
	return out
}

// typeCheckNarrowSyms is the set of symbols a `local val, err = T:is(x)` guard
// proves to be T on the success edge: the value target val and the checked
// argument x when it is an identifier.
func typeCheckNarrowSyms(info *cfg.AssignInfo, errTargetIdx, errRetIdx int, call *cfg.CallInfo, bindings *bind.BindingTable) []cfg.SymbolID {
	seen := make(map[cfg.SymbolID]bool)
	var syms []cfg.SymbolID
	add := func(sym cfg.SymbolID) {
		if sym == 0 || seen[sym] {
			return
		}
		seen[sym] = true
		syms = append(syms, sym)
	}
	valIdx := errTargetIdx - errRetIdx
	if valIdx >= 0 && valIdx < len(info.Targets) {
		vt := info.Targets[valIdx]
		if vt.Kind == cfg.TargetIdent && vt.Symbol != 0 {
			add(vt.Symbol)
		}
	}
	if len(call.Args) == 1 && bindings != nil {
		if ident, ok := call.Args[0].(*ast.IdentExpr); ok && ident != nil {
			if sym, ok := bindings.SymbolOf(ident); ok {
				add(sym)
			}
		}
	}
	return syms
}
