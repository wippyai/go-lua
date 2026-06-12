package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// CallProducerConfig carries constructor input for CallProducer.
type CallProducerConfig struct {
	CalleeSymbol symbol.ID
	CalleePath   path.Path

	ResultTargets []CallResultTarget
}

// CallProducer describes a top-level assignment or return call producer.
type CallProducer struct {
	calleeSymbol symbol.ID
	calleePath   path.Path

	resultTargets []CallResultTarget
}

// NewCallProducer creates a call producer fact.
func NewCallProducer(config CallProducerConfig) CallProducer {
	return CallProducer{
		calleeSymbol:  config.CalleeSymbol,
		calleePath:    copyPath(config.CalleePath),
		resultTargets: copyCallResultTargets(config.ResultTargets),
	}
}

// CalleeSymbol returns the callee's symbol identity.
func (c CallProducer) CalleeSymbol() symbol.ID { return c.calleeSymbol }

// CalleePath returns the callee's path identity.
func (c CallProducer) CalleePath() path.Path { return copyPath(c.calleePath) }

// ResultTargets returns the targets that consume this call's results.
func (c CallProducer) ResultTargets() []CallResultTarget {
	return copyCallResultTargets(c.resultTargets)
}

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
