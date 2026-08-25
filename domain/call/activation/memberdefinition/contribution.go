// Package memberdefinition declares Call activation's generated relation and
// reducer vocabulary. It is generator-only; runtime code consumes the sealed
// catalog and direct generated calls.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	callPackagePath     = "github.com/wippyai/go-lua/domain/call"
	identityPackagePath = "github.com/wippyai/go-lua/analysis/identity"
	branchPackagePath   = "github.com/wippyai/go-lua/domain/call/activation/branch"
)

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func ownerMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: callPackagePath, Name: name,
		Receiver: goType(callPackagePath, "Algebra"), ReceiverPointer: true, ResultIndex: result,
	}
}

func coordinateMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: callPackagePath, Name: name, Receiver: goType(callPackagePath, "CallCoordinate"), ResultIndex: result}
}

// A projection accessor answers one value and its verdict, which is the sole
// result convention: ResultIndex -1 binds one name.
func bodyMethod(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: callPackagePath, Name: name, Receiver: goType(callPackagePath, "Body"), ResultIndex: -1}
}

func branchProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("call"), Member: "call/activation/branches"})
}

func triggerProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"})
}

// Contribution is the whole Call-owned activation vocabulary: the branch set a
// mounted call row carries, the identities each branch is mounted by, and the
// structural fold that settles one of them.
//
// The branch set is a nested member set of the mounted-call directory and is
// ENUMERATED, never read. A branch carries no Call fact any judgment consumes
// - the trigger's own value and the branch's identity settle it - and a branch
// is a body rather than a call site, so it has no coordinate to be read at.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "call",
		Rule: "call-activation",
		Carriers: []definition.Carrier{
			{Name: "CallActivationBranchCarrier", Key: "carrier/call/activation-branch", Type: goType(callPackagePath, "Body")},
			{Name: "CallActivationBranchOrdinalCarrier", Key: "carrier/call/activation-branch-ordinal", Type: definition.GoType{Name: "uint32"}},
			{Name: "CallActivationAxisCarrier", Key: "carrier/call/activation-axis", Type: goType(identityPackagePath, "SemanticKey")},
			{Name: "CallActivationModuleCarrier", Key: "carrier/call/activation-module", Type: goType(identityPackagePath, "ContentID")},
		},
		Relations: []definition.Relation{{
			Name:    "CallActivationBranches",
			Key:     "call/activation/branches",
			Subject: "CallActivationBranchCarrier",
			// Self-provided: the set's rows are addressed by its own directory,
			// which is Call's canonical body order.
			CandidateProvider: branchProvider(),
			// A branch IS a body, and a body is named by its module and the
			// path it occupies there - the same pair this directory's rows
			// publish, so the inverse and the projections agree.
			CandidateResolver: ownerMethod("ActivationBranchForOccurrence", 0),
			CandidateOrdinal:  ownerMethod("ActivationBranchOrdinal", 0),
			CandidateAt:       ownerMethod("ActivationBranchAt", 0),
			// The set hangs off one mounted call row, and its members are
			// reached by the ordinal that row addresses them at.
			MemberParent:  member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"},
			MemberOrdinal: "CallActivationBranchOrdinalCarrier",
			MemberCount:   coordinateMethod("ActivationBranchCount", 0),
			MemberAt:      coordinateMethod("ActivationBranchAt", 0),
		}},
		Projections: []definition.Projection{
			{
				Name: "CallActivationApplication", Key: "call/activation/application",
				Relation: "MountedCallCandidates", Role: member.Identity, Result: "CallActivationAxisCarrier",
				CandidateProvider: triggerProvider(), Accessor: coordinateMethod("ActivationApplication", -1),
			},
			{
				Name: "CallActivationTarget", Key: "call/activation/target",
				Relation: "CallActivationBranches", Role: member.Identity, Result: "CallActivationAxisCarrier",
				CandidateProvider: branchProvider(), Accessor: bodyMethod("ActivationTarget"),
			},
			{
				Name: "CallActivationEndpoint", Key: "call/activation/endpoint",
				Relation: "CallActivationBranches", Role: member.Identity, Result: "CallActivationAxisCarrier",
				CandidateProvider: branchProvider(), Accessor: bodyMethod("ActivationEndpoint"),
			},
			{
				Name: "CallActivationMount", Key: "call/activation/mount",
				Relation: "CallActivationBranches", Role: member.Identity, Result: "CallActivationModuleCarrier",
				CandidateProvider: branchProvider(), Accessor: bodyMethod("ModuleKey"),
			},
			{
				Name: "CallActivationBody", Key: "call/activation/body",
				Relation: "CallActivationBranches", Role: member.Identity, Result: "CallActivationModuleCarrier",
				CandidateProvider: branchProvider(), Accessor: bodyMethod("BodyPath"),
			},
		},
		Reducers: []definition.Reducer{{
			Name:      "CallActivationReducer",
			Key:       "call/activation/reducer",
			Candidate: "CallCoordinateCarrier",
			// A structural fold publishes no fact: its whole result is the
			// disposition of the branch it was invoked for.
			Structural: true,
			Inputs: []definition.ReducerInput{{
				Axis:         axisReference("call"),
				Carrier:      "CallFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			}},
			Derivation: definition.ReducerDerivation{
				State:      goType(branchPackagePath, "Selector"),
				Build:      definition.GoSymbol{PackagePath: branchPackagePath, Name: "Derive", ResultIndex: 0},
				StaticAxes: []schema.EntryReference{axisReference("call")},
			},
			Implementation: definition.GoSymbol{
				PackagePath: branchPackagePath, Name: "Settle",
				Receiver: goType(branchPackagePath, "Selector"), ResultIndex: 0,
			},
		}},
	}
}
