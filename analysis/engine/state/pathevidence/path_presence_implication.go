package pathevidence

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

// PathPresenceImplication records a must implication between two path
// facts: when Trigger has either TriggerPresence or TriggerValue, Target has
// TargetPresence or TargetValue.
type PathPresenceImplication struct {
	Trigger             keyspace.Key
	TriggerOther        keyspace.Key
	TriggerPresence     presence.Value
	TriggerValue        product.Value
	HasTriggerValue     bool
	HasTriggerPresence  bool
	HasTriggerPathEqual bool
	Target              keyspace.Key
	TargetPresence      presence.Value
	TargetValue         product.Value
	HasTargetValue      bool
}

// NewPathPresenceImplication creates a presence-triggered implication.
func NewPathPresenceImplication(
	trigger keyspace.Key,
	triggerPresence presence.Value,
	target keyspace.Key,
	targetPresence presence.Value,
) PathPresenceImplication {
	return PathPresenceImplication{
		Trigger:         trigger,
		TriggerPresence: triggerPresence,
		Target:          target,
		TargetPresence:  targetPresence,
	}
}

// NewPathValuePresenceImplication creates a value-triggered implication.
func NewPathValuePresenceImplication(
	trigger keyspace.Key,
	triggerValue product.Value,
	target keyspace.Key,
	targetPresence presence.Value,
) PathPresenceImplication {
	return PathPresenceImplication{
		Trigger:         trigger,
		TriggerValue:    triggerValue,
		HasTriggerValue: true,
		Target:          target,
		TargetPresence:  targetPresence,
	}
}

// NewPathValueRefinementImplication creates a value-triggered implication that
// refines the target to a stored value when activated.
func NewPathValueRefinementImplication(
	trigger keyspace.Key,
	triggerValue product.Value,
	target keyspace.Key,
	targetValue product.Value,
) PathPresenceImplication {
	return PathPresenceImplication{
		Trigger:         trigger,
		TriggerValue:    triggerValue,
		HasTriggerValue: true,
		Target:          target,
		TargetValue:     targetValue,
		HasTargetValue:  true,
	}
}

// NewPathTruthyValueRefinementImplication creates a value-triggered implication
// that only activates after the trigger path has also been proven truthy.
func NewPathTruthyValueRefinementImplication(
	trigger keyspace.Key,
	triggerValue product.Value,
	target keyspace.Key,
	targetValue product.Value,
) PathPresenceImplication {
	return PathPresenceImplication{
		Trigger:            trigger,
		TriggerPresence:    presence.Present(),
		TriggerValue:       triggerValue,
		HasTriggerValue:    true,
		HasTriggerPresence: true,
		Target:             target,
		TargetValue:        targetValue,
		HasTargetValue:     true,
	}
}

// NewPathEqualValueRefinementImplication creates a path-equality-triggered
// implication that refines the target to a stored value when activated.
func NewPathEqualValueRefinementImplication(
	trigger keyspace.Key,
	other keyspace.Key,
	target keyspace.Key,
	targetValue product.Value,
) PathPresenceImplication {
	return PathPresenceImplication{
		Trigger:             trigger,
		TriggerOther:        other,
		HasTriggerPathEqual: true,
		Target:              target,
		TargetValue:         targetValue,
		HasTargetValue:      true,
	}
}

// AddPathPresenceImplication records a persistent must implication.
func (l Lane) AddPathPresenceImplication(implication PathPresenceImplication) (Lane, bool) {
	if !validPathPresenceImplication(implication) {
		return l, false
	}
	if !l.pathPresenceImplicationsBottom {
		if _, ok := l.pathPresenceImplications[implication]; ok {
			return l, false
		}
	}
	implications := clonePathPresenceImplicationSet(l.pathPresenceImplications)
	if implications == nil {
		implications = make(map[PathPresenceImplication]struct{}, 1)
	}
	implications[implication] = struct{}{}
	out := l.Reachable()
	out.pathPresenceImplications = implications
	return out, true
}

func (l Lane) HasPathPresenceImplication(implication PathPresenceImplication) bool {
	if l.pathPresenceImplicationsBottom {
		return false
	}
	_, ok := l.pathPresenceImplications[implication]
	return ok
}

func validPathPresenceImplication(implication PathPresenceImplication) bool {
	if implication.Trigger == (keyspace.Key{}) || implication.Target == (keyspace.Key{}) {
		return false
	}
	switch {
	case implication.HasTriggerPathEqual:
		if implication.TriggerOther == (keyspace.Key{}) || implication.TriggerOther == implication.Trigger {
			return false
		}
	case implication.HasTriggerValue:
		if implication.TriggerValue == product.Top() {
			return false
		}
		if implication.HasTriggerPresence && !pathPresenceImplicationPresenceValid(implication.TriggerPresence) {
			return false
		}
	default:
		if !pathPresenceImplicationPresenceValid(implication.TriggerPresence) {
			return false
		}
	}
	if implication.HasTargetValue {
		return implication.TargetValue != product.Top()
	}
	return pathPresenceImplicationPresenceValid(implication.TargetPresence)
}

func pathPresenceImplicationPresenceValid(value presence.Value) bool {
	return presence.Equal(value, presence.Present()) || presence.Equal(value, presence.Absent())
}

func clonePathPresenceImplicationSet(
	in map[PathPresenceImplication]struct{},
) map[PathPresenceImplication]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[PathPresenceImplication]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func deletePathPresenceImplicationsMatching(
	in map[PathPresenceImplication]struct{},
	matches func(keyspace.Key) bool,
) (map[PathPresenceImplication]struct{}, bool) {
	return mapedit.DeleteMatching(in, func(implication PathPresenceImplication, _ struct{}) bool {
		return pathPresenceImplicationMatchesPath(implication, matches)
	})
}

func pathPresenceImplicationMatchesPath(
	implication PathPresenceImplication,
	matches func(keyspace.Key) bool,
) bool {
	if matches == nil {
		return false
	}
	return matches(implication.Trigger) || matches(implication.TriggerOther) || matches(implication.Target)
}

func pathPresenceImplicationsFromSet(
	ks *keyspace.KeySpace,
	in map[PathPresenceImplication]struct{},
) []PathPresenceImplication {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathPresenceImplication, 0, len(in))
	for implication := range in {
		out = append(out, implication)
	}
	sort.Slice(out, func(i, j int) bool {
		return pathPresenceImplicationLess(ks, out[i], out[j])
	})
	return out
}

func pathPresenceImplicationLess(ks *keyspace.KeySpace, a, b PathPresenceImplication) bool {
	if a.Trigger != b.Trigger {
		return ks.Less(a.Trigger, b.Trigger)
	}
	if a.HasTriggerPathEqual != b.HasTriggerPathEqual {
		return !a.HasTriggerPathEqual
	}
	if a.HasTriggerPathEqual && a.TriggerOther != b.TriggerOther {
		return ks.Less(a.TriggerOther, b.TriggerOther)
	}
	if a.HasTriggerValue != b.HasTriggerValue {
		return !a.HasTriggerValue
	}
	if a.HasTriggerValue {
		if order := comparePathPresenceImplicationProducts(a.TriggerValue, b.TriggerValue); order != 0 {
			return order < 0
		}
		if a.HasTriggerPresence != b.HasTriggerPresence {
			return !a.HasTriggerPresence
		}
		if a.HasTriggerPresence && a.TriggerPresence != b.TriggerPresence {
			return a.TriggerPresence < b.TriggerPresence
		}
	} else if a.TriggerPresence != b.TriggerPresence {
		return a.TriggerPresence < b.TriggerPresence
	}
	if a.Target != b.Target {
		return ks.Less(a.Target, b.Target)
	}
	if a.HasTargetValue != b.HasTargetValue {
		return !a.HasTargetValue
	}
	if a.HasTargetValue {
		if order := comparePathPresenceImplicationProducts(a.TargetValue, b.TargetValue); order != 0 {
			return order < 0
		}
	} else if a.TargetPresence != b.TargetPresence {
		return a.TargetPresence < b.TargetPresence
	}
	return false
}

// comparePathPresenceImplicationProducts uses the canonical product ordering
// shared by summary normalization. The product hash covers shape, presence,
// and every registered semantic axis, and therefore does not project away
// refinements while ordering implications.
func comparePathPresenceImplicationProducts(a, b product.Value) int {
	left, right := product.CanonicalHash(a), product.CanonicalHash(b)
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
