package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// ProductDomain preserves the previous exported name for callers while the
// condition/narrowing proof domain vocabulary migrates to ConditionProofDomain.
type ProductDomain = ConditionProofDomain

// NewProductDomain preserves the previous constructor name. New internal code
// should call NewConditionProofDomain.
func NewProductDomain(env constraint.Env) *ConditionProofDomain {
	return NewConditionProofDomain(env)
}

// ProductDomainHasNarrowingForSymbol preserves the previous helper name. New
// internal code should call ConditionProofDomainHasNarrowingForSymbol.
func ProductDomainHasNarrowingForSymbol(domain *ConditionProofDomain, sym cfg.SymbolID) bool {
	return ConditionProofDomainHasNarrowingForSymbol(domain, sym)
}
