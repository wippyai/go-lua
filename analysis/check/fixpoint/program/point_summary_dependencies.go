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
	Point cfg.Point
	Owned bool
	// Projection distinguishes post-flow summary projection reads from true
	// pre-flow unowned reads. Both have no equation owner, but only pre-flow
	// reads require a full CFG fallback.
	Projection bool
	Key        summary.SummaryKey
	Present    bool
	Digest     uint64
}

// pointSummaryDependencies is a publication snapshot. Its maps and summaries
// are privately owned and never exposed directly.
type pointSummaryDependencies struct {
	byPoint    map[cfg.Point]map[summary.SummaryKey]pointSummaryRead
	preFlow    map[summary.SummaryKey]pointSummaryRead
	projection map[summary.SummaryKey]pointSummaryRead
	// visited is update-local replacement metadata. A visited point absent from
	// byPoint is an explicit tombstone: its latest transfer made no reads.
	visited            map[cfg.Point]struct{}
	projectionObserved bool
}

type pointSummaryRead struct {
	present bool
	digest  uint64
}

type pointSummaryDependencyTracker struct {
	reg             *axis.Registry
	active          bool
	activePoint     cfg.Point
	current         map[summary.SummaryKey]trackedSummaryRead
	byPoint         map[cfg.Point]map[summary.SummaryKey]trackedSummaryRead
	visited         map[cfg.Point]struct{}
	preFlow         map[summary.SummaryKey]trackedSummaryRead
	projecting      bool
	projectionReads map[summary.SummaryKey]trackedSummaryRead
	projectionSeen  bool
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
	if t.visited == nil {
		t.visited = make(map[cfg.Point]struct{})
	}
	t.visited[point] = struct{}{}
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
	if t.projecting {
		if t.projectionReads == nil {
			t.projectionReads = make(map[summary.SummaryKey]trackedSummaryRead)
		}
		t.projectionReads[key] = dep
		return
	}
	if t.preFlow == nil {
		t.preFlow = make(map[summary.SummaryKey]trackedSummaryRead)
	}
	t.preFlow[key] = dep
}

// beginProjection classifies reads performed while deriving the public
// summary from a completed body Result. Projection reads do not own CFG
// equations and therefore cannot seed regional invalidation, but a changed
// projection dependency requires the completed Result to be projected again.
func (t *pointSummaryDependencyTracker) beginProjection() {
	if t == nil {
		return
	}
	t.projecting = true
	t.projectionReads = nil
	t.projectionSeen = true
}

func (t *pointSummaryDependencyTracker) endProjection() {
	if t == nil {
		return
	}
	t.projecting = false
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
	preFlow := make(map[summary.SummaryKey]pointSummaryRead, len(t.preFlow))
	for key, read := range t.preFlow {
		preFlow[key] = publishPointSummaryRead(t.reg, read)
	}
	projection := make(map[summary.SummaryKey]pointSummaryRead, len(t.projectionReads))
	for key, read := range t.projectionReads {
		projection[key] = publishPointSummaryRead(t.reg, read)
	}
	return pointSummaryDependencies{
		byPoint: byPoint, preFlow: preFlow, projection: projection,
		visited: mapsCloneSet(t.visited), projectionObserved: t.projectionSeen,
	}
}

func mapsCloneSet[K comparable](in map[K]struct{}) map[K]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[K]struct{}, len(in))
	for key := range in {
		out[key] = struct{}{}
	}
	return out
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
	for key, dep := range d.preFlow {
		token := pointSummaryDependencyToken{Key: key, Present: dep.present, Digest: dep.digest}
		out = append(out, token)
	}
	for key, dep := range d.projection {
		token := pointSummaryDependencyToken{Projection: true, Key: key, Present: dep.present, Digest: dep.digest}
		out = append(out, token)
	}
	slices.SortFunc(out, func(a, b pointSummaryDependencyToken) int {
		if a.Owned != b.Owned {
			if a.Owned {
				return -1
			}
			return 1
		}
		if a.Projection != b.Projection {
			if !a.Projection {
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

// impact maps changed summary keys to their consumers. forceFull is true for
// pre-flow unowned reads. reproject is true for post-flow projection reads,
// which consume a completed Result but do not invalidate any CFG equation.
func (d pointSummaryDependencies) impact(changed []summary.SummaryKey) (points []cfg.Point, forceFull, reproject bool) {
	if len(changed) == 0 {
		return nil, false, false
	}
	changedSet := make(map[summary.SummaryKey]struct{}, len(changed))
	for _, key := range changed {
		changedSet[key] = struct{}{}
		if _, ok := d.preFlow[key]; ok {
			forceFull = true
		}
		if _, ok := d.projection[key]; ok {
			reproject = true
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
	return points, forceFull, reproject
}

// affectedPoints preserves the original regional-routing surface. Projection
// reads are deliberately not reported as fallback dependencies.
func (d pointSummaryDependencies) affectedPoints(changed []summary.SummaryKey) (points []cfg.Point, fallback bool) {
	points, fallback, _ = d.impact(changed)
	return points, fallback
}

// mergeUpdate applies one regional observation delta. Unvisited point sets are
// preserved; every visited point replaces its previous set, including an empty
// tombstone. Pre-flow reads are a complete observation for the invocation.
// Projection reads replace the base only when projection actually ran.
func (d pointSummaryDependencies) mergeUpdate(update pointSummaryDependencies) pointSummaryDependencies {
	out := pointSummaryDependencies{
		byPoint:    clonePointSummaryReadMap(d.byPoint),
		preFlow:    clonePointSummaryReads(update.preFlow),
		projection: clonePointSummaryReads(d.projection),
	}
	for point := range update.visited {
		if reads := update.byPoint[point]; len(reads) != 0 {
			if out.byPoint == nil {
				out.byPoint = make(map[cfg.Point]map[summary.SummaryKey]pointSummaryRead)
			}
			out.byPoint[point] = clonePointSummaryReads(reads)
		} else {
			delete(out.byPoint, point)
		}
	}
	if update.projectionObserved {
		out.projection = clonePointSummaryReads(update.projection)
	}
	return out
}

// mergeProjection preserves the completed flow observation and replaces only
// the post-flow projection dependency set.
func (d pointSummaryDependencies) mergeProjection(update pointSummaryDependencies) pointSummaryDependencies {
	out := clonePointSummaryDependencies(d)
	if update.projectionObserved {
		out.projection = clonePointSummaryReads(update.projection)
		out.projectionObserved = true
	}
	return out
}

func clonePointSummaryReadMap(in map[cfg.Point]map[summary.SummaryKey]pointSummaryRead) map[cfg.Point]map[summary.SummaryKey]pointSummaryRead {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]map[summary.SummaryKey]pointSummaryRead, len(in))
	for point, reads := range in {
		out[point] = clonePointSummaryReads(reads)
	}
	return out
}

func clonePointSummaryReads(in map[summary.SummaryKey]pointSummaryRead) map[summary.SummaryKey]pointSummaryRead {
	if len(in) == 0 {
		return nil
	}
	out := make(map[summary.SummaryKey]pointSummaryRead, len(in))
	for key, read := range in {
		out[key] = read
	}
	return out
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
