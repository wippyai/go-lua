package transformer

import (
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// boundaryPrefixStep is the canonical semantic transaction lowered from one
// relationCode step.  It is shared by the forest coordinate builder and its
// factor/access certificates; it is not an executable program or schedule.
type boundaryPrefixStep struct {
	kind           boundaryPrefixStepKind
	point          cfg.Point
	effect         EffectTerm
	slot           statekey.Value
	value          ValueTerm
	access         []valueAccessTerm
	operands       callOutcomeOperandTerms
	writes         []ValueTerm
	memberCall     boundaryMemberCallDiagnosticTerm
	contribution   semanticContribution
	branch         factapply.BranchRelationTransaction
	result         factapply.CallResultTransaction
	resultPhase    factapply.ConcreteCallResultPhase
	presence       factapply.PathValuePresenceImplicationTransaction
	channel        factapply.ChannelSelectTransaction
	covariant      factapply.CovariantExposureTransaction
	rootAssignment rootAssignmentTerm
}

type boundaryPrefixStepKind uint8

const (
	boundaryPrefixInvalid boundaryPrefixStepKind = iota
	boundaryPrefixEffect
	boundaryPrefixExternalCall
	boundaryPrefixRootAssignment
	boundaryPrefixWrite
	boundaryPrefixGenericFor
	boundaryPrefixContribution
	boundaryPrefixBranchRelations
	boundaryPrefixCallResults
	boundaryPrefixPresenceImplications
	boundaryPrefixChannelSelect
	boundaryPrefixCovariantExposure
	// boundaryPrefixPredicateRefinement names the Choice backward transformer
	// in the same coordinate-operation vocabulary. It is not relation syntax;
	// the relation node owns the guard and emits this exact coordinate law.
	boundaryPrefixPredicateRefinement
)

// boundaryTerminalTransaction is the canonical normal/nonreturning terminal
// payload consumed by guarded forest publication.  It carries no solver state.
type boundaryTerminalTransaction struct {
	kind    boundaryTerminalKind
	source  boundaryOutcomeRef
	outcome boundaryOutcomeTuple
	result  factapply.CallResultTransaction
}

type boundaryTerminalKind uint8

const (
	boundaryTerminalInvalid boundaryTerminalKind = iota
	boundaryTerminalNonreturning
	boundaryTerminalNormal
)
