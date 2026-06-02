package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/types/typ"
)

// CallArgDemandTarget captures a callee summary projection required for one
// concrete call-obligation fold.
type CallArgDemandTarget struct {
	Graph                *cfg.Graph
	Function             *ast.FunctionExpr
	Contracts            Contracts
	DeclaredSlotType     func(slot int) typ.Type
	SourceParamAnnotated func(sourceParam int) bool
}

// CallArgDemandProjection joins call-argument obligations projected from all
// target summaries at one call site.
type CallArgDemandProjection struct {
	Call    *ast.FuncCallExpr
	Targets []CallArgDemandTarget
}

// DemandTypes projects each target summary to concrete argument obligations and
// pointwise-joins the resulting vectors.
func (p CallArgDemandProjection) DemandTypes() []typ.Type {
	return callobligation.Types(p.Obligations())
}

// Obligations projects each target summary to concrete argument obligations and
// pointwise-joins the resulting vectors while preserving boundary policy.
func (p CallArgDemandProjection) Obligations() []callobligation.Obligation {
	var out []callobligation.Obligation
	for _, target := range p.Targets {
		demands := CallArgContractObligations(CallArgContractConfig{
			Graph:                target.Graph,
			Function:             target.Function,
			Call:                 p.Call,
			Contracts:            target.Contracts,
			DeclaredSlotType:     target.DeclaredSlotType,
			SourceParamAnnotated: target.SourceParamAnnotated,
		})
		out = JoinCallArgObligations(out, demands)
	}
	return callobligation.Normalize(out)
}
