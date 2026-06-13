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
		calleePath:    copyPath(config.CalleePath),
		resultTargets: copyCallResultTargets(config.ResultTargets),
	}
}

// CallProducerFromSite projects rich call-site evidence to the narrow producer
// fact used by call-result lookup.
func CallProducerFromSite(site CallSite) CallProducer {
	return NewCallProducer(CallProducerConfig{
		CalleeSymbol:  site.CalleeSymbol(),
		CalleePath:    site.CalleePath(),
		ResultTargets: site.ResultTargets(),
	})
}

func callProducerFromFactSite(site CallSite) (CallProducer, bool) {
	switch site.Context() {
	case CallSiteContextAssignmentSource, CallSiteContextReturnSource, CallSiteContextExpressionProducer:
	default:
		return CallProducer{}, false
	}
	return NewCallProducer(CallProducerConfig{
		CalleeSymbol:  site.CalleeSymbol(),
		CalleePath:    site.CalleePath(),
		ResultTargets: strictProducerResultTargets(site.ResultTargets()),
	}), true
}

func strictProducerResultTargets(targets []CallResultTarget) []CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]CallResultTarget, 0, len(targets))
	for _, target := range targets {
		if !strictProducerResultTarget(target) {
			continue
		}
		out = append(out, target.copy())
	}
	return out
}

func strictProducerResultTarget(target CallResultTarget) bool {
	switch target.Kind() {
	case CallResultTargetLocalAssignment:
		return target.TargetSymbol() != 0
	case CallResultTargetOrdinaryAssignment:
		return target.TargetSymbol() != 0 && len(target.TargetPath().Segments) == 0
	case CallResultTargetReturn:
		return true
	case CallResultTargetExpression:
		return target.ResultIndex() >= 0
	default:
		return false
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
