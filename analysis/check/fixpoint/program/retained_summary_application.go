package program

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// retainedSummaryApplicationKey is the complete structural identity of one
// body-to-summary application. Summary inputs are deliberately absent: they
// are validated exactly against the separately published dependency snapshot.
type retainedSummaryApplicationKey struct {
	body       uint64
	input      uint64
	profile    string
	resolution uint64
}

type retainedSummaryApplicationKind uint8

const (
	retainedSummaryApplyOrdinary retainedSummaryApplicationKind = iota
	retainedSummaryApplyReuse
	retainedSummaryApplyBuild
	retainedSummaryApplyRegional
	retainedSummaryApplyReproject
)

// retainedSummaryApplicationDecision is a value-only plan. It contains no
// retained body graph and is safe to inspect after the owning run is released.
type retainedSummaryApplicationDecision struct {
	kind retainedSummaryApplicationKind

	changed   []summary.SummaryKey
	points    []cfg.Point
	forceFull bool
	reproject bool

	// dropped reports that a structurally incompatible retained generation was
	// released before this ordinary application was planned.
	dropped bool
}

// retainedSummaryResource is the only lifecycle contract the policy needs.
// The body adapter can satisfy it without exposing solver generations here.
type retainedSummaryResource interface {
	Release()
}

type retainedSummaryApplicationPublication struct {
	key      retainedSummaryApplicationKey
	deps     map[summary.SummaryKey]trackedSummaryRead
	points   pointSummaryDependencies
	retained bool
	resource retainedSummaryResource
}

// retainedSummaryApplicationOwner belongs to one query function invocation.
// It is intentionally neither shareable nor stored in SummarySolveCache: the
// retained graph and its point identities are meaningful only for this run.
type retainedSummaryApplicationOwner struct {
	reg       *axis.Registry
	published *retainedSummaryApplicationPublication
	epoch     uint64
	released  bool
}

// retainedSummaryApplicationAttempt separates planning from publication. A
// canceled or rejected solve simply aborts the attempt, leaving the last
// compatible publication unchanged.
type retainedSummaryApplicationAttempt struct {
	owner    *retainedSummaryApplicationOwner
	epoch    uint64
	key      retainedSummaryApplicationKey
	decision retainedSummaryApplicationDecision
	done     bool
}

func newRetainedSummaryApplicationOwner(reg *axis.Registry) *retainedSummaryApplicationOwner {
	return &retainedSummaryApplicationOwner{reg: reg}
}

func (o *retainedSummaryApplicationOwner) begin(
	key retainedSummaryApplicationKey,
	reader summary.Reader,
) *retainedSummaryApplicationAttempt {
	decision := retainedSummaryApplicationDecision{kind: retainedSummaryApplyOrdinary}
	if o == nil || o.released {
		return &retainedSummaryApplicationAttempt{decision: decision, done: true}
	}

	if current := o.published; current != nil {
		if current.key != key {
			decision.dropped = current.retained
			o.dropPublished()
		} else {
			decision.changed = changedRetainedSummaryDependencies(o.reg, current.deps, reader)
			switch {
			case len(decision.changed) == 0:
				decision.kind = retainedSummaryApplyReuse
			case !current.retained:
				// The first invalidation pays for one clean retained generation.
				// Point attribution starts there, not on the ordinary hot path.
				decision.kind = retainedSummaryApplyBuild
			default:
				decision.points, decision.forceFull, decision.reproject = current.points.impact(decision.changed)
				if len(decision.points) == 0 && !decision.forceFull && decision.reproject {
					decision.kind = retainedSummaryApplyReproject
				} else {
					decision.kind = retainedSummaryApplyRegional
				}
			}
		}
	}

	o.epoch++
	return &retainedSummaryApplicationAttempt{
		owner: o, epoch: o.epoch, key: key, decision: decision,
	}
}

func (a *retainedSummaryApplicationAttempt) Decision() retainedSummaryApplicationDecision {
	if a == nil {
		return retainedSummaryApplicationDecision{kind: retainedSummaryApplyOrdinary}
	}
	out := a.decision
	out.changed = slices.Clone(out.changed)
	out.points = slices.Clone(out.points)
	return out
}

// Publish atomically advances policy state after body solving and summary
// projection have both succeeded. deps must be the complete dependency
// observation for the resulting generation; points may be an update delta for
// a regional solve and is merged transactionally with the prior publication.
func (a *retainedSummaryApplicationAttempt) Publish(
	deps map[summary.SummaryKey]trackedSummaryRead,
	points pointSummaryDependencies,
	resource retainedSummaryResource,
) bool {
	if a == nil || a.done || a.owner == nil || a.owner.released || a.owner.epoch != a.epoch {
		return false
	}
	if a.decision.kind == retainedSummaryApplyReuse {
		// Reuse returns the already-published summary and generation. It has no
		// candidate state to publish.
		a.done = true
		return false
	}
	if a.decision.kind == retainedSummaryApplyBuild && resource == nil {
		// Do not claim to own a retained generation when the body layer did not
		// actually publish one.
		a.done = true
		return false
	}
	a.done = true
	o := a.owner

	retained := a.decision.kind == retainedSummaryApplyBuild ||
		a.decision.kind == retainedSummaryApplyRegional ||
		a.decision.kind == retainedSummaryApplyReproject
	replaceResource := resource != nil
	if retained && resource == nil && o.published != nil && o.published.key == a.key {
		resource = o.published.resource
	}
	if a.decision.kind == retainedSummaryApplyRegional && o.published != nil && o.published.key == a.key {
		points = o.published.points.mergeUpdate(points)
	}
	if a.decision.kind == retainedSummaryApplyReproject && o.published != nil && o.published.key == a.key {
		points = o.published.points.mergeUpdate(points)
	}

	// A non-nil resource on an update explicitly replaces the old generation;
	// passing nil retains it. Avoid comparing interface values because a valid
	// resource implementation need not have a comparable dynamic type.
	if old := o.published; old != nil && old.resource != nil && replaceResource {
		old.resource.Release()
	}
	o.published = &retainedSummaryApplicationPublication{
		key:      a.key,
		deps:     normalizedRetainedSummaryDependencies(o.reg, deps),
		points:   clonePointSummaryDependencies(points),
		retained: retained,
		resource: resource,
	}
	return true
}

func (a *retainedSummaryApplicationAttempt) Abort() {
	if a != nil {
		a.done = true
	}
}

func (o *retainedSummaryApplicationOwner) Release() {
	if o == nil || o.released {
		return
	}
	o.dropPublished()
	o.epoch++
	o.released = true
}

func (o *retainedSummaryApplicationOwner) dropPublished() {
	if o == nil || o.published == nil {
		return
	}
	if o.published.resource != nil {
		o.published.resource.Release()
	}
	o.published = nil
}

func normalizedRetainedSummaryDependencies(reg *axis.Registry, in map[summary.SummaryKey]trackedSummaryRead) map[summary.SummaryKey]trackedSummaryRead {
	if len(in) == 0 {
		return nil
	}
	out := make(map[summary.SummaryKey]trackedSummaryRead, len(in))
	for key, dep := range in {
		if dep.present {
			dep.sum = summary.Normalize(reg, dep.sum)
		}
		out[key] = dep
	}
	return out
}

func changedRetainedSummaryDependencies(reg *axis.Registry, deps map[summary.SummaryKey]trackedSummaryRead, reader summary.Reader) []summary.SummaryKey {
	if len(deps) == 0 {
		return nil
	}
	changed := make([]summary.SummaryKey, 0)
	for key, dep := range deps {
		got, ok := readOwnedNormalizedSummary(reg, reader, key)
		if ok != dep.present || (ok && !summary.EqualNormalized(reg, got, dep.sum)) {
			changed = append(changed, key)
		}
	}
	slices.SortFunc(changed, func(a, b summary.SummaryKey) int {
		if a.Less(b) {
			return -1
		}
		if b.Less(a) {
			return 1
		}
		return 0
	})
	return changed
}

func clonePointSummaryDependencies(in pointSummaryDependencies) pointSummaryDependencies {
	return pointSummaryDependencies{
		byPoint:            clonePointSummaryReadMap(in.byPoint),
		preFlow:            clonePointSummaryReads(in.preFlow),
		projection:         clonePointSummaryReads(in.projection),
		visited:            mapsCloneSet(in.visited),
		projectionObserved: in.projectionObserved,
	}
}
