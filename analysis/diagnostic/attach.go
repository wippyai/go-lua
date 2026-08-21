package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	publication "github.com/wippyai/go-lua/domain/composite/publication"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// producedValue is the population-neutral view of one observation row that
// measures a value at the rule occurrences producing it. A branch condition
// and a type-conformance subject differ in what they conclude, not in where
// the value is read, so both reach the publication boundary through this view.
type producedValue struct {
	mount     identity.ContentID
	family    schema.Key
	producers []Producer
}

// producedValues projects the rows of one population into that view. The
// observation family is the one the sealed declaration table issues for the
// population, so a population without a declared producer is refused here
// rather than borrowing another population's column.
func producedValues(compilation composite.Compilation, rows []Observation) ([]producedValue, bool) {
	if len(rows) == 0 {
		return nil, true
	}
	kind := rows[0].Kind
	family, familyOK := composite.ObservationProducerForPopulationKind(compilation, kind.Key())
	if !familyOK {
		return nil, false
	}
	values := make([]producedValue, 0, len(rows))
	for _, row := range rows {
		producers, geometryOK := rowProducers(row)
		if row.Kind != kind || !geometryOK || !row.Mount.Available() {
			return nil, false
		}
		values = append(values, producedValue{mount: row.Mount, family: family, producers: producers})
	}
	return values, true
}

// rowProducers is the tagged-union read of a row's producer geometry.
func rowProducers(row Observation) ([]Producer, bool) {
	switch row.Kind {
	case structure.DiagnosticObservationBranchCondition:
		return row.Branch.Producers, row.Branch.Available()
	case structure.DiagnosticObservationTypeConformance:
		return row.Conformance.Producers, row.Conformance.Available()
	default:
		return nil, false
	}
}

// ValueObservationAddress is the Snapshot address of the value summary
// observed at one producing occurrence of a population. It is the same
// identity ValueObservations declares and the same one Publications indexes,
// so a collector that holds a row's own producers reads exactly the column the
// seal admitted for them.
func ValueObservationAddress(compilation composite.Compilation, kind structure.DiagnosticObservationKind, mount, point identity.ContentID) (identity.ContentID, bool) {
	family, familyOK := composite.ObservationProducerForPopulationKind(compilation, kind.Key())
	if !familyOK {
		return identity.ContentID{}, false
	}
	return publication.BranchValueObservationID(mount, point, family)
}

// Publications derives Snapshot observation addresses from sealed producer
// geometry. The identity is the same one Attach writes; detach does not retain
// a second publication table. ObservationKey.Point is the Program evidence
// anchor; Key is the execution-point publication address.
func Publications(compilation composite.Compilation, rows []Observation) ([]ObservationKey, bool) {
	values, valuesOK := producedValues(compilation, rows)
	if !valuesOK {
		return nil, false
	}
	keys := make([]ObservationKey, 0, len(values))
	seen := make(map[pointKey]struct{})
	for _, value := range values {
		for _, producer := range value.producers {
			execution := pointKey{mount: value.mount, point: producer.Point}
			if !execution.mount.Available() || !execution.point.Available() || !producer.Anchor.Available() {
				return nil, false
			}
			key, keyed := publication.BranchValueObservationID(execution.mount, producer.Point, value.family)
			if !keyed {
				return nil, false
			}
			if _, duplicate := seen[execution]; duplicate {
				continue
			}
			seen[execution] = struct{}{}
			keys = append(keys, ObservationKey{Mount: value.mount, Point: producer.Anchor, Key: key})
		}
	}
	return keys, true
}

// ValueObservations states the one solve-local Value read shared by native
// publication and optional diagnostics, as the observation inventory a seal
// binds. Static observations never enter this function. Diagnostic flags
// control only report collectors; they cannot create a second observation
// authority or alter native facts.
//
// One evidence point declares one row. A second producer naming the same
// execution point reauthenticates its own coordinates against the committed
// program and adds no row, so the inventory holds exactly one observation per
// published address, whichever population named it first.
func ValueObservations(committed *engine.CommittedProgram, binding *composite.ProgramBinding, populations ...[]Observation) ([]engine.ProgramObservationAdmission, engine.SolveFailure, bool) {
	if committed == nil || binding == nil || binding.ValueQuery() == nil {
		return nil, engine.ObservationSealArguments(), false
	}
	rules := binding.Rules()
	if rules == nil {
		return nil, engine.ObservationSealArguments(), false
	}
	declared := make([]engine.ProgramObservationAdmission, 0)
	seen := make(map[pointKey]struct{})
	for _, population := range populations {
		values, valuesOK := producedValues(binding.Compilation(), population)
		if !valuesOK {
			return nil, engine.ObservationSealArguments(), false
		}
		for _, value := range values {
			for _, producer := range value.producers {
				execution := pointKey{mount: value.mount, point: producer.Point}
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
				attachment, failure, stated := publication.DeclareBranchValueObservation(committed, binding.ValueQuery(), role, value.family, execution.mount, producer.Point, producer.Occurrence)
				if !stated {
					return nil, failure, false
				}
				row, rowOK := attachment.Observation()
				if _, keyed := attachment.ContentID(); !keyed || !rowOK {
					return nil, engine.ObservationSealPoint(), false
				}
				seen[execution] = struct{}{}
				declared = append(declared, row)
			}
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
