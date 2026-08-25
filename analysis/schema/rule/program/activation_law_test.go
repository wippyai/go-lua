package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// activation_law_test.go states the branch vocabulary of a structural
// publication: the owner-issued identities the construct plane mounts one
// activation member AS.
//
// They are declared and not derived because the analyzer minted none of them.
// A module, a body path, the semantic axis a role is issued under - each names
// a subject outside this analyzer, so no dense coordinate carries one and no
// engine rule could reconstruct one from the row's address.
//
// The execution CONTEXT a branch runs on is deliberately NOT among them. Which
// Contexts two modules are connected by is the Link's own sealed directory,
// which the issuance pass already holds; a rule's axis restating it would be a
// second authority over the Link's relation.

func activationLawProjection(key string) member.ProjectionRef {
	return member.ProjectionRef{Axis: lawMemberAxis(), Member: schema.Key(key)}
}

func activationLawBranch() ActivationDecl {
	return ActivationDecl{
		Branch:      lawRelation("activation-law/branches"),
		Transport:   []TransportDecl{{Axis: AxisRef(lawReference("activation-law/transport"))}},
		Application: activationLawProjection("activation-law/application"),
		Target:      activationLawProjection("activation-law/target"),
		Endpoint:    activationLawProjection("activation-law/endpoint"),
		Mount:       activationLawProjection("activation-law/mount"),
		Body:        activationLawProjection("activation-law/body"),
		Crossing:    lawRelation("activation-law/branches"),
	}
}

// activationLawProgram is the A form as a whole: ONE exact trigger read, a
// structural publication, the transport vector one branch instantiates, the
// family its branches are grouped under, and the branch identities.
//
// There is no branch READ. The branch set is enumerated through its relation's
// own owner - a branch carries no fact any judgment consumes and has no
// coordinate to be read at - so the A form declares one read, not two.
func activationLawProgram() Program {
	branch := activationLawBranch()
	program := seq5742Program(
		"activation-law",
		[]JoinDecl{
			seq5742Join("activation-law/trigger", []SourceRef{CandidateSource()}, Exact, false, false),
		},
		[]JoinRef{0},
		[]OutputDecl{seq5742Output("activation-law/write", ModeStructural, 0)},
	)
	program.ActivationRole = schema.Key("semantic/activation-family/activation-law")
	program.Activation = &branch
	return program
}

// TestAStructuralPublicationDeclaresItsBranchIdentities is the whole clause: a
// structural row that names no branch identities has not said what the
// construct plane would mount, and a fact-writing row that names them has
// declared a vocabulary nothing reads.
func TestAStructuralPublicationDeclaresItsBranchIdentities(t *testing.T) {
	if problem, valid := activationLawProgram().Check(); !valid {
		t.Fatalf("the complete A form was rejected: %+v", problem)
	}
	missing := activationLawProgram()
	missing.Activation = nil
	if problem, valid := missing.Check(); valid || problem.Kind != ProblemTransport {
		t.Fatalf("a structural publication with no branch identities: valid=%v problem=%+v", valid, problem)
	}
	fact := seq5742Specimens()["value-transfer"]
	branch := activationLawBranch()
	branch.Transport = nil
	fact.Activation = &branch
	if problem, valid := fact.Check(); valid || problem.Kind != ProblemActivation {
		t.Fatalf("a fact-writing rule declaring branch identities: valid=%v problem=%+v", valid, problem)
	}
}

// TestEveryBranchIdentityIsRequired keeps the row whole. Each of the five
// names one coordinate the mounted activation member is keyed or resolved by,
// so a declaration missing any one of them mounts a branch the plane cannot
// address.
func TestEveryBranchIdentityIsRequired(t *testing.T) {
	for name, damage := range map[string]func(*ActivationDecl){
		"branch":      func(branch *ActivationDecl) { branch.Branch = member.RelationRef{} },
		"application": func(branch *ActivationDecl) { branch.Application = member.ProjectionRef{} },
		"target":      func(branch *ActivationDecl) { branch.Target = member.ProjectionRef{} },
		"endpoint":    func(branch *ActivationDecl) { branch.Endpoint = member.ProjectionRef{} },
		"mount":       func(branch *ActivationDecl) { branch.Mount = member.ProjectionRef{} },
		"body":        func(branch *ActivationDecl) { branch.Body = member.ProjectionRef{} },
	} {
		t.Run(name, func(t *testing.T) {
			program := activationLawProgram()
			branch := activationLawBranch()
			damage(&branch)
			program.Activation = &branch
			if problem, valid := program.Check(); valid || problem.Kind != ProblemActivation {
				t.Fatalf("a branch missing its %s was admitted: valid=%v problem=%+v", name, valid, problem)
			}
		})
	}
}

// TestTheBranchSetIsNamedAsARelationAndNotAsARead is the shape the A form
// rests on. A branch carries no fact any judgment consumes and has no
// coordinate of its own to be read at, so the vocabulary names the RELATION
// whose members the branches are; the issuance pass walks it through that
// relation's owner. Whether it is a nested member set of this rule's candidate
// is the catalog's answer and is checked where the catalog is in scope.
func TestTheBranchSetIsNamedAsARelationAndNotAsARead(t *testing.T) {
	program := activationLawProgram()
	if program.JoinCount() != 1 {
		t.Fatalf("the A form declares %d reads, want only its trigger read", program.JoinCount())
	}
	branch := activationLawBranch()
	branch.Branch = member.RelationRef{}
	program.Activation = &branch
	if problem, valid := program.Check(); valid || problem.Kind != ProblemActivation {
		t.Fatalf("a vocabulary naming no branch relation: valid=%v problem=%+v", valid, problem)
	}
}

// TestTheBranchIdentitiesAreCarriedIntoTheDigest keeps the declaration's
// canonical content honest: two A forms that mount their branches as different
// identities are different rules, and a digest that ignored the vocabulary
// would call them the same.
func TestTheBranchIdentitiesAreCarriedIntoTheDigest(t *testing.T) {
	left := activationLawProgram()
	right := activationLawProgram()
	branch := activationLawBranch()
	branch.Target = activationLawProjection("activation-law/other-target")
	right.Activation = &branch
	if !left.Digest().Available() || !right.Digest().Available() {
		t.Fatal("a valid A form has no canonical digest")
	}
	if left.Digest() == right.Digest() {
		t.Fatal("two A forms mounting different branch identities share one digest")
	}
}

// TestTheBranchIdentitiesAreResolvableReferences states that the vocabulary
// takes part in upward resolution the way every other member reference does:
// the AXIS each projection is declared on is published, and the member key
// itself is authenticated against that axis's own catalog rather than by the
// surface resolver. That is exactly how a Fold's Destination is carried.
//
// A vocabulary that published nothing would be a set of projections the seal
// never sees, on an axis it never proves exists.
func TestTheBranchIdentitiesAreResolvableReferences(t *testing.T) {
	declared := activationLawProgram()
	absent := activationLawProgram()
	absent.ActivationRole, absent.Activation = "", nil
	// One reference per identity projection, the branch relation itself, the
	// one relation the whole vector crosses the edge as, and one owner
	// reference per ordered transport axis.
	want := len(absent.References()) + len(declared.Activation.projections()) + 2 + len(declared.Activation.Transport)
	if len(declared.References()) != want {
		t.Fatalf("the branch vocabulary contributes %d references, want %d - one per projection, one for the branch relation, one for the crossing and one per transport axis",
			len(declared.References())-len(absent.References()), len(declared.Activation.projections())+2+len(declared.Activation.Transport))
	}
	for _, reference := range declared.Activation.references() {
		if !reference.Declared() {
			t.Fatal("a branch identity published an undeclared axis reference")
		}
	}
}

// foldLawInputsFor builds the reducer input row every join of one declaration
// requires. The laws below are about OUTPUT arity, so the input side is taken
// from the joins themselves rather than restated and left free to disagree.
func foldLawInputsFor(program Program) []member.ReducerInput {
	inputs := make([]member.ReducerInput, 0, len(program.Fold.Inputs))
	for _, reference := range program.Fold.Inputs {
		join := program.Joins[uint64(reference)]
		input := member.ReducerInput{
			Axis:         join.Read.Axis.EntryReference(),
			Carrier:      "carrier/fold-law/fact",
			Form:         join.Read.Form,
			Multiplicity: join.Read.Contract.Multiplicity,
		}
		if join.Read.Form == Summary {
			input.Tag = "carrier/fold-law/ordinal"
		}
		inputs = append(inputs, input)
	}
	return inputs
}

// TestAStructuralFoldAgreesWithItsReducerAboutPublishingNoFact states the
// biconditional the reducer contract now carries. The equality between
// declared output carriers and declared output columns is not weakened: it is
// restated as "equal unless the publication is structural, in which case there
// are no carriers at all", and the reducer's own marker must agree.
func TestAStructuralFoldAgreesWithItsReducerAboutPublishingNoFact(t *testing.T) {
	program := activationLawProgram()
	structuralReducer := member.Reducer{
		Key:        "activation-law/reducer",
		Structural: true,
		Inputs:     foldLawInputsFor(program),
	}
	if problem := program.Fold.checkAgainst(program.Joins, structuralReducer); problem != foldProblemNone {
		t.Fatalf("a structural fold and its carrier-free reducer disagreed: %v", problem)
	}

	// A carrier on a structural fold is a fact it has nowhere to publish.
	publishing := structuralReducer
	publishing.Structural = false
	publishing.Outputs = []member.ReducerOutput{{Axis: lawMemberAxis(), Carrier: "carrier/activation-law/fact"}}
	if program.Fold.checkAgainst(program.Joins, publishing) == foldProblemNone {
		t.Fatal("a structural fold accepted a reducer that publishes a fact")
	}

	// A marker without the shape, and a shape without the marker, are both a
	// declaration disagreeing with itself.
	mismatched := structuralReducer
	mismatched.Structural = false
	if program.Fold.checkAgainst(program.Joins, mismatched) == foldProblemNone {
		t.Fatal("a carrier-free reducer that does not declare itself structural was admitted")
	}
}

// TestAnOrdinaryFoldStillMatchesItsReducerCarrierForCarrier is the half that
// must not move: every fact-writing rule keeps one output carrier per column.
func TestAnOrdinaryFoldStillMatchesItsReducerCarrierForCarrier(t *testing.T) {
	transfer := seq5742Specimens()["value-transfer"]
	ordinary := member.Reducer{
		Key:     "value-transfer/reducer",
		Inputs:  foldLawInputsFor(transfer),
		Outputs: []member.ReducerOutput{{Axis: lawMemberAxis(), Carrier: "carrier/value-transfer/fact"}},
	}
	if problem := transfer.Fold.checkAgainst(transfer.Joins, ordinary); problem != foldProblemNone {
		t.Fatalf("an ordinary fold and its one-carrier reducer disagreed: %v", problem)
	}
	structural := ordinary
	structural.Structural, structural.Outputs = true, nil
	if transfer.Fold.checkAgainst(transfer.Joins, structural) == foldProblemNone {
		t.Fatal("a fact-writing fold accepted a reducer that publishes nothing")
	}
}
