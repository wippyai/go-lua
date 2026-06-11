package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// CallProducerContext identifies the value-list context that produced a call.
type CallProducerContext uint8

const (
	CallProducerContextUnknown CallProducerContext = iota
	CallProducerContextAssignment
	CallProducerContextReturn
)

// CallProducerConfig carries constructor input for CallProducer.
type CallProducerConfig struct {
	Context CallProducerContext

	CalleeSymbol symbol.ID
	CalleePath   path.Path

	ExprRef ExprRef
	HasExpr bool

	ExprIndex int

	ResultTargets []CallResultTarget

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

// CallProducer describes a top-level assignment or return call producer.
type CallProducer struct {
	context CallProducerContext

	calleeSymbol symbol.ID
	calleePath   path.Path

	exprRef ExprRef
	hasExpr bool

	exprIndex int

	resultTargets []CallResultTarget

	final    bool
	expanded bool
	adjusted bool
	openTail bool
}

// NewCallProducer creates a call producer fact.
func NewCallProducer(config CallProducerConfig) CallProducer {
	return CallProducer{
		context:       config.Context,
		calleeSymbol:  config.CalleeSymbol,
		calleePath:    copyPath(config.CalleePath),
		exprRef:       config.ExprRef,
		hasExpr:       config.HasExpr,
		exprIndex:     config.ExprIndex,
		resultTargets: copyCallResultTargets(config.ResultTargets),
		final:         config.Final,
		expanded:      config.Expanded,
		adjusted:      config.Adjusted,
		openTail:      config.OpenTail,
	}
}

// Context returns the producer's value-list context.
func (c CallProducer) Context() CallProducerContext { return c.context }

// CalleeSymbol returns the callee's symbol identity.
func (c CallProducer) CalleeSymbol() symbol.ID { return c.calleeSymbol }

// CalleePath returns the callee's path identity.
func (c CallProducer) CalleePath() path.Path { return copyPath(c.calleePath) }

// Expr returns the producer expression reference, if present.
func (c CallProducer) Expr() (ExprRef, bool) { return c.exprRef, c.hasExpr }

// ExprIndex returns the expression's index in its containing value list.
func (c CallProducer) ExprIndex() int { return c.exprIndex }

// ResultTargets returns the targets that consume this call's results.
func (c CallProducer) ResultTargets() []CallResultTarget {
	return copyCallResultTargets(c.resultTargets)
}

// Final reports whether this producer is the final value-list expression.
func (c CallProducer) Final() bool { return c.final }

// Expanded reports whether this producer contributes multiple result slots.
func (c CallProducer) Expanded() bool { return c.expanded }

// Adjusted reports whether this producer is adjusted to one result.
func (c CallProducer) Adjusted() bool { return c.adjusted }

// OpenTail reports whether this producer is an open tail return.
func (c CallProducer) OpenTail() bool { return c.openTail }

func (c CallProducer) copy() CallProducer {
	c.calleePath = copyPath(c.calleePath)
	c.resultTargets = copyCallResultTargets(c.resultTargets)
	return c
}

func copyCallProducerMap(in map[cfg.Point]CallProducer) map[cfg.Point]CallProducer {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]CallProducer, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}
