package emitlaw

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

const (
	lawAxisKey     = schema.Key("law-axis")
	lawForeignAxis = schema.Key("law-foreign-axis")
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) program.DenominatorRef {
	return program.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// lawDeclaration is a well-formed two-join routed declaration. It stands in
// for a real family here so the emitter's own laws stay upstream of every
// domain: what this package proves about a declaration is true of the shape,
// not of whoever happens to declare it.
func lawDeclaration() program.Program {
	return program.Program{
		OperandRole: "semantic/operand/law/subject",
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis: axisReference(lawForeignAxis), Member: "law/candidates",
		}),
		Joins: []program.JoinDecl{
			{
				Sources:  []program.SourceRef{program.CandidateSource()},
				Relation: member.RelationRef{Axis: axisReference(lawForeignAxis), Member: "law/sources"},
				Key:      member.ProjectionRef{Axis: axisReference(lawForeignAxis), Member: "law/source-key"},
				Read: program.ReadDecl{
					Axis:       program.AxisRef(axisReference(lawForeignAxis)),
					Form:       program.Exact,
					PointBound: program.PointBound,
					Contract: program.ReadContract{
						Order: program.OrderCanonical, Sparse: program.SparseExplicit,
						OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
					},
				},
			},
			{
				Sources:   []program.SourceRef{program.CandidateSource(), program.PriorSource(0)},
				Relation:  member.RelationRef{Axis: axisReference(lawAxisKey), Member: "law/routes"},
				Key:       member.ProjectionRef{Axis: axisReference(lawAxisKey), Member: "law/route-key"},
				Predicate: member.ProjectionRef{Axis: axisReference(lawAxisKey), Member: "law/route-tag"},
				Selection: member.SelectionRef{Axis: axisReference(lawAxisKey), Member: "law/route-selection"},
				Read: program.ReadDecl{
					Axis:       program.AxisRef(axisReference(lawAxisKey)),
					Form:       program.Selected,
					PointBound: program.PointBound,
					Contract: program.ReadContract{
						Order: program.OrderCanonical, Sparse: program.SparseExplicit,
						OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
						DenominatorRef: denominatorReference("coordinates/law"),
					},
				},
			},
		},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: axisReference(lawAxisKey), Member: "law/reducer"},
			Inputs:  []program.JoinRef{0, 1},
			Outputs: []program.OutputDecl{{
				Column:           axis.OutputRef{Axis: axisReference(lawAxisKey), Key: "law/facts"},
				Destination:      member.ProjectionRef{Axis: axisReference(lawAxisKey), Member: "law/route-destination"},
				Mode:             program.ModeRoute,
				RouteJoin:        1,
				RouteJoinPresent: true,
			}},
		},
	}
}

// TestCanonicalFormSeparatesEveryDeclaredTerm is what makes the emitted
// geometry law worth stating. One rendering stands for the whole declaration,
// so a term the rendering does not separate is a term the geometry law does
// not hold: the declaration could move there and no emitted law would notice.
//
// The mutation catalog is the enumeration of the declaration's terms, so
// holding the rendering to it is holding the canonical form to totality over
// exactly the surface the structural suite claims to cover.
func TestCanonicalFormSeparatesEveryDeclaredTerm(t *testing.T) {
	declaration := lawDeclaration()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("the law fixture is malformed: %+v", problem)
	}
	baseline := Canonical(declaration)
	rows := mutations(declaration)
	if len(rows) == 0 {
		t.Fatal("the mutation catalog enumerates no term of a two-join routed declaration")
	}
	for _, row := range rows {
		mutated := declaration.Clone()
		row.apply(&mutated)
		if Canonical(mutated) == baseline {
			t.Errorf("the canonical form does not separate a declaration where %s", row.name)
		}
	}
}

// TestCanonicalFormIsStableAcrossRenderings states that the rendering is a
// function of the declaration alone. A form that varied between two renderings
// of one declaration would make the freshness law report drift that is not
// there and hide drift that is.
func TestCanonicalFormIsStableAcrossRenderings(t *testing.T) {
	declaration := lawDeclaration()
	if Canonical(declaration) != Canonical(declaration.Clone()) {
		t.Fatal("two renderings of one declaration disagree")
	}
}

// TestEveryMandatoryMutationIsObservedRefused holds the emitter's own gate. A
// term the catalog marks as one the declaration cannot do without must be a
// term Check refuses the removal of; if it is not, the suite would be emitting
// a documented allowance over a hole in the checker.
func TestEveryMandatoryMutationIsObservedRefused(t *testing.T) {
	verdicts, err := observe("law/subject", lawDeclaration())
	if err != nil {
		t.Fatalf("observing the law fixture: %v", err)
	}
	refused := 0
	for _, row := range verdicts {
		if row.mandatory && !row.refused {
			t.Errorf("Check admits a declaration where %s, which the catalog holds mandatory", row.name)
		}
		if row.refused {
			refused++
			if problemKindName(row.problem.Kind) == "" {
				t.Errorf("Check refused a declaration where %s without naming a problem", row.name)
			}
		}
	}
	if refused == 0 {
		t.Fatal("no mutation of a well-formed declaration was refused")
	}
}

// TestObserveRefusesADeclarationWhoseRemovedTermIsAdmitted states the gate's
// own behavior: an admitted mandatory removal stops generation with a named
// gap rather than being emitted as an allowance. The fixture reaches it by
// declaring a rule whose only join is unreachable from its fold, which the
// catalog's mandatory rows are stated over.
func TestObserveRefusesADeclarationWhoseRemovedTermIsAdmitted(t *testing.T) {
	rows := []mutation{{
		name:      "a term nothing holds",
		statement: "declaration.Fold.Inputs = declaration.Fold.Inputs",
		apply:     func(*program.Program) {},
		mandatory: true,
	}}
	declaration := lawDeclaration()
	for _, row := range rows {
		mutated := declaration.Clone()
		row.apply(&mutated)
		if _, valid := mutated.Check(); !valid {
			t.Fatal("the no-op mutation fixture is not a no-op")
		}
	}
	if _, err := observeRows("law/subject", declaration, rows); err == nil {
		t.Fatal("an admitted mandatory removal was accepted")
	}
}

// TestRenderRefusesADeclarationItCannotStateLawsOver keeps the emitter's
// refusals named. A rule with no declaration, a rule whose declaration is
// already malformed, and a rule on an axis the roster does not carry are three
// different absences, and each is reported as its own gap rather than as a
// suite that quietly states less.
func TestRenderRefusesADeclarationItCannotStateLawsOver(t *testing.T) {
	for _, item := range []struct {
		name        string
		declaration program.Program
		clause      string
	}{
		{name: "an empty declaration", declaration: program.Program{}, clause: "no Program at all"},
		{name: "a malformed declaration", declaration: withoutReducer(), clause: "a malformed Program"},
		{name: "an unrostered reducer axis", declaration: lawDeclaration(), clause: "an axis the roster does not carry"},
	} {
		_, err := derive(Target{
			PackagePath: "example.test/law",
			PackageName: "law",
			Declaration: "Subject",
			Entry:       "RuleEntry",
			Spec:        specFor(item.declaration),
		}, definition.Roster{})
		if err == nil {
			t.Fatalf("%s was accepted", item.name)
		}
		refusal, isRefusal := err.(Unexpressible)
		if !isRefusal || !strings.Contains(refusal.Clause, item.clause) {
			t.Fatalf("%s refused as %v, want a named %q gap", item.name, err, item.clause)
		}
	}
}

func withoutReducer() program.Program {
	declaration := lawDeclaration()
	declaration.Fold.Reducer = member.ReducerRef{}
	return declaration
}

func specFor(declaration program.Program) rule.Spec {
	return rule.Spec{
		Key: "law/subject", Lane: rule.LaneLink, Writes: lawAxisKey, Owner: lawAxisKey,
		Semantic: "semantic/rule/law/subject", Program: declaration,
	}
}
