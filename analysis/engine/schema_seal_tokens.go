// schema_seal_tokens.go declares the Layer-B tokens a sealed binding issues and the fences that validate them.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// ruleRuntimeBinding is an opaque, cell-issued capability. It binds one exact
// hot implementation and output Factor to the already-issued cold Rule proof.
// A state+ordinal pair is deliberately insufficient.
type ruleRuntimeBinding[K ~uint32 | ~uint64, V, O any] struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	cell      *schemaRuleBindingCellImpl[K, V, O]
	proof     *ruleRuntimeProof
	output    factorRuntimeBinding
	issued    bool
}

func (receipt ruleRuntimeBinding[K, V, O]) valid() bool {
	if !receipt.issued || receipt.state == nil || receipt.authority == nil || receipt.cell == nil || receipt.proof == nil || !receipt.proof.valid() || !receipt.output.valid() || receipt.state.phase != schemaBindingSealed || receipt.state.authority != receipt.authority || receipt.state.schema == nil || receipt.cell.state != receipt.state || receipt.cell.schema != receipt.state.schema || receipt.cell.impl == nil || receipt.cell.impl.state != receipt.state || receipt.cell.ordinal != receipt.proof.ordinal || receipt.proof.state != receipt.state || receipt.proof.bindingAuthority != receipt.authority || receipt.output.state != receipt.state || receipt.output.authority != receipt.authority || receipt.output.schema != receipt.state.schema || receipt.proof.output != receipt.output.semantic {
		return false
	}
	if receipt.proof.ordinal >= uint64(len(receipt.state.rules)) || receipt.state.rules[receipt.proof.ordinal] != receipt.cell {
		return false
	}
	return receipt.cell.schemaRuleComplete() && receipt.cell.schemaRuleProofMatches(receipt.proof)
}

type factorFormReceipt struct {
	ordinal  uint64
	kind     SchemaFormKind
	semantic composition.Key
}

// factorRuntimeBinding is the sealed, private Factor implementation proof
// consumed by the carrier binder. The state and authority pointers fence an
// equal-but-foreign SchemaBinding; the scalar rows are copied only as an
// immutable binding, never as a second schema or Factor registry.
type factorRuntimeBinding struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	schema    *Schema
	ordinal   uint64
	semantic  composition.Key
	keyEnd    uint64
	algebra   anyFactorAlgebra
	forms     []factorFormReceipt
	issued    bool
}

func (receipt factorRuntimeBinding) valid() bool {
	if !receipt.issued || receipt.state == nil || receipt.authority == nil || receipt.state.authority != receipt.authority || receipt.state.phase != schemaBindingSealed || receipt.schema == nil || receipt.state.schema != receipt.schema || !receipt.schema.Available() || !receipt.semantic.Available() || receipt.algebra == nil || receipt.algebra.KeyEnd() != receipt.keyEnd {
		return false
	}
	if receipt.ordinal >= receipt.schema.factorCount() || receipt.ordinal >= uint64(len(receipt.state.factors)) || receipt.schema.factorSemanticAt(receipt.ordinal) != receipt.semantic {
		return false
	}
	cell, cellOK := receipt.state.factors[receipt.ordinal].(interface {
		schemaFactorAlgebra() anyFactorAlgebra
		schemaFactorBindingState() *schemaBindingState
	})
	if !cellOK || cell.schemaFactorBindingState() != receipt.state || cell.schemaFactorAlgebra() != receipt.algebra {
		return false
	}
	return true
}

func (receipt factorRuntimeBinding) validForms() bool {
	if !receipt.valid() {
		return false
	}
	formCount, ok := receipt.schema.factorFormCount(receipt.ordinal)
	if !ok || len(receipt.forms) != formCount {
		return false
	}
	for index, form := range receipt.forms {
		shape, shapeOK := receipt.schema.factorFormShapeAt(receipt.ordinal, uint64(index))
		if !shapeOK || form.ordinal != uint64(index) || form.kind == SchemaFormInvalid {
			return false
		}
		want := composition.Key{}
		if summaryReadRowKind(shape.Kind) {
			want = shape.Semantic
		}
		if form.kind != schemaFormKind(shape.Kind) || form.semantic != want {
			return false
		}
	}
	return true
}

func (receipt factorRuntimeBinding) formAt(ordinal uint64, kind SchemaFormKind, semantic composition.Key) (factorFormReceipt, bool) {
	if !receipt.valid() || ordinal >= uint64(len(receipt.forms)) {
		return factorFormReceipt{}, false
	}
	form := receipt.forms[ordinal]
	return form, form.ordinal == ordinal && form.kind == kind && form.semantic == semantic
}

// factorRuntimeDescriptor is the narrow immutable Factor proof consumed by
// the carrier binder. It contains no declaration callback or
// copied cold row; all shape queries go through the exact Schema slot.
type factorRuntimeDescriptor struct {
	schema   *Schema
	binding  *SchemaBinding
	state    *schemaBindingState
	ordinal  uint64
	semantic composition.Key
	keyEnd   uint64
	algebra  anyFactorAlgebra
}

// schemaRuleReceiptFence is the shared owner proof for the isolated
// selected/selector/route Rule vertical. It carries identity only; all shape
// validation re-reads scalar projections from the exact Schema.
type schemaRuleReceiptFence struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	schema    *Schema
	rule      uint64
	cell      schemaRuleBindingCell
}

func (fence schemaRuleReceiptFence) valid() bool {
	return fence.state != nil && fence.authority != nil && fence.state.authority == fence.authority && fence.state.phase == schemaBindingSealed && fence.state.schema == fence.schema && fence.schema != nil && fence.schema.Available() && fence.rule < fence.schema.ruleCount() && fence.rule < uint64(len(fence.state.rules)) && fence.cell != nil && fence.state.rules[fence.rule] == fence.cell && fence.cell.schemaBindingSchema() == fence.schema && fence.cell.schemaRuleOrdinal() == fence.rule
}

// schemaSelectedRead is opaque selected-read geometry evidence. It is
// intentionally not a runtime callback or a copied Rule row.
type schemaSelectedRead struct {
	fence           schemaRuleReceiptFence
	read            uint64
	factor          uint64
	dependencyCount uint64
	issued          bool
}

// schemaRouteWrite is opaque route-write geometry evidence. Route is
// always tied to the one selected-read predecessor named by the Schema row.
type schemaRouteWrite struct {
	fence  schemaRuleReceiptFence
	write  uint64
	read   uint64
	factor uint64
	issued bool
}

func (receipt schemaSelectedRead) Valid() bool {
	if !receipt.issued || !receipt.fence.valid() {
		return false
	}
	rule, ruleOK := receipt.fence.schema.ruleShapeAt(receipt.fence.rule)
	shape, shapeOK := receipt.fence.schema.ruleReadShapeAt(receipt.fence.rule, receipt.read)
	factor, factorOK := receipt.fence.schema.factorOrdinalOf(shape.Factor)
	return ruleOK && shapeOK && factorOK && receipt.read < rule.ReadCount && shape.Kind == composition.ReadSelect && shape.Semantic == shape.Factor && !shape.Normalizer.Available() && shape.DependencyCount != 0 && receipt.factor == factor && receipt.dependencyCount == shape.DependencyCount
}

func (receipt schemaRouteWrite) Valid() bool {
	if !receipt.issued || !receipt.fence.valid() {
		return false
	}
	rule, ruleOK := receipt.fence.schema.ruleShapeAt(receipt.fence.rule)
	shape, shapeOK := receipt.fence.schema.ruleWriteShapeAt(receipt.fence.rule, receipt.write)
	read, readOK := receipt.fence.schema.ruleReadShapeAt(receipt.fence.rule, receipt.read)
	factor, factorOK := receipt.fence.schema.factorOrdinalOf(shape.Factor)
	readFactor, readFactorOK := receipt.fence.schema.factorOrdinalOf(read.Factor)
	return ruleOK && shapeOK && readOK && factorOK && readFactorOK && receipt.write < rule.WriteCount && receipt.read < rule.ReadCount && rule.WriteCount == 1 && shape.Kind == composition.WriteRoute && shape.Route == receipt.read+1 && read.Kind == composition.ReadSelect && read.Semantic == read.Factor && !read.Normalizer.Available() && read.DependencyCount != 0 && receipt.factor == factor && factor == readFactor
}

func issueSchemaSelectedReadReceiptFence(fence schemaRuleReceiptFence, ok bool, read uint64) (schemaSelectedRead, bool) {
	if !ok {
		return schemaSelectedRead{}, false
	}
	shape, shapeOK := fence.schema.ruleReadShapeAt(fence.rule, read)
	if !shapeOK || shape.Kind != composition.ReadSelect || shape.Semantic != shape.Factor || shape.Normalizer.Available() || shape.DependencyCount == 0 {
		return schemaSelectedRead{}, false
	}
	factor, factorOK := fence.schema.factorOrdinalOf(shape.Factor)
	if !factorOK || !validReadDependencies(fence.schema, fence.rule, read, shape.DependencyCount) {
		return schemaSelectedRead{}, false
	}
	return schemaSelectedRead{fence: fence, read: read, factor: factor, dependencyCount: shape.DependencyCount, issued: true}, true
}

func issueSchemaRouteWriteReceiptFence(fence schemaRuleReceiptFence, ok bool, write uint64) (schemaRouteWrite, bool) {
	if !ok {
		return schemaRouteWrite{}, false
	}
	shape, shapeOK := fence.schema.ruleWriteShapeAt(fence.rule, write)
	ruleShape, ruleOK := fence.schema.ruleShapeAt(fence.rule)
	if !shapeOK || !ruleOK || shape.Kind != composition.WriteRoute || shape.Route == 0 || ruleShape.WriteCount != 1 || shape.Route > ruleShape.ReadCount {
		return schemaRouteWrite{}, false
	}
	read := shape.Route - 1
	readShape, readOK := fence.schema.ruleReadShapeAt(fence.rule, read)
	factor, factorOK := fence.schema.factorOrdinalOf(shape.Factor)
	readFactor, readFactorOK := fence.schema.factorOrdinalOf(readShape.Factor)
	if !readOK || !readFactorOK || !factorOK || readShape.Kind != composition.ReadSelect || readShape.Semantic != readShape.Factor || readShape.Normalizer.Available() || factor != readFactor || readShape.DependencyCount == 0 || !validReadDependencies(fence.schema, fence.rule, read, readShape.DependencyCount) {
		return schemaRouteWrite{}, false
	}
	return schemaRouteWrite{fence: fence, write: write, read: read, factor: factor, issued: true}, true
}

func validReadDependencies(schema *Schema, rule, read, count uint64) bool {
	var previous uint64
	for index := uint64(0); index < count; index++ {
		dependency, ok := schema.ruleReadDependencyAt(rule, read, index)
		if !ok || dependency >= read || index > 0 && dependency <= previous {
			return false
		}
		previous = dependency
		shape, shapeOK := schema.ruleReadShapeAt(rule, dependency)
		if !shapeOK || (shape.Kind != composition.ReadExact && shape.Kind != composition.ReadSelect) {
			return false
		}
		if shape.Kind == composition.ReadSelect && (shape.Semantic != shape.Factor || shape.Normalizer.Available() || shape.DependencyCount == 0 || !validReadDependencies(schema, rule, dependency, shape.DependencyCount)) {
			return false
		}
	}
	return true
}

func (descriptor factorRuntimeDescriptor) valid() bool {
	return descriptor.schema != nil && descriptor.schema.Available() && descriptor.semantic.Available() && descriptor.algebra != nil && descriptor.algebra.KeyEnd() == descriptor.keyEnd && (descriptor.state == nil || descriptor.state.schema == descriptor.schema && descriptor.state.phase == schemaBindingSealed && descriptor.state.authority != nil)
}
