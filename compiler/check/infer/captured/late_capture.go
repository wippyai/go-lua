package captured

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/types/typ"
)

// initializedCapturedField finds a field initialized after a closure is made,
// but before module initialization can call or expose that closure. A local
// table cannot be observed by another module until it escapes; while the
// initializer runs, its field writes remain visible to closures over it.
func initializedCapturedField(graph *cfg.Graph, def cfg.Point, table cfg.SymbolID, field string, synth func(ast.Expr, cfg.Point) typ.Type) typ.Type {
	if graph == nil || graph.Bindings() == nil || def == 0 || table == 0 || field == "" || synth == nil {
		return nil
	}
	if kind, ok := graph.Bindings().Kind(table); !ok || kind != cfg.SymbolLocal {
		return nil
	}
	idom, _ := cfganalysis.ComputeDominators(graph.CFG())
	var write cfg.Point
	var value typ.Type
	valid := true
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if !valid || info == nil {
			return
		}
		for i, target := range info.Targets {
			if target.BaseSymbol != table || target.Kind != cfg.TargetField || len(target.FieldPath) != 1 || target.FieldPath[0] != field {
				continue
			}
			// The field must have one non-nil assignment on every path to module
			// completion. Multiple writes need a path-sensitive proof of their
			// final value, so this shortcut does not apply to them.
			if write != 0 || !cfganalysis.StrictlyDominates(idom, def, p) || !cfganalysis.Dominates(idom, p, graph.Exit()) || i >= len(info.Sources) {
				valid = false
				return
			}
			v := synth(info.Sources[i], p)
			if v == nil || typ.IsAny(v) || typ.IsUnknown(v) || v == typ.Nil {
				valid = false
				return
			}
			if _, nilable := typ.SplitNilableFieldType(v); nilable {
				valid = false
				return
			}
			write, value = p, v
		}
	})
	if !valid || write == 0 {
		return nil
	}

	// Calls can invoke a closure indirectly, including through an earlier
	// callback registration. Require every call that may run after definition
	// to occur after the initializer.
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		if !cfganalysis.Dominates(idom, p, def) && !cfganalysis.Dominates(idom, write, p) {
			valid = false
		}
	})
	// Before initialization, allow only inert local function definitions and
	// writes to the captured local table. Other assignments may publish the
	// closure or the table through an alias that a call can observe.
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if !valid || info == nil || cfganalysis.Dominates(idom, p, def) || cfganalysis.Dominates(idom, write, p) {
			return
		}
		for i, target := range info.Targets {
			if target.Kind == cfg.TargetField && target.BaseSymbol == table {
				continue
			}
			if target.Kind == cfg.TargetIdent && info.IsLocal && i < len(info.Sources) {
				if _, ok := info.Sources[i].(*ast.FunctionExpr); ok {
					continue
				}
			}
			valid = false
		}
	})
	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if !valid || info == nil || cfganalysis.Dominates(idom, p, def) || cfganalysis.Dominates(idom, write, p) {
			return
		}
		if len(info.TargetPath.Segments) != 0 {
			valid = false
			return
		}
		kind, ok := graph.Bindings().Kind(info.TargetPath.Symbol)
		if !ok || kind != cfg.SymbolLocal {
			valid = false
		}
	})
	// Replacing the local table would leave the closure with a different
	// object than the one whose field was initialized.
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !cfganalysis.StrictlyDominates(idom, def, p) {
			return
		}
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Symbol == table {
				valid = false
			}
		}
	})
	if !valid {
		return nil
	}
	return value
}
