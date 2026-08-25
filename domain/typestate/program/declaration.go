// Package program owns Typestate's callback-free rule declaration: the
// coordinate space the judgment is written into, the member vocabulary that
// space is addressed by, and the Program that decides one call site.
//
// The judgment itself is domain/typestate's, the coordinate space is
// domain/typestate/statecell's, and the obligation a callable declares is
// analysis/program/target/protocol's sealed callable-requirement authority.
// This package names those three and adds no fourth authority: it contains no
// engine slot, no runtime callback, and no state machine of its own.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The identities this family declares.
const (
	// AxisKey is Typestate's coordinate space, and so its writer principal:
	// one coordinate is one resource under one protocol, as statecell seals it.
	AxisKey schema.Key = "typestate"
	// StateOutputKey is the one column the axis publishes: the solved abstract
	// state of every cell.
	StateOutputKey schema.Key = "typestate/states"
	// RuleKey is the rule that writes the axis.
	RuleKey schema.Key = "typestate-obligation"
	// FamilyKey is the publication family the typestate codes are gated by.
	FamilyKey schema.Key = "family/typestate"
	// ObservationKey is the population the typestate findings are measured
	// over: the call occurrences that carry a declared obligation.
	ObservationKey schema.Key = "observation/typestate-obligation"
)

// The semantic roles this family is identified by.
const (
	FactorRole  = "factor/typestate"
	RuleRole    = "rule/typestate/obligation"
	OperandRole = "operand/typestate/obligation"
)

// The carriers this axis's members are typed in. Cell is statecell.Cell and
// State is typestate.Abstract; they are nominal, so no domain value enters the
// declaration stream by structural coincidence.
const (
	CellCarrier     member.Carrier = "carrier/typestate/cell"
	StateCarrier    member.Carrier = "carrier/typestate/state"
	ProtocolCarrier member.Carrier = "carrier/typestate/protocol"
)

// The Typestate-owned member keys.
const (
	// StateCells is the relation this rule reads its own axis through: for one
	// obligation occurrence and the receiver value fact that resolves its
	// resource, the state cell that holds that resource's current state.
	StateCells schema.Key = "typestate/state/cells"
	// StateCellKey is the resource the cell is read at. It consumes the
	// receiver's solved Value fact, which is what makes this the computed-
	// coordinate normal form rather than a second candidate relation.
	StateCellKey schema.Key = "typestate/state/cell-key"
	// StateCellProtocol is the tag that selects which of the resource's cells
	// the obligation is about. A resource governed by two protocols has two
	// cells, and the obligation names one of them, so the read is a selected
	// read over the resource's own run rather than a second keyed relation.
	StateCellProtocol schema.Key = "typestate/state/cell-protocol"
	// StateCellDestination is the coordinate the successor state is published
	// at. It is the same cell: an operation moves a resource's state, it does
	// not move the resource.
	StateCellDestination schema.Key = "typestate/state/cell-destination"
	// JudgmentReducer draws the verdict and the successor state from the
	// receiver's Value fact and the cell's current state. Its implementation is
	// typestate.JudgeRequirement, JudgeTransition, and JudgeExit selected by
	// the obligation's declared kind; the reducer holds the sealed callable-
	// requirement authority, so the kind is read from the owner rather than
	// carried as a runtime callback.
	JudgmentReducer schema.Key = "typestate/reducer/judgment"
)

// The Value carriers this axis's members are typed against. A carrier is a
// nominal key rather than a Go type, so the cold catalog names Value's
// spellings the same way Static's does instead of importing Value.
const (
	ValueCoordinateCarrier member.Carrier = "carrier/value/coordinate"
	ValueFactCarrier       member.Carrier = "carrier/value/fact"
)

// The Value-owned columns this judgment requires.
//
// A typestate judgment is drawn at a call occurrence, about the argument that
// carries the resource. Both halves of that are Value's to publish and neither
// is published today:
//
//   - MountedCallArgumentCandidates is the candidate relation over
//     (mounted call occurrence, fixed input ordinal). It cannot be minted here.
//     A read of a foreign axis is declared by the axis being read, keyed by the
//     candidate that axis's owner declared it for - which is exactly how
//     Placement Store reads Value along Value's own storage-transfer candidate.
//     A consumer that minted its own candidate could name no relation to read
//     Value through.
//   - MountedCallArguments and MountedCallArgumentKey are the exact read of
//     that argument's solved Value fact. Value already owns the whole
//     projection as ordinary Go API - pack.MountedActualProjection.ActualAt
//     followed by value.Schema.CoordinateForMountedSemantic - so the column is
//     unpublished, not underived.
//
// They are spelled here because this declaration is what states the
// requirement: the day Value publishes them, this Program seals against the
// real catalog with no change to the rows below.
const (
	MountedCallArgumentCandidates schema.Key     = "value/mounted-call/argument-candidates"
	MountedCallArguments          schema.Key     = "value/mounted-call/arguments"
	MountedCallArgumentKey        schema.Key     = "value/mounted-call/argument-key"
	MountedCallArgumentCarrier    member.Carrier = "carrier/value/mounted-call-argument"
)

const valueAxisKey schema.Key = "value"

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func valueCandidateProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference(valueAxisKey), Member: MountedCallArgumentCandidates}
}

// AxisMemberCatalog is Typestate's declaration-only member vocabulary: the two
// projections of its own coordinate space and the reducer that writes it.
func AxisMemberCatalog() member.Catalog {
	typestateAxis := axisReference(AxisKey)
	valueAxis := axisReference(valueAxisKey)
	provider := valueCandidateProvider()
	catalog, ok := member.NewCatalog(
		[]member.Relation{
			{Key: StateCells, Subject: StateCarrier, Inputs: []member.Carrier{MountedCallArgumentCarrier, ValueFactCarrier}, CandidateProvider: member.AxisRelationCandidate(provider)},
		},
		[]member.Projection{
			{Key: StateCellKey, Relation: StateCells, Role: member.Key, Result: CellCarrier, CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: StateCellProtocol, Relation: StateCells, Role: member.Predicate, Result: ProtocolCarrier, CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: StateCellDestination, Relation: StateCells, Role: member.Destination, Result: CellCarrier, CandidateProvider: member.AxisRelationCandidate(provider)},
		},
		[]member.Reducer{
			{Key: JudgmentReducer, Inputs: []member.ReducerInput{
				{Axis: valueAxis, Carrier: ValueFactCarrier, Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne},
				{Axis: typestateAxis, Carrier: StateCarrier, Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: ProtocolCarrier},
			}, Outputs: []member.ReducerOutput{
				{Axis: typestateAxis, Carrier: StateCarrier},
			}},
		},
		[]member.CarryTransform{},
	)
	if !ok {
		panic("typestate: invalid axis member catalog")
	}
	return catalog
}

// Obligation returns the immutable Typestate rule declaration.
//
// The candidate is one mounted call argument that carries a declared
// obligation. Join 0 is the exact read of that argument's solved Value fact:
// it is what says which resource the call was handed. Join 1 is the dependent
// selected read of this axis's own state cell - it consumes the same candidate
// and Join 0's Value fact, which is the computed-coordinate normal form,
// because the resource is a function of the Value fact, and it selects that
// resource's cell by the obligation's protocol, because a resource governed by
// two protocols has two cells and the obligation names one. The fold draws the
// verdict and routes the successor state back to the cell it was read from: an
// operation moves a resource's state, it does not move the resource.
//
// The Value read propagates authenticated opaque evidence rather than refusing
// it. A receiver the analysis cannot follow is exactly the case the judgment
// answers with an unproven verdict; refusing the read would drop the call from
// the population and report nothing, which is the one answer a soundness
// judgment may not give.
func Obligation() ruleprogram.Program {
	typestateAxis := axisReference(AxisKey)
	valueAxis := axisReference(valueAxisKey)
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate:   member.AxisRelationCandidate(valueCandidateProvider()),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: valueAxis, Member: MountedCallArguments},
				Key:      member.ProjectionRef{Axis: valueAxis, Member: MountedCallArgumentKey},
				Read: ruleprogram.ReadDecl{
					Input: 0,
					Axis:  ruleprogram.AxisRef(valueAxis),
					Form:  ruleprogram.Exact,
					// The argument read is taken against its own transported
					// predecessor at this port.
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
						OnOpaque: ruleprogram.OnOpaquePropagateAuthenticated, Multiplicity: ruleprogram.MultiplicityOne,
					},
				},
			},
			{
				Sources:   []ruleprogram.SourceRef{ruleprogram.CandidateSource(), ruleprogram.PriorSource(0)},
				Relation:  member.RelationRef{Axis: typestateAxis, Member: StateCells},
				Key:       member.ProjectionRef{Axis: typestateAxis, Member: StateCellKey},
				Predicate: member.ProjectionRef{Axis: typestateAxis, Member: StateCellProtocol},
				Read: ruleprogram.ReadDecl{
					Input: 0,
					Axis:  ruleprogram.AxisRef(typestateAxis),
					Form:  ruleprogram.Selected,
					// The cell is selected through Typestate's own directory at
					// this port, so the transported predecessor is the
					// candidate's own point.
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseDefault,
						OnOpaque: ruleprogram.OnOpaquePropagateAuthenticated, Multiplicity: ruleprogram.MultiplicityOne,
						DenominatorRef: ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: "coordinates/" + AxisKey},
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: typestateAxis, Member: JudgmentReducer},
			Inputs:  []ruleprogram.JoinRef{0, 1},
			Outputs: []ruleprogram.OutputDecl{{
				Column:           axis.OutputRef{Axis: typestateAxis, Key: StateOutputKey},
				Destination:      member.ProjectionRef{Axis: typestateAxis, Member: StateCellDestination},
				Mode:             ruleprogram.ModeRoute,
				ValueSlot:        0,
				RouteJoin:        1,
				RouteJoinPresent: true,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}

// axisInputs states what this axis's mount reads from the composition's Link
// input record. It names no composition type, so any record carrying the two
// directories the state-cell space is the product of satisfies it structurally.
type axisInputs interface {
	HeapAllocationKeyCount() int
	TargetProtocolCount() int
}

// AxisEntry is Typestate's axis declaration. One coordinate is one resource
// under one protocol; the space is dense because both directories it is the
// product of are dense, so a cell with no row is a published absence rather
// than ignorance.
func AxisEntry[A axisInputs]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:          AxisKey,
		Storage:      axis.StorageFactor,
		Cardinality:  axis.CardinalityDense,
		Lifetime:     axis.LifetimeLink,
		Mutability:   axis.MutabilitySolve,
		Concurrency:  axis.ConcurrencySingleWriter,
		Dependencies: []schema.Key{"heap", valueAxisKey},
		Frame:        axis.Frame{Outputs: []axis.Output{{Key: StateOutputKey, Writer: AxisKey}}},
		Catalog:      AxisMemberCatalog(),
		Signature:    axis.Signature{Key: CellCarrier, Fact: StateCarrier},
		Semantic:     vocabulary.RoleKey(FactorRole),
	}
}

// RuleEntry is Typestate's rule declaration: the mounted-lane rule that writes
// the state axis from the Program above.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Lane:     rule.LaneMounted,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  Obligation(),
	}
}

// StructureSpecs is this family's contribution to the structural vocabulary:
// the three semantic roles it is identified by, the publication family its
// codes are gated by, and the population its findings are measured over.
func StructureSpecs() []structure.Spec {
	specs := append(vocabulary.RoleSpecs(FactorRole), vocabulary.RuleRoleSpecs("typestate/obligation")...)
	return append(specs,
		structure.Spec{Key: FamilyKey, Category: structure.CategoryDiagnosticFamily, Spelling: "typestate", Accepted: true},
		structure.Spec{Key: ObservationKey, Category: structure.CategoryDiagnosticObservation, Spelling: "typestate-obligation", Accepted: true},
	)
}

// The published typestate codes. Their spelling is the analyzer's one spelling
// of these findings; domain/composite's register names the same three while
// they have no producer, and drops them when this family's collection is
// composed.
const (
	CodeInvalidRequirement  diagnostic.Code = "typestate.invalid_requirement"
	CodeInvalidTransition   diagnostic.Code = "typestate.invalid_transition"
	CodeUnprovenRequirement diagnostic.Code = "typestate.unproven_requirement"
)
