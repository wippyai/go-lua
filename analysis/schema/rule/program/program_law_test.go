package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func lawReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func lawMemberAxis() schema.EntryReference { return lawReference("axis/law-owner") }

func lawRelation(key string) member.RelationRef {
	return member.RelationRef{Axis: lawMemberAxis(), Member: schema.Key(key)}
}

func lawProjection(key string) member.ProjectionRef {
	return member.ProjectionRef{Axis: lawMemberAxis(), Member: schema.Key(key)}
}

func lawReducer(key string) member.ReducerRef {
	return member.ReducerRef{Axis: lawMemberAxis(), Member: schema.Key(key)}
}

func lawColumn(key string) axis.OutputRef {
	return axis.OutputRef{Axis: lawMemberAxis(), Key: schema.Key(key)}
}

func lawDenominator(key string) DenominatorRef {
	return DenominatorRef(schema.EntryReference{Surface: schema.SurfaceKindDenominator, Key: schema.Key(key)})
}

func lawCarryTransform(key string) member.CarryTransformRef {
	return member.CarryTransformRef{Axis: lawMemberAxis(), Member: schema.Key(key)}
}

func lawRead(form ReadForm, key string, denominator bool) ReadDecl {
	read := ReadDecl{
		Axis:       AxisRef(lawReference(key + "/axis")),
		Form:       form,
		PointBound: PointBound,
		Contract: ReadContract{
			Order:        OrderCanonical,
			Sparse:       SparseExplicit,
			OnOpaque:     OnOpaqueRefuse,
			Multiplicity: MultiplicityOne,
		},
	}
	if denominator {
		read.Contract.DenominatorRef = lawDenominator(key + "/denominator")
	}
	return read
}

func lawOutput(slot uint16, key string) OutputDecl {
	return OutputDecl{
		Column:      lawColumn(key + "/column"),
		Destination: lawProjection(key + "/destination"),
		Mode:        ModeExact,
		ValueSlot:   slot,
	}
}

func lawFold(inputs []JoinRef) FoldDecl {
	return FoldDecl{
		Reducer: lawReducer("fold/reducer"),
		Inputs:  inputs,
		Outputs: []OutputDecl{lawOutput(0, "fold/output")},
	}
}

func lawExactJoin(key string) JoinDecl {
	return JoinDecl{
		Sources:  []SourceRef{CandidateSource()},
		Relation: lawRelation(key + "/relation"),
		Key:      lawProjection(key + "/key"),
		Read:     lawRead(Exact, key, false),
	}
}

func lawProgram(t *testing.T) Program {
	t.Helper()
	return Program{
		OperandRole: "semantic/operand/law",
		Candidate:   member.AxisRelationCandidate(lawRelation("candidate")),
		Joins:       []JoinDecl{lawExactJoin("input")},
		Fold:        lawFold([]JoinRef{0}),
	}
}

func TestProgramSealsUniformJoinFoldDeclaration(t *testing.T) {
	program := lawProgram(t)
	if problem, valid := program.Check(); !valid {
		t.Fatalf("program rejected: %+v", problem)
	}
	if !program.Digest().Available() || program.JoinCount() != 1 {
		t.Fatal("valid cold declaration did not produce canonical identity")
	}
}

func TestProgramRequiresOwnerIssuedOperandRole(t *testing.T) {
	program := lawProgram(t)
	program.OperandRole = ""
	problem, valid := program.Check()
	if valid || problem.Kind != ProblemOperand {
		t.Fatalf("missing operand role valid=%v problem=%+v", valid, problem)
	}
}

func TestProgramDigestCoversOperandRole(t *testing.T) {
	left := lawProgram(t)
	right := left.Clone()
	right.OperandRole = "semantic/operand/law-other"
	if left.Digest() == right.Digest() {
		t.Fatal("operand role did not move Program identity")
	}
}

func TestProgramAllowsZeroJoinSeedWithUnitFold(t *testing.T) {
	seed := Program{
		OperandRole: "semantic/operand/law",
		Candidate:   member.AxisRelationCandidate(lawRelation("seed/candidate")),
		Fold:        lawFold(nil),
	}
	if problem, valid := seed.Check(); !valid {
		t.Fatalf("zero-join seed rejected: %+v", problem)
	}
}

func TestProgramRejectsMissingCandidateAndMalformedSeed(t *testing.T) {
	seed := Program{OperandRole: "semantic/operand/law", Fold: lawFold(nil)}
	problem, valid := seed.Check()
	if valid || problem.Kind != ProblemCandidate {
		t.Fatalf("missing candidate valid=%v problem=%+v", valid, problem)
	}
	seed = Program{OperandRole: "semantic/operand/law", Candidate: member.AxisRelationCandidate(lawRelation("seed/candidate")), Fold: lawFold([]JoinRef{0})}
	problem, valid = seed.Check()
	if valid || problem.Kind != ProblemInput {
		t.Fatalf("seed input valid=%v problem=%+v", valid, problem)
	}
}

func TestProgramRequiresOwnerMemberKeysOnTypedReferences(t *testing.T) {
	program := lawProgram(t)
	program.Candidate.AxisRelation.Member = ""
	problem, valid := program.Check()
	if valid || problem.Kind != ProblemCandidate {
		t.Fatalf("missing candidate member valid=%v problem=%+v", valid, problem)
	}

	program = lawProgram(t)
	program.Joins[0].Key.Member = ""
	problem, valid = program.Check()
	if valid || problem.Kind != ProblemJoin {
		t.Fatalf("missing key member valid=%v problem=%+v", valid, problem)
	}

	program = lawProgram(t)
	program.Fold.Reducer.Member = ""
	problem, valid = program.Check()
	if valid || problem.Kind != ProblemFold {
		t.Fatalf("missing reducer member valid=%v problem=%+v", valid, problem)
	}
}

func TestJoinSourcesAreCandidateOrEarlierResultsOnly(t *testing.T) {
	program := lawProgram(t)
	program.Joins[0].Sources = []SourceRef{{Position: 1}}
	problem, valid := program.Check()
	if valid || problem.Kind != ProblemJoin || problem.Join != 0 {
		t.Fatalf("future source valid=%v problem=%+v", valid, problem)
	}

	program = lawProgram(t)
	program.Joins = append(program.Joins, JoinDecl{
		Sources:   []SourceRef{{Position: 0}},
		Relation:  lawRelation("second/relation"),
		Key:       lawProjection("second/key"),
		Selection: lawSelection("second/selection"),
		Read:      lawRead(Exact, "second", false),
	})
	program.Fold.Inputs = []JoinRef{1}
	if problem, valid = program.Check(); !valid {
		t.Fatalf("earlier source rejected: %+v", problem)
	}
}

func TestProgramAdmitsOnlyTheFiveNormalFormCombinations(t *testing.T) {
	cases := []struct {
		name string
		join JoinDecl
	}{
		{name: "exact", join: lawExactJoin("exact")},
		{name: "selected", join: JoinDecl{
			Sources: []SourceRef{CandidateSource()}, Relation: lawRelation("selected/relation"),
			Key: lawProjection("selected/key"), Predicate: lawProjection("selected/predicate"),
			Selection: lawSelection("selected/selection"),
			Read:      lawRead(Selected, "selected", true),
		}},
		// A selected read is a dependent keyed relation read. Its predicate is
		// tag metadata, so a member set already addressed by (parent, ordinal)
		// declares none - and that is the same normal form, not an exception
		// some enclosing output grants.
		{name: "selected-untagged", join: JoinDecl{
			Sources: []SourceRef{CandidateSource()}, Relation: lawRelation("untagged/relation"),
			Key:  lawProjection("untagged/key"),
			Read: lawRead(Selected, "untagged", true),
		}},
		{name: "summary", join: JoinDecl{
			Sources: []SourceRef{CandidateSource()}, Relation: lawRelation("summary/relation"),
			Key: lawProjection("summary/key"), Predicate: lawProjection("summary/predicate"),
			Selection: lawSelection("summary/selection"),
			Read:      lawRead(Summary, "summary", true),
		}},
		// A summary over a self-provided nested member set is already
		// addressed by (parent, ordinal): declaring Parent restates that fact
		// and the Predicate a non-member-set summary needs is left out, the
		// same shape selected-untagged states for Selected reads.
		{name: "summary-nested", join: JoinDecl{
			Sources: []SourceRef{CandidateSource()}, Relation: lawRelation("summary-nested/relation"),
			Key: lawProjection("summary-nested/key"), Parent: lawRelation("summary-nested/parent"),
			Read: lawRead(Summary, "summary-nested", true),
		}},
		{name: "complete", join: JoinDecl{
			Sources: []SourceRef{CandidateSource()}, Relation: lawRelation("complete/relation"),
			Key: lawProjection("complete/key"), Read: lawRead(Complete, "complete", true),
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			program := Program{OperandRole: "semantic/operand/law", Candidate: member.AxisRelationCandidate(lawRelation("candidate/" + test.name)), Joins: []JoinDecl{test.join}, Fold: lawFold([]JoinRef{0})}
			if problem, valid := program.Check(); !valid {
				t.Fatalf("normal form rejected: %+v", problem)
			}
		})
	}

	program := lawProgram(t)
	program.Joins[0].Predicate = lawProjection("unexpected/predicate")
	program.Joins[0].Selection = lawSelection("unexpected/selection")
	if _, valid := program.Check(); valid {
		t.Fatal("exact join with predicate admitted")
	}
	// What a selected read refuses is a predicate it DECLARED and cannot
	// resolve. Declaring none is the untagged form above; declaring a broken
	// one is a row that names a projection nothing issues.
	program = lawProgram(t)
	program.Joins[0].Read = lawRead(Selected, "declared/predicate", true)
	program.Joins[0].Predicate = member.ProjectionRef{Axis: lawMemberAxis()}
	if _, valid := program.Check(); valid {
		t.Fatal("selected join with an unresolvable declared predicate admitted")
	}
	program = lawProgram(t)
	program.Joins[0].Read = lawRead(Summary, "missing/denominator", false)
	program.Joins[0].Predicate = lawProjection("summary/predicate")
	if _, valid := program.Check(); valid {
		t.Fatal("summary without denominator admitted")
	}
	// Parent licenses the summary-nested form above only for the relation it
	// names. A relation that is not a self-provided nested member set still
	// needs Predicate: leaving both out does not seal by omission.
	program = lawProgram(t)
	program.Joins[0].Read = lawRead(Summary, "unlicensed/summary", true)
	problem, valid := program.Check()
	if valid || problem.Kind != ProblemJoin {
		t.Fatalf("summary with neither predicate nor parent admitted: valid=%v problem=%+v", valid, problem)
	}
}

func TestProgramRejectsDisconnectedJoinsAndDuplicateFoldSlots(t *testing.T) {
	program := lawProgram(t)
	program.Joins = append(program.Joins, lawExactJoin("unused"))
	problem, valid := program.Check()
	if valid || problem.Kind != ProblemJoin || problem.Join != 1 {
		t.Fatalf("disconnected join valid=%v problem=%+v", valid, problem)
	}

	program = lawProgram(t)
	program.Fold.Outputs = []OutputDecl{lawOutput(0, "left"), lawOutput(0, "right")}
	problem, valid = program.Check()
	if valid || problem.Kind != ProblemOutput {
		t.Fatalf("duplicate output slot valid=%v problem=%+v", valid, problem)
	}
}

func TestFoldMayPublishDistinctColumnsAtOneOwnedDestination(t *testing.T) {
	program := lawProgram(t)
	destination := lawProjection("shared/destination")
	program.Fold.Outputs = []OutputDecl{
		{Column: lawColumn("left/column"), Destination: destination, Mode: ModeExact, ValueSlot: 0},
		{Column: lawColumn("right/column"), Destination: destination, Mode: ModeExact, ValueSlot: 1},
	}
	if problem, valid := program.Check(); !valid {
		t.Fatalf("distinct columns at one owner-issued coordinate rejected: %+v", problem)
	}
}

func TestProgramClonePreservesOrderedDeclarations(t *testing.T) {
	program := lawProgram(t)
	clone := program.Clone()
	program.Joins[0].Sources[0].Candidate = false
	program.Fold.Inputs[0] = 99
	program.Fold.Outputs[0].Destination = lawProjection("mutated")
	if !clone.Joins[0].Sources[0].Candidate || clone.Fold.Inputs[0] != 0 || clone.Fold.Outputs[0].Destination == lawProjection("mutated") {
		t.Fatal("clone shares declaration storage")
	}
}

func TestProgramReferencesIncludeCandidateInlineReadsAndFold(t *testing.T) {
	program := lawProgram(t)
	program.Joins[0].Read.Contract.DenominatorRef = lawDenominator("exact/denominator")
	program.Joins[0].Read.Contract.Sparse = SparseDefault
	program.Joins[0].Predicate = lawProjection("predicate")
	program.Joins[0].Read.Form = Selected
	references := program.References()
	if len(references) != 9 {
		t.Fatalf("references=%d, want candidate, join relation/key/predicate/axis/denominator, reducer, column, destination", len(references))
	}
	for _, reference := range references {
		if !reference.Available() {
			t.Fatalf("invalid reference: %#v", reference)
		}
	}
}

func TestProgramDigestTracksOrderedColdData(t *testing.T) {
	program := lawProgram(t)
	first := program.Digest()
	program.Joins[0].Key = lawProjection("changed/key")
	if first == program.Digest() {
		t.Fatal("key projection omitted from digest")
	}
}

func TestProgramDigestTracksJoinParent(t *testing.T) {
	program := lawProgram(t)
	program.Joins[0].Read = lawRead(Summary, "digest-parent", true)
	program.Joins[0].Parent = lawRelation("digest-parent/parent")
	first := program.Digest()
	if !first.Available() {
		t.Fatal("summary join with a declared parent has no digest")
	}
	program.Joins[0].Parent = lawRelation("digest-parent/other-parent")
	if first == program.Digest() {
		t.Fatal("join parent omitted from digest")
	}
}

func TestProgramDigestTracksReadInputPort(t *testing.T) {
	program := lawProgram(t)
	second := lawExactJoin("second-input")
	second.Sources = []SourceRef{PriorSource(0)}
	second.Selection = lawSelection("second-input/selection")
	second.Read.Input = 1
	program.Joins = append(program.Joins, second)
	program.Fold.Inputs = []JoinRef{0, 1}
	first := program.Digest()
	if !first.Available() {
		t.Fatal("valid two-input program has no digest")
	}
	program.Joins[1].Read.Input = 0
	if first == program.Digest() {
		t.Fatal("read input port omitted from digest")
	}
}

func TestProgramSealsCarryInputPortUnionAsAContiguousPrefix(t *testing.T) {
	program := lawProgram(t)
	program.Carry = &CarryDecl{Input: 1, Mode: CarryIdentity}
	if problem, valid := program.Check(); !valid {
		t.Fatalf("nonzero carry input rejected: %+v", problem)
	}
	if got, want := program.InputCount(), 2; got != want {
		t.Fatalf("input count=%d, want %d", got, want)
	}
	program.Carry.Input = 2
	problem, valid := program.Check()
	if valid || problem.Kind != ProblemInput {
		t.Fatalf("hole in read/carry input ports valid=%v problem=%+v", valid, problem)
	}
}

func TestProgramAdmitsCarryOnlyInputPortWithoutARead(t *testing.T) {
	program := Program{OperandRole: "semantic/operand/law", Candidate: member.AxisRelationCandidate(lawRelation("carry-only/candidate")), Fold: lawFold(nil), Carry: &CarryDecl{Input: 0, Mode: CarryIdentity}}
	if problem, valid := program.Check(); !valid {
		t.Fatalf("carry-only input port rejected: %+v", problem)
	}
}

func TestProgramRejectsInvalidCarryModeCombinations(t *testing.T) {
	program := lawProgram(t)
	program.Carry = &CarryDecl{Input: 0, Mode: CarryModeInvalid}
	if _, valid := program.Check(); valid {
		t.Fatal("invalid carry mode admitted")
	}
	program.Carry = &CarryDecl{Input: 0, Mode: CarryIdentity, Transform: lawCarryTransform("identity-transform")}
	if _, valid := program.Check(); valid {
		t.Fatal("identity carry with transform reference admitted")
	}
	program.Carry = &CarryDecl{Input: 0, Mode: CarryTransform}
	if _, valid := program.Check(); valid {
		t.Fatal("transform carry without transform reference admitted")
	}
}

func TestProgramCarryDigestTracksModeInputAndTransform(t *testing.T) {
	program := lawProgram(t)
	program.Carry = &CarryDecl{Input: 0, Mode: CarryIdentity}
	identityDigest := program.Digest()
	if !identityDigest.Available() {
		t.Fatal("identity carry program has no digest")
	}
	program.Carry.Input = 1
	if identityDigest == program.Digest() {
		t.Fatal("carry input port omitted from digest")
	}
	program.Carry.Input = 0
	program.Carry.Mode = CarryTransform
	program.Carry.Transform = lawCarryTransform("carry-transform")
	transformDigest := program.Digest()
	if identityDigest == transformDigest || !transformDigest.Available() {
		t.Fatal("carry mode/transform omitted from digest")
	}
}

func TestProgramDigestTracksOwnerMemberIdentity(t *testing.T) {
	program := lawProgram(t)
	first := program.Digest()
	program.Joins[0].Key.Member = "changed/member"
	if first == program.Digest() {
		t.Fatal("owner-issued member key omitted from digest")
	}
}

func TestProgramDigestTracksOwnerOutputIdentity(t *testing.T) {
	program := lawProgram(t)
	first := program.Digest()
	program.Fold.Outputs[0].Column.Key = "changed/output"
	if first == program.Digest() {
		t.Fatal("owner-qualified output key omitted from digest")
	}
}

func TestProgramHasNoSmallJoinOrSourceCap(t *testing.T) {
	const count = 1024
	joins := make([]JoinDecl, count)
	for index := range joins {
		source := SourceRef{Candidate: true}
		if index != 0 {
			source = PriorSource(uint64(index - 1))
		}
		key := "join/" + string(rune(index))
		joins[index] = JoinDecl{
			Sources: []SourceRef{source}, Relation: lawRelation(key + "/relation"), Key: lawProjection(key + "/key"), Read: lawRead(Exact, key, false),
		}
		if index != 0 {
			joins[index].Selection = lawSelection(key + "/selection")
		}
	}
	program := Program{OperandRole: "semantic/operand/law", Candidate: member.AxisRelationCandidate(lawRelation("large/candidate")), Joins: joins, Fold: lawFold([]JoinRef{count - 1})}
	if problem, valid := program.Check(); !valid {
		t.Fatalf("large ordered declaration rejected: %+v", problem)
	}
}

// TestReadDeclPointBoundHasNoSilentDefault proves PointBound must be
// authored: a ReadDecl that never states it is unavailable, and a Program
// built from it refuses to seal rather than falling back to an implicit
// disposition.
func TestReadDeclPointBoundHasNoSilentDefault(t *testing.T) {
	program := lawProgram(t)
	program.Joins[0].Read.PointBound = PointBoundInvalid
	if program.Joins[0].Read.Available() {
		t.Fatal("a ReadDecl with no stated PointBound reported itself available")
	}
	if _, valid := program.Check(); valid {
		t.Fatal("a Program with an undeclared PointBound sealed")
	}
}

// TestReadDeclPointBoundSelfIsAuthoredIndependentlyOfForm proves PointBound
// is stated per read, not derived from Form: an Exact read - the form the
// retired construction-time heuristic always treated as point-bound - may
// still be explicitly declared PointBoundSelf, and the declaration seals on
// that authored value.
func TestReadDeclPointBoundSelfIsAuthoredIndependentlyOfForm(t *testing.T) {
	program := lawProgram(t)
	if program.Joins[0].Read.Form != Exact {
		t.Fatalf("law program's join is Form=%v, want Exact", program.Joins[0].Read.Form)
	}
	program.Joins[0].Read.PointBound = PointBoundSelf
	if _, valid := program.Check(); !valid {
		t.Fatal("an Exact read explicitly declared PointBoundSelf did not seal")
	}
	if program.Joins[0].Read.PointBound != PointBoundSelf {
		t.Fatal("the authored PointBoundSelf was not preserved on the Exact-form read")
	}
}
