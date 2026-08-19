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
	if !receipt.issued || receipt.state == nil || receipt.authority == nil || receipt.cell == nil || receipt.proof == nil || !receipt.proof.valid() || !receipt.output.valid() || receipt.state.phase != schemaBindingSealed || receipt.state.authority != receipt.authority || receipt.state.schema == nil || receipt.cell.state != receipt.state || receipt.cell.schema != receipt.state.schema || receipt.cell.impl == nil || receipt.cell.impl.state != receipt.state || receipt.cell.ordinal != receipt.proof.ordinal || receipt.proof.state != receipt.state || receipt.proof.bindingAuthority != receipt.authority || receipt.output.state != receipt.state || receipt.output.authority != receipt.authority || receipt.proof.output != receipt.output.semanticKey() {
		return false
	}
	if receipt.proof.ordinal >= uint64(len(receipt.state.rules)) || receipt.state.rules[receipt.proof.ordinal] != receipt.cell {
		return false
	}
	return receipt.cell.schemaRuleComplete() && receipt.cell.schemaRuleProofMatches(receipt.proof)
}

// factorRuntimeBinding is the sealed, private Factor implementation proof
// consumed by the carrier binder. State and authority fence an
// equal-but-foreign SchemaBinding and generation; ordinal plus algebra name
// the exact typed Factor cell. All scalar geometry is reread from Schema.
type factorRuntimeBinding struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	ordinal   uint64
	algebra   anyFactorAlgebra
}

func (receipt factorRuntimeBinding) valid() bool {
	if receipt.state == nil || receipt.authority == nil || receipt.state.authority != receipt.authority || receipt.state.phase != schemaBindingSealed || receipt.state.schema == nil || !receipt.state.schema.Available() || receipt.algebra == nil {
		return false
	}
	if receipt.ordinal >= receipt.state.schema.factorCount() || receipt.ordinal >= uint64(len(receipt.state.factors)) || !receipt.state.schema.factorSemanticAt(receipt.ordinal).Available() {
		return false
	}
	cell, cellOK := receipt.state.factors[receipt.ordinal].(interface {
		schemaFactorAlgebra() anyFactorAlgebra
		schemaFactorBindingState() *schemaBindingState
		schemaFactorSchema() *Schema
	})
	if !cellOK || cell.schemaFactorBindingState() != receipt.state || cell.schemaFactorSchema() != receipt.state.schema || cell.schemaFactorAlgebra() != receipt.algebra {
		return false
	}
	return true
}

// factorAddressMatches compares two already-sealed direct Factor rows. The
// state/authority/ordinal tuple is the address; no receipt reconstruction or
// second proof is needed at a hot route/read boundary.
func factorAddressMatches(left, right factorRuntimeBinding) bool {
	return left.valid() && right.valid() && left.state == right.state && left.authority == right.authority && left.ordinal == right.ordinal && left.algebra == right.algebra
}

func (receipt factorRuntimeBinding) semanticKey() composition.Key {
	if !receipt.valid() {
		return composition.Key{}
	}
	return receipt.state.schema.factorSemanticAt(receipt.ordinal)
}

func (receipt factorRuntimeBinding) keyLimit() uint64 {
	if !receipt.valid() {
		return 0
	}
	return receipt.algebra.KeyEnd()
}

func (receipt factorRuntimeBinding) validForms() bool {
	if !receipt.valid() {
		return false
	}
	formCount, ok := receipt.state.schema.factorFormCount(receipt.ordinal)
	if !ok {
		return false
	}
	for index := 0; index < formCount; index++ {
		shape, shapeOK := receipt.state.schema.factorFormShapeAt(receipt.ordinal, uint64(index))
		if !shapeOK || schemaFormKind(shape.Kind) == SchemaFormInvalid {
			return false
		}
		if summaryReadRowKind(shape.Kind) != shape.Semantic.Available() {
			return false
		}
	}
	return true
}

func (receipt factorRuntimeBinding) formAt(ordinal uint64) (SchemaFormKind, composition.Key, bool) {
	if !receipt.valid() {
		return SchemaFormInvalid, composition.Key{}, false
	}
	shape, ok := receipt.state.schema.factorFormShapeAt(receipt.ordinal, ordinal)
	if !ok {
		return SchemaFormInvalid, composition.Key{}, false
	}
	kind := schemaFormKind(shape.Kind)
	if kind == SchemaFormInvalid || summaryReadRowKind(shape.Kind) != shape.Semantic.Available() {
		return SchemaFormInvalid, composition.Key{}, false
	}
	return kind, shape.Semantic, true
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
