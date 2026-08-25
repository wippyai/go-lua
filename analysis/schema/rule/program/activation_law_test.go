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
		Branch:      1,
		Application: activationLawProjection("activation-law/application"),
		Target:      activationLawProjection("activation-law/target"),
		Endpoint:    activationLawProjection("activation-law/endpoint"),
		Mount:       activationLawProjection("activation-law/mount"),
		Body:        activationLawProjection("activation-law/body"),
	}
}

// activationLawProgram is the A form as a whole: an exact trigger read, a
// parent-declaring branch vector over the same candidate row, a structural
// publication, the transport vector one branch instantiates, the family its
// branches are grouped under, and the branch identities.
func activationLawProgram() Program {
	branch := activationLawBranch()
	program := seq5742Program(
		"activation-law",
		[]JoinDecl{
			seq5742Join("activation-law/trigger", []SourceRef{CandidateSource()}, Exact, false, false),
			seq5742Join("activation-law/branch", []SourceRef{CandidateSource()}, Summary, false, true),
		},
		[]JoinRef{0, 1},
		[]OutputDecl{seq5742Output("activation-law/write", ModeStructural, 0)},
	)
	program.Joins[1].Parent = lawRelation("activation-law/candidate")
	program.Transport = []TransportDecl{{Axis: AxisRef(lawReference("activation-law/transport"))}}
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
	if problem, valid := missing.Check(); valid || problem.Kind != ProblemActivation {
		t.Fatalf("a structural publication with no branch identities: valid=%v problem=%+v", valid, problem)
	}
	fact := seq5742Specimens()["value-transfer"]
	branch := activationLawBranch()
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

// TestTheBranchJoinIsTheOneWhoseMembersAreBranches ties the identity
// vocabulary to the read it projects over. A branch reference naming a join
// that does not exist, or one whose members are not a cold member set, names
// rows the issuance pass could not enumerate.
func TestTheBranchJoinIsTheOneWhoseMembersAreBranches(t *testing.T) {
	program := activationLawProgram()
	branch := activationLawBranch()
	branch.Branch = 7
	program.Activation = &branch
	if problem, valid := program.Check(); valid || problem.Kind != ProblemActivation {
		t.Fatalf("a branch reference naming no declared join: valid=%v problem=%+v", valid, problem)
	}

	program = activationLawProgram()
	branch = activationLawBranch()
	branch.Branch = 0
	program.Activation = &branch
	if problem, valid := program.Check(); valid || problem.Kind != ProblemActivation {
		t.Fatalf("a branch reference naming the exact trigger read: valid=%v problem=%+v", valid, problem)
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
	absent.Transport, absent.ActivationRole, absent.Activation = nil, "", nil
	if len(declared.References()) != len(absent.References())+len(declared.Activation.projections())+len(declared.Transport) {
		t.Fatalf("branch identities contribute %d references, want one per projection",
			len(declared.References())-len(absent.References())-len(declared.Transport))
	}
	for _, reference := range declared.Activation.references() {
		if !reference.Declared() {
			t.Fatal("a branch identity published an undeclared axis reference")
		}
	}
}
