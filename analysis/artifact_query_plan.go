package analysis

import (
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// artifactQueryRole is deliberately closed: each mounted artifact point gets
// exactly the two query lanes defined by the sealed Program schema.
type artifactQueryRole uint8

const (
	artifactQueryValueSummary artifactQueryRole = iota + 1
	artifactQueryEffectExact
)

type artifactQueryAttachment struct {
	id    identity.ContentID
	mount identity.ContentID
	point identity.ContentID
	role  artifactQueryRole
}

type artifactQueryPlan struct {
	rows []artifactQueryAttachment
}

// newArtifactQueryPlan derives mount-qualified query identities from semantic
// occurrences in non-callable Program roots. Callable bodies are not global
// result roots: their interiors enter demand only through a selected call,
// callback, or explicit observation interface. This is the runtime cut that
// prevents an uncalled library function from being solved merely because it
// exists in a mounted Program. Synthetic WTO-only points remain in the
// schedule but are not result observation sites. No Program/Flow owner is
// reopened.
func newArtifactQueryPlan(mounts []mountedProgramArtifact) (*artifactQueryPlan, bool) {
	if len(mounts) == 0 {
		return nil, false
	}
	plan := &artifactQueryPlan{}
	expected := 0
	for _, mount := range mounts {
		if mount.artifact == nil || !mount.artifact.Available() || !mount.moduleKey.Available() {
			return nil, false
		}
		rootBodies := make(map[identity.ContentID][]identity.ContentID)
		for bodyIndex := 0; bodyIndex < mount.artifact.BodyCount(); bodyIndex++ {
			body, bodyOK := mount.artifact.BodyAt(bodyIndex)
			if !bodyOK || !body.Available() || !body.ID().Available() {
				return nil, false
			}
			if body.Callable() {
				continue
			}
			entries := make([]identity.ContentID, body.EntryPointCount())
			for entryIndex := range entries {
				entry, entryOK := body.EntryPointAt(entryIndex)
				if !entryOK || !entry.Available() {
					return nil, false
				}
				entries[entryIndex] = entry
			}
			if len(entries) == 0 {
				return nil, false
			}
			rootBodies[body.ID()] = entries
		}
		if len(rootBodies) == 0 {
			return nil, false
		}
		observed := make(map[identity.ContentID]struct{})
		observedBodies := make(map[identity.ContentID]struct{}, len(rootBodies))
		for occurrenceIndex := 0; occurrenceIndex < mount.artifact.OccurrenceCount(); occurrenceIndex++ {
			occurrence, occurrenceOK := mount.artifact.OccurrenceAt(occurrenceIndex)
			body, bodyOK := occurrence.BodyID()
			if !occurrenceOK || !occurrence.Available() {
				return nil, false
			}
			if !bodyOK {
				continue
			}
			if _, root := rootBodies[body]; !root {
				continue
			}
			observedBodies[body] = struct{}{}
			for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
				point, pointOK := occurrence.PointAt(pointIndex)
				if !pointOK || !point.Available() {
					return nil, false
				}
				observed[point] = struct{}{}
			}
		}
		// A non-callable root with no semantic occurrence still needs one exact
		// empty observation anchor. Use its Program-issued entry attachment,
		// never an arbitrary point from a callable body.
		for body, entries := range rootBodies {
			if _, present := observedBodies[body]; present {
				continue
			}
			for _, entry := range entries {
				observed[entry] = struct{}{}
			}
		}
		for index := 0; index < mount.artifact.PointCount(); index++ {
			point, ok := mount.artifact.PointAt(index)
			if !ok || !point.Available() || !point.ID().Available() {
				return nil, false
			}
			pointID := point.ID()
			if _, selected := observed[pointID]; !selected {
				continue
			}
			expected++
			for _, row := range []struct {
				role artifactQueryRole
				name string
			}{
				{artifactQueryValueSummary, "value-summary"},
				{artifactQueryEffectExact, "effect-exact"},
			} {
				id, idOK := identity.DeriveContentID("analysis/artifact-query/v1", mount.moduleKey[:], pointID[:], []byte(row.name))
				if !idOK {
					return nil, false
				}
				plan.rows = append(plan.rows, artifactQueryAttachment{id: id, mount: mount.moduleKey, point: pointID, role: row.role})
			}
		}
	}
	return plan, expected > 0 && len(plan.rows) == 2*expected
}

// AddRows emits query rows inside the assembly's query batch scope.
func (plan *artifactQueryPlan) AddRows(batch *engine.MountedQueryBatch, binding *composite.ProgramBinding) bool {
	if plan == nil || batch == nil || binding == nil || binding.ValueQuery() == nil || binding.EffectQuery() == nil || len(plan.rows) == 0 {
		return false
	}
	for _, row := range plan.rows {
		var ok bool
		if row.role == artifactQueryValueSummary {
			ok = engine.AddMountedSummaryQuery(batch, binding.ValueQuery(), row.id, row.mount, row.point)
		} else if row.role == artifactQueryEffectExact {
			ok = engine.AddMountedExactQuery(batch, binding.EffectQuery(), row.id, row.mount, row.point)
		}
		if !ok {
			return false
		}
	}
	return true
}

// Attach binds every query row to the existing Link-local implementation
// receipts. Rule dispatch remains a separate attachment lane.
func (plan *artifactQueryPlan) Attach(compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, binding *composite.ProgramBinding) bool {
	if plan == nil || compilation == nil || graph == nil || binding == nil || binding.ValueQuery() == nil || binding.EffectQuery() == nil || len(plan.rows) == 0 {
		return false
	}
	for _, row := range plan.rows {
		query, ok := graph.Query(row.id)
		if !ok {
			return false
		}
		if row.role == artifactQueryValueSummary {
			ok = engine.AttachReceiptSummaryQuery(compilation, binding.ValueQuery(), query)
		} else if row.role == artifactQueryEffectExact {
			ok = engine.AttachReceiptExactQuery(compilation, binding.EffectQuery(), query)
		} else {
			ok = false
		}
		if !ok {
			return false
		}
	}
	return true
}

// Keep the concrete query result types in this file's dependency surface so
// the lane remains tied to the existing ProgramBinding implementations.
var _ *engine.SummaryQueryImplementation[valuedomain.Value, valuedomain.ValueSummaryObservation]
var _ *engine.ExactQueryImplementation[factor.Value, factor.EffectObservation]
