package activation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
)

// HotRule is Call activation's mounted callback. It retains only activation's
// private semantic-key column aligned with the canonical Call body order.
type HotRule struct {
	owner          *callowner.HotOwner
	implementation *callowner.ActivationRuleImplementation
	transport      *engine.MountedActivationCandidateIssuer
	read           engine.Read[engine.OrderedCells[calldomain.Value]]
	routes         []routeKeys
}

// BindHot attaches the one exact Call read, the selector callback, and the
// mounted transport vector to activation's callback-free schema fragment.
// Activation seals its semantic keys directly from Call's owner once; no
// caller supplies a route catalog.
//
// The transport vector is bound here rather than by a second call, so a bound
// rule cannot exist without one: the vector is what a candidate body route
// instantiates, and the engine refuses an admission that declares none.
//
// This package owns the call plane's transport roster: the value, call, heap,
// pack, effect and placement lanes are all carried into a mounted body, and
// the lanes the body publishes are carried back out to its trigger. Value and
// Pack carry the body's result relation; Heap and Placement carry its state
// transitions; Effect carries its effects. The call lane is carried in only: a
// body publishes no call value across the activation edge. Every exported axis
// is one of those imported seeds, which is the symmetry the engine's issuer
// seals.
func BindHot[V, C, H, P, E, L any](fragment *SchemaFragment, owner *callowner.HotOwner, value engine.FactorRef[V], calls engine.FactorRef[C], heap engine.FactorRef[H], pack engine.FactorRef[P], effect engine.FactorRef[E], placement engine.FactorRef[L]) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || !fragment.semantic.Available() ||
		owner == nil || owner.Algebra() == nil || !owner.Algebra().Valid() {
		return nil, false
	}
	routes, routesOK := sealTargetRoutes(owner.Algebra())
	if !routesOK {
		return nil, false
	}
	rule := &HotRule{owner: owner, routes: routes}
	implementation, read, bound := callowner.BindExactActivationRule(owner, fragment.slot, fragment.read, engine.HotActivationSpec{
		Fold: rule.fold,
	})
	if !bound || implementation == nil {
		return nil, false
	}
	rule.implementation, rule.read = implementation, read
	imports := []engine.AnyFactorRef{value.Any(), calls.Any(), heap.Any(), pack.Any(), effect.Any(), placement.Any()}
	exports := []engine.AnyFactorRef{value.Any(), heap.Any(), pack.Any(), effect.Any(), placement.Any()}
	transport, transportOK := callowner.BindMountedActivationCandidateIssuer(implementation, imports, exports)
	if !transportOK || transport == nil {
		return nil, false
	}
	rule.transport = transport
	return rule, true
}

// Implementation returns Call's opaque activation issuer after verifying the
// exact sealed Binding.  No engine slot or callback is exposed.
func (rule *HotRule) Implementation() (*callowner.ActivationRuleImplementation, bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := callowner.ResolveActivationRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

func (rule *HotRule) fold(frame engine.ActivationFrame) engine.ActivationResult {
	if rule == nil || rule.owner == nil || rule.owner.Algebra() == nil {
		return engine.ActivationResult{}
	}
	application, applicationOK := engine.ActivationApplication(frame)
	cells, readOK := engine.ActivationReadValue(frame, rule.read)
	if !applicationOK || !readOK || cells.Count() != 1 {
		return engine.ActivationResult{}
	}
	value, present, available := cells.At(0)
	if !available || !present {
		if available {
			return engine.Activated(frame)
		}
		return engine.ActivationResult{}
	}
	locators := make([]engine.ActivationLocator, 0)
	if !rule.visit(value, func(item routeKeys) bool {
		locator, ok := engine.NewActivationLocator(application, item.target, item.endpoint)
		if ok {
			locators = append(locators, locator)
		}
		return ok
	}) {
		return engine.ActivationResult{}
	}
	return engine.Activated(frame, locators...)
}

func (rule *HotRule) visit(value calldomain.Value, apply func(routeKeys) bool) bool {
	if !rule.routesValid() || apply == nil {
		return false
	}
	if value.IsTop() {
		for index := range rule.routes {
			item, ok := rule.routeAt(index)
			if !ok || !apply(item) {
				return false
			}
		}
		return true
	}
	bodies := rule.owner.Algebra().Bodies()
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, ok := value.KnownTargetAt(index)
		if !ok {
			return false
		}
		body, bodyOK := target.Body()
		if !bodyOK {
			continue
		}
		bodyIndex, indexOK := bodies.Index(body)
		if !indexOK {
			return false
		}
		item, routeOK := rule.routeAt(bodyIndex)
		if !routeOK || !apply(item) {
			return false
		}
	}
	return true
}

func (rule *HotRule) routesValid() bool {
	return rule != nil && rule.owner != nil && rule.owner.Algebra() != nil && rule.owner.Algebra().Valid() && len(rule.routes) == rule.owner.Algebra().Bodies().Count()
}

func (rule *HotRule) routeAt(index int) (routeKeys, bool) {
	if !rule.routesValid() || index < 0 || index >= len(rule.routes) {
		return routeKeys{}, false
	}
	item := rule.routes[index]
	return item, item.target.Available() && item.endpoint.Available()
}
