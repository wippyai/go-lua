package program

import "testing"

// TestRouteSealsAnOptionalTagAndAClosedWidth exercises the route row's
// semantic boundary without any owner catalog. A selected route may omit its
// tag projection, but it still needs a finite read and denominator.
func TestRouteSealsAnOptionalTagAndAClosedWidth(t *testing.T) {
	declaration := seq5742Program(
		"route-law",
		[]JoinDecl{seq5742Join("route-law/selected", []SourceRef{CandidateSource()}, Selected, false, true)},
		[]JoinRef{0},
		[]OutputDecl{seq5742Output("route-law/write", ModeRoute, 0)},
	)
	declaration.Fold.Outputs[0].RouteJoinPresent = true
	declaration.Fold.Outputs[0].RouteJoin = 0
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("optional route tag rejected: %+v", problem)
	}

	declaration.Joins[0].Read.Contract.Multiplicity = MultiplicityMany
	if problem, valid := declaration.Check(); valid || problem.Kind != ProblemJoin {
		t.Fatalf("unbounded route width valid=%v problem=%+v", valid, problem)
	}
}

func TestRouteRejectsDuplicateDestinationAndDanglingSource(t *testing.T) {
	declaration := seq5742Program(
		"route-law-duplicate",
		[]JoinDecl{seq5742Join("route-law-duplicate/selected", []SourceRef{CandidateSource()}, Selected, true, true)},
		[]JoinRef{0},
		[]OutputDecl{seq5742Output("route-law-duplicate/write", ModeRoute, 0)},
	)
	declaration.Fold.Outputs[0].RouteJoinPresent = true
	declaration.Fold.Outputs[0].RouteJoin = 0
	duplicate := seq5742Output("route-law-duplicate/write-2", ModeRoute, 1)
	duplicate.Destination = declaration.Fold.Outputs[0].Destination
	declaration.Fold.Outputs = append(declaration.Fold.Outputs, duplicate)
	if problem, valid := declaration.Check(); valid || problem.Kind != ProblemOutput {
		t.Fatalf("duplicate destination valid=%v problem=%+v", valid, problem)
	}

	declaration = seq5742Program(
		"route-law-dangling",
		[]JoinDecl{seq5742Join("route-law-dangling/selected", []SourceRef{PriorSource(1)}, Selected, true, true)},
		[]JoinRef{0},
		[]OutputDecl{seq5742Output("route-law-dangling/write", ModeRoute, 0)},
	)
	declaration.Fold.Outputs[0].RouteJoinPresent = true
	declaration.Fold.Outputs[0].RouteJoin = 0
	if problem, valid := declaration.Check(); valid || problem.Kind != ProblemJoin || problem.Join != 0 {
		t.Fatalf("dangling route source valid=%v problem=%+v", valid, problem)
	}
}

func TestRouteJoinIsRequiredAndPartOfCanonicalDigest(t *testing.T) {
	declaration := seq5742Program(
		"route-law-digest",
		[]JoinDecl{
			seq5742Join("route-law-digest/first", []SourceRef{CandidateSource()}, Selected, true, true),
			seq5742Join("route-law-digest/second", []SourceRef{PriorSource(0)}, Selected, true, true),
		},
		[]JoinRef{0, 1},
		[]OutputDecl{seq5742Output("route-law-digest/write", ModeRoute, 0)},
	)
	declaration.Fold.Outputs[0].RouteJoin = 1
	first := declaration.Digest()
	if !first.Available() {
		t.Fatal("valid explicit route has no digest")
	}
	declaration.Fold.Outputs[0].RouteJoin = 0
	if first == declaration.Digest() {
		t.Fatal("route-producing JoinRef omitted from digest")
	}
	declaration.Fold.Outputs[0].RouteJoinPresent = false
	if problem, valid := declaration.Check(); valid || problem.Kind != ProblemOutput {
		t.Fatalf("missing explicit route JoinRef valid=%v problem=%+v", valid, problem)
	}
}
