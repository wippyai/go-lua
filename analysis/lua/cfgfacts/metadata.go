package cfgfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Metadata stores Lua sidecar facts keyed by CFG point.
type Metadata struct {
	typeDefinitions map[cfg.Point]TypeDefinitionFact
	functionDefs    map[cfg.Point]FunctionDefinitionFact
	numericFors     map[cfg.Point]NumericForFact
	genericFors     map[cfg.Point]GenericForFact
}

func (m Metadata) TypeDefinition(point cfg.Point) (TypeDefinitionFact, bool) {
	fact, ok := m.typeDefinitions[point]
	return fact, ok
}

func (m *Metadata) SetTypeDefinition(point cfg.Point, fact TypeDefinitionFact) {
	if m.typeDefinitions == nil {
		m.typeDefinitions = make(map[cfg.Point]TypeDefinitionFact)
	}
	m.typeDefinitions[point] = fact
}

func (m Metadata) FunctionDefinition(point cfg.Point) (FunctionDefinitionFact, bool) {
	fact, ok := m.functionDefs[point]
	if !ok {
		return FunctionDefinitionFact{}, false
	}
	return copyFunctionDefinitionFact(fact), true
}

func (m *Metadata) SetFunctionDefinition(point cfg.Point, fact FunctionDefinitionFact) {
	if m.functionDefs == nil {
		m.functionDefs = make(map[cfg.Point]FunctionDefinitionFact)
	}
	m.functionDefs[point] = copyFunctionDefinitionFact(fact)
}

func copyFunctionDefinitionFact(fact FunctionDefinitionFact) FunctionDefinitionFact {
	fact.TargetPath = fact.TargetPath.Clone()
	return fact
}

func (m Metadata) NumericFor(point cfg.Point) (NumericForFact, bool) {
	fact, ok := m.numericFors[point]
	return fact, ok
}

func (m *Metadata) SetNumericFor(point cfg.Point, fact NumericForFact) {
	if m.numericFors == nil {
		m.numericFors = make(map[cfg.Point]NumericForFact)
	}
	m.numericFors[point] = fact
}

func (m Metadata) GenericFor(point cfg.Point) (GenericForFact, bool) {
	fact, ok := m.genericFors[point]
	if !ok {
		return GenericForFact{}, false
	}
	return copyGenericForFact(fact), true
}

func (m *Metadata) SetGenericFor(point cfg.Point, fact GenericForFact) {
	if m.genericFors == nil {
		m.genericFors = make(map[cfg.Point]GenericForFact)
	}
	m.genericFors[point] = copyGenericForFact(fact)
}
