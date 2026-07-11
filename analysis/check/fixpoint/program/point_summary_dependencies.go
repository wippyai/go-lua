package program

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// pointSummaryDependencyToken is the immutable, value-only description of one
// summary observation made by a CFG point. Digest is the normalized summary
// payload digest; it is zero for a missing summary.
type pointSummaryDependencyToken struct {
	Point   cfg.Point
	Owned   bool
	Key     summary.SummaryKey
	Present bool
	Digest  uint64
}

// pointSummaryDependencies is a publication snapshot. Its maps and summaries
// are privately owned and never exposed directly.
type pointSummaryDependencies struct {
	byPoint map[cfg.Point]map[summary.SummaryKey]pointSummaryRead
	unowned map[summary.SummaryKey]pointSummaryRead
}

type pointSummaryRead struct {
	present bool
	digest  uint64
}

type pointSummaryDependencyTracker struct {
	reg         *axis.Registry
	active      bool
	activePoint cfg.Point
	current     map[summary.SummaryKey]trackedSummaryRead
	byPoint     map[cfg.Point]map[summary.SummaryKey]trackedSummaryRead
	unowned     map[summary.SummaryKey]trackedSummaryRead
}

// pointTrackingSummaryReader is an opt-in layer over the existing exact read
// tracker. Keeping it separate leaves ordinary one-shot solves with literally
// the same reader and hot path as before.
type pointTrackingSummaryReader struct {
	base    *trackingSummaryReader
	tracker *pointSummaryDependencyTracker
}

func newPointTrackingSummaryReader(reg *axis.Registry, base summary.Reader) *pointTrackingSummaryReader {
	return &pointTrackingSummaryReader{
		base:    &trackingSummaryReader{reg: reg, base: base},
		tracker: &pointSummaryDependencyTracker{reg: reg},
	}
}

func (r *pointTrackingSummaryReader) Read(key summary.SummaryKey) (summary.Summary, bool) {
	got, ok := r.base.Read(key)
	r.tracker.remember(key, r.base.deps[key])
	return got, ok
}

func (r *pointTrackingSummaryReader) ReadOwnedNormalized(key summary.SummaryKey) (summary.Summary, bool) {
	got, ok := r.base.ReadOwnedNormalized(key)
	r.tracker.remember(key, r.base.deps[key])
	return got, ok
}

func (r *pointTrackingSummaryReader) dependencies() pointSummaryDependencies {
	return r.tracker.snapshot()
}

func (t *pointSummaryDependencyTracker) before(point cfg.Point) {
	t.active = true
	t.activePoint = point
	// A point can transfer more than once. Its latest transfer completely
	// replaces the previous observation set, so a conditional read that
	// disappears does not remain a false dependency forever.
	// Stage into a fresh map. The previously committed point set must remain
	// untouched until AfterPoint installs the completed transfer observation.
	t.current = nil
}

func (t *pointSummaryDependencyTracker) after(point cfg.Point) {
	if !t.active || t.activePoint != point {
		return
	}
	if len(t.current) == 0 {
		if t.byPoint != nil {
			delete(t.byPoint, point)
		}
	} else {
		if t.byPoint == nil {
			t.byPoint = make(map[cfg.Point]map[summary.SummaryKey]trackedSummaryRead)
		}
		t.byPoint[point] = t.current
	}
	t.current = nil
	t.active = false
}

func (t *pointSummaryDependencyTracker) remember(key summary.SummaryKey, dep trackedSummaryRead) {
	if t.active {
		if t.current == nil {
			t.current = make(map[summary.SummaryKey]trackedSummaryRead)
		}
		t.current[key] = dep
		return
	}
	if t.unowned == nil {
		t.unowned = make(map[summary.SummaryKey]trackedSummaryRead)
	}
	t.unowned[key] = dep
}

func (t *pointSummaryDependencyTracker) snapshot() pointSummaryDependencies {
	if t == nil {
		return pointSummaryDependencies{}
	}
	byPoint := make(map[cfg.Point]map[summary.SummaryKey]pointSummaryRead, len(t.byPoint))
	for point, reads := range t.byPoint {
		if len(reads) != 0 {
			published := make(map[summary.SummaryKey]pointSummaryRead, len(reads))
			for key, read := range reads {
				published[key] = publishPointSummaryRead(t.reg, read)
			}
			byPoint[point] = published
		}
	}
	unowned := make(map[summary.SummaryKey]pointSummaryRead, len(t.unowned))
	for key, read := range t.unowned {
		unowned[key] = publishPointSummaryRead(t.reg, read)
	}
	return pointSummaryDependencies{
		byPoint: byPoint,
		unowned: unowned,
	}
}

func publishPointSummaryRead(reg *axis.Registry, read trackedSummaryRead) pointSummaryRead {
	out := pointSummaryRead{present: read.present}
	if read.present {
		out.digest = uint64(summary.NormalizedPayloadDigest(reg, read.sum))
	}
	return out
}

func (d pointSummaryDependencies) tokens() []pointSummaryDependencyToken {
	var out []pointSummaryDependencyToken
	for point, reads := range d.byPoint {
		for key, dep := range reads {
			token := pointSummaryDependencyToken{Point: point, Owned: true, Key: key, Present: dep.present, Digest: dep.digest}
			out = append(out, token)
		}
	}
	for key, dep := range d.unowned {
		token := pointSummaryDependencyToken{Key: key, Present: dep.present, Digest: dep.digest}
		out = append(out, token)
	}
	slices.SortFunc(out, func(a, b pointSummaryDependencyToken) int {
		if a.Owned != b.Owned {
			if a.Owned {
				return -1
			}
			return 1
		}
		if a.Point < b.Point {
			return -1
		}
		if a.Point > b.Point {
			return 1
		}
		if a.Key.Less(b.Key) {
			return -1
		}
		if b.Key.Less(a.Key) {
			return 1
		}
		return 0
	})
	return out
}

// affectedPoints maps changed summary keys back to their owning transfer
// points. fallback is true exactly when a changed key was read outside a point.
func (d pointSummaryDependencies) affectedPoints(changed []summary.SummaryKey) (points []cfg.Point, fallback bool) {
	if len(changed) == 0 {
		return nil, false
	}
	changedSet := make(map[summary.SummaryKey]struct{}, len(changed))
	for _, key := range changed {
		changedSet[key] = struct{}{}
		if _, ok := d.unowned[key]; ok {
			fallback = true
		}
	}
	seen := make(map[cfg.Point]struct{})
	for point, reads := range d.byPoint {
		for key := range reads {
			if _, ok := changedSet[key]; !ok {
				continue
			}
			seen[point] = struct{}{}
			break
		}
	}
	for point := range seen {
		points = append(points, point)
	}
	slices.Sort(points)
	return points, fallback
}

func attachPointSummaryTracking(config *body.Config, tracker *pointSummaryDependencyTracker) {
	before, after := config.BeforePoint, config.AfterPoint
	config.BeforePoint = func(point cfg.Point) {
		tracker.before(point)
		if before != nil {
			before(point)
		}
	}
	config.AfterPoint = func(point cfg.Point) {
		if after != nil {
			after(point)
		}
		tracker.after(point)
	}
}
