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

// NewCallProducerFromView creates a call producer from read-only call-site
// evidence without round-tripping result targets through two defensive copies.
func NewCallProducerFromView(site CallSiteView) CallProducer {
	out := CallProducer{
		calleeSymbol: site.site.calleeSymbol,
		calleePath:   site.site.calleePath.Clone(),
	}
	if len(site.site.resultTargets) != 0 {
		out.resultTargets = make([]CallResultTarget, len(site.site.resultTargets))
		for i := range site.site.resultTargets {
			out.resultTargets[i] = site.site.resultTargets[i].copy()
		}
	}
	return out
}

// CalleeSymbol returns the callee's symbol identity.
func (c CallProducer) CalleeSymbol() symbol.ID { return c.calleeSymbol }

// CalleePath returns the callee's path identity.
func (c CallProducer) CalleePath() path.Path { return c.calleePath.Clone() }

// CalleePathRef returns the callee path without a defensive copy for read-only
// use. The returned path shares the fact's segment storage and must never be
// mutated in place.
func (c CallProducer) CalleePathRef() path.Path { return c.calleePath }

// ResultTargets returns the targets that consume this call's results.
func (c CallProducer) ResultTargets() []CallResultTarget {
	return copyCallResultTargets(c.resultTargets)
}
