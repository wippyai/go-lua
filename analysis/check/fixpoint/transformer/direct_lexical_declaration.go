package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// DirectLexicalDeclarationAuthority is an opaque binder-to-compiler proof.
// Its zero value cannot admit function values.
type DirectLexicalDeclarationAuthority struct {
	plan         *operationplan.Plan
	bindings     *bind.Result
	declarations operationplan.DirectLexicalDeclarations
}

// SealDirectLexicalDeclarationAuthority derives authority directly from the
// immutable binder result. Callers cannot manufacture positive escape or
// use-closure fields and cannot transplant the authority to another plan.
func SealDirectLexicalDeclarationAuthority(plan *operationplan.Plan, bindings *bind.Result, owner *ast.FunctionExpr) (DirectLexicalDeclarationAuthority, error) {
	if plan == nil || bindings == nil {
		return DirectLexicalDeclarationAuthority{}, fmt.Errorf("direct lexical declarations: plan and bindings are required")
	}
	closures := make(map[symbol.ID]bind.LocalFunctionUseClosure)
	closuresByTarget := make(map[symbol.ID]bind.LocalFunctionUseClosure)
	for _, closure := range bindings.LocalFunctionUseClosures() {
		if closure.FunctionSymbol != 0 {
			closures[closure.FunctionSymbol] = closure
		}
		if closure.TargetSymbol != 0 {
			closuresByTarget[closure.TargetSymbol] = closure
		}
	}
	entries := make([]operationplan.DirectLexicalDeclaration, 0)
	valid := true
	var invalid string
	plan.Facts().ForEachExpressionFunction(func(ref factflow.ExprRef, function symbol.ID) bool {
		closure, ok := closures[function]
		fn, found := bindings.FunctionBySymbol(function)
		origin, originated := bindings.FunctionOrigin(fn)
		stable, stableOK := bindings.StableLocalFunctionIdentity(closure.TargetSymbol)
		capturesClosed := found
		if capturesClosed {
			for _, capture := range bindings.DirectCaptures(fn) {
				captured, certified := closuresByTarget[capture.Captured]
				identity, identityOK := bindings.StableLocalFunctionIdentity(capture.Captured)
				if !certified || !captured.DirectCallSetComplete || !identityOK || identity != captured.FunctionSymbol {
					capturesClosed = false
					break
				}
			}
		}
		if !ok || !found || !originated || stable != function || !stableOK ||
			!closure.RuntimeUseScanComplete || !closure.DirectCallSetComplete ||
			origin.Parent != owner || origin.Kind != bind.FunctionOriginLocalAssignment || origin.TargetSymbol != closure.TargetSymbol ||
			!capturesClosed {
			valid = false
			invalid = fmt.Sprintf("function=%d target=%d closure=%#v found=%v origin=%#v stable=%d/%v direct-captures=%d", function, closure.TargetSymbol, closure, found, origin, stable, stableOK, len(bindings.DirectCaptures(fn)))
			return false
		}
		entries = append(entries, operationplan.DirectLexicalDeclaration{Expression: ref, Function: function, Target: closure.TargetSymbol})
		return true
	})
	if !valid {
		return DirectLexicalDeclarationAuthority{}, fmt.Errorf("function value is captured, escaping, mutable, or not direct-call complete: %s", invalid)
	}
	declarations := operationplan.SealDirectLexicalDeclarations(plan, entries)
	if !declarations.Matches(plan) {
		return DirectLexicalDeclarationAuthority{}, fmt.Errorf("declaration census does not match plan")
	}
	return DirectLexicalDeclarationAuthority{plan: plan, bindings: bindings, declarations: declarations}, nil
}

func (a DirectLexicalDeclarationAuthority) matches(plan *operationplan.Plan) bool {
	return a.plan == plan && a.bindings != nil && a.declarations.Matches(plan)
}
