// Package memberdefinition is the generator-only contribution for the
// typestate obligation rule: the rows its Program names, the operations that
// publish the produced ones, and its fold signature.
//
// It names no runtime rule protocol and creates no second state authority: the
// cells are the axis's own coordinate space, the edges are the sealed protocol
// declarations, and the judgment is the one sealed state both are read
// through.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const (
	obligationPackagePath = "github.com/wippyai/go-lua/domain/typestate/obligation"
	typestatePackagePath  = "github.com/wippyai/go-lua/domain/typestate"
	statecellPackagePath  = "github.com/wippyai/go-lua/domain/typestate/statecell"
	callPackagePath       = "github.com/wippyai/go-lua/domain/call"
	valuePackagePath      = "github.com/wippyai/go-lua/domain/value"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func typestateAxis() schema.EntryReference { return axisReference("typestate") }

func goType(packagePath, name string) definition.GoType {
	return definition.GoType{PackagePath: packagePath, Name: name}
}

func cellMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: obligationPackagePath, Name: name,
		Receiver: goType(obligationPackagePath, "Cell"), ResultIndex: result,
	}
}

func callMethod(name, receiver string, result int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: callPackagePath, Name: name,
		Receiver: goType(callPackagePath, receiver), ResultIndex: result,
	}
}

func judgmentType() definition.GoType { return goType(obligationPackagePath, "Judgment") }

// provider is the candidate authority every row here hangs off: Value's own
// mounted call actual. A typestate judgment is drawn at a call occurrence
// about the actual that carries the resource, and that actual is Value's row,
// so neither the cell vector nor the call site mints a second directory.
func provider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("value"), Member: valuedomain.MountedCallArgumentCandidates,
	})
}

// Contribution declares the typestate obligation judgment together with the
// rows it reads.
//
// The call site is Call data whichever rule addresses it, so it is declared on
// the Call axis and the roster places it there; the cell vector and the fold
// are the typestate axis's own.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "typestate",
		Rule: "typestate-obligation",
		Carriers: []definition.Carrier{
			{Name: "CellCarrier", Key: "carrier/typestate/cell", Type: goType(statecellPackagePath, "Cell")},
			{Name: "StateCarrier", Key: "carrier/typestate/state", Type: goType(typestatePackagePath, "Abstract")},
			{Name: "StateCellCarrier", Key: "carrier/typestate/state-cell", Type: goType(obligationPackagePath, "Cell")},
			{Name: "ProtocolTagCarrier", Key: "carrier/typestate/protocol-tag", Type: definition.GoType{Name: "uint64"}},
			{Name: "MountedCallArgumentCarrier", Key: "carrier/value/mounted-call-argument", Type: goType(valuePackagePath, "MountedCallArgument")},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: goType(valuePackagePath, "Value")},
			{Name: "CallFactCarrier", Key: "carrier/call/fact", Type: goType(callPackagePath, "Value")},
			{Name: "CallKeyCarrier", Key: "carrier/call/key", Type: goType(callPackagePath, "Key")},
			{Name: "CallCoordinateCarrier", Key: "carrier/call/mounted-call", Type: goType(callPackagePath, "CallCoordinate")},
		},
		Relations: []definition.Relation{
			{
				// The Call coordinate one actual's own site is read at. It is
				// a relation of the Call axis because it is addressed by a Call
				// coordinate and its key is Call's own; it hangs off the
				// actual's directory rather than declaring a correspondence,
				// because several actuals of one call resolve to one
				// coordinate and the two directories number different
				// subjects.
				Name: "TypestateCallSites", Key: "call/typestate/sites", Axis: "call",
				Subject:           "CallCoordinateCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "MountedCallArgumentCarrier"}},
				CandidateProvider: provider(),
			},
			{
				// One row per state cell this actual's obligation is about:
				// the resource is what its Value fact names and the protocols
				// are what the dispatched operations declare about it, so the
				// rows do not exist until both facts are known and the axis
				// publishes them through an operation.
				Name: "StateCells", Key: "typestate/state/cells",
				Subject: "StateCellCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "MountedCallArgumentCarrier"},
					{Carrier: "ValueFactCarrier"},
					{Carrier: "CallFactCarrier"},
				},
				CandidateProvider: provider(),
			},
		},
		Projections: []definition.Projection{
			{
				Name: "TypestateCallSiteKey", Key: "call/typestate/site-key", Axis: "call",
				Relation: "TypestateCallSites", Role: member.Key, Result: "CallKeyCarrier",
				Accessor: callMethod("Key", "CallCoordinate", -1), CandidateProvider: provider(),
			},
			{
				// The resource's cell is where the state is read, and it is
				// where the successor is published: an operation moves a
				// resource's state, it does not move the resource.
				Name: "StateCellKey", Key: "typestate/state/cell-key",
				Relation: "StateCells", Role: member.Key, Result: "CellCarrier",
				Accessor: cellMethod("Coordinates", 0), CandidateProvider: provider(),
			},
			{
				Name: "StateCellProtocol", Key: "typestate/state/cell-protocol",
				Relation: "StateCells", Role: member.Predicate, Result: "ProtocolTagCarrier",
				Accessor: cellMethod("Predicate", -1), CandidateProvider: provider(),
			},
			{
				Name: "StateCellDestination", Key: "typestate/state/cell-destination",
				Relation: "StateCells", Role: member.Destination, Result: "CellCarrier",
				Accessor: cellMethod("Coordinates", 1), CandidateProvider: provider(),
			},
		},
		Selections: []definition.Selection{
			{
				Name: "StateCellSelection", Key: "typestate/state/cell-selection",
				Relation: "StateCells", Tag: "StateCellProtocol",
			},
		},
		Reducers: []definition.Reducer{{
			Name:      "JudgmentReducer",
			Key:       "typestate/reducer/judgment",
			Candidate: "MountedCallArgumentCarrier",
			Inputs: []definition.ReducerInput{
				{
					// The actual's own solved Value fact: what says which
					// resource the call was handed.
					Axis: axisReference("value"), Carrier: "ValueFactCarrier",
					Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
				},
				{
					// The site's own solved Call fact: what says which
					// operation the call reaches, and so which declaration the
					// actual is judged against. An authenticated-opaque callee
					// is delivered here rather than filtered, because the fold
					// answers for every actual it is indexed by.
					Axis: axisReference("call"), Carrier: "CallFactCarrier",
					Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
				},
				{
					// The cell's current state, selected by the protocol the
					// obligation names.
					Axis: typestateAxis(), Carrier: "StateCarrier",
					Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne,
					Tag: "ProtocolTagCarrier",
				},
			},
			Outputs: []definition.ReducerOutput{{Axis: typestateAxis(), Carrier: "StateCarrier"}},
			Derivation: definition.ReducerDerivation{
				State: judgmentType(),
				Build: definition.GoSymbol{PackagePath: obligationPackagePath, Name: "Derive", ResultIndex: 0},
				// Value owns the actual, Call classifies the operation the site
				// dispatches to, and Pack answers which actual a declared
				// operation input lands at. The declared edges themselves are
				// reached through Call's own sealed target contract, so this
				// judgment opens no protocol table of its own.
				StaticAxes: []schema.EntryReference{
					axisReference("value"),
					axisReference("call"),
					axisReference("pack"),
				},
			},
			Implementation: definition.GoSymbol{
				PackagePath: obligationPackagePath, Name: "Judge",
				Receiver: judgmentType(), ResultIndex: 0,
			},
		}},
	}
}
