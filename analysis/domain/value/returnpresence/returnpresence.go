// Package returnpresence holds the shared logic for inferring presence
// implications between return slots from per-return-point presence vectors. It
// is used both when lowering transfer facts and when projecting summaries.
package returnpresence

import "github.com/wippyai/go-lua/analysis/domain/value/axis/presence"

// Point is one return point's presence vector: Presence[i] is slot i's presence
// and Known[i] reports whether that slot's presence is determined at the point.
type Point struct {
	Presence []presence.Value
	Known    []bool
}

// KnownPresence returns value with true when it is a definite presence (neither
// bottom nor top), and bottom/false otherwise.
func KnownPresence(value presence.Value) (presence.Value, bool) {
	if value.IsBottom() || value.IsTop() {
		return presence.Bottom(), false
	}
	return value, true
}

// TargetPresence infers the presence of target implied across points whenever
// trigger holds triggerPresence. It returns false when no single consistent
// target presence holds on every qualifying point.
func TargetPresence(points []Point, trigger int, triggerPresence presence.Value, target int) (presence.Value, bool) {
	var out presence.Value
	var saw bool
	for _, point := range points {
		if trigger >= len(point.Presence) || target >= len(point.Presence) ||
			!point.Known[trigger] || !point.Known[target] {
			return presence.Bottom(), false
		}
		if !canBe(point.Presence[trigger], triggerPresence) {
			continue
		}
		targetPresence := point.Presence[target]
		if !definite(targetPresence) {
			return presence.Bottom(), false
		}
		if !saw {
			out = targetPresence
			saw = true
			continue
		}
		if !presence.Equal(out, targetPresence) {
			return presence.Bottom(), false
		}
	}
	return out, saw
}

func canBe(value, want presence.Value) bool {
	return presence.Equal(value, want) || presence.Equal(value, presence.Maybe())
}

func definite(value presence.Value) bool {
	return presence.Equal(value, presence.Present()) || presence.Equal(value, presence.Absent())
}
