// Package program owns Typestate's callback-free rule declaration: the
// coordinate space the judgment is written into, the member vocabulary that
// space is addressed by, and the Program that decides one call site.
//
// The judgment itself is domain/typestate/obligation's, the coordinate space
// is domain/typestate/statecell's, and the obligation a callable declares is
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
	calldomain "github.com/wippyai/go-lua/domain/call"
	statecell "github.com/wippyai/go-lua/domain/typestate/statecell"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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

// The member vocabulary this Program names. Every row below is composed:
// Typestate's own rows come from its axis source and this rule's contribution,
// Value owns the actual the judgment is indexed by, and Call owns the site
// relation the actual's own row addresses. Naming them through their owners is
// what keeps one spelling of each key in the analyzer.
const (
	// StateCells is the relation this rule reads its own axis through: for one
	// obligation occurrence and the receiver value fact that resolves its
	// resource, the state cell that holds that resource's current state.
	StateCells schema.Key = statecell.StateCells
	// StateCellKey is the resource the cell is read at. It consumes the
	// receiver's solved Value fact, which is what makes this the computed-
	// coordinate normal form rather than a second candidate relation.
	StateCellKey schema.Key = statecell.StateCellKey
	// StateCellProtocol is the tag that selects which of the resource's cells
	// the obligation is about. A resource governed by two protocols has two
	// cells, and the obligation names one of them, so the read is a selected
	// read over the resource's own run rather than a second keyed relation.
	StateCellProtocol schema.Key = statecell.StateCellProtocol
	// StateCellDestination is the coordinate the successor state is published
	// at. It is the same cell: an operation moves a resource's state, it does
	// not move the resource.
	StateCellDestination schema.Key = statecell.StateCellDestination
	// JudgmentReducer draws the verdict and the successor state from the
	// actual's Value fact, the site's Call fact and the cell's current state.
	// Its implementation is the sealed judgment that holds the callable-
	// requirement authority, so which of JudgeRequirement, JudgeTransition and
	// JudgeExit answers one call is decided inside the judgment from the
	// declaration, rather than carried to it as a runtime callback.
	JudgmentReducer schema.Key = statecell.JudgmentReducer
	// StateCellSelection is the operation this axis publishes the cell rows
	// through. Which cell a mounted call reaches is computed from the receiver
	// fact the read before it delivered, so the row does not exist until that
	// cell is known and the read names the operation that publishes it.
	StateCellSelection schema.Key = statecell.StateCellSelection

	// The Value-owned rows this judgment is indexed by: the mounted call
	// actual directory, and the exact read of that actual's solved fact.
	MountedCallArgumentCandidates schema.Key = valuedomain.MountedCallArgumentCandidates
	MountedCallArguments          schema.Key = valuedomain.MountedCallArguments
	MountedCallArgumentKey        schema.Key = valuedomain.MountedCallArgumentKey

	// The Call-owned rows this judgment reads the dispatched operation
	// through. The site is addressed from the actual's own row, because
	// several actuals of one call are judged against the one call fact.
	TypestateCallSites   schema.Key = calldomain.TypestateCallSites
	TypestateCallSiteKey schema.Key = calldomain.TypestateCallSiteKey
)

const (
	valueAxisKey schema.Key = "value"
	callAxisKey  schema.Key = "call"
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func valueCandidateProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference(valueAxisKey), Member: MountedCallArgumentCandidates}
}

// Obligation returns the immutable Typestate rule declaration.
//
// The candidate is one mounted call actual. Join 0 is the exact read of that
// actual's solved Value fact: it is what says which resource the call was
// handed. Join 1 is the exact read of the site's own solved Call fact: it is
// what says which operation the call reaches, and so which declaration the
// actual is judged against - an obligation is a property of the operation
// dispatched at the site, and no cold row of a call names it. Join 2 is the
// dependent selected read of this axis's own state cell - it consumes the same
// candidate and both facts before it, which is the computed-coordinate normal
// form, because the resource is a function of the Value fact and which of its
// cells the obligation is about is a function of the Call fact. The fold draws
// the verdict and routes the successor state back to the cell it was read
// from: an operation moves a resource's state, it does not move the resource.
//
// Every read propagates authenticated opaque evidence rather than refusing it,
// and the population does not shrink when it arrives: the reads are
// candidate-only, so the row set is the actual's own, and only the outcome of
// a read varies. A receiver or a callee the analysis cannot follow is exactly
// the case the judgment answers with an unproven verdict; refusing the read
// would drop the call from the population and report nothing, which is the one
// answer a soundness judgment may not give.
func Obligation() ruleprogram.Program {
	typestateAxis := axisReference(AxisKey)
	valueAxis := axisReference(valueAxisKey)
	callAxis := axisReference(callAxisKey)
	exact := ruleprogram.ReadContract{
		Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
		OnOpaque: ruleprogram.OnOpaquePropagateAuthenticated, Multiplicity: ruleprogram.MultiplicityOne,
	}
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
					Contract:   exact,
				},
			},
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: callAxis, Member: TypestateCallSites},
				Key:      member.ProjectionRef{Axis: callAxis, Member: TypestateCallSiteKey},
				Read: ruleprogram.ReadDecl{
					Input: 0,
					Axis:  ruleprogram.AxisRef(callAxis),
					Form:  ruleprogram.Exact,
					// The site's fact is taken against its own transported
					// predecessor at this port, exactly as the actual's is.
					PointBound: ruleprogram.PointBound,
					Contract:   exact,
				},
			},
			{
				Sources:   []ruleprogram.SourceRef{ruleprogram.CandidateSource(), ruleprogram.PriorSource(0), ruleprogram.PriorSource(1)},
				Relation:  member.RelationRef{Axis: typestateAxis, Member: StateCells},
				Key:       member.ProjectionRef{Axis: typestateAxis, Member: StateCellKey},
				Predicate: member.ProjectionRef{Axis: typestateAxis, Member: StateCellProtocol},
				Selection: member.SelectionRef{Axis: typestateAxis, Member: StateCellSelection},
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
			Inputs:  []ruleprogram.JoinRef{0, 1, 2},
			Outputs: []ruleprogram.OutputDecl{{
				Column:           axis.OutputRef{Axis: typestateAxis, Key: StateOutputKey},
				Destination:      member.ProjectionRef{Axis: typestateAxis, Member: StateCellDestination},
				Mode:             ruleprogram.ModeRoute,
				ValueSlot:        0,
				RouteJoin:        2,
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
		Dependencies: []schema.Key{"heap", valueAxisKey, callAxisKey},
		Frame:        axis.Frame{Outputs: []axis.Output{{Key: StateOutputKey, Writer: AxisKey}}},
		Catalog:      statecell.AxisMemberCatalog(),
		Signature:    axis.Signature{Key: statecell.CellCarrier, Fact: statecell.StateCarrier},
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
