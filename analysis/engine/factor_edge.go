package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/equation"

// FactorEdge attaches one structural, Factor-local transport to a target
// Point.  The boundary remains the exact SourceAssembly-owned relation; the
// CarryForm is the only authority that may select the Factor plane.  No Rule,
// Group, Member, callback, or raw key enters this edge.
func (value *Assembly) FactorEdge(target AssemblyPoint, boundary SourceBoundary, carry CarryForm) bool {
	if value == nil || value.sourceAssembly == nil || target.assembly != value || !target.Available() || boundary.owner != value.sourceAssembly || !boundary.Available() || carry.composition != value.composition || !knownAssemblyFactor(value.composition, carry.factor) {
		if value != nil {
			failAssembly(value)
		}
		return false
	}
	return admitFactorEdge(value, target.value, boundary.descriptor.input, carry.factor)
}

func admitFactorEdge(value *Assembly, target *assemblyPoint, boundary equation.Input, factor *factorSchema) bool {
	if value == nil || !value.gate.begin() {
		return false
	}
	defer value.gate.end()
	if !validPoint(value, target) || !boundary.Available() || !boundary.Target().Same(target.site) || !assemblyContainsSite(value, boundary.Source()) || !knownAssemblyFactor(value.composition, factor) {
		failAssembly(value)
		return false
	}
	for _, edge := range value.factorEdges {
		if edge != nil && edge.target == target && edge.factor == factor && edge.input.Key() == boundary.Key() {
			failAssembly(value)
			return false
		}
	}
	value.factorEdges = append(value.factorEdges, &assemblyFactorEdge{assembly: value, target: target, input: boundary, factor: factor})
	return true
}
