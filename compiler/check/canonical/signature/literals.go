package signature

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

// MethodResolver resolves whether a function literal is the body of a method or
// field definition whose callable shape needs an implicit receiver.
type MethodResolver func(*ast.FunctionExpr) *cfg.FuncDefInfo

// LiteralInput is the canonical lowering input for function-literal signatures
// in one CFG.
type LiteralInput struct {
	Graph *cfg.Graph
	Base  *scope.State

	ResolveType     ResolveType
	InferredReturns func(*ast.FunctionExpr) []typ.Type
	MethodFor       MethodResolver
}

// LiteralSignatures lowers every function literal directly nested in Graph to
// its canonical callable signature. The returned map is the external lookup
// shape expected by diagnostics; construction policy lives here so the driver
// does not own a literal-specific graph walk.
func LiteralSignatures(in LiteralInput) map[*ast.FunctionExpr]*typ.Function {
	g := in.Graph
	if g == nil {
		return nil
	}
	enclosing := TypeParamScope(ScopeInput{
		Function:    g.Func(),
		Base:        in.Base,
		ResolveType: in.ResolveType,
	})
	out := make(map[*ast.FunctionExpr]*typ.Function)
	for _, nested := range g.NestedFunctions() {
		fn := nested.Func
		if fn == nil {
			continue
		}
		if in.MethodFor != nil {
			if method := in.MethodFor(fn); method != nil {
				if sig := Build(Input{
					Method:          method,
					Base:            enclosing,
					ResolveType:     in.ResolveType,
					InferredReturns: in.InferredReturns,
					ReturnMode:      ReturnDeclaredThenInferred,
				}); sig != nil {
					out[fn] = sig
					continue
				}
			}
		}
		if sig := Build(Input{
			Function:        fn,
			Base:            enclosing,
			ResolveType:     in.ResolveType,
			InferredReturns: in.InferredReturns,
			ReturnMode:      ReturnDeclaredThenInferred,
		}); sig != nil {
			out[fn] = sig
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
