package pathevidence

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// PathPresenceImplication records a must implication between two path
// presences: when Trigger has TriggerPresence, Target has TargetPresence.
type PathPresenceImplication struct {
	Trigger         pathdom.PathKey
	TriggerPresence presence.Value
	Target          pathdom.PathKey
	TargetPresence  presence.Value
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
	if implication.Trigger == "" || implication.Target == "" {
		return false
	}
	if !pathPresenceImplicationPresenceValid(implication.TriggerPresence) {
		return false
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
	matches func(pathdom.PathKey) bool,
) (map[PathPresenceImplication]struct{}, bool) {
	if len(in) == 0 {
		return in, false
	}
	out := make(map[PathPresenceImplication]struct{}, len(in))
	changed := false
	for implication := range in {
		if pathPresenceImplicationMatchesPath(implication, matches) {
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
	matches func(pathdom.PathKey) bool,
) bool {
	if matches == nil {
		return false
	}
	return matches(implication.Trigger) || matches(implication.Target)
}

func pathPresenceImplicationsFromSet(
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
		return pathPresenceImplicationLess(out[i], out[j])
	})
	return out
}

func pathPresenceImplicationLess(a, b PathPresenceImplication) bool {
	if a.Trigger != b.Trigger {
		return a.Trigger < b.Trigger
	}
	if a.Target != b.Target {
		return a.Target < b.Target
	}
	if a.TriggerPresence.String() != b.TriggerPresence.String() {
		return a.TriggerPresence.String() < b.TriggerPresence.String()
	}
	return a.TargetPresence.String() < b.TargetPresence.String()
}
