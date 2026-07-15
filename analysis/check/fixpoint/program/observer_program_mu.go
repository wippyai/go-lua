package program

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// observerBoundaryTuple is one guarded alternative. Its lanes are compared
// and widened atomically; callers must never place the values in independent
// pointwise maps because that would manufacture cross-world combinations.
type observerBoundaryTuple struct {
	values []product.Value
	paths  []observerProgramPathArtifact
}

// observerMuIngressKey names one guarded entry alternative without an
// invocation path. Proof and Worlds are a single edge-local namespace; equal
// DecisionID integers in different proofs are intentionally different keys.
type observerMuIngressKey struct {
	anchor     lexicalObserverTemplateRef
	target     lexicalObserverTemplateRef
	parent     observerProgramInstanceID
	proof      observerProgramProofID
	worlds     evaluated.WorldSet
	point      cfg.Point
	occurrence observation.Occurrence
}

type observerGuardedBoundaryAlternative struct {
	ingress observerMuIngressKey
	tuple   observerBoundaryTuple
}

type observerBoundaryEnvironment struct {
	alternatives map[observerMuIngressKey]observerGuardedBoundaryAlternative
}

// merge keeps incomparable guard/proof ingresses as separate alternatives and
// widens only recurrence of the exact same ingress key. The map value is one
// indivisible correlated tuple, never a set of independently joined lanes.
func (e *observerBoundaryEnvironment) merge(reg *axis.Registry, key observerMuIngressKey, next observerBoundaryTuple) (bool, error) {
	if e == nil || key.anchor == (lexicalObserverTemplateRef{}) || key.target == (lexicalObserverTemplateRef{}) ||
		key.proof == 0 || key.worlds.Root == evaluated.DecisionFalse {
		return false, fmt.Errorf("observer program: incomplete guarded mu ingress")
	}
	if e.alternatives == nil {
		e.alternatives = make(map[observerMuIngressKey]observerGuardedBoundaryAlternative)
	}
	prior, exists := e.alternatives[key]
	if !exists {
		e.alternatives[key] = observerGuardedBoundaryAlternative{ingress: key, tuple: next}
		return true, nil
	}
	widened, changed, err := widenObserverBoundaryTuple(reg, prior.tuple, next, lexicalObserverMuRef{Anchor: key.anchor, Target: key.target})
	if err != nil || !changed {
		return false, err
	}
	e.alternatives[key] = observerGuardedBoundaryAlternative{ingress: key, tuple: widened}
	return true, nil
}

func concreteObserverBoundaryTuple(values []product.Value, paths []pathdom.Path) (observerBoundaryTuple, error) {
	if len(values) != len(paths) {
		return observerBoundaryTuple{}, fmt.Errorf("observer program: boundary tuple width mismatch")
	}
	out := observerBoundaryTuple{values: append([]product.Value(nil), values...), paths: make([]observerProgramPathArtifact, len(paths))}
	for index := range paths {
		out.paths[index] = observerProgramPathArtifact{concrete: paths[index].Clone()}
	}
	return out, nil
}

func observerBoundaryTupleLessOrEq(reg *axis.Registry, left, right observerBoundaryTuple) bool {
	if reg == nil || len(left.values) != len(right.values) || len(left.paths) != len(right.paths) || len(left.values) != len(left.paths) {
		return false
	}
	for index := range left.values {
		if !product.LessOrEq(reg, left.values[index], right.values[index]) || !observerProgramPathEqual(left.paths[index], right.paths[index]) {
			return false
		}
	}
	return true
}

// widenObserverBoundaryTuple returns one upper-bound tuple. Each product lane
// uses the registry's descriptor-owned widening, but the result and its path
// provenance are committed only as one indivisible alternative.
func widenObserverBoundaryTuple(reg *axis.Registry, previous, next observerBoundaryTuple, mu lexicalObserverMuRef) (observerBoundaryTuple, bool, error) {
	if reg == nil || mu.Anchor == (lexicalObserverTemplateRef{}) || mu.Target == (lexicalObserverTemplateRef{}) ||
		len(previous.values) != len(next.values) || len(previous.paths) != len(next.paths) || len(previous.values) != len(previous.paths) {
		return observerBoundaryTuple{}, false, fmt.Errorf("observer program: incomplete recursive boundary tuple")
	}
	if observerBoundaryTupleLessOrEq(reg, next, previous) {
		return previous, false, nil
	}
	out := observerBoundaryTuple{values: make([]product.Value, len(previous.values)), paths: make([]observerProgramPathArtifact, len(previous.paths))}
	changed := false
	for index := range previous.values {
		out.values[index] = product.Widen(reg, previous.values[index], next.values[index])
		changed = changed || !product.Equal(reg, out.values[index], previous.values[index])
		path, pathChanged, err := widenObserverProgramPath(previous.paths[index], next.paths[index], mu)
		if err != nil {
			return observerBoundaryTuple{}, false, err
		}
		out.paths[index] = path
		changed = changed || pathChanged
	}
	return out, changed, nil
}

func widenObserverProgramPath(previous, next observerProgramPathArtifact, mu lexicalObserverMuRef) (observerProgramPathArtifact, bool, error) {
	if observerProgramPathEqual(previous, next) {
		return previous, false, nil
	}
	if previous.mu != nil {
		if previous.mu.mu != mu {
			return observerProgramPathArtifact{}, false, fmt.Errorf("observer program: recursive path crossed mu ownership")
		}
		return previous, false, nil
	}
	if next.mu != nil {
		if next.mu.mu != mu {
			return observerProgramPathArtifact{}, false, fmt.Errorf("observer program: recursive path crossed mu ownership")
		}
		return next, true, nil
	}
	return observerProgramPathArtifact{mu: &observerProgramMuPathArtifact{
		mu: mu, previous: previous.concrete.Key(), next: next.concrete.Key(),
	}}, true, nil
}

func observerProgramPathEqual(left, right observerProgramPathArtifact) bool {
	if left.mu != nil || right.mu != nil {
		return left.mu != nil && right.mu != nil && *left.mu == *right.mu
	}
	return left.concrete.Equal(right.concrete)
}
