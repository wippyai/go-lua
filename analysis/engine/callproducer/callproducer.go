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

// FromView projects read-only call-site evidence to the narrow producer DTO.
func FromView(site factflow.CallSiteView) factflow.CallProducer {
	targets := make([]factflow.CallResultTarget, 0, site.ResultTargetCount())
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		targets = append(targets, target.CallResultTarget())
		return true
	})
	return factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol:  site.CalleeSymbol(),
		CalleePath:    site.CalleePath(),
		ResultTargets: targets,
	})
}

// FromFacts returns the strict call-result producer projection for point's
// canonical call-site evidence.
func FromFacts(facts factflow.Facts, point cfg.Point) (factflow.CallProducer, bool) {
	site, ok := facts.CallSiteView(point)
	if !ok || !eligibleView(site) {
		return factflow.CallProducer{}, false
	}
	return factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol:  site.CalleeSymbol(),
		CalleePath:    site.CalleePath(),
		ResultTargets: strictResultTargetsView(site),
	}), true
}

// Has reports whether point has producer-eligible call-site evidence.
func Has(facts factflow.Facts, point cfg.Point) bool {
	site, ok := facts.CallSiteView(point)
	return ok && eligibleView(site)
}

func eligible(site factflow.CallSite) bool {
	switch site.Context() {
	case factflow.CallSiteContextAssignmentSource, factflow.CallSiteContextReturnSource, factflow.CallSiteContextExpressionProducer:
		return true
	default:
		return false
	}
}

func eligibleView(site factflow.CallSiteView) bool {
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

func strictResultTargetsView(site factflow.CallSiteView) []factflow.CallResultTarget {
	if site.ResultTargetCount() == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, 0, site.ResultTargetCount())
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if strictResultTargetView(target) {
			out = append(out, target.CallResultTarget())
		}
		return true
	})
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

func strictResultTargetView(target factflow.CallResultTargetView) bool {
	switch target.Kind() {
	case factflow.CallResultTargetLocalAssignment:
		return target.TargetSymbol() != 0
	case factflow.CallResultTargetOrdinaryAssignment:
		return target.TargetSymbol() != 0 && target.TargetPathSegmentCount() == 0
	case factflow.CallResultTargetReturn:
		return true
	case factflow.CallResultTargetExpression:
		return target.ResultIndex() >= 0
	default:
		return false
	}
}
