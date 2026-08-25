package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// prerequisiteRouteProgram is the shape four routed Placement declarations
// were written in: two joins whose only role is to materialize the third
// join's relation, and one selected route join the fold actually consumes.
func prerequisiteRouteProgram(t testing.TB, inputs []JoinRef) Program {
	t.Helper()
	declaration := seq5742Program(
		"reducer-shape",
		[]JoinDecl{
			seq5742Join("reducer-shape/first", []SourceRef{CandidateSource()}, Exact, false, false),
			seq5742Join("reducer-shape/second", []SourceRef{CandidateSource()}, Exact, false, false),
			seq5742Join("reducer-shape/route", []SourceRef{PriorSource(0), PriorSource(1)}, Selected, true, true),
		},
		inputs,
		[]OutputDecl{seq5742Output("reducer-shape/write", ModeRoute, 0)},
	)
	declaration.Fold.Outputs[0].RouteJoinPresent = true
	declaration.Fold.Outputs[0].RouteJoin = 2
	return declaration
}

// routeCellReducer is the owner row those folds actually declare: one input,
// the authenticated selected route cell.
func routeCellReducer() member.Reducer {
	return member.Reducer{
		Key: "reducer-shape/reducer",
		Inputs: []member.ReducerInput{{
			Axis:         lawReference("reducer-shape/route/axis"),
			Form:         member.ReadFormSelected,
			Multiplicity: member.MultiplicityOne,
		}},
		Outputs: []member.ReducerOutput{{Axis: lawMemberAxis()}},
	}
}

// TestAPrerequisiteJoinIsNotAReducerArgument is the gate Check alone cannot
// be, stated over the exact declaration that got past it.
//
// A join the reducer does not consume is a PREREQUISITE: it is what another
// join's relation is materialized from, and the graph still reaches it because
// that join depends on it. Naming it as a fold argument is well-formed to a
// Program-local check - the join exists, it is not duplicated, it is reachable
// - and wrong against the owner row, whose call has no parameter for it.
//
// So the arity is decided where the row is, and a declaration package can
// close it against its own catalog before any schema is sealed.
func TestAPrerequisiteJoinIsNotAReducerArgument(t *testing.T) {
	drifted := prerequisiteRouteProgram(t, []JoinRef{0, 1, 2})
	if problem, valid := drifted.Check(); !valid {
		t.Fatalf("the Program-local check refused the specimen for another reason: %+v", problem)
	}
	if problem, valid := drifted.CheckAgainst(routeCellReducer()); valid {
		t.Fatal("a fold passing two prerequisite joins as arguments was admitted against a one-input reducer")
	} else if problem.Kind != ProblemInput {
		t.Fatalf("refusal named %+v, want the fold's argument list", problem)
	}

	corrected := prerequisiteRouteProgram(t, []JoinRef{2})
	if problem, valid := corrected.CheckAgainst(routeCellReducer()); !valid {
		t.Fatalf("the fold that consumes only the route cell was refused: %+v", problem)
	}
}

// TestAFoldArgumentMustArriveInTheFormItsOwnerDeclared states the other half
// of the agreement. Position is not enough: an argument the owner declared as
// one selected member and the Program reads as a whole denominator is a
// different call, and the two spellings of it must not both be admitted.
func TestAFoldArgumentMustArriveInTheFormItsOwnerDeclared(t *testing.T) {
	declaration := prerequisiteRouteProgram(t, []JoinRef{2})
	for _, test := range []struct {
		name  string
		amend func(*member.Reducer)
	}{
		{name: "another-form", amend: func(reducer *member.Reducer) { reducer.Inputs[0].Form = member.ReadFormExact }},
		{name: "another-multiplicity", amend: func(reducer *member.Reducer) {
			reducer.Inputs[0].Multiplicity = member.MultiplicityMany
		}},
		{name: "another-axis", amend: func(reducer *member.Reducer) {
			reducer.Inputs[0].Axis = lawReference("reducer-shape/first/axis")
		}},
		{name: "another-output-arity", amend: func(reducer *member.Reducer) { reducer.Outputs = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			reducer := routeCellReducer()
			test.amend(&reducer)
			if _, valid := declaration.CheckAgainst(reducer); valid {
				t.Fatal("a fold was admitted against a row that declares a different call")
			}
		})
	}
}

// TestASelectionConcludedOnceIsFoldedOverEveryMember states the one place the
// declaration carries two multiplicities that are not the same number.
//
// A read contract's multiplicity bounds the cells of ONE member; a reducer
// input's says whether the fold is handed one cell or the whole delivery. A
// selection the output publishes THROUGH is a cadence - one invocation per
// member - so the two agree by equality. A selection the output does not
// publish through is concluded once over every member, so the fold is
// many-valued while each member is still one cell. Requiring equality there
// would force the read to declare a per-member bound its own execution
// primitive refuses outright.
func TestASelectionConcludedOnceIsFoldedOverEveryMember(t *testing.T) {
	routed := prerequisiteRouteProgram(t, []JoinRef{2})
	cadence := routeCellReducer()
	cadence.Inputs[0].Multiplicity = member.MultiplicityMany
	if _, valid := routed.CheckAgainst(cadence); valid {
		t.Fatal("a fold claiming the whole delivery was admitted over a selection its output publishes through")
	}

	concluded := prerequisiteRouteProgram(t, []JoinRef{2})
	concluded.Fold.Outputs[0].Mode = ModeExact
	concluded.Fold.Outputs[0].RouteJoinPresent = false
	concluded.Fold.Outputs[0].RouteJoin = 0
	whole := routeCellReducer()
	whole.Inputs[0].Multiplicity = member.MultiplicityMany
	if problem, valid := concluded.CheckAgainst(whole); !valid {
		t.Fatalf("a fold over every member of a selection it does not publish through was refused: %+v", problem)
	}
	if problem, valid := concluded.CheckAgainst(routeCellReducer()); !valid {
		t.Fatalf("a fold over one member of that same selection was refused: %+v", problem)
	}
}
