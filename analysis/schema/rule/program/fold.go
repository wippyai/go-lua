package program

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// OutputMode is the closed publication mode vocabulary. The destination is
// always an owner-issued projection, including exact writes.
type OutputMode uint8

const (
	ModeInvalid OutputMode = iota
	ModeExact
	ModeRoute
	ModeStructural
)

func (mode OutputMode) Available() bool { return mode >= ModeExact && mode <= ModeStructural }

// OutputDecl maps one reducer slot to a declared output column and destination
// projection. A routed output must name the JoinDecl that produces its route;
// the zero JoinRef is valid, so RouteJoinPresent is explicit. ValueSlot is
// bounded by the exact output arity (len(Outputs)) during sealing.
type OutputDecl struct {
	Column           axis.OutputRef
	Destination      member.ProjectionRef
	Mode             OutputMode
	ValueSlot        uint16
	RouteJoin        JoinRef
	RouteJoinPresent bool
}

func (output OutputDecl) Available() bool {
	return output.Column.Available() && output.Destination.Available() && output.Mode.Available()
}

// CarryMode is the closed disposition of an optional whole-output carry.
// Identity carries the output fact as-is; Transform applies one owner-issued
// candidate-indexed transform. The transform reference is meaningful only in
// the latter mode.
type CarryMode uint8

const (
	CarryModeInvalid CarryMode = iota
	CarryIdentity
	CarryTransform
)

func (mode CarryMode) Available() bool {
	return mode == CarryIdentity || mode == CarryTransform
}

// CarryDecl is the optional rule-level carry declaration. Its input port is
// part of the same contiguous input prefix as ReadDecl.Input values.
type CarryDecl struct {
	Input     InputRef
	Mode      CarryMode
	Transform member.CarryTransformRef
	// Output is the authored destination geometry of a transforming carry.
	// It is the same sealed algebra vocabulary used by an ordinary Apply: a
	// scalar/span source names one exact child cell and OwnerNamed delegates
	// the row identity to the operation.  The relcompiler transports this
	// value unchanged; it must not rediscover a destination by relation or
	// ordinal.
	Output algebra.OutputAddress
}

func (carry CarryDecl) Available() bool {
	if !carry.Mode.Available() {
		return false
	}
	switch carry.Mode {
	case CarryIdentity:
		return !carry.Transform.Declared() && !carry.Output.Available()
	case CarryTransform:
		// A transforming carry is a complete declaration only when it carries
		// both the owner-issued transform and its authenticated destination
		// geometry. Resolve/Compile repeat the boundary check for independently
		// assembled inputs, but Program.Check must fail closed at declaration
		// seal rather than make a malformed program look valid.
		return carry.Transform.Available() && carry.Output.Available()
	default:
		return false
	}
}

func (carry CarryDecl) References() schema.EntryReferences {
	if carry.Transform.Declared() {
		return schema.EntryReferences{carry.Transform.EntryReference()}
	}
	return nil
}

// FoldDecl is the sole family semantic declaration. Reducer is data naming an
// owner-issued reducer; execution functions and AxisRuntime bindings do not
// cross this package boundary.
type FoldDecl struct {
	Reducer member.ReducerRef
	// Inputs is the ordered semantic argument list. A JoinRef may occur more
	// than once when one correlated read row supplies several reducer slots;
	// its later cell addresses distinguish the slots. It is not a set of
	// prerequisite joins.
	Inputs  []JoinRef
	Outputs []OutputDecl
}

type foldProblem uint8

const (
	foldProblemNone foldProblem = iota
	foldProblemReducer
	foldProblemInputs
	foldProblemOutputs
)

// checkAgainst is the reducer-shape agreement: the clauses that need the owner
// reducer row this fold names, and nothing more.
//
// A fold's argument list is positional against that row. A join the reducer
// does not consume is a PREREQUISITE - the materialization another join's
// relation depends on - and naming it as an argument is well-formed in
// isolation and wrong against the owner: the call would be handed a carrier
// the fold has no parameter for. Nothing local to a Program can see that, so
// this clause exists to be reachable from a declaration package that has its
// own catalog, and the Plan compiler calls the same one.
//
// The carrier and tag clauses stay with the compiler. Which carrier a join
// yields is the joined axis's statement, and a declaration package holds only
// its own axis's catalog.
func (fold FoldDecl) checkAgainst(joins []JoinDecl, reducer member.Reducer) foldProblem {
	if problem := fold.check(len(joins)); problem != foldProblemNone {
		return problem
	}
	if len(reducer.Inputs) != len(fold.Inputs) {
		return foldProblemInputs
	}
	// A fold's declared output CARRIERS are the facts it publishes, and its
	// declared output COLUMNS are where it publishes them. For every ordinary
	// publication those are the same count: one carrier per column.
	//
	// A structural publication writes no fact into any column - its output is
	// the activation row set its branches mount - so it declares no carrier at
	// all while still naming the column its rows are indexed by. The equality
	// therefore holds exactly when the publication is not structural, and the
	// reducer's own Structural marker must agree with the modes the fold
	// declares. A declaration where the two disagree is one half of it
	// describing a fact the other half does not publish.
	structural, uniform := fold.structuralPublication()
	if !uniform || reducer.Structural != structural {
		return foldProblemOutputs
	}
	if structural {
		if len(reducer.Outputs) != 0 {
			return foldProblemOutputs
		}
	} else if len(reducer.Outputs) != len(fold.Outputs) {
		return foldProblemOutputs
	}
	for position, input := range fold.Inputs {
		if uint64(input) >= uint64(len(joins)) {
			return foldProblemInputs
		}
		join := joins[uint64(input)]
		signature := reducer.Inputs[position]
		if signature.Axis != join.Read.Axis.EntryReference() ||
			signature.Form != join.Read.Form ||
			!multiplicityAgrees(join.Read.Form, fold.publishesThrough(input), join.Read.Contract.Multiplicity, signature.Multiplicity) {
			return foldProblemInputs
		}
	}
	return foldProblemNone
}

// publishesThrough reports whether this fold's output publishes AT the members
// of the named join - that is, whether the join is the route the output is
// addressed by. A fold that publishes through a join is a cadence over it.
func (fold FoldDecl) publishesThrough(input JoinRef) bool {
	for _, output := range fold.Outputs {
		if output.Mode == ModeRoute && output.RouteJoinPresent && output.RouteJoin == input {
			return true
		}
	}
	return false
}

// multiplicityAgrees states the two multiplicities a declaration carries and
// why they are not one number.
//
// A READ contract's multiplicity bounds the cells of ONE member: a selection
// declares one because each member is observed at one exact coordinate, and
// the number of members is bounded by the denominator instead. A REDUCER
// input's multiplicity says whether the fold is handed one of those cells or
// the whole delivery.
//
// They coincide everywhere except one shape. A selection the output PUBLISHES
// THROUGH is folded once per member, so the fold takes one cell and the two
// agree by equality - a many-valued fold there would be claiming to conclude
// once over a delivery it is invoked across. A selection the output does not
// publish through is concluded ONCE, so its fold takes every member as one
// argument: many-valued over a read whose members are one cell each. Requiring
// the two numbers to be equal there would force such a read to declare a
// per-member bound its own primitive refuses.
func multiplicityAgrees(form ReadForm, routed bool, read, fold Multiplicity) bool {
	if form == Selected && !routed && read == MultiplicityOne {
		return fold == MultiplicityOne || fold == MultiplicityMany
	}
	return read == fold
}

// structuralPublication answers whether this fold publishes structurally, and
// whether its columns agree with each other about it. A fold whose columns
// disagree publishes a fact and no fact at once, which no reducer signature
// could satisfy.
func (fold FoldDecl) structuralPublication() (structural bool, uniform bool) {
	if len(fold.Outputs) == 0 {
		return false, false
	}
	structural = fold.Outputs[0].Mode == ModeStructural
	for _, output := range fold.Outputs[1:] {
		if (output.Mode == ModeStructural) != structural {
			return false, false
		}
	}
	return structural, true
}

func (fold FoldDecl) check(joinCount int) foldProblem {
	if !fold.Reducer.Available() {
		return foldProblemReducer
	}
	if joinCount == 0 {
		if len(fold.Inputs) != 0 {
			return foldProblemInputs
		}
	} else if len(fold.Inputs) == 0 {
		return foldProblemInputs
	}
	for _, input := range fold.Inputs {
		if uint64(input) >= uint64(joinCount) {
			return foldProblemInputs
		}
	}
	if len(fold.Outputs) == 0 {
		return foldProblemOutputs
	}
	seenColumns := make(map[axis.OutputRef]struct{}, len(fold.Outputs))
	seenSlots := make(map[uint16]struct{}, len(fold.Outputs))
	for _, output := range fold.Outputs {
		if !output.Available() || int(output.ValueSlot) >= len(fold.Outputs) {
			return foldProblemOutputs
		}
		if _, duplicate := seenColumns[output.Column]; duplicate {
			return foldProblemOutputs
		}
		seenColumns[output.Column] = struct{}{}
		if _, duplicate := seenSlots[output.ValueSlot]; duplicate {
			return foldProblemOutputs
		}
		seenSlots[output.ValueSlot] = struct{}{}
	}
	return foldProblemNone
}

func (fold FoldDecl) References() schema.EntryReferences {
	var references schema.EntryReferences
	if fold.Reducer.Declared() {
		references = append(references, fold.Reducer.EntryReference())
	}
	for _, output := range fold.Outputs {
		if output.Column.Declared() {
			references = append(references, output.Column.AxisReference())
		}
		if output.Destination.Declared() {
			references = append(references, output.Destination.EntryReference())
		}
	}
	return references
}

func cloneFold(fold FoldDecl) FoldDecl {
	fold.Inputs = append([]JoinRef(nil), fold.Inputs...)
	fold.Outputs = append([]OutputDecl(nil), fold.Outputs...)
	return fold
}
