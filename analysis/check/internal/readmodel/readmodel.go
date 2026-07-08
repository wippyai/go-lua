package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

// Reader projects solved body boundary values into typed diagnostic read data.
type Reader struct {
	result     *body.Result
	parents    []*body.Result
	typeValues *typevalue.Cache
}

type SourceSpan = readapi.SourceSpan
type CallSite = readapi.CallSite
type CallArgument = readapi.CallArgument
type SendSafety = readapi.SendSafety
type SendSafetyVerdict = readapi.SendSafetyVerdict
type CallArgumentMismatch = readapi.CallArgumentMismatch
type CallArgumentCheck = readapi.CallArgumentCheck
type OptionalAssignmentTarget = readapi.OptionalAssignmentTarget
type UnresolvedTypeReference = readapi.UnresolvedTypeReference
type MissingMemberRead = readapi.MissingMemberRead
type ResultShapeExhaustiveness = readapi.ResultShapeExhaustiveness

const (
	CallArgumentMismatchMayBeNil = readapi.CallArgumentMismatchMayBeNil
	SendSafetyUnknown            = readapi.SendSafetyUnknown
	SendSafetyProvenIsolated     = readapi.SendSafetyProvenIsolated
	SendSafetyProvenImmutable    = readapi.SendSafetyProvenImmutable
)

type CallGenericInferenceConflict = readapi.CallGenericInferenceConflict
type CallGenericInferenceContribution = readapi.CallGenericInferenceContribution
type CallArgumentReport = readapi.CallArgumentReport
type CallArgumentObligation = readapi.CallArgumentObligation
type CallArityReport = readapi.CallArityReport
type CallCalleeReport = readapi.CallCalleeReport
type CallContractSource = readapi.CallContractSource
type Assignment = readapi.Assignment
type AssignmentCheck = readapi.AssignmentCheck
type Return = readapi.Return
type ReturnCheck = readapi.ReturnCheck
type NonNilAssertion = readapi.NonNilAssertion
type NumericForOperand = readapi.NumericForOperand
type ConcatOperand = readapi.ConcatOperand
type FrozenTableMutation = readapi.FrozenTableMutation
type LifecycleObligation = readapi.LifecycleObligation
type UnusedLocal = readapi.UnusedLocal
type DeadAssignment = readapi.DeadAssignment
type DeadAssignmentOverwrite = readapi.DeadAssignmentOverwrite
type DeadAssignmentExit = readapi.DeadAssignmentExit
type ChannelSelectExhaustiveness = readapi.ChannelSelectExhaustiveness
type UnresolvedValueReference = readapi.UnresolvedValueReference
type RedundantConditionBranch = readapi.RedundantConditionBranch
type DominatingBranchProof = readapi.DominatingBranchProof
type RedundantClaim = readapi.RedundantClaim
type AlwaysTrueGuard = readapi.AlwaysTrueGuard
type InvariantLoopRead = readapi.InvariantLoopRead
type SplitBirthDiscriminant = readapi.SplitBirthDiscriminant
type SplitBirthPayloadWrite = readapi.SplitBirthPayloadWrite

func New(result *body.Result) Reader {
	return Reader{result: result, typeValues: result.TypeValues()}
}

func NewWithParents(result *body.Result, parents ...*body.Result) Reader {
	r := New(result)
	for _, parent := range parents {
		if parent != nil {
			r.parents = append(r.parents, parent)
		}
	}
	return r
}
