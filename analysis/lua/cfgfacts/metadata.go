package cfgfacts

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Metadata stores Lua sidecar facts keyed by CFG point.
type Metadata struct {
	assignments     map[cfg.Point]AssignmentFact
	loops           map[cfg.Point]LoopFact
	typeDefinitions map[cfg.Point]TypeDefinitionFact
	functionDefs    map[cfg.Point]FunctionDefinitionFact
	numericFors     map[cfg.Point]NumericForFact
	genericFors     map[cfg.Point]GenericForFact
	labels          map[cfg.Point]LabelFact
	gotos           map[cfg.Point]GotoFact
	shortCircuits   map[cfg.Point]ShortCircuitGuardFact
	exprEvaluations map[cfg.Point]ExpressionEvaluationFact
}

func (m Metadata) Assignment(point cfg.Point) (AssignmentFact, bool) {
	fact, ok := m.assignments[point]
	return fact, ok
}

func (m *Metadata) SetAssignment(point cfg.Point, fact AssignmentFact) {
	if m.assignments == nil {
		m.assignments = make(map[cfg.Point]AssignmentFact)
	}
	m.assignments[point] = fact
}

func (m Metadata) Loop(point cfg.Point) (LoopFact, bool) {
	fact, ok := m.loops[point]
	if !ok {
		return LoopFact{}, false
	}
	return copyLoopFact(fact), true
}

func (m *Metadata) SetLoop(point cfg.Point, fact LoopFact) {
	if m.loops == nil {
		m.loops = make(map[cfg.Point]LoopFact)
	}
	m.loops[point] = copyLoopFact(fact)
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

func (m Metadata) Label(point cfg.Point) (LabelFact, bool) {
	fact, ok := m.labels[point]
	return fact, ok
}

func (m *Metadata) SetLabel(point cfg.Point, fact LabelFact) {
	if m.labels == nil {
		m.labels = make(map[cfg.Point]LabelFact)
	}
	m.labels[point] = fact
}

func (m Metadata) ShortCircuitGuard(point cfg.Point) (ShortCircuitGuardFact, bool) {
	fact, ok := m.shortCircuits[point]
	return fact, ok
}

func (m *Metadata) SetShortCircuitGuard(point cfg.Point, fact ShortCircuitGuardFact) {
	if m.shortCircuits == nil {
		m.shortCircuits = make(map[cfg.Point]ShortCircuitGuardFact)
	}
	m.shortCircuits[point] = fact
}

func (m Metadata) ExpressionEvaluation(point cfg.Point) (ExpressionEvaluationFact, bool) {
	fact, ok := m.exprEvaluations[point]
	return fact, ok
}

func (m *Metadata) SetExpressionEvaluation(point cfg.Point, fact ExpressionEvaluationFact) {
	if m.exprEvaluations == nil {
		m.exprEvaluations = make(map[cfg.Point]ExpressionEvaluationFact)
	}
	m.exprEvaluations[point] = fact
}

// ShortCircuitGuardPoints returns the points carrying short-circuit guard facts
// in ascending order for deterministic extraction.
func (m Metadata) ShortCircuitGuardPoints() []cfg.Point {
	if len(m.shortCircuits) == 0 {
		return nil
	}
	points := make([]cfg.Point, 0, len(m.shortCircuits))
	for point := range m.shortCircuits {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	return points
}

func (m Metadata) Goto(point cfg.Point) (GotoFact, bool) {
	fact, ok := m.gotos[point]
	return fact, ok
}

func (m *Metadata) SetGoto(point cfg.Point, fact GotoFact) {
	if m.gotos == nil {
		m.gotos = make(map[cfg.Point]GotoFact)
	}
	m.gotos[point] = fact
}
