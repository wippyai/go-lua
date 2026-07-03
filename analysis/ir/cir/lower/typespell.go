package lower

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
)

// spellType renders a type expression to a stable syntactic spelling for the cir
// type intern pool. This is a prototype surface: the resolved type identity
// (typ.Type / ShapeID) is attached later by the transfer and codegen layers, not
// stored in lowering. Unsupported forms fall back to a generic marker so the
// stream stays printable.
func spellType(t ast.TypeExpr) string {
	switch t := t.(type) {
	case nil:
		return ""
	case *ast.PrimitiveTypeExpr:
		return t.Name
	case *ast.TypeRefExpr:
		return strings.Join(t.Path, ".")
	case *ast.OptionalTypeExpr:
		return spellType(t.Inner) + "?"
	case *ast.ArrayTypeExpr:
		return "{" + spellType(t.Element) + "}"
	case *ast.MapTypeExpr:
		return "{" + spellType(t.Key) + ": " + spellType(t.Value) + "}"
	case *ast.UnionTypeExpr:
		parts := make([]string, len(t.Types))
		for i, m := range t.Types {
			parts[i] = spellType(m)
		}
		return strings.Join(parts, " | ")
	case *ast.LiteralTypeExpr:
		return spellLiteralType(t.Value)
	default:
		return "<type>"
	}
}

func spellLiteralType(v interface{}) string {
	switch v := v.(type) {
	case string:
		return "\"" + v + "\""
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return "<lit>"
	}
}
