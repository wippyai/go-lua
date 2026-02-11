package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// FieldPathWithBaseSymbol resolves an expression to (base symbol, dotted field path)
// for static field access chains like obj.f or obj.f.g.
func FieldPathWithBaseSymbol(bindings *bind.BindingTable, expr ast.Expr) (cfg.SymbolID, string, bool) {
	baseSym, segs, ok := StaticPathWithBaseSymbol(bindings, expr)
	if !ok || baseSym == 0 {
		return 0, "", false
	}
	if len(segs) == 0 {
		return baseSym, "", true
	}
	path := ""
	for _, seg := range segs {
		switch seg.Kind {
		case constraint.SegmentField:
			if seg.Name == "" {
				return 0, "", false
			}
			if path == "" {
				path = seg.Name
			} else {
				path += "." + seg.Name
			}
		default:
			return 0, "", false
		}
	}
	return baseSym, path, true
}
