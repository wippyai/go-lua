package analysis

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// artifactQueryRole is deliberately closed: each mounted artifact point gets
// exactly the two query lanes defined by the sealed Program schema.
type artifactQueryRole uint8

const (
	artifactQueryValueSummary artifactQueryRole = iota + 1
	artifactQueryEffectExact
)

type artifactQueryAttachment struct {
	id         identity.ContentID
	mount      identity.ContentID
	point      identity.ContentID
	role       artifactQueryRole
	authority  schema.Key
	projection schema.Key
}

// artifactQueryPublication is the Result-facing address of one mounted query.
// Receipt authentication ends before this value is constructed; Result reads
// the committed Snapshot by this plain owner-issued key.
type artifactQueryPublication struct {
	attachment artifactQueryAttachment
	key        identity.ContentID
}

type artifactQueryPlan struct {
	rows []artifactQueryAttachment
}

// newArtifactQueryPlan derives mount-qualified query identities from semantic
// occurrences in selected Program bodies. Non-callable roots are always
// selected. A callable body is selected only when a sealed DirectFunctions
// join from an already-selected body names it. Uncalled interiors stay off
// the plan so an unused library function is not solved merely because it
// exists in a mounted Program. Synthetic WTO-only points remain in the
// schedule but are not result observation sites. No Program/Flow owner is
// reopened.
func newArtifactQueryPlan(mounts []mountedProgramArtifact) (*artifactQueryPlan, bool) {
	if len(mounts) == 0 {
		return nil, false
	}
	selectedFamilies, familiesOK := selectedPointQueryIssuance()
	if !familiesOK {
		return nil, false
	}
	plan := &artifactQueryPlan{}
	expected := 0
	for _, mount := range mounts {
		if mount.artifact == nil || !mount.artifact.Available() || !mount.moduleKey.Available() {
			return nil, false
		}
		bodyEntries := make(map[identity.ContentID][]identity.ContentID)
		callable := make(map[identity.ContentID]struct{})
		rootBodies := make(map[identity.ContentID][]identity.ContentID)
		for bodyIndex := 0; bodyIndex < mount.artifact.BodyCount(); bodyIndex++ {
			body, bodyOK := mount.artifact.BodyAt(bodyIndex)
			if !bodyOK || !body.Available() || !body.ID().Available() {
				return nil, false
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
			bodyEntries[body.ID()] = entries
			if body.Callable() {
				callable[body.ID()] = struct{}{}
				continue
			}
			rootBodies[body.ID()] = entries
		}
		if len(rootBodies) == 0 {
			return nil, false
		}
		selectedBodies := make(map[identity.ContentID][]identity.ContentID, len(rootBodies))
		for body, entries := range rootBodies {
			selectedBodies[body] = entries
		}
		for changed := true; changed; {
			changed = false
			for callIndex := 0; callIndex < mount.artifact.CallCount(); callIndex++ {
				call, callOK := mount.artifact.CallAt(callIndex)
				target, targetOK := call.DirectTargetBody()
				if !callOK || !targetOK {
					continue
				}
				if _, ownerSelected := selectedBodies[call.BodyID()]; !ownerSelected {
					continue
				}
				if _, already := selectedBodies[target]; already {
					continue
				}
				entries, known := bodyEntries[target]
				if _, isCallable := callable[target]; !known || !isCallable {
					return nil, false
				}
				selectedBodies[target] = entries
				changed = true
			}
		}
		pointIDs := make(map[identity.ContentID]struct{}, mount.artifact.PointCount())
		for index := 0; index < mount.artifact.PointCount(); index++ {
			point, ok := mount.artifact.PointAt(index)
			if !ok || !point.Available() || !point.ID().Available() {
				return nil, false
			}
			pointIDs[point.ID()] = struct{}{}
		}
		observed := make(map[identity.ContentID]struct{})
		observedBodies := make(map[identity.ContentID]struct{}, len(selectedBodies))
		for occurrenceIndex := 0; occurrenceIndex < mount.artifact.OccurrenceCount(); occurrenceIndex++ {
			occurrence, occurrenceOK := mount.artifact.OccurrenceAt(occurrenceIndex)
			body, bodyOK := occurrence.BodyID()
			if !occurrenceOK || !occurrence.Available() {
				return nil, false
			}
			if !bodyOK {
				continue
			}
			if _, selected := selectedBodies[body]; !selected {
				continue
			}
			for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
				point, pointOK := occurrence.PointAt(pointIndex)
				if !pointOK || !point.Available() {
					return nil, false
				}
				if _, known := pointIDs[point]; !known {
					continue
				}
				observed[point] = struct{}{}
				observedBodies[body] = struct{}{}
			}
		}
		// A selected body with no semantic occurrence still needs one exact
		// empty observation anchor. Use its Program-issued entry attachment,
		// never a point from an unselected callable body.
		for body, entries := range selectedBodies {
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
			for _, family := range selectedFamilies {
				role, roleOK := artifactQueryRoleOf(family.Projection)
				id, idOK := identity.DeriveContentID("analysis/artifact-query/v1", mount.moduleKey[:], pointID[:], []byte(family.Family))
				if !roleOK || !idOK {
					return nil, false
				}
				plan.rows = append(plan.rows, artifactQueryAttachment{
					id: id, mount: mount.moduleKey, point: pointID,
					role: role, authority: family.Authority, projection: family.Projection,
				})
			}
		}
	}
	return plan, expected > 0 && len(plan.rows) == len(selectedFamilies)*expected
}

// selectedPointQueryIssuance is the sealed families asked at selected Artifact
// points, in catalog order. Construction walks this list; it does not restate
// a family name.
func selectedPointQueryIssuance() ([]composite.IssuedQuery, bool) {
	issued := composite.QueryIssuance()
	if len(issued) == 0 {
		return nil, false
	}
	selected := make([]composite.IssuedQuery, 0, len(issued))
	for _, family := range issued {
		if family.Population != query.PopulationSelectedPoint {
			continue
		}
		if !family.Family.Available() || !family.Authority.Available() || !family.Projection.Available() {
			return nil, false
		}
		if _, ok := artifactQueryRoleOf(family.Projection); !ok {
			return nil, false
		}
		selected = append(selected, family)
	}
	return selected, len(selected) > 0
}

func artifactQueryRoleOf(projection schema.Key) (artifactQueryRole, bool) {
	switch projection {
	case query.ProjectionSummary:
		return artifactQueryValueSummary, true
	case query.ProjectionExact:
		return artifactQueryEffectExact, true
	default:
		return 0, false
	}
}

// AddRows emits query rows inside the assembly's query batch scope.
func (plan *artifactQueryPlan) AddRows(batch *engine.MountedQueryBatch, binding *composite.ProgramBinding) bool {
	if plan == nil || batch == nil || binding == nil || len(plan.rows) == 0 {
		return false
	}
	for _, row := range plan.rows {
		cell, held := binding.Query(row.authority)
		if !held {
			return false
		}
		var ok bool
		switch row.projection {
		case query.ProjectionSummary:
			implementation, recovered := query.Payload[*engine.SummaryQueryImplementation[valuedomain.Value, valuedomain.ValueSummaryObservation]](cell)
			ok = recovered && engine.AddMountedSummaryQuery(batch, implementation, row.id, row.mount, row.point)
		case query.ProjectionExact:
			implementation, recovered := query.Payload[*engine.ExactQueryImplementation[factor.Value, factor.EffectObservation]](cell)
			ok = recovered && engine.AddMountedExactQuery(batch, implementation, row.id, row.mount, row.point)
		}
		if !ok {
			return false
		}
	}
	return true
}

// Attach binds every query row to the existing Link-local implementation
// receipts. Rule dispatch remains a separate attachment lane.
func (plan *artifactQueryPlan) Attach(compilation *engine.ProgramConstruction, graph *engine.ReceiptGraph, binding *composite.ProgramBinding) bool {
	if plan == nil || compilation == nil || graph == nil || binding == nil || len(plan.rows) == 0 {
		return false
	}
	for _, row := range plan.rows {
		cell, held := binding.Query(row.authority)
		if !held {
			return false
		}
		var ok bool
		switch row.projection {
		case query.ProjectionSummary:
			implementation, recovered := query.Payload[*engine.SummaryQueryImplementation[valuedomain.Value, valuedomain.ValueSummaryObservation]](cell)
			ok = recovered && engine.AttachSummaryQuery(compilation, implementation, row.id)
		case query.ProjectionExact:
			implementation, recovered := query.Payload[*engine.ExactQueryImplementation[factor.Value, factor.EffectObservation]](cell)
			ok = recovered && engine.AttachExactQuery(compilation, implementation, row.id)
		}
		if !ok {
			return false
		}
	}
	return true
}

// Publications resolves the Engine-owned attachment identities once, before
// Result projection. No ReceiptQuery crosses the Result boundary.
func (plan *artifactQueryPlan) Publications(graph *engine.ReceiptGraph) ([]artifactQueryPublication, bool) {
	if plan == nil || graph == nil || len(plan.rows) == 0 {
		return nil, false
	}
	rows := make([]artifactQueryPublication, len(plan.rows))
	for index, attachment := range plan.rows {
		query, ok := graph.Query(attachment.id)
		if !ok {
			return nil, false
		}
		key, keyed := query.PublicationKey()
		if !keyed {
			return nil, false
		}
		rows[index] = artifactQueryPublication{attachment: attachment, key: key}
	}
	return rows, true
}

// Keep the concrete query result types in this file's dependency surface so
// the lane remains tied to the existing ProgramBinding implementations.
var _ *engine.SummaryQueryImplementation[valuedomain.Value, valuedomain.ValueSummaryObservation]
var _ *engine.ExactQueryImplementation[factor.Value, factor.EffectObservation]
