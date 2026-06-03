package facts

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
)

const metatableIndexField = "__index"

// collectMetatableIndexes extracts the static metatable -> prototype topology
// from syntax/name-resolution only. It intentionally stores symbols, not solved
// types:
//
//   - local mt = { __index = methods }
//   - Class.__index = Class
func collectMetatableIndexes(p Program) []metatable.Index {
	byMeta := make(map[cfg.SymbolID]cfg.SymbolID)
	for _, r := range p.Refs {
		g := graphOf(p, r)
		if g == nil || g.Bindings() == nil || p.Evidence == nil {
			continue
		}
		bindings := g.Bindings()
		for _, assign := range p.Evidence(g).Assignments {
			info := assign.Info
			if info == nil {
				continue
			}
			for i, target := range info.Targets {
				src := info.SourceAt(i)
				switch {
				case target.Kind == cfg.TargetIdent && target.Symbol != 0:
					tbl, ok := src.(*ast.TableExpr)
					if !ok {
						continue
					}
					if proto := indexFieldSourceSymbol(tbl, bindings); proto != 0 {
						byMeta[target.Symbol] = proto
					}
				case target.Kind == cfg.TargetField && target.BaseSymbol != 0 &&
					len(target.FieldPath) == 1 && target.FieldPath[0] == metatableIndexField:
					if proto := identSymbol(src, bindings); proto != 0 {
						byMeta[target.BaseSymbol] = proto
					}
				}
			}
		}
	}
	if len(byMeta) == 0 {
		return nil
	}
	out := make([]metatable.Index, 0, len(byMeta))
	for mt, proto := range byMeta {
		out = append(out, metatable.Index{MetatableSym: mt, PrototypeSym: proto})
	}
	return compactMetatableIndexEntries(sortedMetatableIndexes(out))
}

// collectMethodReceivers extracts method-body receiver topology. A method body is
// keyed by its function ref and the receiver prototype symbol; its self slot is
// the graph slot occupied by the implicit/explicit `self` parameter, currently
// slot 0 in canonical CFG layout.
func collectMethodReceivers(p Program) []methodReceiverEntry {
	if p.RefForFuncSymbol == nil {
		return nil
	}
	var out []methodReceiverEntry
	for _, owner := range p.Refs {
		g := graphOf(p, owner)
		if g == nil {
			continue
		}
		g.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
			if info == nil || info.Symbol == 0 || info.ReceiverSymbol == 0 {
				return
			}
			if info.TargetKind != cfg.FuncDefMethod && info.TargetKind != cfg.FuncDefField {
				return
			}
			r, ok := p.RefForFuncSymbol(info.Symbol)
			if !ok {
				return
			}
			if !methodHasSelfSlot(graphOf(p, r)) {
				return
			}
			out = append(out, methodReceiverEntry{
				FuncRef: r,
				Info: metatable.MethodReceiver{
					PrototypeSym: info.ReceiverSymbol,
					SelfSlot:     0,
				},
			})
		})
	}
	return compactMethodReceiverEntries(sortedMethodReceivers(out))
}

// collectPrototypeMethods extracts prototype method identities from already
// normalized field-function topology. It stores function identity, not a method
// signature; return precision belongs to the summary/product fixed point reached
// through FunctionRefs.
func collectPrototypeMethods(fields []topology.FieldFunction) []metatable.PrototypeMethod {
	if len(fields) == 0 {
		return nil
	}
	out := make([]metatable.PrototypeMethod, 0, len(fields))
	for _, fn := range fields {
		if fn.ContainerSym == 0 || fn.Field == (fieldkey.Key{}) || fn.FuncRef == (canonref.FuncRef{}) {
			continue
		}
		out = append(out, metatable.PrototypeMethod{
			PrototypeSym: fn.ContainerSym,
			Field:        fn.Field,
			FuncRef:      canonref.ToFlow(fn.FuncRef),
		})
	}
	return compactPrototypeMethodEntries(sortedPrototypeMethods(out))
}

// collectSetMetatableSites extracts setmetatable call sites whose metatable
// argument resolves to a static prototype edge. Transfer later evaluates the
// instance expression at the point and updates PointState.PrototypeSelf.
func collectSetMetatableSites(p Program, metas []metatable.Index) []setMetatableSiteEntry {
	if p.Evidence == nil {
		return nil
	}
	byMeta := make(map[cfg.SymbolID]cfg.SymbolID, len(metas))
	for _, e := range metas {
		byMeta[e.MetatableSym] = e.PrototypeSym
	}
	var out []setMetatableSiteEntry
	for _, r := range p.Refs {
		g := graphOf(p, r)
		if g == nil || g.Bindings() == nil {
			continue
		}
		bindings := g.Bindings()
		for _, call := range p.Evidence(g).Calls {
			info := call.Info
			if info == nil || info.CalleeName != "setmetatable" || len(info.Args) < 2 {
				continue
			}
			mt, proto := setMetatablePrototypeArg(info.Args[1], bindings, byMeta)
			if proto == 0 {
				continue
			}
			out = append(out, setMetatableSiteEntry{
				FuncRef: r,
				Info: metatable.SetMetatableSite{
					Point:        call.Point,
					MetatableSym: mt,
					PrototypeSym: proto,
				},
			})
		}
	}
	return compactSetMetatableSiteEntries(sortedSetMetatableSites(out))
}

func methodHasSelfSlot(g *cfg.Graph) bool {
	if g == nil || g.Bindings() == nil {
		return false
	}
	params := g.ParamSymbols()
	return len(params) > 0 && params[0] != 0 && g.Bindings().Name(params[0]) == "self"
}

func indexFieldSourceSymbol(tbl *ast.TableExpr, bindings *bind.BindingTable) cfg.SymbolID {
	for _, field := range tbl.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		name, ok := fieldkey.StringKeyFromTableField(field)
		if !ok || name != metatableIndexField {
			continue
		}
		return identSymbol(field.Value, bindings)
	}
	return 0
}

func setMetatablePrototypeArg(e ast.Expr, bindings *bind.BindingTable, byMeta map[cfg.SymbolID]cfg.SymbolID) (cfg.SymbolID, cfg.SymbolID) {
	if mt := identSymbol(e, bindings); mt != 0 {
		if proto, ok := byMeta[mt]; ok {
			return mt, proto
		}
	}
	if tbl, ok := e.(*ast.TableExpr); ok {
		return 0, indexFieldSourceSymbol(tbl, bindings)
	}
	return 0, 0
}

func identSymbol(e ast.Expr, bindings *bind.BindingTable) cfg.SymbolID {
	ident, ok := e.(*ast.IdentExpr)
	if !ok || bindings == nil {
		return 0
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok {
		return 0
	}
	return sym
}

func sortedMetatableIndexes(in []metatable.Index) []metatable.Index {
	slices.SortFunc(in, compareMetatableIndexEntry)
	return in
}

func sortedMethodReceivers(in []methodReceiverEntry) []methodReceiverEntry {
	slices.SortFunc(in, compareMethodReceiverEntry)
	return in
}

func sortedPrototypeMethods(in []metatable.PrototypeMethod) []metatable.PrototypeMethod {
	slices.SortFunc(in, comparePrototypeMethodEntry)
	return in
}

func sortedSetMetatableSites(in []setMetatableSiteEntry) []setMetatableSiteEntry {
	slices.SortFunc(in, compareSetMetatableSiteEntry)
	return in
}
