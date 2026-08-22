// schema_seal_tokens.go declares the Layer-B tokens a sealed binding issues and the fences that validate them.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func validReadDependencies(schema *Schema, rule, read, count uint64) bool {
	var previous uint64
	for index := uint64(0); index < count; index++ {
		dependency, ok := schema.ruleReadDependencyAt(rule, read, index)
		if !ok || dependency >= read || index > 0 && dependency <= previous {
			return false
		}
		previous = dependency
		shape, shapeOK := schema.ruleReadShapeAt(rule, dependency)
		if !shapeOK || (shape.Kind != composition.ReadExact && shape.Kind != composition.ReadSelect && shape.Kind != composition.ReadSummary) {
			return false
		}
		if shape.Kind == composition.ReadSelect && (shape.Semantic != shape.Factor || shape.Normalizer.Available() || shape.DependencyCount == 0 || !validReadDependencies(schema, rule, dependency, shape.DependencyCount)) {
			return false
		}
		if shape.Kind == composition.ReadSummary && (!shape.Semantic.Available() || shape.Normalizer != shape.Semantic || shape.DependencyCount != 0) {
			return false
		}
	}
	return true
}
