package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

// programObservationAdmit is the erased optional-observation row. Its
// implementation lives on the sealed query cell that materializes the answer;
// the inventory states the coordinates and the committed program binds them.
type programObservationAdmit interface {
	bindProgramObservation(plane *programPlane, id identity.ContentID, member equation.RuleMember, point equation.Point) (observationRow, bool)
}

// ProgramObservationAdmission is one optional observation to bind: the
// snapshot identity it answers under and the mounted member coordinates it
// reads. The typed query implementation stays sealed inside the row.
type ProgramObservationAdmission struct {
	admit      programObservationAdmit
	ID         identity.ContentID
	Role       RuleSlotCapability
	Mount      identity.ContentID
	Point      identity.ContentID
	Occurrence identity.ContentID
}

// Available reports whether this row states a complete observation.
func (admission ProgramObservationAdmission) Available() bool {
	return admission.admit != nil && admission.ID.Available() && admission.Role.mounted() &&
		admission.Mount.Available() && admission.Point.Available() && admission.Occurrence.Available()
}

// NewSummaryObservationAdmission seals one summary observation row against the
// mounted member at the authored coordinates.
func NewSummaryObservationAdmission[V, R any](implementation *SummaryQueryImplementation[V, R], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID) (ProgramObservationAdmission, bool) {
	if implementation == nil {
		return ProgramObservationAdmission{}, false
	}
	admission := ProgramObservationAdmission{admit: implementation, ID: id, Role: role, Mount: mount, Point: point, Occurrence: occurrence}
	return admission, admission.Available()
}

// NewExactObservationAdmission seals one exact observation row against the
// mounted member at the authored coordinates.
func NewExactObservationAdmission[V, R any](implementation *ExactQueryImplementation[V, R], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID) (ProgramObservationAdmission, bool) {
	if implementation == nil {
		return ProgramObservationAdmission{}, false
	}
	admission := ProgramObservationAdmission{admit: implementation, ID: id, Role: role, Mount: mount, Point: point, Occurrence: occurrence}
	return admission, admission.Available()
}

// bindProgramObservation lowers one optional observation to a runtimeProgram
// row using the query implementation's sealed schema row.
func (implementation *SummaryQueryImplementation[V, R]) bindProgramObservation(plane *programPlane, id identity.ContentID, member equation.RuleMember, point equation.Point) (observationRow, bool) {
	return bindSummaryObservationRow(plane, implementation, id, member, point)
}

func (implementation *ExactQueryImplementation[V, R]) bindProgramObservation(plane *programPlane, id identity.ContentID, member equation.RuleMember, point equation.Point) (observationRow, bool) {
	return bindExactObservationRow(plane, implementation, id, member, point)
}

// observationSealFailure is the closed generic admission predicate
// for an optional observation. It contains no diagnostic rule or domain name.
type observationSealFailure uint8

const (
	observationSealFailureNone observationSealFailure = iota
	observationSealFailureArguments
	observationSealFailureCompilation
	observationSealFailureBinding
	observationSealFailureProjection
	observationSealFailurePoint
	observationSealFailureMapping
	observationSealFailureFactor
	observationSealFailureUnit
	observationSealFailureDuplicate
)

func (failure observationSealFailure) Failure() SolveFailure {
	if failure == observationSealFailureNone {
		return SolveFailure{}
	}
	return boundaryFailure(SolveFailureFamilyObservation, "observation-seal", uint64(failure))
}

// ProgramCallStage is the committed native Call stage handle.
type ProgramCallStage struct {
	handle programCallRow
}

func (stage ProgramCallStage) Available() bool { return stage.handle.Available() }

func (stage ProgramCallStage) Kind() rows.ArtifactRuleStage { return stage.handle.Stage() }

func (stage ProgramCallStage) MountID() identity.ContentID { return stage.handle.MountID() }

func (stage ProgramCallStage) OccurrenceID() identity.ContentID {
	return stage.handle.OccurrenceID()
}

func (stage ProgramCallStage) PointID() identity.ContentID {
	return stage.handle.ReusablePointID()
}

func (stage ProgramCallStage) HasMember() bool {
	_, ok := stage.handle.RuleMember()
	return ok
}
