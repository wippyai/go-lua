package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/engine"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	publication "github.com/wippyai/go-lua/domain/composite/publication"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Publications derives Snapshot observation addresses from sealed branch
// geometry. The identity is the same one Attach writes; detach does not retain
// a second publication table. ObservationKey.Point is the Program evidence
// anchor; Key is the execution-point publication address.
func Publications(branches []Observation) ([]ObservationKey, bool) {
	family, familyOK := composite.ObservationProducerForPopulationKind(structure.DiagnosticObservationBranchCondition.Key())
	if !familyOK {
		return nil, false
	}
	rows := make([]ObservationKey, 0)
	seen := make(map[pointKey]struct{})
	for _, observation := range branches {
		if observation.Kind != structure.DiagnosticObservationBranchCondition || !observation.Mount.Available() || len(observation.Points) == 0 || len(observation.Producers) == 0 {
			return nil, false
		}
		for _, producer := range observation.Producers {
			execution := pointKey{mount: observation.Mount, point: producer.Point}
			if !execution.mount.Available() || !execution.point.Available() || !producer.Anchor.Available() {
				return nil, false
			}
			if _, duplicate := seen[execution]; duplicate {
				continue
			}
			key, keyed := publication.BranchValueObservationID(execution.mount, producer.Point, family)
			if !keyed {
				return nil, false
			}
			seen[execution] = struct{}{}
			rows = append(rows, ObservationKey{Mount: observation.Mount, Point: producer.Anchor, Key: key})
		}
	}
	return rows, true
}

// QuerySummaries indexes QueryFamily value-summary rows by mount and point.
func QuerySummaries(queries []composite.QueryPublication) ([]QuerySummaryKey, bool) {
	summaries := make([]QuerySummaryKey, 0, len(queries))
	seen := make(map[pointKey]struct{}, len(queries))
	for _, query := range queries {
		if query.Site.Projection != queryschema.ProjectionSummary || !query.Site.Mount.Available() || !query.Site.Point.Available() || !query.Key.Available() {
			continue
		}
		key := pointKey{mount: query.Site.Mount, point: query.Site.Point}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		summaries = append(summaries, QuerySummaryKey{Mount: query.Site.Mount, Point: query.Site.Point, Key: query.Key})
	}
	return summaries, true
}

// BranchValueObservations states the one solve-local Value read shared by
// native publication and optional diagnostics, as the observation inventory a
// seal binds. Static observations never enter this function. Diagnostic flags
// control only report collectors; they cannot create a second observation
// authority or alter native facts.
//
// One evidence point declares one row. A second producer naming the same
// execution point reauthenticates its own coordinates against the committed
// program and adds no row, so the inventory holds exactly one observation per
// published address.
func BranchValueObservations(committed *engine.CommittedProgram, binding *composite.ProgramBinding, branches []Observation) ([]engine.ProgramObservationAdmission, engine.SolveFailure, bool) {
	if committed == nil || binding == nil || binding.ValueQuery() == nil {
		return nil, engine.ObservationSealArguments(), false
	}
	family, familyOK := composite.ObservationProducerForPopulationKind(structure.DiagnosticObservationBranchCondition.Key())
	if !familyOK {
		return nil, engine.ObservationSealArguments(), false
	}
	rules := binding.Rules()
	if rules == nil {
		return nil, engine.ObservationSealArguments(), false
	}
	declared := make([]engine.ProgramObservationAdmission, 0)
	seen := make(map[pointKey]publication.BranchValueObservationAttachment)
	for _, observation := range branches {
		if observation.Kind != structure.DiagnosticObservationBranchCondition || !observation.Mount.Available() || len(observation.Points) == 0 || len(observation.Producers) == 0 {
			return nil, engine.ObservationSealArguments(), false
		}
		for _, producer := range observation.Producers {
			execution := pointKey{mount: observation.Mount, point: producer.Point}
			if !execution.mount.Available() || !execution.point.Available() || !producer.Anchor.Available() {
				return nil, engine.ObservationSealArguments(), false
			}
			role, roleOK := rules.CapabilityByKey(producer.Key)
			if !roleOK || !role.Mounted() {
				return nil, engine.ObservationSealPoint(), false
			}
			if _, duplicate := seen[execution]; duplicate {
				if !publication.MemberPublished(committed, role, execution.mount, producer.Point, producer.Occurrence) {
					return nil, engine.ObservationSealPoint(), false
				}
				continue
			}
			attachment, failure, stated := publication.DeclareBranchValueObservation(committed, binding.ValueQuery(), role, family, execution.mount, producer.Point, producer.Occurrence)
			if !stated {
				return nil, failure, false
			}
			row, rowOK := attachment.Observation()
			if _, keyed := attachment.ContentID(); !keyed || !rowOK {
				return nil, engine.ObservationSealPoint(), false
			}
			seen[execution] = attachment
			declared = append(declared, row)
		}
	}
	return declared, engine.SolveFailure{}, true
}

// BindConditionCoordinates validates the Value-schema coordinate already
// carried by each projected observation. ProjectSites obtains that coordinate
// from the sealed mounted ObservationSite.ValueID; this boundary must not
// reconstruct it from producer occurrences, which are execution geometry and
// not a second identity authority.
func BindConditionCoordinates(branches []Observation, schema *valuedomain.Schema) bool {
	if schema == nil {
		return true
	}
	for _, observation := range branches {
		if observation.Kind != structure.DiagnosticObservationBranchCondition {
			continue
		}
		if uint64(observation.ValueIndex) >= uint64(schema.CoordinateCount()) {
			return false
		}
	}
	return true
}
