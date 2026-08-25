package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// programObservationAdmit is the erased optional-observation row. Its
// implementation lives on the sealed query cell that materializes the answer;
// the inventory states the coordinates and the committed program binds them.
type programObservationAdmit interface {
	bindProgramObservation(plane *programPlane, id identity.ContentID, member equation.RuleMember, point equation.Point, context executioncontext.Context, explicit RuleReadSurface, explicitOK bool) (observationRow, bool)
}

// ProgramObservationAdmission is one optional observation to bind: the
// snapshot identity it answers under and the mounted member coordinates it
// reads. The typed query implementation stays sealed inside the row.
type ProgramObservationAdmission struct {
	admit       programObservationAdmit
	memberPoint identity.ContentID
	readPoint   identity.ContentID
	// exactSurface is present only for an owner-issued exact coordinate. It is
	// deliberately not constructible by callers: the explicit constructor
	// below mints it from Ref, and the binder re-checks its authority and Factor
	// against the sealed query row before using it.
	exactSurface   RuleReadSurface
	exactSurfaceOK bool
	ID             identity.ContentID
	Role           RuleSlotCapability
	Mount          identity.ContentID
	Point          identity.ContentID
	Occurrence     identity.ContentID
	Context        executioncontext.Context
}

// Available reports whether this row states a complete observation.
func (admission ProgramObservationAdmission) Available() bool {
	return admission.admit != nil && admission.ID.Available() && admission.Role.mounted() &&
		admission.Mount.Available() && admission.memberPoint.Available() && admission.Point.Available() &&
		(!admission.readPoint.Available() || admission.Point == admission.readPoint) &&
		admission.Occurrence.Available() && admission.Context.Available()
}

// NewSummaryObservationAdmission seals one summary observation row against the
// mounted member at the authored coordinates.
func NewSummaryObservationAdmission[V, R any](implementation *SummaryQueryImplementation[V, R], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID, context executioncontext.Context) (ProgramObservationAdmission, bool) {
	if implementation == nil {
		return ProgramObservationAdmission{}, false
	}
	admission := ProgramObservationAdmission{admit: implementation, memberPoint: point, ID: id, Role: role, Mount: mount, Point: point, Occurrence: occurrence, Context: context}
	return admission, admission.Available()
}

// NewExactObservationAdmission seals one exact observation row against the
// mounted member at the authored coordinates.
func NewExactObservationAdmission[V, R any](implementation *ExactQueryImplementation[V, R], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID, context executioncontext.Context) (ProgramObservationAdmission, bool) {
	if implementation == nil {
		return ProgramObservationAdmission{}, false
	}
	admission := ProgramObservationAdmission{admit: implementation, memberPoint: point, ID: id, Role: role, Mount: mount, Point: point, Occurrence: occurrence, Context: context}
	return admission, admission.Available()
}

// NewExactObservationAdmissionAt seals an exact observation against an
// owner-issued Factor coordinate. This is the routed-write path: the caller
// supplies only the opaque Ref minted by that Factor owner, never a raw dense
// coordinate or an engine RuleReadSurface. The query binder still authenticates
// the resulting surface against the query's sealed binding and Factor.
func NewExactObservationAdmissionAt[K ~uint32 | ~uint64, V, R any](implementation *ExactQueryImplementation[V, R], ref Ref[K], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID, context executioncontext.Context) (ProgramObservationAdmission, bool) {
	if implementation == nil {
		return ProgramObservationAdmission{}, false
	}
	surface, surfaceOK := ExactReadSurface(ref)
	if !surfaceOK {
		return ProgramObservationAdmission{}, false
	}
	admission := ProgramObservationAdmission{
		admit: implementation, memberPoint: point, exactSurface: surface, exactSurfaceOK: true,
		ID: id, Role: role, Mount: mount, Point: point, Occurrence: occurrence, Context: context,
	}
	return admission, admission.Available()
}

// NewSummaryCallInputObservationAdmission admits a summary query over the
// authenticated input state of one committed Call stage.
func NewSummaryCallInputObservationAdmission[V, R any](implementation *SummaryQueryImplementation[V, R], id identity.ContentID, stage ProgramCallStage, context executioncontext.Context) (ProgramObservationAdmission, bool) {
	if implementation == nil {
		return ProgramObservationAdmission{}, false
	}
	return newCallInputObservationAdmission(implementation, id, stage, context)
}

// NewExactCallInputObservationAdmission admits an exact query over the
// authenticated input state of one committed Call stage.
func NewExactCallInputObservationAdmission[V, R any](implementation *ExactQueryImplementation[V, R], id identity.ContentID, stage ProgramCallStage, context executioncontext.Context) (ProgramObservationAdmission, bool) {
	if implementation == nil {
		return ProgramObservationAdmission{}, false
	}
	return newCallInputObservationAdmission(implementation, id, stage, context)
}

func newCallInputObservationAdmission(admit programObservationAdmit, id identity.ContentID, stage ProgramCallStage, context executioncontext.Context) (ProgramObservationAdmission, bool) {
	if admit == nil || !stage.Available() {
		return ProgramObservationAdmission{}, false
	}
	admission := ProgramObservationAdmission{
		admit:       admit,
		memberPoint: stage.PointID(),
		readPoint:   stage.handle.stage.mountedInput,
		ID:          id,
		Role:        stage.handle.key.role,
		Mount:       stage.MountID(),
		// Point is the canonical read coordinate for this observation. The
		// reusable predecessor remains available through ProgramCallStage, but
		// the sealed observation row must carry the exact mounted identity that
		// lookupPoint authenticates.
		Point:      stage.handle.stage.mountedInput,
		Occurrence: stage.OccurrenceID(),
		Context:    context,
	}
	return admission, admission.Available()
}

// bindProgramObservation lowers one optional observation to a runtimeProgram
// row using the query implementation's sealed schema row.
func (implementation *SummaryQueryImplementation[V, R]) bindProgramObservation(plane *programPlane, id identity.ContentID, member equation.RuleMember, point equation.Point, context executioncontext.Context, _ RuleReadSurface, _ bool) (observationRow, bool) {
	return bindSummaryObservationRow(plane, implementation, id, member, point, context)
}

func (implementation *ExactQueryImplementation[V, R]) bindProgramObservation(plane *programPlane, id identity.ContentID, member equation.RuleMember, point equation.Point, context executioncontext.Context, explicit RuleReadSurface, explicitOK bool) (observationRow, bool) {
	return bindExactObservationRow(plane, implementation, id, member, point, context, explicit, explicitOK)
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

func (stage ProgramCallStage) Kind() schema.Key { return stage.handle.Stage() }

func (stage ProgramCallStage) MountID() identity.ContentID { return stage.handle.MountID() }

func (stage ProgramCallStage) OccurrenceID() identity.ContentID {
	return stage.handle.OccurrenceID()
}

func (stage ProgramCallStage) PointID() identity.ContentID {
	return stage.handle.ReusablePointID()
}

// InputPointID returns the authenticated predecessor point of this native
// Call stage. It is the point the stage declaration reads; consumers that
// decide an effect from pre-effect evidence must observe this coordinate
// rather than reconstructing a predecessor from stage order.
func (stage ProgramCallStage) InputPointID() identity.ContentID {
	return stage.handle.ReusableInputPointID()
}

func (stage ProgramCallStage) HasMember() bool {
	_, ok := stage.handle.RuleMember()
	return ok
}
