// Package callproducer owns the engine projection from call-site evidence to
// call-result producer facts.
package callproducer

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// FromSite projects rich call-site evidence to the narrow producer DTO used by
// call-result lookup and signature identity reads.
func FromSite(site factflow.CallSite) factflow.CallProducer {
	return factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol:  site.CalleeSymbol(),
		CalleePath:    site.CalleePath(),
		ResultTargets: site.ResultTargets(),
	})
}

// FromFacts returns the strict call-result producer projection for point's
// canonical call-site evidence.
func FromFacts(facts factflow.Facts, point cfg.Point) (factflow.CallProducer, bool) {
	site, ok := facts.CallSite(point)
	if !ok || !eligible(site) {
		return factflow.CallProducer{}, false
	}
	return factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol:  site.CalleeSymbol(),
		CalleePath:    site.CalleePath(),
		ResultTargets: strictResultTargets(site.ResultTargets()),
	}), true
}

// Has reports whether point has producer-eligible call-site evidence.
func Has(facts factflow.Facts, point cfg.Point) bool {
	site, ok := facts.CallSite(point)
	return ok && eligible(site)
}

func eligible(site factflow.CallSite) bool {
	switch site.Context() {
	case factflow.CallSiteContextAssignmentSource, factflow.CallSiteContextReturnSource, factflow.CallSiteContextExpressionProducer:
		return true
	default:
		return false
	}
}

func strictResultTargets(targets []factflow.CallResultTarget) []factflow.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, 0, len(targets))
	for _, target := range targets {
		if !strictResultTarget(target) {
			continue
		}
		out = append(out, target)
	}
	return out
}

func strictResultTarget(target factflow.CallResultTarget) bool {
	switch target.Kind() {
	case factflow.CallResultTargetLocalAssignment:
		return target.TargetSymbol() != 0
	case factflow.CallResultTargetOrdinaryAssignment:
		return target.TargetSymbol() != 0 && len(target.TargetPath().Segments) == 0
	case factflow.CallResultTargetReturn:
		return true
	case factflow.CallResultTargetExpression:
		return target.ResultIndex() >= 0
	default:
		return false
	}
}
