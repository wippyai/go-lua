package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

// This file declares the rows both call-result transfers are addressed
// through. Two rules answer the first result of one mounted call - the Target
// alias and the executable body - and they are indexed by one directory,
// publish at one coordinate, and read the Call fact through one corresponded
// site relation. Declaring those rows once, in the axis owner's own generator
// source, is what keeps the two rules from carrying two copies of a
// vocabulary that is neither one's own.

func callAxisReference() schema.EntryReference { return axisReference("call") }

func callResultMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: callPackagePath, Name: name,
		Receiver:        callGoType(receiver),
		ReceiverPointer: pointer, ResultIndex: resultIndex,
	}
}

// MountedCallResultSlotProvider is the candidate authority both call-result
// rules are addressed through.
func MountedCallResultSlotProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("value"), Member: "value/mounted-call-result/candidates",
	})
}

// CallResultSiteProvider is the Call-side directory the corresponded site
// relation below owns its order in.
func CallResultSiteProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: callAxisReference(), Member: "call/call-result/sites",
	})
}

// MountedCallResultSlotCarrier is the candidate row both call-result folds are
// indexed by: Value's sealed projection of one mounted call's first result
// slot. It is named on both axes, because the Call-side relation these rules
// read is joined from one of these rows and states so.
func MountedCallResultSlotCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "MountedCallResultSlotCarrier", Key: "carrier/value/mounted-call-result-slot",
		Type: valueGoType("MountedCallResultSlot"),
	}
}

// CallFactCarrier is the Call fact both folds read. It is named here, beside
// the rows that consume it, and the carrier key is Call's own.
func CallFactCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "CallFactCarrier", Key: "carrier/call/fact", Type: callGoType("Value"),
	}
}

// MountedCallResultSlotCandidates is Value's result-zero directory: one row per
// mounted call whose first result Value issued a coordinate for. That is the
// exact set the result-slot requirement issues a placement for, so this
// declaration introduces no second denominator for those rows.
func MountedCallResultSlotCandidates() definition.Relation {
	return definition.Relation{
		Name:              "MountedCallResultSlotCandidates",
		Key:               "value/mounted-call-result/candidates",
		Subject:           "MountedCallResultSlotCarrier",
		CandidateProvider: MountedCallResultSlotProvider(),
		CandidateResolver: valueMethod("MountedCallResultSlotForMountedOccurrence", "Schema", true, 0),
		CandidateOrdinal:  valueMethod("MountedCallResultSlotOrdinal", "Schema", true, 0),
		CandidateAt:       valueMethod("MountedCallResultSlotAt", "Schema", true, 0),
	}
}

// MountedCallResultSlotCoordinate is the call-result Value coordinate both
// rules publish at. Value issued it while sealing the mounted slot, so a rule
// writes an existing coordinate rather than minting one.
func MountedCallResultSlotCoordinate() definition.Projection {
	return definition.Projection{
		Name:              "MountedCallResultSlotCoordinate",
		Key:               "value/mounted-call-result/coordinate",
		Relation:          "MountedCallResultSlotCandidates",
		CandidateProvider: MountedCallResultSlotProvider(),
		Role:              member.Destination,
		Result:            "ValueCoordinateCarrier",
		Accessor:          valueMethod("Coordinate", "MountedCallResultSlot", false, -1),
	}
}

// CallResultSites is Call's mounted call directory as a RESULT-SLOT-addressed
// read.
//
// It is a relation of the Call axis because it is addressed by a Call
// coordinate and its key is Call's own; it is a second relation rather than a
// second input on call/mounted-call/facts because a relation's input carrier
// states which candidate joins it, and that one is joined from a Call
// coordinate. This one is joined from a result-slot row, so it owns its own
// order and declares the correspondence that makes the two enumerable as one:
// both directories are addressed by the mounted call occurrence, and neither
// assumes the other numbers its rows alike.
func CallResultSites() definition.Relation {
	return definition.Relation{
		Name: "CallResultSlotSites", Key: "call/call-result/sites", Axis: "call",
		Subject:           "CallCoordinateCarrier",
		Inputs:            []definition.RelationInput{{Carrier: "MountedCallResultSlotCarrier"}},
		CandidateProvider: CallResultSiteProvider(),
		CandidateResolver: callResultMethod("CallCoordinateForOccurrence", "Algebra", true, 0),
		CandidateOrdinal:  callResultMethod("CallCoordinateOrdinal", "Algebra", true, 0),
		CandidateAt:       callResultMethod("CallCoordinateAt", "Algebra", true, 0),
		Correspondences: []member.RelationRef{{
			Axis: axisReference("value"), Member: "value/mounted-call-result/candidates",
		}},
	}
}

// CallResultSiteKey addresses the Call cell one result slot's fact is read at.
// The accessor is a method on Call's own row, because that is the row the
// correspondence resolves through the shared occurrence.
func CallResultSiteKey() definition.Projection {
	return definition.Projection{
		Name: "CallResultSlotSiteKey", Key: "call/call-result/site-key", Axis: "call",
		Relation:          "CallResultSlotSites",
		CandidateProvider: CallResultSiteProvider(),
		Role:              member.Key,
		Result:            "CallKeyCarrier",
		Accessor:          callResultMethod("Key", "CallCoordinate", false, -1),
	}
}
