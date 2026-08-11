package engine

// SolveFailureReason identifies the first engine lifecycle boundary that made
// a solve incomplete.  It is deliberately a closed enum: callers receive no
// implementation error text, mutable runtime object, or second diagnostic
// authority.
type SolveFailureReason uint8

const (
	SolveFailureReasonNone SolveFailureReason = iota
	SolveFailureReasonEpoch
	// Activation relation merge failed or produced no new delta.
	SolveFailureReasonActivationMerge
	// The accepted activation revision could not compile.
	SolveFailureReasonActivationCompile
	// The solver activation revision counter is exhausted.
	SolveFailureReasonActivationRevisionOverflow
	// Evicting the prior retained publication failed.
	SolveFailureReasonActivationRetainedClose
	SolveFailureReasonExecution
	SolveFailureReasonQuery
	SolveFailureReasonPublication
)

// SolveFailurePhase identifies the engine lifecycle phase that made a solve
// incomplete. It covers both cold/runtime compilation and bound RuleMember
// execution. It carries no typed/domain diagnostic; the zero value is used
// when no more specific phase was reached.
type SolveFailurePhase uint8

const (
	SolveFailurePhaseNone SolveFailurePhase = iota
	// The sealed cold input or accepted activation graph was invalid.
	SolveFailurePhaseCompileValidation
	// The accepted graph could not produce an operand revision.
	SolveFailurePhaseCompileOperandRevision
	// Factor binding or runtime composition preparation failed.
	SolveFailurePhaseCompileComposition
	// A graph member could not bind to its sealed rule schema.
	SolveFailurePhaseCompileMemberBinding
	// A graph query could not bind to its sealed query schema.
	SolveFailurePhaseCompileQueryBinding
	// The bound rows could not be assembled into the runtime.
	SolveFailurePhaseCompileRuntimeAssembly
	SolveFailurePhasePreflight
	SolveFailurePhaseTransfer
	SolveFailurePhaseCheckpoint
	SolveFailurePhaseDerivation
	SolveFailurePhaseAdmission
	SolveFailurePhasePublication
)

// SolveReport is a detached first-failure certificate for one incomplete
// solve.  Every coordinate is an opaque SemanticKey copied out of the sealed
// engine authority.  The zero value means that no incomplete solve was
// reported; all fields are private so the report cannot be forged or retain a
// Solver, State, callback, domain value, or mutable slice.
type SolveReport struct {
	reason SolveFailureReason
	phase  SolveFailurePhase
	point  SemanticKey
	group  SemanticKey
	member SemanticKey
	rule   SemanticKey
}

// Available reports whether this report is a certificate for SolveIncomplete.
func (report SolveReport) Available() bool { return report.reason != SolveFailureReasonNone }

// Reason returns the first lifecycle boundary recorded by the solver.
func (report SolveReport) Reason() SolveFailureReason { return report.reason }

// Phase returns the engine lifecycle phase that failed. It is None when the
// report identifies a boundary without a more specific phase.
func (report SolveReport) Phase() SolveFailurePhase { return report.phase }

// Point returns the failed Point semantic identity when the boundary had one.
func (report SolveReport) Point() SemanticKey { return report.point }

// Group returns the failed Group semantic identity when the boundary had one.
func (report SolveReport) Group() SemanticKey { return report.group }

// Member returns the failed RuleMember semantic identity when member execution
// reached one.
func (report SolveReport) Member() SemanticKey { return report.member }

// Rule returns the failed Rule semantic identity when member execution reached
// one.
func (report SolveReport) Rule() SemanticKey { return report.rule }

func (report *SolveReport) record(reason SolveFailureReason, phase SolveFailurePhase, point, group, member, rule SemanticKey) {
	if report == nil || report.Available() || reason == SolveFailureReasonNone {
		return
	}
	report.reason = reason
	report.phase = phase
	report.point = point
	report.group = group
	report.member = member
	report.rule = rule
}

func reportFailureQuery(report *SolveReport, reason SolveFailureReason, point SemanticKey) {
	if report != nil {
		report.record(reason, SolveFailurePhaseNone, point, SemanticKey{}, SemanticKey{}, SemanticKey{})
	}
}
