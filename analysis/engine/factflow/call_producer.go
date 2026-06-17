package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// CallProducerConfig carries constructor input for CallProducer.
type CallProducerConfig struct {
	CalleeSymbol symbol.ID
	CalleePath   path.Path

	ResultTargets []CallResultTarget
}

// CallProducer describes a call whose return slots can be read by value-source
// consumers.
type CallProducer struct {
	calleeSymbol symbol.ID
	calleePath   path.Path

	resultTargets []CallResultTarget
}

// NewCallProducer creates a call producer fact.
func NewCallProducer(config CallProducerConfig) CallProducer {
	return CallProducer{
		calleeSymbol:  config.CalleeSymbol,
		calleePath:    config.CalleePath.Clone(),
		resultTargets: copyCallResultTargets(config.ResultTargets),
	}
}

// CalleeSymbol returns the callee's symbol identity.
func (c CallProducer) CalleeSymbol() symbol.ID { return c.calleeSymbol }

// CalleePath returns the callee's path identity.
func (c CallProducer) CalleePath() path.Path { return c.calleePath.Clone() }

// ResultTargets returns the targets that consume this call's results.
func (c CallProducer) ResultTargets() []CallResultTarget {
	return copyCallResultTargets(c.resultTargets)
}

func (c CallProducer) copy() CallProducer {
	c.calleePath = c.calleePath.Clone()
	c.resultTargets = copyCallResultTargets(c.resultTargets)
	return c
}
