package readmodel

import (
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// nilabilityProvenance projects already-solved source facts for a nilable use.
// It does not infer new facts: the body owns optional receiver evidence and the
// call-contract readmodel owns result-slot provenance.
func (r Reader) nilabilityProvenance(point cfg.Point, expr ast.Expr, value product.Value) readapi.NilabilityProvenance {
	if r.result == nil {
		return readapi.NilabilityProvenance{}
	}
	provenance := readapi.NilabilityProvenance{
		NilableAccesses:    assignmentNilableAccessEvidenceFromBody(r.result.AssignmentNilableAccessEvidence(point, expr)),
		ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
		UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
	}
	if attr, ok := expr.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyDot {
		provenance.OptionalField = true
	}
	if _, ok := expr.(*ast.FuncCallExpr); ok {
		provenance.CallResult = r.nilabilityCallResult(point)
	}
	return provenance
}

func (r Reader) nilabilityCallResult(point cfg.Point) readapi.CallResultAssignmentSource {
	if r.result == nil {
		return readapi.CallResultAssignmentSource{}
	}
	site, ok := r.result.CallSiteView(point)
	if !ok {
		return readapi.CallResultAssignmentSource{}
	}
	contract, ok := r.callContractAt(point)
	if !ok {
		return readapi.CallResultAssignmentSource{}
	}
	name := contract.Source.Name
	if name == "" {
		name = r.callContractSourceName(site)
	}
	_, produced := contract.Contract.ResultAt(0)
	if !produced {
		return readapi.CallResultAssignmentSource{
			Present:       true,
			CallableName:  name,
			ResultIndex:   0,
			UnderSupplied: true,
		}
	}
	return readapi.CallResultAssignmentSource{
		Present:      true,
		CallableName: name,
		ResultIndex:  0,
		ReturnSpan:   contract.Source.ResultSpan(0),
	}
}

func nilabilityProvenanceForCallArgument(r Reader, point cfg.Point, index int, value product.Value) readapi.NilabilityProvenance {
	if r.result == nil {
		return readapi.NilabilityProvenance{}
	}
	call, ok := r.result.SourceCall(point)
	if !ok || index < 0 || index >= len(call.Args) {
		return readapi.NilabilityProvenance{
			ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
			UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
		}
	}
	return r.nilabilityProvenance(point, call.Args[index], value)
}

func nilabilityProvenanceForCallee(r Reader, point cfg.Point, value product.Value) readapi.NilabilityProvenance {
	if r.result == nil {
		return readapi.NilabilityProvenance{}
	}
	call, ok := r.result.SourceCall(point)
	if !ok {
		return readapi.NilabilityProvenance{
			ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
			UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
		}
	}
	expr := call.Func
	if call.Receiver != nil {
		expr = call.Receiver
	}
	return r.nilabilityProvenance(point, expr, value)
}
