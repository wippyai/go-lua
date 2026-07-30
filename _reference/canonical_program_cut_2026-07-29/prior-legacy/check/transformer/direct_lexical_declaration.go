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
	empty        bool
}

// sealEmptyDirectLexicalDeclarationAuthority proves the unique empty census
// directly from an exact Plan. It cannot admit a function value: any
// ExpressionFunction sidecar makes sealing fail closed.
func sealEmptyDirectLexicalDeclarationAuthority(plan *operationplan.Plan) (DirectLexicalDeclarationAuthority, error) {
	declarations := operationplan.SealEmptyDirectLexicalDeclarations(plan)
	if !declarations.Matches(plan) {
		return DirectLexicalDeclarationAuthority{}, fmt.Errorf("direct lexical declarations: binder authority is required for a non-empty census")
	}
	return DirectLexicalDeclarationAuthority{plan: plan, declarations: declarations, empty: true}, nil
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
		fn, found := bindings.FunctionBySymbol(function)
		origin, originated := bindings.FunctionOrigin(fn)
		if !found || !originated || origin.Parent != owner || origin.Symbol != function {
			valid = false
			invalid = fmt.Sprintf("function=%d found=%v origin=%#v", function, found, origin)
			return false
		}
		closure, ok := closures[function]
		// Only a stable local declaration whose complete runtime use set is
		// direct-callable belongs to this authority. Every other function
		// expression remains an ordinary exact function value in the relation.
		if !ok || origin.Kind != bind.FunctionOriginLocalAssignment || origin.TargetSymbol != closure.TargetSymbol {
			return true
		}
		stable, stableOK := bindings.StableLocalFunctionIdentity(closure.TargetSymbol)
		capturesClosed := found
		if capturesClosed {
			for _, capture := range bindings.DirectCaptures(fn) {
				if capture.Captured != 0 && capture.CapturedName != "" && !bindings.HasWrite(capture.Captured) {
					continue
				}
				captured, certified := closuresByTarget[capture.Captured]
				identity, identityOK := bindings.StableLocalFunctionIdentity(capture.Captured)
				if !certified || !captured.DirectCallSetComplete || !identityOK || identity != captured.FunctionSymbol {
					capturesClosed = false
					break
				}
			}
		}
		if stable != function || !stableOK ||
			!closure.RuntimeUseScanComplete || !closure.DirectCallSetComplete ||
			!capturesClosed {
			return true
		}
		candidate := operationplan.DirectLexicalDeclaration{Expression: ref, Function: function, Target: closure.TargetSymbol}
		// Stable direct use is necessary but not sufficient to erase the value
		// declaration. An annotated/overlaid declaration still owns executable
		// root semantics and therefore remains an ordinary exact function value;
		// direct call composition may consume it without admitting it to the
		// erased-declaration subset.
		if !plan.AdmitsDirectLexicalDeclaration(candidate) {
			return true
		}
		entries = append(entries, candidate)
		return true
	})
	if !valid {
		return DirectLexicalDeclarationAuthority{}, fmt.Errorf("function value has no exact binder origin: %s", invalid)
	}
	declarations := operationplan.SealDirectLexicalDeclarations(plan, entries)
	if !declarations.Matches(plan) {
		return DirectLexicalDeclarationAuthority{}, fmt.Errorf("declaration census does not match plan")
	}
	return DirectLexicalDeclarationAuthority{plan: plan, bindings: bindings, declarations: declarations}, nil
}

func (a DirectLexicalDeclarationAuthority) matches(plan *operationplan.Plan) bool {
	return a.plan == plan && (a.bindings != nil || a.empty) && a.declarations.Matches(plan)
}
