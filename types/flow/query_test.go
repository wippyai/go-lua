package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeAt_EmptyPath(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	result := s.TypeAt(c.Entry(), constraint.Path{})
	if result != nil {
		t.Errorf("TypeAt(empty path) = %v, want nil", result)
	}
}

func TestTypeAt_DeclaredType(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.String

	s := Solve(inputs, testResolver())

	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.TypeAt(c.Entry(), path)
	if result != typ.String {
		t.Errorf("TypeAt(x) = %v, want string", result)
	}
}

func TestTypeAt_AssignedType(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Any
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "x", Symbol: symX},
			Type:       typ.Integer,
		},
	}

	s := Solve(inputs, testResolver())

	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.TypeAt(c.Entry(), path)
	if result != typ.Integer {
		t.Errorf("TypeAt(x) = %v, want integer", result)
	}
}

func TestSolveConditionView_PropagatesBranchFactsWithoutTransfer(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", points)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range points {
		setVersion(g, p, symX, verX)
	}

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
	}
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: pathX,
			Type:       typ.Boolean,
		},
	}

	s := SolveConditionView(inputs, testResolver())
	if got := s.NarrowedTypeAt(thenNode, pathX); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("condition view NarrowedTypeAt(then, x) = %v, want string", got)
	}
	if got := s.ConditionTypeAt(thenNode, pathX); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("condition view ConditionTypeAt(then, x) = %v, want string", got)
	}
	if got := s.TypeAt(thenNode, pathX); typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("condition view ran transfer assignment, got %v", got)
	}
}

func TestConditionTypeAt_UsesConditionProofNotSolvedTransferState(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", points)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range points {
		setVersion(g, p, symX, verX)
	}

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
	}
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: pathX,
			Type:       typ.Boolean,
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.TypeAt(thenNode, pathX); !typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("test setup TypeAt(then, x) = %v, want solved boolean assignment", got)
	}
	if got := s.ConditionTypeAt(thenNode, pathX); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ConditionTypeAt(then, x) = %v, want branch-proven string", got)
	}
}

func TestConditionTypeAt_ProjectsDiscriminatedChildFromDeclaredProduct(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", points)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	for _, p := range points {
		setVersion(g, p, symResult, verResult)
	}

	okVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("ok")).
		Field("value", typ.String).
		Build()
	errVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("err")).
		Field("value", typ.Number).
		Build()
	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	valuePath := pathResult.Field("value")

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(okVariant, errVariant)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(constraint.FieldEquals{
				Target: pathResult,
				Field:  "kind",
				Value:  typ.LiteralString("ok"),
			}),
		},
	}
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      thenNode,
			TargetPath: valuePath,
			Type:       typ.Boolean,
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.TypeAt(thenNode, valuePath); !typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("test setup TypeAt(then, result.value) = %v, want solved boolean assignment", got)
	}
	if got := s.ConditionTypeAt(thenNode, valuePath); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ConditionTypeAt(then, result.value) = %v, want string from ok variant", got)
	}
}

func TestConditionTypeAt_DNFDescendantAbsentDisjunctContributesNil(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", points)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	for _, p := range points {
		setVersion(g, p, symResult, verResult)
	}

	aVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("a_field", typ.Number).
		Build()
	bVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("b")).
		Field("b_field", typ.String).
		Build()
	cVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("c")).
		Field("c_field", typ.Boolean).
		Build()
	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	aFieldPath := pathResult.Field("a_field")

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(aVariant, bVariant, cVariant)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.Or(
				constraint.FromConstraints(constraint.FieldEquals{
					Target: pathResult,
					Field:  "kind",
					Value:  typ.LiteralString("a"),
				}),
				constraint.FromConstraints(constraint.FieldEquals{
					Target: pathResult,
					Field:  "kind",
					Value:  typ.LiteralString("b"),
				}),
			),
		},
	}

	s := Solve(inputs, testResolver())
	want := typ.NewOptional(typ.Number)
	if got := s.ConditionTypeAt(thenNode, aFieldPath); !typ.TypeEquals(got, want) {
		t.Fatalf("ConditionTypeAt(then, result.a_field) = %v, want %v", got, want)
	}
}

func TestConditionTypeAt_NegativeDiscriminantUnmatchedLiteralDoesNotBottomChild(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symEvent := setupSymbol(g, "event", points)
	verEvent := cfg.Version{Root: "event", Symbol: symEvent, ID: 1}
	for _, p := range points {
		setVersion(g, p, symEvent, verEvent)
	}

	exitVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("exit")).
		Field("code", typ.Number).
		Build()
	dataVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("data")).
		Field("payload", typ.String).
		Build()
	pathEvent := constraint.Path{Root: "event", Symbol: symEvent}
	payloadPath := pathEvent.Field("payload")

	inputs := newInputs(g)
	inputs.DeclaredTypes[symEvent] = typ.NewUnion(exitVariant, dataVariant)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEquals{
				Target: pathEvent,
				Field:  "kind",
				Value:  typ.LiteralString("unknown"),
			}),
		},
	}

	s := Solve(inputs, testResolver())
	want := typ.NewOptional(typ.String)
	if got := s.NarrowedTypeAt(thenNode, payloadPath); !typ.TypeEquals(got, want) {
		t.Fatalf("NarrowedTypeAt(then, event.payload) = %v, want %v", got, want)
	}
	if got := s.ConditionTypeAt(thenNode, payloadPath); got != nil {
		t.Fatalf("ConditionTypeAt(then, event.payload) = %v, want no condition-only projection", got)
	}
}

func TestConditionTypeAt_NegativeDiscriminantMatchedLiteralProjectsChild(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symEvent := setupSymbol(g, "event", points)
	verEvent := cfg.Version{Root: "event", Symbol: symEvent, ID: 1}
	for _, p := range points {
		setVersion(g, p, symEvent, verEvent)
	}

	exitVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("exit")).
		Field("code", typ.Number).
		Build()
	dataVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("data")).
		Field("payload", typ.String).
		Build()
	pathEvent := constraint.Path{Root: "event", Symbol: symEvent}
	payloadPath := pathEvent.Field("payload")

	inputs := newInputs(g)
	inputs.DeclaredTypes[symEvent] = typ.NewUnion(exitVariant, dataVariant)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEquals{
				Target: pathEvent,
				Field:  "kind",
				Value:  typ.LiteralString("exit"),
			}),
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.ConditionTypeAt(thenNode, payloadPath); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ConditionTypeAt(then, event.payload) = %v, want string from data variant", got)
	}
}

func TestConditionedTypeAt_ProjectsExpressionLocalTypeGuard(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), c.Exit()}
	symRaw := setupSymbol(g, "raw", points)
	verRaw := cfg.Version{Root: "raw", Symbol: symRaw, ID: 1}
	for _, p := range points {
		setVersion(g, p, symRaw, verRaw)
	}

	rawPath := constraint.Path{Root: "raw", Symbol: symRaw}
	attemptsPath := rawPath.Field("attempts")
	inputs := newInputs(g)
	inputs.DeclaredTypes[symRaw] = typ.Any

	s := Solve(inputs, testResolver())
	cond := constraint.FromConstraints(constraint.HasType{
		Path: attemptsPath,
		Type: narrow.BuiltinTypeKey("number"),
	})
	if got := s.ConditionedTypeAt(c.Entry(), attemptsPath, cond); !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("ConditionedTypeAt(raw.attempts under type guard) = %v, want number", got)
	}
	if got := s.ConditionTypeAt(c.Entry(), attemptsPath); got != nil {
		t.Fatalf("ConditionTypeAt(raw.attempts without local guard) = %v, want nil", got)
	}
}

func TestNarrowedTypeAtWithCondition_ProjectsShortCircuitDescendant(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), c.Exit()}
	symEntry := setupSymbol(g, "entry", points)
	verEntry := cfg.Version{Root: "entry", Symbol: symEntry, ID: 1}
	for _, p := range points {
		setVersion(g, p, symEntry, verEntry)
	}

	metaRecord := typ.NewRecord().Field("suite", typ.String).Build()
	entryType := typ.NewRecord().
		Field("meta", typ.NewUnion(typ.False, metaRecord)).
		Build()
	entryPath := constraint.Path{Root: "entry", Symbol: symEntry}
	metaPath := entryPath.Field("meta")
	suitePath := metaPath.Field("suite")

	inputs := newInputs(g)
	inputs.DeclaredTypes[symEntry] = entryType
	s := Solve(inputs, testResolver())

	cond := constraint.FromConstraints(constraint.Truthy{Path: metaPath})
	if got := s.NarrowedTypeAtWithCondition(c.Entry(), suitePath, cond); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("NarrowedTypeAtWithCondition(entry.meta.suite under truthy entry.meta) = %v, want string", got)
	}
}

func TestConditionedTypeAt_ProjectsSiblingFieldFalsyThroughRoot(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), c.Exit()}
	symResult := setupSymbol(g, "result", points)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	for _, p := range points {
		setVersion(g, p, symResult, verResult)
	}

	errVariant := typ.NewRecord().Field("error", typ.String).Build()
	okVariant := typ.NewRecord().Field("name", typ.String).Build()
	resultType := typ.NewUnion(errVariant, okVariant)
	resultPath := constraint.Path{Root: "result", Symbol: symResult}
	namePath := resultPath.Field("name")
	errorPath := resultPath.Field("error")

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = resultType
	s := Solve(inputs, testResolver())

	cond := constraint.FromConstraints(constraint.Falsy{Path: errorPath})
	if got := s.ConditionedTypeAt(c.Entry(), namePath, cond); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ConditionedTypeAt(result.name under falsy result.error) = %v, want string", got)
	}
}

func TestConditionedSeedTypeAt_ProjectsTruthyAncestorFromOverlaySeed(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), c.Exit()}
	symEntry := setupSymbol(g, "entry", points)
	verEntry := cfg.Version{Root: "entry", Symbol: symEntry, ID: 1}
	for _, p := range points {
		setVersion(g, p, symEntry, verEntry)
	}

	metaRecord := typ.NewRecord().Field("suite", typ.String).Build()
	entryType := typ.NewRecord().
		Field("meta", typ.NewUnion(typ.False, metaRecord)).
		Build()
	entryPath := constraint.Path{Root: "entry", Symbol: symEntry}
	metaPath := entryPath.Field("meta")
	suitePath := metaPath.Field("suite")

	inputs := newInputs(g)
	s := Solve(inputs, testResolver())
	cond := constraint.FromConstraints(constraint.Truthy{Path: metaPath})
	if got := s.ConditionedSeedTypeAt(c.Entry(), entryPath, entryType, suitePath, cond); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ConditionedSeedTypeAt(entry.meta.suite under truthy entry.meta) = %v, want string", got)
	}
}

func TestConditionTypeAt_DerivesDescendantFromAncestorConditionProof(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symResult := setupSymbol(g, "result", points)
	verResult := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	for _, p := range points {
		setVersion(g, p, symResult, verResult)
	}

	item := typ.NewRecord().Field("text", typ.String).Build()
	resultType := typ.NewRecord().
		Field("items", typ.NewOptional(typ.NewArray(item))).
		Build()
	pathResult := constraint.Path{Root: "result", Symbol: symResult}
	itemsPath := pathResult.Field("items")
	textPath := constraint.Path{
		Root:   "result",
		Symbol: symResult,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "items"},
			{Kind: constraint.SegmentIndexInt, Index: 1},
			{Kind: constraint.SegmentField, Name: "text"},
		},
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = resultType
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: itemsPath}),
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.ConditionTypeAt(thenNode, textPath); !typ.TypeEquals(got, typ.NewOptional(typ.String)) {
		t.Fatalf("ConditionTypeAt(then, result.items[1].text) = %v, want string? (items proven present; the array index is bounds-optional without a length proof)", got)
	}
}

func TestNarrowedTypeAt_LengthLowerBoundFiltersEmptyShape(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symRows := setupSymbol(g, "rows", points)
	verRows := cfg.Version{Root: "rows", Symbol: symRows, ID: 1}
	for _, p := range points {
		setVersion(g, p, symRows, verRows)
	}

	row := typ.NewRecord().Field("text", typ.String).Build()
	rowsArray := typ.NewArray(row)
	rowsPath := constraint.Path{Root: "rows", Symbol: symRows}
	textPath := rowsPath.IndexInt(1).Field("text")

	inputs := newInputs(g)
	inputs.DeclaredTypes[symRows] = typ.NewUnion(typ.NewRecord().Build(), rowsArray)
	inputs.EdgeNumericConstraints = []EdgeNumericConstraint{{
		From: branch,
		To:   thenNode,
		Constraints: []constraint.NumericConstraint{
			constraint.LenGeConst{Array: rowsPath, C: 1},
		},
	}}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(thenNode, rowsPath); !typ.TypeEquals(got, rowsArray) {
		t.Fatalf("NarrowedTypeAt(then, rows) = %v, want %v", got, rowsArray)
	}
	if got := s.NarrowedTypeAt(thenNode, textPath); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("NarrowedTypeAt(then, rows[1].text) = %v, want string", got)
	}
}

func TestNarrowedTypeAt_LengthLowerBoundRejectsClosedEmptyShape(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symRows := setupSymbol(g, "rows", points)
	verRows := cfg.Version{Root: "rows", Symbol: symRows, ID: 1}
	for _, p := range points {
		setVersion(g, p, symRows, verRows)
	}

	rowsPath := constraint.Path{Root: "rows", Symbol: symRows}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symRows] = typ.NewRecord().Build()
	inputs.EdgeNumericConstraints = []EdgeNumericConstraint{{
		From: branch,
		To:   thenNode,
		Constraints: []constraint.NumericConstraint{
			constraint.LenGeConst{Array: rowsPath, C: 1},
		},
	}}

	s := Solve(inputs, testResolver())
	if !s.IsPointDead(thenNode) {
		t.Fatal("positive length branch over closed empty record should be unreachable")
	}
	if got := s.NarrowedTypeAt(thenNode, rowsPath); !typ.IsNever(got) {
		t.Fatalf("NarrowedTypeAt(then, rows) = %v, want never", got)
	}
}

func TestChildTypesAt_CachesSolvedProjectionAndReturnsCopy(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symResult := setupSymbol(g, "result", []cfg.Point{c.Entry()})
	setVersion(g, c.Entry(), symResult, cfg.Version{Root: "result", Symbol: symResult, ID: 1})

	root := constraint.Path{Root: "result", Symbol: symResult}
	errPath := root.Field("err")
	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewRecord().Build()
	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: errPath, Type: typ.String},
	}

	s := Solve(inputs, testResolver())
	first := s.ChildTypesAt(c.Entry(), root)
	if len(first) != 1 || !typ.TypeEquals(first[0].Type, typ.String) {
		t.Fatalf("first child projection = %#v, want err:string", first)
	}
	if len(s.childTypesCache) != 1 {
		t.Fatalf("expected one child projection cache entry, got %d", len(s.childTypesCache))
	}

	first[0].Type = typ.Number
	second := s.ChildTypesAt(c.Entry(), root)
	if len(second) != 1 || !typ.TypeEquals(second[0].Type, typ.String) {
		t.Fatalf("cached child projection leaked caller mutation: %#v", second)
	}

	key := s.pkResolver.KeyAt(c.Entry(), errPath)
	if key == "" {
		t.Fatal("expected canonical key for err path")
	}
	s.setValue(string(key), typ.Boolean)
	if len(s.childTypesCache) != 0 {
		t.Fatalf("state write left stale child projection cache entries: %d", len(s.childTypesCache))
	}
}

func TestTypeAt_DeclaredAnySurvivesUnknownObservation(t *testing.T) {
	c := cfg.New()
	assign := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), assign, true)
	c.AddEdge(assign, c.Exit(), true)
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), assign, c.Exit()})
	setVersion(g, c.Entry(), symX, cfg.Version{Root: "x", Symbol: symX, ID: 1})
	setVersion(g, assign, symX, cfg.Version{Root: "x", Symbol: symX, ID: 2})
	setVersion(g, c.Exit(), symX, cfg.Version{Root: "x", Symbol: symX, ID: 2})

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Any
	inputs.Assignments = []UnifiedAssignment{{
		Point:      assign,
		TargetPath: constraint.Path{Root: "x", Symbol: symX},
		Type:       typ.Unknown,
	}}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(c.Exit(), constraint.Path{Root: "x", Symbol: symX})
	if got != typ.Any {
		t.Fatalf("TypeAt declared any after unknown observation = %v, want any", got)
	}
}

func TestTypeAt_DeclaredAnyChildPathSurvivesUnknownObservation(t *testing.T) {
	c := cfg.New()
	assign := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), assign, true)
	c.AddEdge(assign, c.Exit(), true)
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), assign, c.Exit()})
	setVersion(g, c.Entry(), symX, cfg.Version{Root: "x", Symbol: symX, ID: 1})
	setVersion(g, assign, symX, cfg.Version{Root: "x", Symbol: symX, ID: 2})
	setVersion(g, c.Exit(), symX, cfg.Version{Root: "x", Symbol: symX, ID: 2})

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Any
	inputs.Assignments = []UnifiedAssignment{{
		Point:      assign,
		TargetPath: constraint.Path{Root: "x", Symbol: symX},
		Type:       typ.Unknown,
	}}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(c.Exit(), constraint.Path{
		Root:     "x",
		Symbol:   symX,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "field"}},
	})
	if got != typ.Any {
		t.Fatalf("TypeAt declared any child after unknown observation = %v, want any", got)
	}
}

func TestConditionAt_NoCondition(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	cond := s.ConditionAt(c.Entry())
	if !cond.IsTrue() {
		t.Errorf("ConditionAt(entry) = %v, want true condition", cond)
	}
}

func TestConditionAt_WithEdgeCondition(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.String)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: pathX}),
		},
	}

	s := Solve(inputs, testResolver())

	cond := s.ConditionAt(thenNode)
	if cond.IsTrue() || !cond.HasConstraints() {
		t.Errorf("ConditionAt(thenNode) = %v, want condition with Truthy", cond)
	}
}

func TestProvesTypeAt_HasTypeCondition(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Any
	inputs.EdgeConditions = []EdgeCondition{{
		From:      branch,
		To:        thenNode,
		Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
	}}

	s := Solve(inputs, testResolver())
	if !s.ProvesTypeAt(thenNode, pathX, typ.String) {
		t.Fatal("expected condition proof for x:string")
	}
	if s.ProvesTypeAt(c.Entry(), pathX, typ.String) {
		t.Fatal("entry has no path condition proof")
	}
}

func TestNarrowedTypeAt_FalseConditionIsNever(t *testing.T) {
	c := cfg.New()
	deadNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), deadNode, true)
	c.AddEdge(deadNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), deadNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, deadNode, symX, ver)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.String
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      c.Entry(),
			To:        deadNode,
			Condition: constraint.FalseCondition(),
		},
	}

	s := Solve(inputs, testResolver())
	if cond := s.ConditionAt(deadNode); !cond.IsFalse() {
		t.Fatalf("ConditionAt(deadNode) = %v, want false", cond)
	}
	if !s.IsPointDead(deadNode) {
		t.Fatalf("IsPointDead(deadNode) = false, want true")
	}
	if got := s.NarrowedTypeAt(deadNode, pathX); got != typ.Never {
		t.Fatalf("NarrowedTypeAt(deadNode, x) = %v, want never", got)
	}
}

func TestNarrowedTypeAt_TruthyFalseOptionalIsNever(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.LiteralBool(false))
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: pathX}),
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(thenNode, pathX); got != typ.Never {
		t.Fatalf("NarrowedTypeAt(thenNode, x) = %v, want never", got)
	}
}

func TestNarrowedTypeAt_TruthyOptionalRootElidesChildNilability(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeCall, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathInput := pathX.Field("input")
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.NewRecord().SetOpen(true).Build())
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: pathX}),
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(thenNode, pathInput); !typ.TypeEquals(got, typ.Unknown) {
		t.Fatalf("NarrowedTypeAt(thenNode, x.input) = %v, want unknown", got)
	}
}

func TestNarrowedTypeAt_TruthyOptionalRootKeepsOptionalChildField(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeCall, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathFlag := pathX.Field("flag")
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewOptional(typ.NewRecord().OptField("flag", typ.String).Build())
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: pathX}),
		},
	}

	s := Solve(inputs, testResolver())
	want := typ.NewOptional(typ.String)
	if got := s.NarrowedTypeAt(thenNode, pathFlag); !typ.TypeEquals(got, want) {
		t.Fatalf("NarrowedTypeAt(thenNode, x.flag) = %v, want %v", got, want)
	}
}

func TestNarrowedTypeAt_TruthyFalseOptionalFieldIsNever(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)

	fieldType := typ.NewOptional(typ.LiteralBool(false))
	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathField := pathX.Field("flag")
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewRecord().Field("flag", fieldType).Build()
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.Truthy{Path: pathField}),
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(thenNode, pathField); got != typ.Never {
		t.Fatalf("NarrowedTypeAt(thenNode, x.flag) = %v, want never", got)
	}
}

func TestNarrowedTypeAt_TruthyFalseOptionalFieldDNFIsNever(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)

	fieldType := typ.NewOptional(typ.LiteralBool(false))
	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathField := pathX.Field("flag")
	pathOther := pathX.Field("other")
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewRecord().Field("flag", fieldType).Field("other", typ.String).Build()
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromDisjuncts([][]constraint.Constraint{
				{
					constraint.HasField{Path: pathX, Field: "flag"},
					constraint.Truthy{Path: pathField},
					constraint.Falsy{Path: pathOther},
				},
				{
					constraint.HasField{Path: pathX, Field: "flag"},
					constraint.Truthy{Path: pathField},
					constraint.Falsy{Path: pathField},
				},
			}),
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(thenNode, pathField); got != typ.Never {
		t.Fatalf("NarrowedTypeAt(thenNode, x.flag) = %v, want never", got)
	}
}

func TestNarrowedTypeAt_ContradictoryTruthyFalsyUnknownIsNever(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Unknown
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.Truthy{Path: pathX},
				constraint.Falsy{Path: pathX},
			),
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(thenNode, pathX); got != typ.Never {
		t.Fatalf("NarrowedTypeAt(thenNode, x) = %v, want never", got)
	}
}

func TestNarrowedTypeAt_DNFPrunesContradictoryTruthyFalsyUnknown(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	symY := setupSymbol(g, "y", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	setVersion(g, c.Entry(), symX, ver)
	setVersion(g, branch, symX, ver)
	setVersion(g, thenNode, symX, ver)
	setVersion(g, c.Entry(), symY, verY)
	setVersion(g, branch, symY, verY)
	setVersion(g, thenNode, symY, verY)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}.Field("other")
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Unknown
	inputs.DeclaredTypes[symY] = typ.Unknown
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromDisjuncts([][]constraint.Constraint{
				{
					constraint.Truthy{Path: pathX},
					constraint.Falsy{Path: pathX},
				},
				{
					constraint.Truthy{Path: pathX},
					constraint.Falsy{Path: pathY},
				},
			}),
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(thenNode, pathX); got != typ.Unknown {
		t.Fatalf("NarrowedTypeAt(thenNode, x) = %v, want unknown", got)
	}
}

func TestNarrowedTypeAt_ContradictoryTruthyFalsyAliasIsNever(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(thenNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, thenNode})
	symY := setupSymbol(g, "y", []cfg.Point{c.Entry(), branch, thenNode})
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	setVersion(g, c.Entry(), symX, verX)
	setVersion(g, branch, symX, verX)
	setVersion(g, thenNode, symX, verX)
	setVersion(g, c.Entry(), symY, verY)
	setVersion(g, branch, symY, verY)
	setVersion(g, thenNode, symY, verY)

	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Unknown
	inputs.DeclaredTypes[symY] = typ.Unknown
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.NewEqPath(pathX, pathY),
				constraint.Truthy{Path: pathX},
				constraint.Falsy{Path: pathY},
			),
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(thenNode, pathX); got != typ.Never {
		t.Fatalf("NarrowedTypeAt(thenNode, x) = %v, want never", got)
	}
	if got := s.NarrowedTypeAt(thenNode, pathY); got != typ.Never {
		t.Fatalf("NarrowedTypeAt(thenNode, y) = %v, want never", got)
	}
}

func TestBaseTypeAt_NoSegments(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Number

	s := Solve(inputs, testResolver())

	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.baseTypeAt(c.Entry(), path)
	if result != typ.Number {
		t.Errorf("baseTypeAt(x) = %v, want number", result)
	}
}

func TestBaseTypeAt_WithSegments_ExplicitPreferred(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symR := setupSymbol(g, "r", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "r", Symbol: symR, ID: 1}
	setVersion(g, c.Entry(), symR, ver)

	errType := typ.NewInterface("Err", nil)
	recordType := typ.NewRecord().Field("err", typ.Nil).Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symR] = recordType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "r", Symbol: symR},
			Type:       recordType,
		},
		{
			Point: c.Entry(),
			TargetPath: constraint.Path{
				Root:     "r",
				Symbol:   symR,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
			},
			Type: errType,
		},
	}

	s := Solve(inputs, testResolver())

	path := constraint.Path{
		Root:     "r",
		Symbol:   symR,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
	}
	result := s.baseTypeAt(c.Entry(), path)
	if !typ.TypeEquals(result, errType) {
		t.Errorf("baseTypeAt(r.err) = %v, want Err (explicit assignment)", result)
	}
}

func TestNarrowedTypeAt_AncestorConditionOverridesStaleChildFact(t *testing.T) {
	c, branch, thenNode, _, _, _ := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	symR := setupSymbol(g, "r", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "r", Symbol: symR, ID: 1}
	setVersion(g, c.Entry(), symR, ver)
	setVersion(g, branch, symR, ver)
	setVersion(g, thenNode, symR, ver)

	typeA := typ.NewRecord().
		Field("tag", typ.LiteralString("a")).
		Field("value", typ.String).
		Build()
	typeB := typ.NewRecord().
		Field("tag", typ.LiteralString("b")).
		Field("value", typ.Number).
		Build()
	unionType := typ.NewUnion(typeA, typeB)
	rootPath := constraint.Path{Root: "r", Symbol: symR}
	valuePath := rootPath.Field("value")

	inputs := newInputs(g)
	inputs.DeclaredTypes[symR] = unionType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: rootPath,
			Type:       unionType,
		},
		{
			Point:      c.Entry(),
			TargetPath: valuePath,
			Type:       typ.String,
		},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(constraint.FieldNotEquals{
				Target: rootPath,
				Field:  "tag",
				Value:  typ.LiteralString("a"),
			}),
		},
	}

	s := Solve(inputs, testResolver())
	if !s.activeConditionNarrowsAncestor(thenNode, valuePath) {
		t.Fatalf("expected active parent discriminant condition for r.value")
	}
	got := s.NarrowedTypeAt(thenNode, valuePath)
	if got != typ.Number {
		t.Fatalf("NarrowedTypeAt(r.value) under parent discriminant = %v, want number", got)
	}
}

func TestNarrowedTypeAt_FieldFalsySiblingRemovesMissingFieldOptionality(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	falseNode := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, falseNode, false)
	c.AddEdge(falseNode, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), branch, falseNode, c.Exit()}
	symResult := setupSymbol(g, "result", points)
	ver := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	for _, p := range points {
		setVersion(g, p, symResult, ver)
	}

	errVariant := typ.NewRecord().Field("error", typ.LiteralString("bad")).Build()
	okName := typ.LiteralString("ok")
	okVariant := typ.NewRecord().Field("name", okName).Build()
	resultType := typ.NewUnion(errVariant, okVariant)
	resultPath := constraint.Path{Root: "result", Symbol: symResult}
	namePath := resultPath.Field("name")
	errorPath := resultPath.Field("error")

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewUnion(
		typ.NewRecord().Field("error", typ.String).Build(),
		typ.NewRecord().Field("name", typ.String).Build(),
	)
	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: resultPath, Type: resultType},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: falseNode, Condition: constraint.FromConstraints(constraint.Falsy{Path: errorPath})},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(falseNode, namePath); !typ.TypeEquals(got, okName) {
		t.Fatalf("NarrowedTypeAt(result.name under falsy result.error) = %v, want %v", got, okName)
	}
}

func TestNarrowedTypeAt_HasFieldDoesNotReintroduceStaleRootField(t *testing.T) {
	c, branch, thenNode, _, _, _ := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	symResult := setupSymbol(g, "result", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	setVersion(g, c.Entry(), symResult, ver)
	setVersion(g, branch, symResult, ver)
	setVersion(g, thenNode, symResult, ver)

	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	rootPath := constraint.Path{Root: "result", Symbol: symResult}
	errPath := rootPath.Field("err")

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: rootPath,
			Type:       typ.NewRecord().Field("err", typ.Nil).Build(),
		},
		{
			Point:      c.Entry(),
			TargetPath: errPath,
			Type:       typ.NewOptional(errType),
		},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.Truthy{Path: errPath},
				constraint.HasField{Path: rootPath, Field: "err"},
			),
		},
	}

	s := Solve(inputs, testResolver())
	got := s.NarrowedTypeAt(thenNode, errPath)
	if !typ.TypeEquals(got, errType) {
		t.Fatalf("NarrowedTypeAt(result.err) = %v, want non-optional Err", got)
	}
}

func TestDerivedTypeAt_FromNarrowedParent(t *testing.T) {
	c, branch, thenNode, _, _, _ := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	symR := setupSymbol(g, "r", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "r", Symbol: symR, ID: 1}
	setVersion(g, c.Entry(), symR, ver)
	setVersion(g, branch, symR, ver)
	setVersion(g, thenNode, symR, ver)

	msgA := typ.NewInterface("MsgA", nil)
	msgB := typ.NewInterface("MsgB", nil)
	resultA := typ.NewRecord().Field("value", msgA).Build()
	resultB := typ.NewRecord().Field("value", msgB).Build()
	unionType := typ.NewUnion(resultA, resultB)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symR] = unionType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "r", Symbol: symR},
			Type:       unionType,
		},
	}

	pathR := constraint.Path{Root: "r", Symbol: symR}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathR, Type: narrow.HashTypeKey(resultA.Hash())}),
		},
	}
	inputs.TypeKeys = map[uint64]typ.Type{
		resultA.Hash(): resultA,
	}

	s := Solve(inputs, testResolver())

	childPath := constraint.Path{
		Root:     "r",
		Symbol:   symR,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}},
	}
	result := s.derivedTypeAt(thenNode, childPath)
	if result == nil {
		t.Fatal("derivedTypeAt(r.value) = nil, want derived type from narrowed parent")
	}
	if !typ.TypeEquals(result, msgA) {
		t.Errorf("derivedTypeAt(r.value) = %v, want MsgA", result)
	}
}

func TestDerivedTypeAt_FromNarrowedIntermediateAncestor(t *testing.T) {
	c, branch, thenNode, _, _, _ := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	symR := setupSymbol(g, "r", []cfg.Point{c.Entry(), branch, thenNode})
	ver := cfg.Version{Root: "r", Symbol: symR, ID: 1}
	setVersion(g, c.Entry(), symR, ver)
	setVersion(g, branch, symR, ver)
	setVersion(g, thenNode, symR, ver)

	systemItem := typ.NewRecord().Field("text", typ.String).Build()
	systemType := typ.NewOptional(typ.NewArray(systemItem))
	rootType := typ.NewRecord().Field("system", systemType).Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symR] = rootType
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "r", Symbol: symR},
			Type:       rootType,
		},
	}

	pathSystem := constraint.Path{
		Root:     "r",
		Symbol:   symR,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "system"}},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.NotNil{Path: pathSystem}),
		},
	}

	s := Solve(inputs, testResolver())

	pathText := constraint.Path{
		Root:   "r",
		Symbol: symR,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "system"},
			{Kind: constraint.SegmentIndexInt, Index: 1},
			{Kind: constraint.SegmentField, Name: "text"},
		},
	}
	result := s.derivedTypeAt(thenNode, pathText)
	if result == nil {
		t.Fatal("derivedTypeAt(r.system[1].text) = nil, want derived type from narrowed intermediate ancestor")
	}
	if !typ.TypeEquals(result, typ.NewOptional(typ.String)) {
		t.Errorf("derivedTypeAt(r.system[1].text) = %v, want string? (array index is bounds-optional without a length proof)", result)
	}
}

func TestProductDomainProjectedTypeSkipsRootPath(t *testing.T) {
	dom := NewProductDomain(constraint.Env{})
	dom.Type.Narrowed["sym1@1.field"] = typ.String
	dom.Shape.Narrowed["sym1@1.other"] = typ.Number

	if got := dom.ProjectedTypeAt("sym1@1", testResolver()); got != nil {
		t.Fatalf("root path cannot have a narrowed ancestor projection, got %v", got)
	}
}

func TestProductDomainProjectedTypeDerivesFromIntermediateAncestor(t *testing.T) {
	dom := NewProductDomain(constraint.Env{})
	item := typ.NewRecord().Field("text", typ.String).Build()
	dom.Type.Narrowed["sym1@1.system"] = typ.NewArray(item)

	got := dom.ProjectedTypeAt("sym1@1.system[1].text", testResolver())
	if !typ.TypeEquals(got, typ.NewOptional(typ.String)) {
		t.Fatalf("ProjectedTypeAt(system[1].text) = %v, want string? (array index is bounds-optional without a length proof)", got)
	}
}

func TestProductDomainProjectedTypeFiltersUnionByChildDiscriminant(t *testing.T) {
	dom := NewProductDomain(constraint.Env{})
	a := typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("value", typ.String).Build()
	b := typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("count", typ.Integer).Build()
	dom.Type.Narrowed["sym1@1"] = typ.NewUnion(a, b)
	dom.Type.Narrowed["sym1@1.kind"] = typ.LiteralString("a")

	got := dom.ProjectedTypeAt("sym1@1", testResolver())
	if !typ.TypeEquals(got, a) {
		t.Fatalf("ProjectedTypeAt(sym1@1) = %v, want discriminant-filtered %v", got, a)
	}
}

func TestCarryForwardStructuredVersionFacts_PreservesIntegerIndexSibling(t *testing.T) {
	c := cfg.New()
	first := c.AddNode(cfg.NodeAssign, 0, "")
	second := c.AddNode(cfg.NodeAssign, 0, "")
	exit := c.Exit()
	c.AddEdge(c.Entry(), first, true)
	c.AddEdge(first, second, true)
	c.AddEdge(second, exit, true)
	g := newMockSSAGraph(c)

	symArr := setupSymbol(g, "arr", []cfg.Point{c.Entry(), first, second, exit})
	setVersion(g, c.Entry(), symArr, cfg.Version{Root: "arr", Symbol: symArr, ID: 1})
	setVersion(g, first, symArr, cfg.Version{Root: "arr", Symbol: symArr, ID: 2})
	setVersion(g, second, symArr, cfg.Version{Root: "arr", Symbol: symArr, ID: 3})
	setVersion(g, exit, symArr, cfg.Version{Root: "arr", Symbol: symArr, ID: 3})

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: first,
			TargetPath: constraint.Path{
				Root:   "arr",
				Symbol: symArr,
				Segments: []constraint.Segment{
					{Kind: constraint.SegmentIndexInt, Index: 1},
				},
			},
			Type: typ.String,
		},
		{
			Point: second,
			TargetPath: constraint.Path{
				Root:   "arr",
				Symbol: symArr,
				Segments: []constraint.Segment{
					{Kind: constraint.SegmentIndexInt, Index: 2},
				},
			},
			Type: typ.String,
		},
	}

	s := Solve(inputs, testResolver())
	got := s.NarrowedTypeAt(exit, constraint.Path{
		Root:   "arr",
		Symbol: symArr,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentIndexInt, Index: 1},
		},
	})
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("arr[1] after sibling index write = %v, want string", got)
	}
}

func TestMapMutatorAssignment_MaterializesEqualWideningAtCurrentVersion(t *testing.T) {
	c := cfg.New()
	init := c.AddNode(cfg.NodeAssign, 0, "")
	first := c.AddNode(cfg.NodeAssign, 0, "")
	second := c.AddNode(cfg.NodeAssign, 0, "")
	read := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), init, true)
	c.AddEdge(init, first, true)
	c.AddEdge(first, second, true)
	c.AddEdge(second, read, true)
	c.AddEdge(read, c.Exit(), true)
	g := newMockSSAGraph(c)

	symArr := setupSymbol(g, "arr", []cfg.Point{c.Entry(), init, first, second, read})
	setVersion(g, c.Entry(), symArr, cfg.Version{Root: "arr", Symbol: symArr, ID: 0})
	setVersion(g, init, symArr, cfg.Version{Root: "arr", Symbol: symArr, ID: 1})
	setVersion(g, first, symArr, cfg.Version{Root: "arr", Symbol: symArr, ID: 2})
	setVersion(g, second, symArr, cfg.Version{Root: "arr", Symbol: symArr, ID: 3})
	setVersion(g, read, symArr, cfg.Version{Root: "arr", Symbol: symArr, ID: 3})

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{{
		Point:      init,
		TargetPath: constraint.Path{Root: "arr", Symbol: symArr},
		Type:       typ.NewRecord().Build(),
	}}
	inputs.MapMutatorAssignments = []MapMutatorAssignment{
		{
			Point:     first,
			Target:    constraint.Path{Root: "arr", Symbol: symArr},
			KeyType:   typ.Integer,
			ValueType: typ.String,
		},
		{
			Point:     second,
			Target:    constraint.Path{Root: "arr", Symbol: symArr},
			KeyType:   typ.Integer,
			ValueType: typ.String,
		},
	}

	s := Solve(inputs, testResolver())
	got := s.NarrowedTypeAt(read, constraint.Path{
		Root:   "arr",
		Symbol: symArr,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentIndexInt, Index: 1},
		},
	})
	if !typ.TypeEquals(got, typ.NewOptional(typ.String)) {
		t.Fatalf("arr[1] after repeated equal index widening = %v, want string?", got)
	}
	tv := s.EffectiveTypeAt(read, symArr)
	if tv.State != StateResolved || !typ.TypeEquals(tv.Type, typ.NewMap(typ.Integer, typ.String)) {
		t.Fatalf("EffectiveTypeAt(arr) after mutable root materialization = %v/%v, want resolved integer map", tv.Type, tv.State)
	}
}

func TestPhiJoin_RekeysStructuredChildWrites(t *testing.T) {
	c := cfg.New()
	init := c.AddNode(cfg.NodeAssign, 0, "")
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	thenEnd := c.AddNode(cfg.NodeJoin, 0, "")
	elseNode := c.AddNode(cfg.NodeAssign, 0, "")
	elseEnd := c.AddNode(cfg.NodeJoin, 0, "")
	join := c.AddNode(cfg.NodeJoin, 0, "")
	read := c.AddNode(cfg.NodeCall, 0, "")
	c.AddEdge(c.Entry(), init, true)
	c.AddEdge(init, branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, elseNode, false)
	c.AddEdge(thenNode, thenEnd, true)
	c.AddEdge(elseNode, elseEnd, true)
	c.AddEdge(thenEnd, join, true)
	c.AddEdge(elseEnd, join, true)
	c.AddEdge(join, read, true)
	c.AddEdge(read, c.Exit(), true)
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), init, branch, thenNode, thenEnd, elseNode, elseEnd, join, read}
	symMessages := setupSymbol(g, "messages", allPoints)
	verInit := cfg.Version{Root: "messages", Symbol: symMessages, ID: 1}
	verElse := cfg.Version{Root: "messages", Symbol: symMessages, ID: 2}
	verThen := cfg.Version{Root: "messages", Symbol: symMessages, ID: 3}
	verJoin := cfg.Version{Root: "messages", Symbol: symMessages, ID: 4}
	setVersion(g, c.Entry(), symMessages, verInit)
	setVersion(g, init, symMessages, verInit)
	setVersion(g, branch, symMessages, verInit)
	setVersion(g, thenNode, symMessages, verThen)
	setVersion(g, thenEnd, symMessages, verThen)
	setVersion(g, elseNode, symMessages, verElse)
	setVersion(g, elseEnd, symMessages, verElse)
	setVersion(g, join, symMessages, verJoin)
	setVersion(g, read, symMessages, verJoin)
	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: verJoin,
		Operands: []cfg.PhiOperand{
			{From: thenEnd, Version: verThen},
			{From: elseEnd, Version: verElse},
		},
	})

	messageType := typ.NewRecord().Field("topic", typ.String).Build()
	rootPath := constraint.Path{Root: "messages", Symbol: symMessages}
	rootField := rootPath.Field("root")
	inputs := newInputs(g)
	inputs.DeclaredTypes[symMessages] = typ.NewMap(typ.String, messageType)
	inputs.Assignments = []UnifiedAssignment{
		{Point: init, TargetPath: rootPath, Type: typ.NewMap(typ.String, messageType)},
		{Point: thenNode, TargetPath: rootField, Type: messageType},
		{Point: elseNode, TargetPath: rootField, Type: messageType},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(read, rootField); !typ.TypeEquals(got, messageType) {
		t.Fatalf("NarrowedTypeAt(read, messages.root) = %v, want %v", got, messageType)
	}
}

func TestPhiJoin_SkipsSemanticallyDeadStructuredOperand(t *testing.T) {
	c := cfg.New()
	init := c.AddNode(cfg.NodeAssign, 0, "")
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	thenNode := c.AddNode(cfg.NodeAssign, 0, "")
	thenEnd := c.AddNode(cfg.NodeJoin, 0, "")
	elseNode := c.AddNode(cfg.NodeAssign, 0, "")
	elseEnd := c.AddNode(cfg.NodeJoin, 0, "")
	join := c.AddNode(cfg.NodeJoin, 0, "")
	read := c.AddNode(cfg.NodeCall, 0, "")
	c.AddEdge(c.Entry(), init, true)
	c.AddEdge(init, branch, true)
	c.AddEdge(branch, thenNode, true)
	c.AddEdge(branch, elseNode, false)
	c.AddEdge(thenNode, thenEnd, true)
	c.AddEdge(elseNode, elseEnd, true)
	c.AddEdge(thenEnd, join, true)
	c.AddEdge(elseEnd, join, true)
	c.AddEdge(join, read, true)
	c.AddEdge(read, c.Exit(), true)
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), init, branch, thenNode, thenEnd, elseNode, elseEnd, join, read}
	symMessages := setupSymbol(g, "messages", allPoints)
	symCond := setupSymbol(g, "cond", allPoints)
	verInit := cfg.Version{Root: "messages", Symbol: symMessages, ID: 1}
	verElse := cfg.Version{Root: "messages", Symbol: symMessages, ID: 2}
	verThen := cfg.Version{Root: "messages", Symbol: symMessages, ID: 3}
	verJoin := cfg.Version{Root: "messages", Symbol: symMessages, ID: 4}
	for _, p := range []cfg.Point{c.Entry(), init, branch} {
		setVersion(g, p, symMessages, verInit)
	}
	setVersion(g, thenNode, symMessages, verThen)
	setVersion(g, thenEnd, symMessages, verThen)
	setVersion(g, elseNode, symMessages, verElse)
	setVersion(g, elseEnd, symMessages, verElse)
	setVersion(g, join, symMessages, verJoin)
	setVersion(g, read, symMessages, verJoin)
	condVer := cfg.Version{Root: "cond", Symbol: symCond, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symCond, condVer)
	}
	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: verJoin,
		Operands: []cfg.PhiOperand{
			{From: elseEnd, Version: verElse},
			{From: thenEnd, Version: verThen},
		},
	})

	messageType := typ.NewRecord().Field("topic", typ.String).Build()
	rootPath := constraint.Path{Root: "messages", Symbol: symMessages}
	rootField := rootPath.Field("root")
	condPath := constraint.Path{Root: "cond", Symbol: symCond}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symMessages] = typ.NewMap(typ.String, messageType)
	inputs.DeclaredTypes[symCond] = typ.True
	inputs.Assignments = []UnifiedAssignment{
		{Point: init, TargetPath: rootPath, Type: typ.NewMap(typ.String, messageType)},
		{Point: thenNode, TargetPath: rootField, Type: messageType},
		{Point: elseNode, TargetPath: rootField, Type: messageType},
	}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.Truthy{Path: condPath})},
		{From: branch, To: elseNode, Condition: constraint.FromConstraints(constraint.Falsy{Path: condPath})},
	}

	s := Solve(inputs, testResolver())
	if !s.IsPointDead(elseEnd) {
		t.Fatalf("elseEnd is reachable, want semantic dead point")
	}
	if got := s.NarrowedTypeAt(read, rootField); !typ.TypeEquals(got, messageType) {
		t.Fatalf("NarrowedTypeAt(read, messages.root) = %v, want %v", got, messageType)
	}
}

func TestPointConditionProjectionCacheSurvivesStateWrites(t *testing.T) {
	s := &Solution{}
	p := cfg.Point(7)
	path := constraint.Path{Root: "options", Symbol: 101}.Field("filters").Field("created_after")
	unrelated := constraint.Path{Root: "query", Symbol: 202}
	cond := constraint.FromConstraints(
		constraint.Truthy{Path: path},
		constraint.NotNil{Path: unrelated},
	)

	projected := s.projectPointConditionForPath(p, path, cond)
	if got := len(projected.DisjunctConstraints(0)); got != 1 {
		t.Fatalf("projected constraints = %d, want 1", got)
	}
	if len(s.pointConditionCache) != 1 {
		t.Fatalf("point condition cache entries = %d, want 1", len(s.pointConditionCache))
	}

	s.setValue("sym202@1", typ.String)
	if len(s.pointConditionCache) != 1 {
		t.Fatalf("state write cleared point condition cache entries: got %d, want 1", len(s.pointConditionCache))
	}

	projectedAgain := s.projectPointConditionForPath(p, path, cond)
	if !projectedAgain.Equals(projected) {
		t.Fatalf("cached projected condition changed: got %v, want %v", projectedAgain, projected)
	}
}

func TestIsFalseLiteral(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil type", nil, false},
		{"false literal", typ.False, true},
		{"true literal", typ.True, false},
		{"string literal", typ.LiteralString("hi"), false},
		{"number type", typ.Number, false},
		{"nil singleton", typ.Nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFalseLiteral(tt.t)
			if got != tt.want {
				t.Errorf("isFalseLiteral(%v) = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}
