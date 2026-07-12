package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// AllocationTemplateTerm is one correlated symbolic allocation transaction.
// Result ValueTerms and the heap/fresh EffectTerm reference this same node.
type AllocationTemplateTerm uint32

type allocationTemplateNode struct {
	op operationplan.SignatureAllocationOperation
}

func (a *Arena) AllocationTemplate(op operationplan.SignatureAllocationOperation) AllocationTemplateTerm {
	if a == nil || op.Site().Owner == 0 || op.Site().Ordinal == 0 || op.Site().Template == "" {
		return 0
	}
	key := fmt.Sprintf("%d:%s:%d", op.Site().Owner, op.Site().Template, op.Site().Ordinal)
	for _, term := range a.allocationKeys[key] {
		if allocationOperationEqual(a.allocations[term].op, op) {
			return term
		}
	}
	term := AllocationTemplateTerm(len(a.allocations))
	a.allocations = append(a.allocations, allocationTemplateNode{op: op})
	a.allocationKeys[key] = append(a.allocationKeys[key], term)
	return term
}

func (a *Arena) AllocationResultValue(allocation AllocationTemplateTerm, resultIndex int) ValueTerm {
	if !a.validAllocation(allocation) || resultIndex < 0 || resultIndex != a.allocations[allocation].op.Template().ReturnIndex {
		return 0
	}
	return a.internValue(valueNode{op: valueAllocationResult, allocation: allocation, resultIndex: resultIndex})
}

func (a *Arena) validAllocation(term AllocationTemplateTerm) bool {
	return a != nil && term != 0 && int(term) < len(a.allocations)
}

func (a *Arena) allocationResult(term AllocationTemplateTerm, resultIndex int) (product.Value, bool) {
	if a == nil || a.reg == nil || !a.validAllocation(term) {
		return product.Value{}, false
	}
	op := a.allocations[term].op
	template := op.Template()
	if resultIndex != template.ReturnIndex {
		return product.Value{}, false
	}
	materialized, ok := effectlowering.MaterializeStaticAllocation(a.reg, nil, keyspace.New(), cfg.Point(op.Site().Ordinal), template)
	return materialized.Result, ok
}

func allocationOperationEqual(a, b operationplan.SignatureAllocationOperation) bool {
	if a.Site() != b.Site() {
		return false
	}
	return (signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{a.Template()}}).Equals(
		signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{b.Template()}},
	)
}
