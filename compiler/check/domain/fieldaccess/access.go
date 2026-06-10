package fieldaccess

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type fieldResolver interface {
	TypeOf(expr ast.Expr, p cfg.Point) typ.Type
	Field(t typ.Type, name string) (typ.Type, bool)
}

type presenceResolver interface {
	FieldAccessHasPresentValue(fullExpr *ast.AttrGetExpr, p cfg.Point) bool
}

// Result is the resolved outcome for a field or index access.
type Result struct {
	Type         typ.Type
	Found        bool
	SkipCheck    bool
	NotIndexable bool
}

// Resolve resolves field or index access on an object type.
func Resolve(
	resolver fieldResolver,
	fullExpr *ast.AttrGetExpr,
	objType typ.Type,
	fieldName string,
	p cfg.Point,
) Result {
	if resolver != nil && fullExpr != nil {
		fullType := resolver.TypeOf(fullExpr, p)
		if fullType != nil && canTrustFullExprType(resolver, objType, fieldName, fullExpr, p, fullType) {
			return Result{Type: fullType, Found: true, SkipCheck: true}
		}
	}

	if objType == nil {
		return Result{SkipCheck: true}
	}

	unwrapped := unwrap.Alias(objType)

	if unwrapped.Kind().IsPlaceholder() {
		return Result{SkipCheck: true}
	}

	if fullExpr != nil {
		if _, isIdent := fullExpr.Key.(*ast.IdentExpr); isIdent {
			return Result{SkipCheck: true}
		}
	}

	if fieldName == "" {
		switch unwrapped.(type) {
		case *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple, *typ.Record:
			return Result{SkipCheck: true}
		case *typ.TypeParam, *typ.Instantiated:
			return Result{SkipCheck: true}
		default:
			return Result{NotIndexable: true}
		}
	}

	switch unwrapped := unwrapped.(type) {
	case *typ.Record, *typ.Interface, *typ.Union, *typ.Intersection, *typ.Optional:
	case *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
	case *typ.TypeParam, *typ.Instantiated:
		return Result{SkipCheck: true}
	case *typ.Function:
		return Result{NotIndexable: true}
	case *typ.Literal:
		if unwrapped.Base == kind.String {
			break
		}
		return Result{NotIndexable: true}
	default:
		if unwrapped.Kind() == kind.Recursive {
			return Result{SkipCheck: true}
		}
		if unwrapped == typ.Nil {
			return Result{NotIndexable: true}
		}
		if unwrapped.Kind() == kind.String {
			break
		}
		return Result{NotIndexable: true}
	}

	if resolver != nil {
		if ft, ok := resolver.Field(objType, fieldName); ok {
			return Result{Type: ft, Found: true}
		}
	}

	return Result{Found: false}
}

func canTrustFullExprType(resolver fieldResolver, objType typ.Type, fieldName string, fullExpr *ast.AttrGetExpr, p cfg.Point, fullType typ.Type) bool {
	if fullType == nil {
		return false
	}
	if fullType.Kind().IsPlaceholder() {
		return hasPresentValue(resolver, fullExpr, p)
	}
	if resolver == nil || objType == nil || fieldName == "" || !querycore.MissingFieldReadsNil(objType) {
		return true
	}
	if _, ok := resolver.Field(objType, fieldName); ok {
		return true
	}
	return hasPresentValue(resolver, fullExpr, p)
}

func hasPresentValue(resolver fieldResolver, fullExpr *ast.AttrGetExpr, p cfg.Point) bool {
	if presence, ok := resolver.(presenceResolver); ok {
		return presence.FieldAccessHasPresentValue(fullExpr, p)
	}
	return false
}
