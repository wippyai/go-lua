package engine

import (
	"encoding/hex"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/canonical"
)

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

// SolveFailureFamily names the engine lifecycle stage a solve stopped in. It
// is the whole rendered classification a caller receives; the exact internal
// boundary reached inside that stage travels beside it as an opaque identity,
// so the engine refines its own boundaries without moving public vocabulary.
type SolveFailureFamily uint8

const (
	SolveFailureFamilyNone SolveFailureFamily = iota
	// Cold seal, binding, and runtime assembly of the accepted graph.
	SolveFailureFamilyCompile
	// Bound member and activation execution of one compiled rule.
	SolveFailureFamilyExecution
	// Point refresh: candidate order, acyclic fold, and Region transition.
	SolveFailureFamilyRefresh
	// Executor traversal, postfix discharge, and narrow scheduling.
	SolveFailureFamilySchedule
	// Optional read-only observation of a settled Factor.
	SolveFailureFamilyObservation
)

func (family SolveFailureFamily) String() string {
	switch family {
	case SolveFailureFamilyCompile:
		return "compile"
	case SolveFailureFamilyExecution:
		return "execution"
	case SolveFailureFamilyRefresh:
		return "refresh"
	case SolveFailureFamilySchedule:
		return "schedule"
	case SolveFailureFamilyObservation:
		return "observation"
	default:
		return "none"
	}
}

// SolveFailure is the engine's whole public failure vocabulary: the lifecycle
// family a caller can act on, the universal schema disposition, and the
// opaque identity of the exact internal boundary. Site is a framed digest, so
// the executor keeps full internal precision without any internal name
// crossing the API.
type SolveFailure struct {
	Family      SolveFailureFamily
	Disposition schema.Disposition
	Site        identity.ContentID
}

// Available reports whether this value classifies a failure.
func (failure SolveFailure) Available() bool { return failure.Family != SolveFailureFamilyNone }

// String renders the family, the disposition, and the leading bytes of the
// site digest. The prefix separates boundaries within a family without naming
// any of them.
func (failure SolveFailure) String() string {
	if !failure.Available() {
		return "none"
	}
	return failure.Family.String() + "/" + failure.Disposition.String() + "@" + hex.EncodeToString(failure.Site[:4])
}

// solveBoundary is the engine-internal failure coordinate. The site name and
// the raising authority's own sub-ordinal stay inside this package; a caller
// receives their framed digest and nothing else.
type solveBoundary struct {
	family      SolveFailureFamily
	disposition schema.Disposition
	site        string
	transport   carrier.PointTransportBoundary
}

// boundaryNone is the coordinate of a boundary that was not reached.
var boundaryNone = solveBoundary{}

// refused names one internal boundary whose fence rejected its input.
func refused(family SolveFailureFamily, site string) solveBoundary {
	return solveBoundary{family: family, disposition: schema.DispositionMalformed, site: site}
}

// stalled names one internal boundary that found no defect yet could not
// finish or make progress.
func stalled(family SolveFailureFamily, site string) solveBoundary {
	return solveBoundary{family: family, disposition: schema.DispositionIncomplete, site: site}
}

// withTransport carries carrier's own transport sub-boundary into the site
// identity, so the exact failed substage survives without a second engine
// enum restating carrier's vocabulary.
func (boundary solveBoundary) withTransport(transport carrier.PointTransportBoundary) solveBoundary {
	boundary.transport = transport
	return boundary
}

func (boundary solveBoundary) available() bool { return boundary.family != SolveFailureFamilyNone }

const (
	solveFailureSiteDomain  = "engine/solve-failure-site"
	solveFailureSiteVersion = 1
)

// failure projects the internal coordinate onto the public vocabulary. The
// family, the site name, and the transport sub-ordinal all enter the framed
// preimage, so two boundaries never share an identity.
func (boundary solveBoundary) failure() SolveFailure {
	if !boundary.available() {
		return SolveFailure{}
	}
	site := framedContentID(solveFailureSiteDomain, solveFailureSiteVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Uint(uint64(boundary.family)) == nil &&
			writer.Bytes([]byte(boundary.site)) == nil &&
			writer.Uint(uint64(boundary.transport)) == nil
	})
	return SolveFailure{Family: boundary.family, Disposition: boundary.disposition, Site: site}
}

// boundaryFailure mints one internal boundary onto the same public
// vocabulary. The authority names the engine table an ordinal belongs to and
// the ordinals are that table's own; both stay inside this package, so a
// caller distinguishes two boundaries by their digests alone.
func boundaryFailure(family SolveFailureFamily, authority string, ordinals ...uint64) SolveFailure {
	site := framedContentID(solveFailureSiteDomain, solveFailureSiteVersion, func(writer *canonical.DigestWriter) bool {
		if writer.Uint(uint64(family)) != nil || writer.Bytes([]byte(authority)) != nil || writer.Count(uint64(len(ordinals))) != nil {
			return false
		}
		for _, ordinal := range ordinals {
			if writer.Uint(ordinal) != nil {
				return false
			}
		}
		return true
	})
	return SolveFailure{Family: family, Disposition: schema.DispositionMalformed, Site: site}
}

// compilationSealFailure mints the compile-family boundary for one ordered
// runtime seal phase. The phase ordinal enters the site preimage.
func compilationSealFailure(phase uint64) SolveFailure {
	// Runtime sealing and the public ProgramSealStage table are distinct
	// authorities.  Keeping their ordinals in separate digest namespaces is
	// what prevents ProgramSealStageOf from misclassifying a runtime phase.
	return boundaryFailure(SolveFailureFamilyCompile, "runtime-program-seal", phase)
}

// ProgramSealFailure is the construction-plane mint for one first seal phase.
func ProgramSealFailure(phase uint64) SolveFailure {
	return compilationSealFailure(phase)
}

// ProgramSealStage names one refusal boundary of the program seal: the pass
// that binds the committed geometry and its declared rows into a Solver.
type ProgramSealStage uint8

const (
	ProgramSealStageNone ProgramSealStage = iota
	// ProgramSealStageAdmission is the program's input fence:
	// the construction handle, the committed topology it is bound against, and
	// the sealed directory that topology published. The mounted artifact
	// schedule gate runs at the topology's commit against artifact rows that are
	// released before construction, so a schedule verdict reaches the
	// constructor as a refused admission of the committed topology.
	ProgramSealStageAdmission
	// ProgramSealStageTopologySeal is the topology commit itself: the
	// mounted artifact schedule gate, the graph materialization, and the
	// directory publication that the admission fence later re-checks.
	ProgramSealStageTopologySeal
	// ProgramSealStageQueryAddress is the published query address table:
	// one canonical key per directory ordinal, resolved against the graph's own
	// queries so no ordinal publishes a foreign or duplicate query and no solved
	// query lacks a column to answer on.
	ProgramSealStageQueryAddress
	// ProgramSealStageObservationAddress is the published observation
	// address table: one row per admitted observation identity, so every issued
	// observation ordinal addresses a sealed row rather than a position past its end.
	ProgramSealStageObservationAddress
	// ProgramSealStageFactorBind is the bound Factor table: the dense
	// owner index and the runtime slot each bound Factor occupies.
	ProgramSealStageFactorBind
	// ProgramSealStageMemberBind is the member bind: the per-Group fold
	// of the cold draft answers and the hot rows minted from those drafts.
	ProgramSealStageMemberBind
	// ProgramSealStageProgramSeal is the seal of the row-model program
	// and the runtime assembled from it.
	ProgramSealStageProgramSeal
	// ProgramSealStageSolverMint is the Solver mint: the initial
	// relation and the one store issuance this Solver's addresses are named in.
	ProgramSealStageSolverMint
	// programSealStageCount bounds the closed stage set. Nothing is
	// declared past it.
	programSealStageCount
)

// programSealAuthority is the engine table the seal stages
// ordinals belong to. It stays inside this package; a caller separates two
// stages by their site digests alone.
const programSealAuthority = "program-seal"

// ProgramStageFailure mints the compile-family boundary for one program seal
// stage. The stage ordinal enters the site preimage.
func ProgramStageFailure(stage ProgramSealStage) SolveFailure {
	if stage <= ProgramSealStageNone || stage >= programSealStageCount {
		return SolveFailure{}
	}
	return boundaryFailure(SolveFailureFamilyCompile, programSealAuthority, uint64(stage))
}

// programSealStages is every stage's site digest, minted once. The
// digest is the only seal coordinate that leaves the engine, so
// recovering the stage is a lookup on it rather than a second published field.
// An unencodable preimage yields the unavailable identity, which no stage may
// occupy: one such entry would make every unavailable site classify as it.
var programSealStages = func() map[identity.ContentID]ProgramSealStage {
	sites := make(map[identity.ContentID]ProgramSealStage, programSealStageCount)
	for stage := ProgramSealStageAdmission; stage < programSealStageCount; stage++ {
		site := ProgramStageFailure(stage).Site
		if !site.Available() {
			continue
		}
		sites[site] = stage
	}
	return sites
}()

// ProgramSealStageOf recovers the seal stage one failure names.
// It reports false for every boundary raised outside the program seal.
func ProgramSealStageOf(failure SolveFailure) (ProgramSealStage, bool) {
	if failure.Family != SolveFailureFamilyCompile || !failure.Site.Available() {
		return ProgramSealStageNone, false
	}
	stage, named := programSealStages[failure.Site]
	return stage, named
}

// SolveReport is a detached first-failure certificate for one incomplete
// solve.  Every coordinate is an opaque identity.SemanticKey copied out of the sealed
// engine authority.  The zero value means that no incomplete solve was
// reported; all fields are private so the report cannot be forged or retain a
// Solver, State, callback, domain value, or mutable slice.
type SolveReport struct {
	reason  SolveFailureReason
	failure SolveFailure
	point   identity.SemanticKey
	group   identity.SemanticKey
	member  identity.SemanticKey
	rule    identity.SemanticKey
}

// Available reports whether this report is a certificate for SolveIncomplete.
func (report SolveReport) Available() bool { return report.reason != SolveFailureReasonNone }

// Reason returns the first lifecycle boundary recorded by the solver.
func (report SolveReport) Reason() SolveFailureReason { return report.reason }

// Failure returns the lifecycle family, disposition, and opaque site identity
// of the boundary that failed. It is the zero value when the report identifies
// a lifecycle reason without reaching a more specific boundary.
func (report SolveReport) Failure() SolveFailure { return report.failure }

// Point returns the failed Point semantic identity when the boundary had one.
func (report SolveReport) Point() identity.SemanticKey { return report.point }

// Group returns the failed Group semantic identity when the boundary had one.
func (report SolveReport) Group() identity.SemanticKey { return report.group }

// Member returns the failed RuleMember semantic identity when member execution
// reached one.
func (report SolveReport) Member() identity.SemanticKey { return report.member }

// Rule returns the failed Rule semantic identity when member execution reached
// one.
func (report SolveReport) Rule() identity.SemanticKey { return report.rule }

func (report *SolveReport) record(reason SolveFailureReason, boundary solveBoundary, point, group, member, rule identity.SemanticKey) {
	if report == nil || report.Available() || reason == SolveFailureReasonNone {
		return
	}
	report.reason = reason
	report.failure = boundary.failure()
	report.point = point
	report.group = group
	report.member = member
	report.rule = rule
}

func reportFailureQuery(report *SolveReport, reason SolveFailureReason, point identity.SemanticKey) {
	if report != nil {
		report.record(reason, boundaryNone, point, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
	}
}
