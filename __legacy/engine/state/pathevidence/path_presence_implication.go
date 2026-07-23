package pathevidence

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

// PathPresenceImplication records a must implication between two path
// facts: when Trigger has either TriggerPresence or TriggerValue, Target has
// TargetPresence or TargetValue.
type PathPresenceImplication struct {
	Trigger              keyspace.Key
	TriggerOther         keyspace.Key
	TriggerPresence      presence.Value
	TriggerValue         product.Value
	HasTriggerValue      bool
	HasTriggerPresence   bool
	HasTriggerPathEqual  bool
	HasTriggerTruthiness bool
	TriggerTruthy        bool
	Target               keyspace.Key
	TargetPresence       presence.Value
	TargetValue          product.Value
	HasTargetValue       bool
}

// NewPathTruthinessValueRefinementImplication creates a Lua-truthiness
// triggered implication. Truthy and falsy are exact complements over the
// product value; they are not aliases for boolean literals.
func NewPathTruthinessValueRefinementImplication(
	trigger keyspace.Key,
	truthy bool,
	target keyspace.Key,
	targetValue product.Value,
) PathPresenceImplication {
	return PathPresenceImplication{
		Trigger: trigger, HasTriggerTruthiness: true, TriggerTruthy: truthy,
		Target: target, TargetValue: targetValue, HasTargetValue: true,
	}
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
	return l.AddPathPresenceImplications([]PathPresenceImplication{implication})
}

// AddPathPresenceImplications records valid persistent must implications with
// one copy-on-write set update. It is equivalent to repeated
// AddPathPresenceImplication calls, but lets prepared coordinate builders keep
// their immutable evidence set shared until the complete publication batch is
// known.
func (l Lane) AddPathPresenceImplications(additions []PathPresenceImplication) (Lane, bool) {
	if len(additions) == 0 {
		return l, false
	}
	var implications map[PathPresenceImplication]struct{}
	changed := false
	for _, implication := range additions {
		_, present := l.pathPresenceImplications[implication]
		insert, _ := pathPresenceImplicationPublication(implication, present, l.pathPresenceImplicationsBottom)
		if !insert {
			continue
		}
		if implications == nil {
			implications = clonePathPresenceImplicationSet(l.pathPresenceImplications)
			if implications == nil {
				implications = make(map[PathPresenceImplication]struct{}, len(additions))
			}
		}
		if _, exists := implications[implication]; exists {
			// Bottom ignores the retained set. Republishing a valid member must
			// still establish reachability even when that member was retained
			// beneath the Bottom marker.
			changed = changed || l.pathPresenceImplicationsBottom
			continue
		}
		implications[implication] = struct{}{}
		changed = true
	}
	if !changed {
		return l, false
	}
	out := l.Reachable()
	out.pathPresenceImplications = implications
	return out, true
}

// pathPresenceImplicationPublication is the sole semantic insertion law for
// the persistent must set. Lane and coordinate storage adapters both call it.
func pathPresenceImplicationPublication(implication PathPresenceImplication, present, bottom bool) (insert, establishesReachability bool) {
	if !validPathPresenceImplication(implication) || present && !bottom {
		return false, false
	}
	return true, true
}

// CanonicalPathPresenceImplications returns the strict semantic order consumed
// by factorwise publication. Sorting happens once when an immutable operation
// plan is prepared, never once per guarded decision leaf.
func CanonicalPathPresenceImplications(reg *axis.Registry, ks *keyspace.KeySpace, in []PathPresenceImplication) ([]PathPresenceImplication, bool) {
	if reg == nil || ks == nil || !ks.Valid() {
		return nil, false
	}
	out := append([]PathPresenceImplication(nil), in...)
	for _, implication := range out {
		key, scalar := implicationCoordinateParts(implication)
		if !CoordinateKeyValid(key, ks, reg) || !CoordinateScalarValid(key, scalar, reg) {
			return nil, false
		}
	}
	sort.Slice(out, func(i, j int) bool { return pathPresenceImplicationLess(ks, out[i], out[j]) })
	write := 0
	for _, implication := range out {
		if write != 0 && out[write-1] == implication {
			continue
		}
		out[write] = implication
		write++
	}
	return out[:write], true
}

// PathPresenceImplicationsCanonical reports whether in is already the strict
// semantic order produced by snapshots. It performs one linear validation and
// never copies or re-sorts the input.
func PathPresenceImplicationsCanonical(reg *axis.Registry, ks *keyspace.KeySpace, in []PathPresenceImplication) bool {
	if reg == nil || ks == nil || !ks.Valid() {
		return false
	}
	for index, implication := range in {
		key, scalar := implicationCoordinateParts(implication)
		if !CoordinateKeyValid(key, ks, reg) || !CoordinateScalarValid(key, scalar, reg) {
			return false
		}
		if index != 0 && !pathPresenceImplicationLess(ks, in[index-1], implication) {
			return false
		}
	}
	return true
}

func (l Lane) HasPathPresenceImplication(implication PathPresenceImplication) bool {
	if l.pathPresenceImplicationsBottom {
		return false
	}
	_, ok := l.pathPresenceImplications[implication]
	return ok
}

func validPathPresenceImplication(implication PathPresenceImplication) bool {
	if !validPathPresenceImplicationShape(implication) {
		return false
	}
	if implication.HasTriggerValue && implication.TriggerValue == product.Top() {
		return false
	}
	if implication.HasTargetValue {
		return implication.TargetValue != product.Top()
	}
	return true
}

// validPathPresenceImplicationShape validates the address portion of an
// implication independently of its product-valued clause payload. Coordinate
// factorization uses this form so provider-created values remain DD terminal
// data rather than becoming new coordinate identities at execution time.
func validPathPresenceImplicationShape(implication PathPresenceImplication) bool {
	if implication.Trigger == (keyspace.Key{}) || implication.Target == (keyspace.Key{}) {
		return false
	}
	triggerKinds := 0
	if implication.HasTriggerPathEqual {
		triggerKinds++
	}
	if implication.HasTriggerValue {
		triggerKinds++
	}
	if implication.HasTriggerTruthiness {
		triggerKinds++
	}
	if !implication.HasTriggerPathEqual && !implication.HasTriggerValue && !implication.HasTriggerTruthiness {
		triggerKinds++ // ordinary presence trigger
	}
	if triggerKinds != 1 || implication.HasTriggerTruthiness && implication.HasTriggerPresence {
		return false
	}
	switch {
	case implication.HasTriggerPathEqual:
		if implication.TriggerOther == (keyspace.Key{}) || implication.TriggerOther == implication.Trigger {
			return false
		}
	case implication.HasTriggerTruthiness:
		// The bool selects one half of the exact Lua truthiness partition.
	case implication.HasTriggerValue:
		if implication.HasTriggerPresence && !pathPresenceImplicationPresenceValid(implication.TriggerPresence) {
			return false
		}
	default:
		if !pathPresenceImplicationPresenceValid(implication.TriggerPresence) {
			return false
		}
	}
	return implication.HasTargetValue || pathPresenceImplicationPresenceValid(implication.TargetPresence)
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

// deletePathPresenceImplicationsMatchingExcept performs one native set
// rewrite. It removes implications touching matches unless preserve proves the
// complete implication remains valid. No presentation snapshot, key formatting,
// ordering, or per-preserved-element clone participates.
func deletePathPresenceImplicationsMatchingExcept(
	in map[PathPresenceImplication]struct{},
	matches func(keyspace.Key) bool,
	preserve func(PathPresenceImplication) bool,
) (map[PathPresenceImplication]struct{}, bool) {
	if len(in) == 0 {
		return nil, false
	}
	out := make(map[PathPresenceImplication]struct{}, len(in))
	changed := false
	for implication := range in {
		remove := pathPresenceImplicationMatchesPath(implication, matches) && (preserve == nil || !preserve(implication))
		if remove {
			changed = true
			continue
		}
		out[implication] = struct{}{}
	}
	if !changed {
		return in, false
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
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
	if a.HasTriggerTruthiness != b.HasTriggerTruthiness {
		return !a.HasTriggerTruthiness
	}
	if a.HasTriggerTruthiness && a.TriggerTruthy != b.TriggerTruthy {
		return !a.TriggerTruthy
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
