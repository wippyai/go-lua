// Package memberdefinition is the generator-only owner source for Value's
// storage-transfer member vocabulary. The runtime value package imports only
// the generated cold catalog; this package is imported by the member
// definition generator, so typed symbol descriptions never become runtime
// callbacks or a second production schema.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	valuePackagePath = "github.com/wippyai/go-lua/domain/value"
	// freshResultPackagePath names the fresh-result rule's own package. The
	// route rows a routed carry is indexed by are that rule's, and the value
	// axis declares the transition they issue because a carry transform is an
	// axis-level row.
	freshResultPackagePath = "github.com/wippyai/go-lua/domain/value/freshresult"
	callPackagePath        = "github.com/wippyai/go-lua/domain/call"
)

func valueGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name}
}

func valueGoPointerType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name, Pointer: true}
}

func freshResultGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: freshResultPackagePath, Name: name}
}

func callGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: callPackagePath, Name: name}
}

func builtinGoType(name string) definition.GoType { return definition.GoType{Name: name} }

func valueMethod(name, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     valuePackagePath,
		Name:            name,
		Receiver:        valueGoType(receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// StorageTransfer returns Value's one authored member definition. The member
// generator projects this source into the cold member.Catalog and the
// callback-free typed binding metadata consumed later by composition code.
func StorageTransfer() definition.Definition {
	coordinate := valueGoType("Coordinate")
	storageTransfer := valueGoType("StorageTransfer")
	binaryArithmetic := valueGoType("BinaryArithmetic")
	binaryEquality := valueGoType("BinaryEquality")
	binaryOrder := valueGoType("BinaryOrder")
	value := valueGoType("Value")
	allocationResult := valueGoPointerType("AllocationResult")
	freshResultCall := valueGoType("FreshResultCall")
	mountedCallArgument := valueGoType("MountedCallArgument")
	mountedCallActuals := valueGoType("MountedCallActuals")
	returnBoundary := valueGoType("ReturnBoundary")
	returnBoundaryMember := valueGoType("ReturnBoundaryMember")
	return definition.Definition{
		Name:       "ValueStorageTransfer",
		Axis:       "value",
		ImportPath: "github.com/wippyai/go-lua/domain/value",
		Binding: definition.Binding{
			Key: definition.KeyNormalization{
				Carrier:    "ValueCoordinateCarrier",
				Dense:      builtinGoType("uint32"),
				Normalizer: valueMethod("CoordinateIndex", "Schema", true, 0),
			},
		},
		Signature: definition.Signature{
			Key:  "ValueCoordinateCarrier",
			Fact: "ValueFactCarrier",
		},
		Carriers: []definition.Carrier{
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: coordinate},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: value},
			{Name: "ValueAtomCarrier", Key: "carrier/value/atom", Type: valueGoType("Atom")},
			{Name: "StorageTransferCarrier", Key: "carrier/value/storage-transfer", Type: storageTransfer},
			{Name: "BinaryArithmeticCarrier", Key: "carrier/value/binary-arithmetic", Type: binaryArithmetic},
			{Name: "BinaryEqualityCarrier", Key: "carrier/value/binary-equality", Type: binaryEquality},
			{Name: "BinaryOrderCarrier", Key: "carrier/value/binary-order", Type: binaryOrder},
			{Name: "PresenceRefinementCarrier", Key: "carrier/value/presence-refinement", Type: valueGoType("PresenceRefinement")},
			{Name: "SourceSeedCarrier", Key: "carrier/value/source-seed", Type: valueGoType("SourceSeed")},
			// The owner answers this row by POINTER everywhere it answers it -
			// resolver, dense accessor, and the materializer that turns one
			// into a fact - so the carrier is the pointer, not the struct. A
			// carrier that named the value would derive a call shape no symbol
			// on either side of it has.
			{Name: "GlobalBootstrapResultCarrier", Key: "carrier/value/global-bootstrap-result", Type: valueGoPointerType("GlobalBootstrapResult")},
			// These are the owner-issued candidate relationships whose
			// transformed carries write the Value factor. Each is the subject of
			// its own published directory below; no receipt or callback is
			// retained in the cold catalog.
			{Name: "AllocationResultCarrier", Key: "carrier/value/allocation-result", Type: allocationResult},
			{Name: "FreshResultCallCarrier", Key: "carrier/value/fresh-result-call", Type: freshResultCall},
			{Name: "MountedCallArgumentCarrier", Key: "carrier/value/mounted-call-argument", Type: mountedCallArgument},
			// The return boundary and its member rows. Both types are declared by
			// the value package, so the relations that subject them are Value's
			// own; a placement rule that joins a return's members names these
			// rows rather than rebuilding the topology from Program occurrences.
			{Name: "ReturnBoundaryCarrier", Key: "carrier/value/return-boundary", Type: returnBoundary},
			{Name: "ReturnBoundaryMemberCarrier", Key: "carrier/value/return-boundary-member", Type: returnBoundaryMember},
			// The address a member is reached by under its return. A child that
			// never sees Value's Go symbols still addresses member k through this
			// carrier, which is why the nested set declares it beside its parent.
			{Name: "ReturnBoundaryMemberOrdinalCarrier", Key: "carrier/value/return-boundary-member-ordinal", Type: builtinGoType("uint64")},
			{Name: "MountedCallActualsCarrier", Key: "carrier/value/mounted-call-actuals", Type: mountedCallActuals},
			{Name: "MountedCallActualTagCarrier", Key: "carrier/value/mounted-call-actual-tag", Type: builtinGoType("uint64")},
			// The address a fresh result is reached by under its mounted call.
			{Name: "FreshResultTagCarrier", Key: "carrier/value/fresh-result-tag", Type: builtinGoType("uint64")},
			// One row of the fresh-result rule's derived route set: a Value
			// coordinate one mounted call publishes at, and the roots admitted
			// there. It is the row that rule's routed carry is indexed by.
			{Name: "FreshResultRouteCarrier", Key: "carrier/value/fresh-result-route", Type: freshResultGoType("Route")},
			// CallCoordinate is a foreign input coordinate, not a second Call
			// vocabulary. Repeating its canonical carrier lets Value declare the
			// correspondence between its parent rows and Call's candidate rows.
			{Name: "CallCoordinateCarrier", Key: "carrier/call/mounted-call", Type: callGoType("CallCoordinate")},
		},
		Enumerations: []definition.Enumeration{
			{
				// The atoms one Value relation decomposes to. The census and
				// the accessor are the SCHEMA's, not the value's: an atom is
				// only an atom of this owner, and answering without the
				// ownership fence would answer for a value this schema never
				// issued.
				Name: "Atoms", Over: "ValueFactCarrier", Item: "ValueAtomCarrier",
				Count: valueMethod("ValueAtomCount", "Schema", true, -1),
				At:    valueMethod("ValueAtomAt", "Schema", true, 0),
			},
		},
		Relations: []definition.Relation{
			{
				Name:              "StorageTransferCandidates",
				Key:               "value/storage-transfer/candidates",
				Subject:           "StorageTransferCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/storage-transfer/candidates"}),
				CandidateResolver: valueMethod("StorageTransferForArtifactOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("StorageTransferOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("StorageTransferAt", "Schema", true, 0),
			},
			{
				// Arithmetic candidates are addressed by the one endpoint table
				// sealed by Value. BinaryArithmeticOrdinal and BinaryArithmeticAt
				// therefore use the endpoint ordinal directly; no family-local
				// candidate directory is authored here.
				Name:              "BinaryArithmeticCandidates",
				Key:               "value/binary-arithmetic/candidates",
				Subject:           "BinaryArithmeticCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/binary-arithmetic/candidates"}),
				CandidateResolver: valueMethod("BinaryArithmeticForArtifactOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("BinaryArithmeticOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("BinaryArithmeticAt", "Schema", true, 0),
			},
			{
				// Equality candidates use the same sealed endpoint projection as
				// arithmetic, but retain their own owner-issued candidate relation.
				// The family directory is therefore canonical and disjoint: no
				// equality row is inferred from arithmetic rows or renumbered in a
				// family-local table.
				Name:              "BinaryEqualityCandidates",
				Key:               "value/binary-equality/candidates",
				Subject:           "BinaryEqualityCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/binary-equality/candidates"}),
				CandidateResolver: valueMethod("BinaryEqualityForArtifactOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("BinaryEqualityOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("BinaryEqualityAt", "Schema", true, 0),
			},
			{
				// Order candidates are likewise a distinct owner directory over
				// the shared endpoint table. Its canonical ordinal is issued by
				// Value; this declaration does not create a second denominator.
				Name:              "BinaryOrderCandidates",
				Key:               "value/binary-order/candidates",
				Subject:           "BinaryOrderCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/binary-order/candidates"}),
				CandidateResolver: valueMethod("BinaryOrderForArtifactOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("BinaryOrderOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("BinaryOrderAt", "Schema", true, 0),
			},
			{
				// The one refinement candidate directory. Its endpoint
				// vector is the same sealed table arithmetic/equality/order
				// use; the refinement family retains its own owner-issued
				// candidate relation over that shared table.
				Name:              "PresenceRefinementCandidates",
				Key:               "value/presence-refinement/candidates",
				Subject:           "PresenceRefinementCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/presence-refinement/candidates"}),
				CandidateResolver: valueMethod("PresenceRefinementForArtifactOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("PresenceRefinementOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("PresenceRefinementAt", "Schema", true, 0),
			},
			{
				Name:              "StorageTransferSources",
				Key:               "value/storage-transfer/sources",
				Subject:           "ValueFactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "StorageTransferCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/storage-transfer/candidates"}),
			},
			{
				Name:              "SourceSeeds",
				Key:               "value/source/candidates",
				Subject:           "SourceSeedCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/source/candidates"}),
				CandidateResolver: valueMethod("SourceSeedForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("SourceSeedOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("SourceSeedAt", "Schema", true, 0),
				CandidateCount:    valueMethod("SourceSeedCount", "Schema", true, 0),
				Materialize:       definition.GoSymbol{PackagePath: valuePackagePath, Name: "SourceFact", ResultIndex: 0},
			},
			{
				// The Host global bindings are a Link-global directory: they are
				// addressed by the binding identity alone, and the axis publishes
				// that directory itself rather than an artifact declaring rows for
				// it.
				Name:                "GlobalBootstrapResults",
				Key:                 "value/global-bootstrap/candidates",
				Subject:             "GlobalBootstrapResultCarrier",
				CandidateProvider:   member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/global-bootstrap/candidates"}),
				CandidateResolver:   valueMethod("GlobalBootstrapResultForID", "Schema", true, 0),
				CandidateOrdinal:    valueMethod("GlobalBootstrapResultOrdinal", "Schema", true, 0),
				CandidateAt:         valueMethod("GlobalBootstrapResultAt", "Schema", true, 0),
				CandidateCount:      valueMethod("GlobalBootstrapResultCount", "Schema", true, 0),
				CandidateIdentityAt: valueMethod("GlobalBootstrapResultIDAt", "Schema", true, 0),
				Materialize:         definition.GoSymbol{PackagePath: valuePackagePath, Name: "GlobalBootstrapFact", ResultIndex: 0},
			},
			{
				Name:              "MountedCallArgumentCandidates",
				Key:               "value/mounted-call/argument-candidates",
				Subject:           "MountedCallArgumentCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/argument-candidates"}),
				CandidateResolver: valueMethod("MountedCallArgumentForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("MountedCallArgumentOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("MountedCallArgumentAt", "Schema", true, 0),
			},
			{
				Name:              "MountedCallArguments",
				Key:               "value/mounted-call/arguments",
				Subject:           "ValueFactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "MountedCallArgumentCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/argument-candidates"}),
			},
			{
				// The two allocation-form candidate directories. Value publishes
				// rows of its own constructor receipts because it declares them:
				// the receipt type lives in the value package, so the relation
				// whose subject it is can exist. A carry transform may only name
				// a candidate some relation of its axis subjects, and these are
				// the relations that subject the two the transforms below name.
				Name:              "AllocationResults",
				Key:               "value/allocation/candidates",
				Subject:           "AllocationResultCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/allocation/candidates"}),
				CandidateResolver: valueMethod("AllocationResultForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("AllocationResultOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("AllocationResultAt", "Schema", true, 0),
			},
			{
				// The fresh results one mounted call produces: Value's nested
				// member set over the mounted-call parent, addressed by
				// (parent, owner-issued tag). A call site whose callee is not
				// resolved to a single Target operation produces one member per
				// admitted (operation, outcome, result) arm, so the set is a
				// membership Value seals rather than geometry a consumer
				// rebuilds from Heap's fresh roots.
				Name:              "FreshResultCalls",
				Key:               "value/fresh-result/candidates",
				Subject:           "FreshResultCallCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "CallCoordinateCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/fresh-result/candidates"}),
				CandidateResolver: valueMethod("FreshResultCallForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("FreshResultCallOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("FreshResultCallAt", "Schema", true, 0),
				MemberParent:      member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/parents"},
				MemberOrdinal:     "FreshResultTagCarrier",
				MemberCount:       valueMethod("FreshResultCount", "MountedCallActuals", false, 0),
				MemberAt:          valueMethod("FreshResultAt", "MountedCallActuals", false, 0),
			},
			{
				// The return-boundary directory. One row per mounted executable
				// return, in sealed mount-then-occurrence order.
				Name:              "ReturnBoundaryCandidates",
				Key:               "value/return-boundary/candidates",
				Subject:           "ReturnBoundaryCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/return-boundary/candidates"}),
				CandidateResolver: valueMethod("ReturnBoundary", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("ReturnBoundaryOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("ReturnBoundaryAt", "Schema", true, 0),
			},
			{
				// The root anchor a return's fact is read at. It is a dependent
				// relation over the candidate row: the coordinate is already
				// issued, and the root projection below is where it is read from.
				Name:              "ReturnBoundaryRoots",
				Key:               "value/return-boundary/roots",
				Subject:           "ValueFactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "ReturnBoundaryCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/return-boundary/candidates"}),
			},
			{
				// The ordered fixed member set one return carries: a nested
				// member set parented by the candidate directory, addressed by
				// (return, ordinal). The set is self-provided, so a member is
				// densified through this relation's own directory and projects
				// the way every other row does. The open tail is candidate
				// metadata and is deliberately not a row here: it has no
				// coordinate to be one.
				Name:    "ReturnBoundaryMembers",
				Key:     "value/return-boundary/members",
				Subject: "ReturnBoundaryMemberCarrier",
				// This Input carries no Derivation semantics for a resolver-based
				// relation - Complete() never inspects Inputs on that path - it
				// exists purely to satisfy the Program join arity check, mirroring
				// ReturnBoundaryRoots exactly.
				Inputs:            []definition.RelationInput{{Carrier: "ReturnBoundaryCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/return-boundary/members"}),
				CandidateResolver: valueMethod("ReturnBoundaryMemberForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("ReturnBoundaryMemberOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("ReturnBoundaryMemberAt", "Schema", true, 0),
				MemberParent:      member.RelationRef{Axis: axisReference("value"), Member: "value/return-boundary/candidates"},
				MemberOrdinal:     "ReturnBoundaryMemberOrdinalCarrier",
				MemberCount:       valueMethod("MemberCount", "ReturnBoundary", false, 0),
				MemberAt:          valueMethod("MemberAt", "ReturnBoundary", false, 0),
			},
			{
				// One Value-owned parent row per mounted call. The occurrence is the
				// shared semantic address used to translate between Call's candidate
				// directory and Value's independently dense parent directory.
				Name:              "MountedCallParents",
				Key:               "value/mounted-call/parents",
				Subject:           "MountedCallActualsCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "CallCoordinateCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/parents"}),
				CandidateResolver: valueMethod("MountedCallActualsForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("MountedCallActualsOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("MountedCallActualsAt", "Schema", true, 0),
				Correspondences: []member.RelationRef{{
					Axis: axisReference("call"), Member: "call/mounted-call/candidates",
				}},
			},
			{
				// The ordered actuals are Value's nested member set, addressed by
				// (parent, owner-issued ordinal). Consumers select these rows instead
				// of rebuilding call-to-actual geometry.
				Name:              "MountedCallActualMembers",
				Key:               "value/mounted-call/actual-members",
				Subject:           "MountedCallArgumentCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "CallCoordinateCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/actual-members"}),
				CandidateResolver: valueMethod("MountedCallArgumentForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("MountedCallArgumentOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("MountedCallArgumentAt", "Schema", true, 0),
				MemberParent:      member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/parents"},
				MemberOrdinal:     "MountedCallActualTagCarrier",
				MemberCount:       valueMethod("MemberCount", "MountedCallActuals", false, 0),
				MemberAt:          valueMethod("MemberAt", "MountedCallActuals", false, 0),
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "StorageTransferSourceKey",
				Key:               "value/storage-transfer/source-key",
				Relation:          "StorageTransferSources",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/storage-transfer/candidates"}),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Endpoints", "StorageTransfer", false, 0),
			},
			{
				Name:              "StorageTransferTarget",
				Key:               "value/storage-transfer/target",
				Relation:          "StorageTransferCandidates",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/storage-transfer/candidates"}),
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Endpoints", "StorageTransfer", false, 1),
			},
			{
				Name:              "SourceCoordinate",
				Key:               "value/source/coordinate",
				Relation:          "SourceSeeds",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/source/candidates"}),
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Result", "SourceSeed", false, 0),
			},
			{
				Name:              "GlobalBootstrapCoordinate",
				Key:               "value/global-bootstrap/coordinate",
				Relation:          "GlobalBootstrapResults",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/global-bootstrap/candidates"}),
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Result", "GlobalBootstrapResult", true, 0),
			},
			{
				Name:              "MountedCallArgumentKey",
				Key:               "value/mounted-call/argument-key",
				Relation:          "MountedCallArguments",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/argument-candidates"}),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Coordinate", "MountedCallArgument", false, -1),
			},
			{
				Name:              "AllocationResultCoordinate",
				Key:               "value/allocation/coordinate",
				Relation:          "AllocationResults",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/allocation/candidates"}),
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Coordinate", "AllocationResult", true, -1),
			},
			{
				Name:              "FreshResultCoordinate",
				Key:               "value/fresh-result/coordinate",
				Relation:          "FreshResultCalls",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/fresh-result/candidates"}),
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Coordinate", "FreshResultCall", false, -1),
			},
			{
				Name:              "ReturnBoundaryRootKey",
				Key:               "value/return-boundary/root-key",
				Relation:          "ReturnBoundaryRoots",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/return-boundary/candidates"}),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Root", "ReturnBoundary", false, -1),
			},
			{
				Name:              "ReturnBoundaryMemberKey",
				Key:               "value/return-boundary/member-key",
				Relation:          "ReturnBoundaryMembers",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/return-boundary/members"}),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Coordinate", "ReturnBoundaryMember", false, -1),
			},
			{
				Name:              "MountedCallCalleeKey",
				Key:               "value/mounted-call/callee-key",
				Relation:          "MountedCallParents",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/parents"}),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("CalleeCoordinate", "MountedCallActuals", false, -1),
			},
			{
				Name:              "MountedCallActualKey",
				Key:               "value/mounted-call/actual-key",
				Relation:          "MountedCallActualMembers",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/actual-members"}),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Coordinate", "MountedCallArgument", false, -1),
			},
			{
				Name:              "MountedCallActualTag",
				Key:               "value/mounted-call/actual-tag",
				Relation:          "MountedCallActualMembers",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("value"), Member: "value/mounted-call/actual-members"}),
				Role:              member.Predicate,
				Result:            "MountedCallActualTagCarrier",
				Accessor:          valueMethod("ActualTag", "MountedCallArgument", false, -1),
			},
		},
		CarryTransforms: []definition.CarryTransform{
			{
				Name:           "AllocationCarryTransform",
				Key:            "transform/value/allocation",
				Candidate:      "AllocationResultCarrier",
				Input:          "ValueFactCarrier",
				Output:         "ValueFactCarrier",
				Implementation: valueMethod("Age", "AllocationResult", true, 0),
			},
			{
				// The transition one published fresh-result row carries the
				// image at its own destination through. It is issued by the
				// ROUTE row rather than by the call: a call that publishes at
				// several destinations has one transition per destination, and
				// asking which of them is the call's has no answer.
				Name:           "FreshResultRouteCarryTransform",
				Key:            "transform/value/fresh-result-route",
				Candidate:      "FreshResultRouteCarrier",
				Input:          "ValueFactCarrier",
				Output:         "ValueFactCarrier",
				Implementation: definition.GoSymbol{PackagePath: freshResultPackagePath, Name: "Age", Receiver: freshResultGoType("Route"), ResultIndex: 0},
			},
		},
	}
}
