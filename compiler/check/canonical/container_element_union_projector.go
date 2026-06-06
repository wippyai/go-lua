package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

// ContainerElementUnionsFromValues projects contract-declared container element
// union effects for a product call. The call package owns spec extraction; this
// adapter supplies module-local summary signatures and the product-context type
// resolver without making driver.go wire that policy inline.
func (ct callTyper) ContainerElementUnionsFromValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) []effect.ContainerElementUnion {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return nil
	}
	return (canonicalcall.ContainerElementUnionProjection{
		Call: call,
		SummarySignature: func(call *ast.FuncCallExpr) typ.Type {
			if ref, ok := ct.resolveCalleeRef(call, d.activeProgram); ok {
				return d.signatureForRef(d.activeProgram, ref)
			}
			return nil
		},
		Resolver: ct.callTypeResolver(ctx.ExprType),
	}).Effects()
}
