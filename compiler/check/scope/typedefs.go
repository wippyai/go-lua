package scope

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// TypeDefResolver resolves a type definition to its concrete type.
//
// This callback is invoked during scope construction when a TypeDef CFG node
// is encountered. The resolver translates the AST type expression into a
// concrete typ.Type, using the current scope state to resolve type references.
//
// Parameters:
//   - name: The type alias name being defined
//   - typeExpr: The AST type expression (right-hand side of the definition)
//   - typeParams: Generic type parameters for the definition (may be empty)
//   - sc: Current scope state for resolving type references
//
// The resolver should return a typ.Generic for parameterized types, or the
// resolved type directly for non-generic definitions.
type TypeDefResolver func(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *State) typ.Type

// EnrichWithTypeDefs walks TypeDef nodes in RPO order and returns a scope
// that includes all type definitions from the graph.
//
// Common use case: Building the scope for a module's top-level, where all
// type definitions should be visible to subsequent analysis.
func EnrichWithTypeDefs(
	graph *cfg.Graph,
	base *State,
	resolver TypeDefResolver,
) *State {
	current := base

	for _, p := range graph.RPO() {
		current = applyTypeDefAtPoint(graph, p, current, resolver)
	}

	return current
}

func applyTypeDefAtPoint(graph *cfg.Graph, p cfg.Point, current *State, resolver TypeDefResolver) *State {
	if graph == nil || resolver == nil {
		return current
	}
	info := graph.TypeDef(p)
	if info == nil || info.Name == "" || info.TypeExpr == nil {
		return current
	}
	resolved := resolver(info.Name, info.TypeExpr, ToTypeParamExprs(info.TypeParams), current)
	if resolved == nil {
		return current
	}
	if _, isGeneric := resolved.(*typ.Generic); isGeneric {
		return current.WithType(info.Name, resolved)
	}
	return current.WithType(info.Name, typ.NewAlias(info.Name, resolved))
}

// ToTypeParamExprs converts cfg type params to ast type param expressions.
//
// CFG stores type parameters in a simplified form (TypeParamInfo) that captures
// name and constraint. The AST representation (TypeParamExpr) is needed for
// type resolution, which operates on AST nodes. This function converts between
// the two representations.
func ToTypeParamExprs(params []cfg.TypeParamInfo) []ast.TypeParamExpr {
	if len(params) == 0 {
		return nil
	}
	out := make([]ast.TypeParamExpr, len(params))
	for i, p := range params {
		out[i] = ast.TypeParamExpr{Name: p.Name, Constraint: p.Constraint}
	}
	return out
}
