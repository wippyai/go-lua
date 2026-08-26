package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// seq5742Join is deliberately a declaration helper rather than a shape
// constructor. Every specimen below is still one ordinary JoinDecl; the
// normal form is derived by Check from Sources, projections, and Read.
func seq5742Join(key string, sources []SourceRef, form ReadForm, predicate, denominator bool) JoinDecl {
	join := JoinDecl{
		Sources:  append([]SourceRef(nil), sources...),
		Relation: lawRelation(key + "/relation"),
		Key:      lawProjection(key + "/key"),
		Read:     lawRead(form, key, denominator),
	}
	if predicate {
		join.Predicate = lawProjection(key + "/predicate")
	}
	// Production is explicit authority, never inferred by JoinDecl. These
	// hostile selected/tagged specimens are producers, so author Selection
	// directly; exact prior-source consumers intentionally leave it absent.
	if predicate {
		join.Selection = lawSelection(key + "/selection")
	}
	return join
}

func lawSelection(key string) member.SelectionRef {
	return member.SelectionRef{Axis: lawMemberAxis(), Member: schema.Key(key)}
}

func seq5742Output(key string, mode OutputMode, slot uint16) OutputDecl {
	output := lawOutput(slot, key)
	output.Mode = mode
	if mode == ModeRoute {
		output.RouteJoinPresent = true
	}
	return output
}

func seq5742Program(key string, joins []JoinDecl, inputs []JoinRef, outputs []OutputDecl) Program {
	return Program{
		OperandRole: schema.Key("semantic/operand/" + key),
		Candidate:   member.AxisRelationCandidate(lawRelation(key + "/candidate")),
		Joins:       joins,
		Fold: FoldDecl{
			Reducer: lawReducer(key + "/reducer"),
			Inputs:  inputs,
			Outputs: outputs,
		},
	}
}

// seq5742Specimens is the hostile declaration inventory. The names are
// descriptions only; no runtime or domain type is part of the Program ABI.
func seq5742Specimens() map[string]Program {
	return map[string]Program{
		// S1: an ordinary value transfer is a candidate-rooted exact read.
		"value-transfer": seq5742Program(
			"value-transfer",
			[]JoinDecl{seq5742Join("value-transfer/read", []SourceRef{CandidateSource()}, Exact, false, false)},
			[]JoinRef{0},
			[]OutputDecl{seq5742Output("value-transfer/write", ModeExact, 0)},
		),

		// S2: RawGet is the ordered selector DAG: receiver exact -> key
		// selected -> call selected -> heap selected -> pack selected -> source
		// selected. Each selected read names the prior results it consumes.
		"heap-raw-get": seq5742Program(
			"heap-raw-get",
			[]JoinDecl{
				seq5742Join("heap-raw-get/receiver", []SourceRef{CandidateSource()}, Exact, false, false),
				seq5742Join("heap-raw-get/key", []SourceRef{PriorSource(0)}, Selected, true, true),
				seq5742Join("heap-raw-get/call", []SourceRef{PriorSource(0), PriorSource(1)}, Selected, true, true),
				seq5742Join("heap-raw-get/heap", []SourceRef{PriorSource(0), PriorSource(1), PriorSource(2)}, Selected, true, true),
				seq5742Join("heap-raw-get/pack", []SourceRef{PriorSource(1), PriorSource(3)}, Selected, true, true),
				seq5742Join("heap-raw-get/source", []SourceRef{PriorSource(1), PriorSource(3), PriorSource(4)}, Selected, true, true),
			},
			[]JoinRef{0, 1, 2, 3, 4, 5},
			[]OutputDecl{seq5742Output("heap-raw-get/write", ModeExact, 0)},
		),

		// Call activation reads the exact Call candidate and nothing else.
		//
		// Its candidate branches are named by the declaration's own branch
		// vocabulary and enumerated through that relation's owner, never read.
		// A branch carries no fact any judgment consumes - the trigger's value
		// and the branch's identity settle it - and a branch has no coordinate
		// of its own to be read at, so a read here would deliver the trigger's
		// cell once per branch of a program-wide table.
		"call-activation": seq5742Program(
			"call-activation",
			[]JoinDecl{
				seq5742Join("call-activation/call", []SourceRef{CandidateSource()}, Exact, false, false),
			},
			[]JoinRef{0},
			[]OutputDecl{seq5742Output("call-activation/write", ModeStructural, 0)},
		),

		// RawSet is the same uniform chain with its Heap route write: receiver
		// exact -> key selected -> heap selected -> pack selected -> source
		// selected.
		"heap-raw-set": func() Program {
			program := seq5742Program(
				"heap-raw-set",
				[]JoinDecl{
					seq5742Join("heap-raw-set/receiver", []SourceRef{CandidateSource()}, Exact, false, false),
					seq5742Join("heap-raw-set/key", []SourceRef{PriorSource(0)}, Selected, true, true),
					seq5742Join("heap-raw-set/heap", []SourceRef{PriorSource(0), PriorSource(1)}, Selected, true, true),
					seq5742Join("heap-raw-set/pack", []SourceRef{PriorSource(0), PriorSource(1), PriorSource(2)}, Selected, true, true),
					seq5742Join("heap-raw-set/source", []SourceRef{PriorSource(0), PriorSource(1), PriorSource(2), PriorSource(3)}, Selected, true, true),
				},
				[]JoinRef{0, 1, 2, 3, 4},
				[]OutputDecl{seq5742Output("heap-raw-set/write", ModeRoute, 0)},
			)
			program.Fold.Outputs[0].RouteJoin = 4
			return program
		}(),

		// S5: closed allocation joins its predecessor Heap exact read to a
		// complete Value vector under an explicit denominator. Both reads are
		// direct Fold inputs and the output slots make vector arity explicit.
		"heap-closed-allocation": seq5742Program(
			"heap-closed-allocation",
			[]JoinDecl{
				seq5742Join("heap-closed-allocation/heap", []SourceRef{CandidateSource()}, Exact, false, false),
				seq5742Join("heap-closed-allocation/vector", []SourceRef{PriorSource(0)}, Complete, false, true),
			},
			[]JoinRef{0, 1},
			[]OutputDecl{
				seq5742Output("heap-closed-allocation/field-0", ModeStructural, 0),
				seq5742Output("heap-closed-allocation/field-1", ModeStructural, 1),
				seq5742Output("heap-closed-allocation/field-2", ModeStructural, 2),
			},
		),
	}
}

type seq5742Shape struct {
	sources      [][]SourceRef
	forms        []ReadForm
	predicates   []bool
	parents      []bool
	denominators []bool
	inputs       []JoinRef
	modes        []OutputMode
	slots        []uint16
}

func assertSeq5742Shape(t *testing.T, name string, program Program, want seq5742Shape) {
	t.Helper()
	if len(program.Joins) != len(want.sources) || len(want.sources) != len(want.forms) ||
		len(want.sources) != len(want.predicates) || len(want.sources) != len(want.parents) ||
		len(want.sources) != len(want.denominators) {
		t.Fatalf("%s join shape metadata has inconsistent arity", name)
	}
	for position, join := range program.Joins {
		if len(join.Sources) != len(want.sources[position]) {
			t.Fatalf("%s join %d sources=%d, want %d", name, position, len(join.Sources), len(want.sources[position]))
		}
		for sourceIndex, source := range join.Sources {
			if source != want.sources[position][sourceIndex] {
				t.Fatalf("%s join %d source %d=%#v, want %#v", name, position, sourceIndex, source, want.sources[position][sourceIndex])
			}
		}
		if join.Read.Form != want.forms[position] {
			t.Fatalf("%s join %d form=%v, want %v", name, position, join.Read.Form, want.forms[position])
		}
		if join.Predicate.Declared() != want.predicates[position] {
			t.Fatalf("%s join %d predicate declared=%v, want %v", name, position, join.Predicate.Declared(), want.predicates[position])
		}
		if join.Parent.Declared() != want.parents[position] {
			t.Fatalf("%s join %d parent declared=%v, want %v", name, position, join.Parent.Declared(), want.parents[position])
		}
		if join.Read.Contract.DenominatorRef.Declared() != want.denominators[position] {
			t.Fatalf("%s join %d denominator declared=%v, want %v", name, position, join.Read.Contract.DenominatorRef.Declared(), want.denominators[position])
		}
	}
	if len(program.Fold.Inputs) != len(want.inputs) {
		t.Fatalf("%s fold inputs=%d, want %d", name, len(program.Fold.Inputs), len(want.inputs))
	}
	for index, input := range program.Fold.Inputs {
		if input != want.inputs[index] {
			t.Fatalf("%s fold input %d=%d, want %d", name, index, input, want.inputs[index])
		}
	}
	if len(program.Fold.Outputs) != len(want.modes) || len(want.modes) != len(want.slots) {
		t.Fatalf("%s fold output shape has inconsistent arity", name)
	}
	for index, output := range program.Fold.Outputs {
		if output.Mode != want.modes[index] || output.ValueSlot != want.slots[index] {
			t.Fatalf("%s fold output %d=(mode %v, slot %d), want (mode %v, slot %d)", name, index, output.Mode, output.ValueSlot, want.modes[index], want.slots[index])
		}
	}
}

func TestProgramSeq5742HostileShapesUseAuditedIncidence(t *testing.T) {
	specimens := seq5742Specimens()
	want := map[string]seq5742Shape{
		"value-transfer": {
			sources:      [][]SourceRef{{CandidateSource()}},
			forms:        []ReadForm{Exact},
			predicates:   []bool{false},
			parents:      []bool{false},
			denominators: []bool{false},
			inputs:       []JoinRef{0},
			modes:        []OutputMode{ModeExact},
			slots:        []uint16{0},
		},
		"heap-raw-get": {
			sources: [][]SourceRef{
				{CandidateSource()},
				{PriorSource(0)},
				{PriorSource(0), PriorSource(1)},
				{PriorSource(0), PriorSource(1), PriorSource(2)},
				{PriorSource(1), PriorSource(3)},
				{PriorSource(1), PriorSource(3), PriorSource(4)},
			},
			forms:        []ReadForm{Exact, Selected, Selected, Selected, Selected, Selected},
			predicates:   []bool{false, true, true, true, true, true},
			parents:      []bool{false, false, false, false, false, false},
			denominators: []bool{false, true, true, true, true, true},
			inputs:       []JoinRef{0, 1, 2, 3, 4, 5},
			modes:        []OutputMode{ModeExact},
			slots:        []uint16{0},
		},
		"call-activation": {
			sources:      [][]SourceRef{{CandidateSource()}},
			forms:        []ReadForm{Exact},
			predicates:   []bool{false},
			parents:      []bool{false},
			denominators: []bool{false},
			inputs:       []JoinRef{0},
			modes:        []OutputMode{ModeStructural},
			slots:        []uint16{0},
		},
		"heap-raw-set": {
			sources: [][]SourceRef{
				{CandidateSource()},
				{PriorSource(0)},
				{PriorSource(0), PriorSource(1)},
				{PriorSource(0), PriorSource(1), PriorSource(2)},
				{PriorSource(0), PriorSource(1), PriorSource(2), PriorSource(3)},
			},
			forms:        []ReadForm{Exact, Selected, Selected, Selected, Selected},
			predicates:   []bool{false, true, true, true, true},
			parents:      []bool{false, false, false, false, false},
			denominators: []bool{false, true, true, true, true},
			inputs:       []JoinRef{0, 1, 2, 3, 4},
			modes:        []OutputMode{ModeRoute},
			slots:        []uint16{0},
		},
		"heap-closed-allocation": {
			sources:      [][]SourceRef{{CandidateSource()}, {PriorSource(0)}},
			forms:        []ReadForm{Exact, Complete},
			predicates:   []bool{false, false},
			parents:      []bool{false, false},
			denominators: []bool{false, true},
			inputs:       []JoinRef{0, 1},
			modes:        []OutputMode{ModeStructural, ModeStructural, ModeStructural},
			slots:        []uint16{0, 1, 2},
		},
	}
	if len(want) != len(specimens) {
		t.Fatalf("shape inventory=%d, specimen inventory=%d", len(want), len(specimens))
	}
	for name, shape := range want {
		t.Run(name, func(t *testing.T) {
			assertSeq5742Shape(t, name, specimens[name], shape)
		})
	}
}

// assertSeq5742OrderedSources states the rooting law every specimen obeys:
// every source is either the candidate or a result declared before it, and at
// least one join is rooted at the candidate.
//
// Whether a specimen ALSO chains through a prior result is a property of the
// geometry it models, not of being well-formed. call-activation reads only its
// trigger: its candidate branches are named by its own branch vocabulary and
// enumerated through that relation's owner, so they are not a read at all. The
// inventory-wide coverage of the dependent shape is asserted once, over the
// inventory, below.
func assertSeq5742OrderedSources(t *testing.T, program Program) bool {
	t.Helper()
	seenCandidate, seenPrior := false, false
	for position, join := range program.Joins {
		for _, source := range join.Sources {
			if source.Candidate {
				seenCandidate = true
				continue
			}
			seenPrior = true
			if !source.AvailableBefore(position) {
				t.Fatalf("join %d has unordered source %#v", position, source)
			}
		}
	}
	if !seenCandidate {
		t.Fatal("specimen has no candidate-rooted source")
	}
	return seenPrior
}

func TestProgramExpressesSeq5742HostileSpecimens(t *testing.T) {
	specimens := seq5742Specimens()
	if len(specimens) != 5 {
		t.Fatalf("specimen count=%d, want five", len(specimens))
	}
	dependent := 0
	for name, specimen := range specimens {
		t.Run(name, func(t *testing.T) {
			if problem, valid := specimen.Check(); !valid {
				t.Fatalf("specimen rejected: %+v", problem)
			}
			if digest := specimen.Digest(); !digest.Available() {
				t.Fatal("valid specimen has no canonical digest")
			}
			if assertSeq5742OrderedSources(t, specimen) {
				dependent++
			}
		})
	}
	// The inventory is hostile only if it still exercises the chained shape:
	// a selector DAG whose every read names the results before it.
	if dependent == 0 {
		t.Fatal("no specimen chains through a prior result")
	}
}

func TestProgramRejectsSeq5742NearestNegativeDeclarations(t *testing.T) {
	// A dependent source may name only an earlier result, never itself or a
	// result that has not yet been declared.
	future := seq5742Specimens()["heap-raw-set"]
	future.Joins[1].Sources = []SourceRef{PriorSource(1)}
	if problem, valid := future.Check(); valid || problem.Kind != ProblemJoin || problem.Join != 1 {
		t.Fatalf("future dependency valid=%v problem=%+v", valid, problem)
	}
	if future.Digest().Available() {
		t.Fatal("future dependency received a digest")
	}

	// Selected, summary, and complete reads are closed only when their
	// denominator is declared. An absent denominator is not an open-ended
	// fallback.
	missingDenominator := seq5742Specimens()["heap-closed-allocation"]
	missingDenominator.Joins[1].Read.Contract.DenominatorRef = DenominatorRef{}
	if problem, valid := missingDenominator.Check(); valid || problem.Kind != ProblemJoin || problem.Join != 1 {
		t.Fatalf("missing denominator valid=%v problem=%+v", valid, problem)
	}
	if missingDenominator.Digest().Available() {
		t.Fatal("missing denominator received a digest")
	}

	// An incomplete optional projection is still a declared reference; its
	// missing owner-issued member must be refused before external resolution.
	foreignIncomplete := seq5742Specimens()["value-transfer"]
	foreignIncomplete.Joins[0].Predicate = member.ProjectionRef{
		Axis: lawMemberAxis(),
	}
	if problem, valid := foreignIncomplete.Check(); valid || problem.Kind != ProblemJoin || problem.Join != 0 {
		t.Fatalf("foreign incomplete reference valid=%v problem=%+v", valid, problem)
	}
	if foreignIncomplete.Digest().Available() {
		t.Fatal("foreign incomplete reference received a digest")
	}
}
