package check

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// funcResultDependencyEqual compares the part of a function result that is
// observable to dependent queries. The freshly computed FuncResult is returned
// to the current caller even when this returns true; equality only decides
// whether Salsa-style dependents need a new UpdatedAt revision. Transient
// phase products such as Scope maps, FlowInputs, and FlowSolution are
// intentionally excluded because they contain local abstract-interpreter state,
// including recursive receiver products. Scope identity is already in the
// FuncKey parent hash; effects that matter to callers are exposed through
// canonical signatures, literal signatures, and refinements below.
func funcResultDependencyEqual(a, b *api.FuncResult) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Graph == b.Graph &&
		a.ModuleBindings == b.ModuleBindings &&
		analysisContextEqual(a.AnalysisContext, b.AnalysisContext) &&
		refinementEqual(a.FnRefinement, b.FnRefinement) &&
		functionTypeEqual(a.SourceSignature, b.SourceSignature) &&
		functionTypeEqual(a.PublicSeedSignature, b.PublicSeedSignature) &&
		literalSignaturesEqual(a.LiteralSignatures, b.LiteralSignatures) &&
		a.DepthLimitExceeded == b.DepthLimitExceeded
}

func analysisContextEqual(a, b api.AnalysisContext) bool {
	if len(a.GlobalOverlay) != len(b.GlobalOverlay) {
		return false
	}
	for name, av := range a.GlobalOverlay {
		if !product.Equal(av, b.GlobalOverlay[name]) {
			return false
		}
	}
	return functionTypeEqual(a.ExpectedFunction, b.ExpectedFunction)
}

func refinementEqual(a, b any) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if eq, ok := a.(interface{ Equals(any) bool }); ok {
		return eq.Equals(b)
	}
	return false
}

func literalSignaturesEqual(a, b map[*ast.FunctionExpr]*typ.Function) bool {
	if len(a) != len(b) {
		return false
	}
	for fn, av := range a {
		if !functionTypeEqual(av, b[fn]) {
			return false
		}
	}
	return true
}

func functionTypeEqual(a, b *typ.Function) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return value.FactTypeEqual(a, b)
}
