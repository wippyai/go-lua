package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type testTypeDecomposer struct{}

func (testTypeDecomposer) ElementType(t typ.Type) typ.Type    { return core.ElementType(t) }
func (testTypeDecomposer) KeyType(t typ.Type) typ.Type        { return core.KeyType(t) }
func (testTypeDecomposer) ValueType(t typ.Type) typ.Type      { return core.ValueType(t) }
func (testTypeDecomposer) EntryValueType(t typ.Type) typ.Type { return core.EntryValueType(t) }

func TestProcessPointReturnChangedKeys_NoChanges(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	keys := s.processPointReturnChangedKeys(c.Entry())
	if len(keys) != 0 {
		t.Errorf("processPointReturnChangedKeys = %v, want empty", keys)
	}
}

func TestProcessAssignmentReturnChangedKeys_SingleAssignment(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, c.Entry(), symX, ver)

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      c.Entry(),
			TargetPath: constraint.Path{Root: "x", Symbol: symX},
			Type:       typ.String,
		},
	}

	s := Solve(inputs, testResolver())

	// Check that value was set
	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.TypeAt(c.Entry(), path)
	if result != typ.String {
		t.Errorf("TypeAt after assignment = %v, want string", result)
	}
}

func TestProcessAssignmentReturnChangedKeys_IteratorSourceUsesDeclaredContainer(t *testing.T) {
	c := cfg.New()
	assignPoint := c.AddNode(cfg.NodeAssign, 0, "generic for")
	c.AddEdge(c.Entry(), assignPoint, true)
	c.AddEdge(assignPoint, c.Exit(), true)
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), assignPoint, c.Exit()}
	symItems := setupSymbol(g, "items", points)
	symIndex := setupSymbol(g, "i", []cfg.Point{assignPoint, c.Exit()})
	symItem := setupSymbol(g, "item", []cfg.Point{assignPoint, c.Exit()})
	symCounts := setupSymbol(g, "counts", points)
	symKey := setupSymbol(g, "key", []cfg.Point{assignPoint, c.Exit()})
	symValue := setupSymbol(g, "value", []cfg.Point{assignPoint, c.Exit()})

	for _, p := range points {
		setVersion(g, p, symItems, cfg.Version{Root: "items", Symbol: symItems, ID: 1})
		setVersion(g, p, symCounts, cfg.Version{Root: "counts", Symbol: symCounts, ID: 1})
	}
	setVersion(g, assignPoint, symIndex, cfg.Version{Root: "i", Symbol: symIndex, ID: 1})
	setVersion(g, c.Exit(), symIndex, cfg.Version{Root: "i", Symbol: symIndex, ID: 1})
	setVersion(g, assignPoint, symItem, cfg.Version{Root: "item", Symbol: symItem, ID: 1})
	setVersion(g, c.Exit(), symItem, cfg.Version{Root: "item", Symbol: symItem, ID: 1})
	setVersion(g, assignPoint, symKey, cfg.Version{Root: "key", Symbol: symKey, ID: 1})
	setVersion(g, c.Exit(), symKey, cfg.Version{Root: "key", Symbol: symKey, ID: 1})
	setVersion(g, assignPoint, symValue, cfg.Version{Root: "value", Symbol: symValue, ID: 1})
	setVersion(g, c.Exit(), symValue, cfg.Version{Root: "value", Symbol: symValue, ID: 1})

	itemsPath := constraint.Path{Root: "items", Symbol: symItems}
	countsPath := constraint.Path{Root: "counts", Symbol: symCounts}
	inputs := newInputs(g)
	inputs.Decomposer = testTypeDecomposer{}
	inputs.DeclaredTypes[symItems] = typ.NewArray(typ.String)
	inputs.DeclaredTypes[symCounts] = typ.NewMap(typ.String, typ.Number)
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      assignPoint,
			TargetPath: constraint.Path{Root: "i", Symbol: symIndex},
			Type:       typ.Any,
			Source: AssignmentSource{
				Kind:         AssignmentSourceIterator,
				Path:         itemsPath,
				IteratorKind: IterateIndexed,
				VarIndex:     0,
			},
		},
		{
			Point:      assignPoint,
			TargetPath: constraint.Path{Root: "item", Symbol: symItem},
			Type:       typ.Any,
			Source: AssignmentSource{
				Kind:         AssignmentSourceIterator,
				Path:         itemsPath,
				IteratorKind: IterateIndexed,
				VarIndex:     1,
			},
		},
		{
			Point:      assignPoint,
			TargetPath: constraint.Path{Root: "key", Symbol: symKey},
			Type:       typ.Any,
			Source: AssignmentSource{
				Kind:         AssignmentSourceIterator,
				Path:         countsPath,
				IteratorKind: IterateKeyed,
				VarIndex:     0,
			},
		},
		{
			Point:      assignPoint,
			TargetPath: constraint.Path{Root: "value", Symbol: symValue},
			Type:       typ.Any,
			Source: AssignmentSource{
				Kind:         AssignmentSourceIterator,
				Path:         countsPath,
				IteratorKind: IterateKeyed,
				VarIndex:     1,
			},
		},
	}

	s := Solve(inputs, testResolver())
	assertType := func(path constraint.Path, want typ.Type) {
		t.Helper()
		got := s.TypeAt(assignPoint, path)
		if !typ.TypeEquals(got, want) {
			t.Fatalf("TypeAt(%s) = %v, want %v", path.Root, got, want)
		}
	}

	assertType(constraint.Path{Root: "i", Symbol: symIndex}, typ.Integer)
	assertType(constraint.Path{Root: "item", Symbol: symItem}, typ.String)
	assertType(constraint.Path{Root: "key", Symbol: symKey}, typ.String)
	assertType(constraint.Path{Root: "value", Symbol: symValue}, typ.Number)
}

func TestLengthIndexAssignmentSourceDerivesFromSolvedContainerWhenStaticIndexIsStaleNil(t *testing.T) {
	c := cfg.New()
	assignPoint := c.AddNode(cfg.NodeAssign, 0, "assign")
	c.AddEdge(c.Entry(), assignPoint, true)
	c.AddEdge(assignPoint, c.Exit(), true)
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), assignPoint, c.Exit()}
	symMessages := setupSymbol(g, "messages", points)
	symLast := setupSymbol(g, "last_msg", []cfg.Point{assignPoint, c.Exit()})

	messagesVersion := cfg.Version{Root: "messages", Symbol: symMessages, ID: 1}
	for _, p := range points {
		setVersion(g, p, symMessages, messagesVersion)
	}
	lastVersion := cfg.Version{Root: "last_msg", Symbol: symLast, ID: 1}
	setVersion(g, assignPoint, symLast, lastVersion)
	setVersion(g, c.Exit(), symLast, lastVersion)

	tool := typ.NewRecord().
		Field("role", typ.LiteralString("user")).
		Field("content", typ.NewArray(typ.NewRecord().
			Field("type", typ.LiteralString("tool_result")).
			Field("tool_use_id", typ.String).
			Build())).
		Build()
	text := typ.NewRecord().
		Field("role", typ.LiteralString("assistant")).
		Field("content", typ.NewArray(typ.NewRecord().
			Field("type", typ.LiteralString("text")).
			Field("text", typ.String).
			Build())).
		Build()
	message := typ.NewUnion(tool, text)
	messagesPath := constraint.Path{Root: "messages", Symbol: symMessages}
	lastPath := constraint.Path{Root: "last_msg", Symbol: symLast}

	inputs := newInputs(g)
	inputs.Decomposer = testTypeDecomposer{}
	inputs.DeclaredTypes[symMessages] = typ.NewArray(message)
	inputs.EdgeNumericConstraints = []EdgeNumericConstraint{{
		From: c.Entry(),
		To:   assignPoint,
		Constraints: []constraint.NumericConstraint{
			constraint.LenGeConst{Array: messagesPath, C: 1},
		},
	}}
	inputs.Assignments = []UnifiedAssignment{{
		Point:      assignPoint,
		TargetPath: lastPath,
		Type:       typ.Nil,
		Source: AssignmentSource{
			Kind:          AssignmentSourceLengthIndex,
			ContainerPath: messagesPath,
		},
	}}

	s := Solve(inputs, testResolver())
	got := s.NarrowedTypeAt(assignPoint, lastPath)
	if !typ.TypeEquals(got, message) {
		t.Fatalf("NarrowedTypeAt(last_msg) = %v, want current messages element %v", got, message)
	}
}

func TestPathAliasPropagatesSourceFieldWritesToAliasDescendants(t *testing.T) {
	c := cfg.New()
	aliasPoint := c.AddNode(cfg.NodeAssign, 0, "alias")
	methodPoint := c.AddNode(cfg.NodeAssign, 0, "method")
	c.AddEdge(c.Entry(), aliasPoint, true)
	c.AddEdge(aliasPoint, methodPoint, true)
	c.AddEdge(methodPoint, c.Exit(), true)
	g := newMockSSAGraph(c)

	points := []cfg.Point{c.Entry(), aliasPoint, methodPoint, c.Exit()}
	symProto := setupSymbol(g, "Class", points)
	symMeta := setupSymbol(g, "class_mt", points)
	protoVersion := cfg.Version{Root: "Class", Symbol: symProto, ID: 1}
	metaVersion := cfg.Version{Root: "class_mt", Symbol: symMeta, ID: 1}
	for _, p := range points {
		setVersion(g, p, symProto, protoVersion)
		setVersion(g, p, symMeta, metaVersion)
	}

	protoPath := constraint.Path{Root: "Class", Symbol: symProto}
	metaIndexPath := constraint.Path{Root: "class_mt", Symbol: symMeta}.Field("__index")
	methodPath := protoPath.Field("is_empty")
	methodType := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symProto] = typ.NewRecord().Build()
	inputs.DeclaredTypes[symMeta] = typ.NewRecord().Field("__index", typ.NewRecord().Build()).Build()
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      aliasPoint,
			TargetPath: metaIndexPath,
			Type:       typ.NewRecord().Build(),
			Source: AssignmentSource{
				Kind: AssignmentSourcePath,
				Path: protoPath,
			},
		},
		{
			Point:      methodPoint,
			TargetPath: methodPath,
			Type:       methodType,
		},
	}

	s := Solve(inputs, testResolver())
	got := s.NarrowedTypeAt(methodPoint, metaIndexPath.Field("is_empty"))
	if !typ.TypeEquals(got, methodType) {
		t.Fatalf("alias descendant method = %v, want %v", got, methodType)
	}
}

func TestTransferPlanIndexesOperationsByPoint(t *testing.T) {
	c := cfg.New()
	first := c.AddNode(cfg.NodeAssign, 0, "")
	second := c.AddNode(cfg.NodeAssign, 0, "")
	g := newMockSSAGraph(c)
	g.addPhiNode(cfg.PhiNode{Point: second, Target: cfg.Version{Symbol: 1, ID: 2}})

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{
		{Point: first, TargetPath: constraint.Path{Root: "a", Symbol: 1}, Type: typ.String},
		{Point: second, TargetPath: constraint.Path{Root: "b", Symbol: 2}, Type: typ.Integer},
	}
	inputs.MapMutatorAssignments = []MapMutatorAssignment{{Point: first, Target: constraint.Path{Root: "a", Symbol: 1}}}
	inputs.TableMutatorAssignments = []TableMutatorAssignment{{Point: second, Target: constraint.Path{Root: "b", Symbol: 2}}}
	inputs.ContainerMutatorAssignments = []ContainerMutatorAssignment{{Point: first, Target: constraint.Path{Root: "a", Symbol: 1}}}

	s := &Solution{inputs: inputs}
	s.buildTransferPlan(c.Size())

	if got := len(s.assignmentsAt(first)); got != 1 {
		t.Fatalf("assignmentsAt(first) = %d, want 1", got)
	}
	if got := len(s.assignmentsAt(second)); got != 1 {
		t.Fatalf("assignmentsAt(second) = %d, want 1", got)
	}
	if got := len(s.phisAt(second)); got != 1 {
		t.Fatalf("phisAt(second) = %d, want 1", got)
	}
	if got := len(s.mapMutatorAssignmentsAt(first)); got != 1 {
		t.Fatalf("mapMutatorAssignmentsAt(first) = %d, want 1", got)
	}
	if got := len(s.tableMutatorAssignmentsAt(second)); got != 1 {
		t.Fatalf("tableMutatorAssignmentsAt(second) = %d, want 1", got)
	}
	if got := len(s.containerMutatorAssignmentsAt(first)); got != 1 {
		t.Fatalf("containerMutatorAssignmentsAt(first) = %d, want 1", got)
	}
	if got := len(s.phisAt(first)); got != 0 {
		t.Fatalf("phisAt(first) = %d, want 0", got)
	}
}

func TestProcessAssignmentReturnChangedKeys_CallReturnUsesNarrowedReceiver(t *testing.T) {
	c := cfg.New()
	assignPoint := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), assignPoint, true)
	c.AddEdge(assignPoint, c.Exit(), true)
	g := newMockSSAGraph(c)

	symVersion := setupSymbol(g, "test_version", []cfg.Point{c.Entry(), assignPoint})
	setVersion(g, c.Entry(), symVersion, cfg.Version{Root: "test_version", Symbol: symVersion, ID: 1})
	setVersion(g, assignPoint, symVersion, cfg.Version{Root: "test_version", Symbol: symVersion, ID: 1})

	symID := setupSymbol(g, "test_id", []cfg.Point{assignPoint})
	setVersion(g, assignPoint, symID, cfg.Version{Root: "test_id", Symbol: symID, ID: 1})

	versionType := typ.NewInterface("Version", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.Number).Build()},
	})
	inputs := newInputs(g)
	inputs.DeclaredTypes[symVersion] = typ.NewOptional(versionType)
	inputs.EdgeConditions = []EdgeCondition{{
		From: c.Entry(),
		To:   assignPoint,
		Condition: constraint.FromConstraints(constraint.Truthy{
			Path: constraint.Path{Root: "test_version", Symbol: symVersion},
		}),
	}}
	inputs.Assignments = []UnifiedAssignment{{
		Point:      assignPoint,
		TargetPath: constraint.Path{Root: "test_id", Symbol: symID},
		Type:       typ.Unknown,
		Source: AssignmentSource{
			Kind:         AssignmentSourceCallReturn,
			ReceiverPath: constraint.Path{Root: "test_version", Symbol: symVersion},
			Method:       "id",
			ReturnIndex:  0,
		},
	}}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(assignPoint, constraint.Path{Root: "test_id", Symbol: symID})
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("call return from narrowed receiver = %v, want number", got)
	}
}

func TestProcessAssignmentReturnChangedKeys_MethodSelfReturnUsesReceiver(t *testing.T) {
	c := cfg.New()
	assignPoint := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), assignPoint, true)
	c.AddEdge(assignPoint, c.Exit(), true)
	g := newMockSSAGraph(c)

	symQuery := setupSymbol(g, "query", []cfg.Point{c.Entry(), assignPoint})
	setVersion(g, c.Entry(), symQuery, cfg.Version{Root: "query", Symbol: symQuery, ID: 1})
	setVersion(g, assignPoint, symQuery, cfg.Version{Root: "query", Symbol: symQuery, ID: 1})

	symNext := setupSymbol(g, "next_query", []cfg.Point{assignPoint})
	setVersion(g, assignPoint, symNext, cfg.Version{Root: "next_query", Symbol: symNext, ID: 1})

	queryType := typ.NewRecord().
		Field("where", typ.Func().Param("self", typ.Self).Param("clause", typ.String).Returns(typ.Self).Build()).
		Build()
	inputs := newInputs(g)
	inputs.DeclaredTypes[symQuery] = queryType
	inputs.Assignments = []UnifiedAssignment{{
		Point:      assignPoint,
		TargetPath: constraint.Path{Root: "next_query", Symbol: symNext},
		Type:       typ.Unknown,
		Source: AssignmentSource{
			Kind:         AssignmentSourceCallReturn,
			ReceiverPath: constraint.Path{Root: "query", Symbol: symQuery},
			Method:       "where",
			ReturnIndex:  0,
		},
	}}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(assignPoint, constraint.Path{Root: "next_query", Symbol: symNext})
	if got == typ.Self {
		t.Fatal("method self return leaked typ.Self into flow value")
	}
	if !typ.TypeEquals(got, queryType) {
		t.Fatalf("method self return = %v, want receiver %v", got, queryType)
	}
}

func TestAssignmentEvidenceTypeKeepsEffectProjectedStaticType(t *testing.T) {
	rawSelectResult := typ.NewRecord().
		Field("channel", typ.Any).
		Field("value", typ.Unknown).
		Field("ok", typ.Boolean).
		Build()
	preciseSelectResult := typ.NewUnion(
		typ.NewRecord().
			Field("__select_case_id", typ.LiteralInt(0)).
			Field("channel", typ.NewInterface("Channel<Event>", nil)).
			Field("value", typ.NewRecord().Field("kind", typ.String).Build()).
			Field("ok", typ.Boolean).
			Build(),
		typ.NewRecord().
			Field("__select_case_id", typ.LiteralInt(1)).
			Field("channel", typ.NewInterface("Channel<Time>", nil)).
			Field("value", typ.NewRecord().Field("sec", typ.Number).Build()).
			Field("ok", typ.Boolean).
			Build(),
	)

	got := assignmentEvidenceType(preciseSelectResult, rawSelectResult)
	if !typ.TypeEquals(got, preciseSelectResult) {
		t.Fatalf("assignment evidence = %v, want effect-projected %v", got, preciseSelectResult)
	}
}

func TestAssignmentEvidenceTypeKeepsEffectProjectedInstantiatedStaticType(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	channelGeneric := typ.NewGeneric("Channel", []*typ.TypeParam{tp}, typ.NewInterface("Channel", nil))
	eventType := typ.NewRecord().Field("kind", typ.String).Build()
	timeType := typ.NewRecord().Field("sec", typ.Number).Build()
	rawSelectResult := typ.NewRecord().
		Field("channel", typ.Any).
		Field("value", typ.Unknown).
		Field("ok", typ.Boolean).
		Build()
	preciseSelectResult := typ.NewUnion(
		typ.NewRecord().
			Field("__select_case_id", typ.LiteralInt(0)).
			Field("channel", typ.Instantiate(channelGeneric, eventType)).
			Field("value", eventType).
			Field("ok", typ.Boolean).
			Build(),
		typ.NewRecord().
			Field("__select_case_id", typ.LiteralInt(1)).
			Field("channel", typ.Instantiate(channelGeneric, timeType)).
			Field("value", timeType).
			Field("ok", typ.Boolean).
			Build(),
	)

	if !typ.MorePrecise(preciseSelectResult, rawSelectResult) {
		t.Fatal("test setup expected effect-projected select result to refine raw result")
	}
	got := assignmentEvidenceType(preciseSelectResult, rawSelectResult)
	if !typ.TypeEquals(got, preciseSelectResult) {
		t.Fatalf("assignment evidence = %v, want effect-projected instantiated result %v", got, preciseSelectResult)
	}
}

func TestAssignmentEvidenceTypeKeepsConcreteStaticOverUnsolvedGenericSource(t *testing.T) {
	sourceType := typ.NewTypeParam("T", nil)
	staticType := typ.NewRecord().
		Field("x", typ.Integer).
		Field("y", typ.Integer).
		Build()

	got := assignmentEvidenceType(staticType, sourceType)
	if !typ.TypeEquals(got, staticType) {
		t.Fatalf("assignment evidence = %v, want concrete static evidence %v", got, staticType)
	}
}

func TestProcessAssignmentReturnChangedKeys_CallReturnKeepsEffectProjectedStaticType(t *testing.T) {
	c := cfg.New()
	assignPoint := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), assignPoint, true)
	c.AddEdge(assignPoint, c.Exit(), true)
	g := newMockSSAGraph(c)

	symSelect := setupSymbol(g, "select", []cfg.Point{c.Entry(), assignPoint})
	symResult := setupSymbol(g, "result", []cfg.Point{assignPoint})
	setVersion(g, c.Entry(), symSelect, cfg.Version{Root: "select", Symbol: symSelect, ID: 1})
	setVersion(g, assignPoint, symSelect, cfg.Version{Root: "select", Symbol: symSelect, ID: 1})
	setVersion(g, assignPoint, symResult, cfg.Version{Root: "result", Symbol: symResult, ID: 1})

	rawSelectResult := typ.NewRecord().
		Field("channel", typ.Any).
		Field("value", typ.Unknown).
		Field("ok", typ.Boolean).
		Build()
	eventResult := typ.NewRecord().
		Field("__select_case_id", typ.LiteralInt(0)).
		Field("channel", typ.NewInterface("Channel<Event>", nil)).
		Field("value", typ.NewRecord().Field("kind", typ.String).Build()).
		Field("ok", typ.Boolean).
		Build()
	timeResult := typ.NewRecord().
		Field("__select_case_id", typ.LiteralInt(1)).
		Field("channel", typ.NewInterface("Channel<Time>", nil)).
		Field("value", typ.NewRecord().Field("sec", typ.Number).Build()).
		Field("ok", typ.Boolean).
		Build()
	preciseSelectResult := typ.NewUnion(eventResult, timeResult)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symSelect] = typ.Func().Returns(rawSelectResult).Build()
	inputs.Assignments = []UnifiedAssignment{{
		Point:      assignPoint,
		TargetPath: constraint.Path{Root: "result", Symbol: symResult},
		Type:       preciseSelectResult,
		Source: AssignmentSource{
			Kind:        AssignmentSourceCallReturn,
			CalleePath:  constraint.Path{Root: "select", Symbol: symSelect},
			ReturnIndex: 0,
		},
	}}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(assignPoint, constraint.Path{Root: "result", Symbol: symResult})
	if !typ.TypeEquals(got, preciseSelectResult) {
		t.Fatalf("call-return assignment = %v, want effect-projected %v", got, preciseSelectResult)
	}
}

func TestProcessAssignmentReturnChangedKeys_CallReturnUsesSourceProjection(t *testing.T) {
	c := cfg.New()
	assignPoint := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), assignPoint, true)
	c.AddEdge(assignPoint, c.Exit(), true)
	g := newMockSSAGraph(c)

	symRun := setupSymbol(g, "run", []cfg.Point{c.Entry(), assignPoint})
	symResult := setupSymbol(g, "result", []cfg.Point{assignPoint})
	setVersion(g, c.Entry(), symRun, cfg.Version{Root: "run", Symbol: symRun, ID: 1})
	setVersion(g, assignPoint, symRun, cfg.Version{Root: "run", Symbol: symRun, ID: 1})
	setVersion(g, assignPoint, symResult, cfg.Version{Root: "result", Symbol: symResult, ID: 1})

	projected := typ.NewRecord().Field("answer", typ.String).Build()
	inputs := newInputs(g)
	inputs.DeclaredTypes[symRun] = typ.Func().Returns(typ.Nil).Build()
	inputs.Assignments = []UnifiedAssignment{{
		Point:      assignPoint,
		TargetPath: constraint.Path{Root: "result", Symbol: symResult},
		Type:       typ.Unknown,
		Source: AssignmentSource{
			Kind:           AssignmentSourceCallReturn,
			CalleePath:     constraint.Path{Root: "run", Symbol: symRun},
			ReturnIndex:    0,
			ProjectionKind: AssignmentSourceProjectionCallReturn,
			ProjectedType:  projected,
		},
	}}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(assignPoint, constraint.Path{Root: "result", Symbol: symResult})
	if !typ.TypeEquals(got, projected) {
		t.Fatalf("call-return source projection = %v, want %v", got, projected)
	}
}

func TestProcessAssignmentReturnChangedKeys_PathUsesCallableSourceProjection(t *testing.T) {
	c := cfg.New()
	assignPoint := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), assignPoint, true)
	c.AddEdge(assignPoint, c.Exit(), true)
	g := newMockSSAGraph(c)

	symRun := setupSymbol(g, "run", []cfg.Point{c.Entry(), assignPoint})
	symAlias := setupSymbol(g, "alias", []cfg.Point{assignPoint})
	setVersion(g, c.Entry(), symRun, cfg.Version{Root: "run", Symbol: symRun, ID: 1})
	setVersion(g, assignPoint, symRun, cfg.Version{Root: "run", Symbol: symRun, ID: 1})
	setVersion(g, assignPoint, symAlias, cfg.Version{Root: "alias", Symbol: symAlias, ID: 1})

	projected := typ.Func().Returns(typ.NewRecord().Field("answer", typ.String).Build()).Build()
	inputs := newInputs(g)
	inputs.DeclaredTypes[symRun] = typ.Func().Returns(typ.Nil).Build()
	inputs.Assignments = []UnifiedAssignment{{
		Point:      assignPoint,
		TargetPath: constraint.Path{Root: "alias", Symbol: symAlias},
		Type:       typ.Unknown,
		Source: AssignmentSource{
			Kind:           AssignmentSourcePath,
			Path:           constraint.Path{Root: "run", Symbol: symRun},
			ProjectionKind: AssignmentSourceProjectionCallable,
			ProjectedType:  projected,
		},
	}}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(assignPoint, constraint.Path{Root: "alias", Symbol: symAlias})
	if !typ.TypeEquals(got, projected) {
		t.Fatalf("callable source projection = %v, want %v", got, projected)
	}
}

func TestProcessAssignmentReturnChangedKeys_CallReturnAppliesErrorReturnProjection(t *testing.T) {
	c := cfg.New()
	assignPoint := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), assignPoint, true)
	c.AddEdge(assignPoint, c.Exit(), true)
	g := newMockSSAGraph(c)

	symCall := setupSymbol(g, "load", []cfg.Point{c.Entry(), assignPoint})
	symValue := setupSymbol(g, "value", []cfg.Point{assignPoint})
	setVersion(g, c.Entry(), symCall, cfg.Version{Root: "load", Symbol: symCall, ID: 1})
	setVersion(g, assignPoint, symCall, cfg.Version{Root: "load", Symbol: symCall, ID: 1})
	setVersion(g, assignPoint, symValue, cfg.Version{Root: "value", Symbol: symValue, ID: 1})

	inputs := newInputs(g)
	inputs.DeclaredTypes[symCall] = typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
		Build()
	inputs.Assignments = []UnifiedAssignment{{
		Point:      assignPoint,
		TargetPath: constraint.Path{Root: "value", Symbol: symValue},
		Type:       typ.NewOptional(typ.String),
		Source: AssignmentSource{
			Kind:        AssignmentSourceCallReturn,
			CalleePath:  constraint.Path{Root: "load", Symbol: symCall},
			ReturnIndex: 0,
		},
	}}

	s := Solve(inputs, testResolver())
	got := s.TypeAt(assignPoint, constraint.Path{Root: "value", Symbol: symValue})
	want := typ.NewOptional(typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("call-return error projection = %v, want %v", got, want)
	}
}

func TestProcessAssignmentReturnChangedKeys_SkipsDeadPoint(t *testing.T) {
	c := cfg.New()
	dead := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), dead, true)
	c.AddEdge(dead, c.Exit(), true)
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), dead})
	ver := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	setVersion(g, dead, symX, ver)

	inputs := newInputs(g)
	inputs.EdgeConditions = []EdgeCondition{
		{From: c.Entry(), To: dead, Condition: constraint.FalseCondition()},
	}
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      dead,
			TargetPath: constraint.Path{Root: "x", Symbol: symX},
			Type:       typ.NewMap(typ.Any, typ.Any),
		},
	}

	s := Solve(inputs, testResolver())
	if _, ok := s.values[canonicalVersionKey(ver)]; ok {
		t.Fatalf("dead assignment wrote value for %v", ver)
	}
}

func TestProcessAssignmentReturnChangedKeys_SkipsSemanticallyDeadPoint(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	dead := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, dead, true)
	c.AddEdge(dead, c.Exit(), true)
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, dead})
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range []cfg.Point{c.Entry(), branch, dead} {
		setVersion(g, p, symX, verX)
	}

	symY := setupSymbol(g, "y", []cfg.Point{dead})
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	setVersion(g, dead, symY, verY)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Nil
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   dead,
			Condition: constraint.FromConstraints(constraint.Truthy{
				Path: constraint.Path{Root: "x", Symbol: symX},
			}),
		},
	}
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      dead,
			TargetPath: constraint.Path{Root: "y", Symbol: symY},
			Type:       typ.String,
		},
	}

	s := Solve(inputs, testResolver())
	if !s.IsPointDead(dead) {
		t.Fatal("truthy(nil) point should be semantically dead")
	}
	if _, ok := s.values[canonicalVersionKey(verY)]; ok {
		t.Fatalf("semantically dead assignment wrote value for %v", verY)
	}
}

func TestPointReachabilityCacheInvalidatesOnValueWrite(t *testing.T) {
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, 0, "")
	dead := c.AddNode(cfg.NodeAssign, 0, "")
	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, dead, true)
	c.AddEdge(dead, c.Exit(), true)
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, dead})
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range []cfg.Point{c.Entry(), branch, dead} {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Nil
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   dead,
			Condition: constraint.FromConstraints(constraint.Truthy{
				Path: constraint.Path{Root: "x", Symbol: symX},
			}),
		},
	}

	s := Solve(inputs, testResolver())
	if !s.IsPointDead(dead) {
		t.Fatal("truthy(nil) point should start semantically dead")
	}
	if _, ok := s.reachabilityCache[dead]; !ok {
		t.Fatal("semantic reachability query did not cache dead point")
	}

	s.setValue("sym99@1", typ.String)
	if _, ok := s.reachabilityCache[dead]; !ok {
		t.Fatal("unrelated value write invalidated reachability cache")
	}

	s.setValue(canonicalVersionKey(verX), typ.Boolean)
	if _, ok := s.reachabilityCache[dead]; ok {
		t.Fatal("related value write left stale reachability cache entry")
	}
	if s.IsPointDead(dead) {
		t.Fatal("reachability cache stayed stale after value write")
	}
}

func TestMergeIteratorAssignedType_PreservesPreciseExtractedAgainstDynamicDerived(t *testing.T) {
	if got := mergeIteratorAssignedType(typ.String, typ.Any); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("dynamic iterator derivation should not erase extracted string, got %v", got)
	}
	if got := mergeIteratorAssignedType(typ.Any, typ.String); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("concrete iterator derivation should refine extracted any, got %v", got)
	}
	if got := mergeIteratorAssignedType(typ.String, typ.Number); !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("incompatible concrete iterator derivation should remain authoritative, got %v", got)
	}
}

func TestProcessAssignmentReturnChangedKeys_FieldNilDeletesDeclaredField(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symResult := setupSymbol(g, "result", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "result", Symbol: symResult, ID: 1}
	setVersion(g, c.Entry(), symResult, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symResult] = typ.NewRecord().Field("err", typ.String).Build()
	inputs.Assignments = []UnifiedAssignment{
		{
			Point: c.Entry(),
			TargetPath: constraint.Path{
				Root:     "result",
				Symbol:   symResult,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
			},
			Type: typ.Nil,
		},
	}

	s := Solve(inputs, testResolver())
	path := constraint.Path{
		Root:     "result",
		Symbol:   symResult,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "err"}},
	}
	got := s.TypeAt(c.Entry(), path)
	if got == nil {
		t.Fatal("expected field type after nil assignment")
	}
	if !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("expected nil after declared field deletion, got %v", got)
	}
	if subtype.IsSubtype(typ.String, got) {
		t.Fatalf("deleted field should not retain declared string type, got %v", got)
	}
}

func TestResolveSymbolKeyType_ZeroSymbol(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	result := s.resolveSymbolKeyType(c.Entry(), 0, "x")
	if result != nil {
		t.Errorf("resolveSymbolKeyType(0) = %v, want nil", result)
	}
}

func TestResolveSymbolKeyType_ValidSymbol(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symK := setupSymbol(g, "k", []cfg.Point{c.Entry()})
	ver := cfg.Version{Root: "k", Symbol: symK, ID: 1}
	setVersion(g, c.Entry(), symK, ver)

	inputs := newInputs(g)
	inputs.DeclaredTypes[symK] = typ.String

	s := Solve(inputs, testResolver())

	result := s.resolveSymbolKeyType(c.Entry(), symK, "k")
	if result != typ.String {
		t.Errorf("resolveSymbolKeyType(k) = %v, want string", result)
	}
}

func TestAdmitArrayElementMutation_EmptyArray(t *testing.T) {
	result := value.AdmitArrayElementMutation(nil, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("value.AdmitArrayElementMutation(nil, string) = %T, want *typ.Array", result)
	}
	if arr.Element != typ.String {
		t.Errorf("value.AdmitArrayElementMutation(nil, string).Element = %v, want string", arr.Element)
	}
}

func TestAdmitArrayElementMutation_WidensScalarLiteralElement(t *testing.T) {
	result := value.AdmitArrayElementMutation(nil, typ.LiteralInt(1), typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("value.AdmitArrayElementMutation(nil, 1) = %T, want *typ.Array", result)
	}
	if arr.Element != typ.Integer {
		t.Fatalf("value.AdmitArrayElementMutation(nil, 1).Element = %v, want integer", arr.Element)
	}
}

func TestAdmitArrayElementMutation_PreservesRecordDiscriminants(t *testing.T) {
	event := typ.NewRecord().
		Field("kind", typ.LiteralString("tool_call")).
		Field("ids", typ.NewArray(typ.LiteralInt(1))).
		Build()
	result := value.AdmitArrayElementMutation(nil, event, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("value.AdmitArrayElementMutation(nil, record) = %T, want *typ.Array", result)
	}
	want := typ.NewRecord().
		Field("kind", typ.LiteralString("tool_call")).
		Field("ids", typ.NewArray(typ.Integer)).
		Build()
	if !typ.TypeEquals(arr.Element, want) {
		t.Fatalf("array element = %v, want %v", arr.Element, want)
	}
}

func TestAdmitArrayElementMutation_ExistingArray(t *testing.T) {
	existing := typ.NewArray(typ.Integer)
	result := value.AdmitArrayElementMutation(existing, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("value.AdmitArrayElementMutation(int[], string) = %T, want *typ.Array", result)
	}
	// Element should be union of integer and string
	union, ok := arr.Element.(*typ.Union)
	if !ok {
		t.Fatalf("AdmitArrayElementMutation element = %T, want union", arr.Element)
	}
	if len(union.Members) != 2 {
		t.Errorf("union members = %d, want 2", len(union.Members))
	}
}

func TestAdmitArrayElementMutation_StableArrayIsIdempotent(t *testing.T) {
	existing := typ.NewArray(typ.String)
	result := value.AdmitArrayElementMutation(existing, typ.String, typ.JoinPreferNonSoft)
	if result != existing {
		t.Fatalf("stable array widening rebuilt type: got %p, want %p", result, existing)
	}
}

func TestAdmitArrayElementMutation_StableUnionArrayIsIdempotent(t *testing.T) {
	existing := typ.NewUnion(typ.NewArray(typ.String), typ.Nil)
	result := value.AdmitArrayElementMutation(existing, typ.String, typ.JoinPreferNonSoft)
	if result != existing {
		t.Fatalf("stable union array widening rebuilt type: got %p, want %p", result, existing)
	}
}

func TestAdmitArrayElementMutation_EmptyRecord(t *testing.T) {
	emptyRecord := typ.NewRecord().Build()
	result := value.AdmitArrayElementMutation(emptyRecord, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("value.AdmitArrayElementMutation({}, string) = %T, want *typ.Array", result)
	}
	if arr.Element != typ.String {
		t.Errorf("value.AdmitArrayElementMutation({}, string).Element = %v, want string", arr.Element)
	}
}

func TestAdmitIndexedWrite_NilBase(t *testing.T) {
	result := value.AdmitIndexedWrite(nil, typ.String, typ.Integer)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("value.AdmitIndexedWrite(nil, string, int) = %T, want *typ.Map", result)
	}
	if m.Key != typ.String {
		t.Errorf("AdmitIndexedWrite.Key = %v, want string", m.Key)
	}
	if m.Value != typ.Integer {
		t.Errorf("AdmitIndexedWrite.Value = %v, want integer", m.Value)
	}
}

func TestAdmitIndexedWrite_EmptyRecord(t *testing.T) {
	emptyRecord := typ.NewRecord().Build()
	result := value.AdmitIndexedWrite(emptyRecord, typ.String, typ.Number)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("value.AdmitIndexedWrite({}, string, number) = %T, want *typ.Map", result)
	}
	if m.Key != typ.String {
		t.Errorf("AdmitIndexedWrite.Key = %v, want string", m.Key)
	}
	if m.Value != typ.Number {
		t.Errorf("AdmitIndexedWrite.Value = %v, want number", m.Value)
	}
}

func TestAdmitIndexedWrite_ExistingMap(t *testing.T) {
	existingMap := typ.NewMap(typ.String, typ.Integer)
	result := value.AdmitIndexedWrite(existingMap, typ.Number, typ.Boolean)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("value.AdmitIndexedWrite(map, number, bool) = %T, want *typ.Map", result)
	}
	// Key should be union of string and number
	keyUnion, ok := m.Key.(*typ.Union)
	if !ok {
		t.Fatalf("AdmitIndexedWrite.Key = %T, want union", m.Key)
	}
	if len(keyUnion.Members) != 2 {
		t.Errorf("key union members = %d, want 2", len(keyUnion.Members))
	}
}

func TestMutableConvergenceMerge_FoldsRecursiveMapGrowth(t *testing.T) {
	base := typ.NewMap(typ.String, typ.Boolean)
	growth := typ.NewMap(typ.String, typ.NewUnion(typ.Boolean, base))

	merged := value.MergeForConvergence(base, growth)
	if _, ok := unwrap.Alias(merged).(*typ.Recursive); !ok {
		t.Fatalf("mutable convergence merge recursive map growth = %T %[1]v, want recursive upper bound", merged)
	}

	next := typ.NewMap(typ.String, typ.NewUnion(typ.Boolean, merged))
	again := value.MergeForConvergence(merged, next)
	if !typ.TypeEquals(again, merged) {
		t.Fatalf("recursive mutable upper bound did not stabilize: first=%v next=%v", merged, again)
	}
}

func TestMutableStateChangedKeys_UsesValueDomainRecursiveEquality(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	refined := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	s := &Solution{
		mutableValues: liftPointFlowValues(map[cfg.Point]map[string]typ.Type{
			7: {"sym1@1.children": right},
		}),
	}
	if changed := s.mutableStateChangedKeys(liftFlowValues(map[string]typ.Type{"sym1@1.children": left}), 7); len(changed) != 0 {
		t.Fatalf("equivalent recursive flow value reported changed keys: %v", changed)
	}

	s.mutableValues[7]["sym1@1.children"] = liftFlowValue(refined)
	changed := s.mutableStateChangedKeys(liftFlowValues(map[string]typ.Type{"sym1@1.children": left}), 7)
	if len(changed) != 1 || changed[0] != "sym1@1.children" {
		t.Fatalf("strict recursive refinement changed keys = %v, want sym1@1.children", changed)
	}
}

func TestWidenContainerElementType_UnknownGenericElementIsEvidenceHole(t *testing.T) {
	elem := typ.NewTypeParam("T", nil)
	body := typ.NewInterface("Channel", []typ.Method{
		{Name: "receive", Type: typ.Func().Param("self", typ.Self).Returns(elem).Build()},
	})
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{elem}, body)
	base := typ.Instantiate(channel, typ.Unknown)
	value := typ.NewRecord().Field("name", typ.String).Build()

	got := widenContainerElementType(base, value)
	want := typ.Instantiate(channel, value)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("widenContainerElementType(Channel<unknown>, record) = %v, want %v", got, want)
	}
}

func TestAdmitMapArrayElementMutation_NilBase(t *testing.T) {
	result := value.AdmitMapArrayElementMutation(nil, typ.String, typ.Integer)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("value.AdmitMapArrayElementMutation(nil) = %T, want *typ.Map", result)
	}
	arr, ok := m.Value.(*typ.Array)
	if !ok {
		t.Fatalf("AdmitMapArrayElementMutation.Value = %T, want *typ.Array", m.Value)
	}
	if arr.Element != typ.Integer {
		t.Errorf("AdmitMapArrayElementMutation.Value.Element = %v, want integer", arr.Element)
	}
}

func TestAdmitMapArrayElementMutation_WidensStoredScalarLiteralElement(t *testing.T) {
	result := value.AdmitMapArrayElementMutation(nil, typ.LiteralString("suite"), typ.LiteralInt(1))
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("value.AdmitMapArrayElementMutation(nil) = %T, want *typ.Map", result)
	}
	if m.Key != typ.String {
		t.Fatalf("AdmitMapArrayElementMutation key = %v, want string", m.Key)
	}
	arr, ok := m.Value.(*typ.Array)
	if !ok {
		t.Fatalf("AdmitMapArrayElementMutation value = %T, want *typ.Array", m.Value)
	}
	if arr.Element != typ.Integer {
		t.Fatalf("AdmitMapArrayElementMutation element = %v, want integer", arr.Element)
	}
}

func TestAdmitMapArrayElementMutation_PrefersNonSoftElement(t *testing.T) {
	base := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	elem := typ.NewRecord().Field("id", typ.String).Build()
	result := value.AdmitMapArrayElementMutation(base, typ.String, elem)
	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("value.AdmitMapArrayElementMutation(map) = %T, want *typ.Map", result)
	}
	arr, ok := m.Value.(*typ.Array)
	if !ok {
		t.Fatalf("AdmitMapArrayElementMutation.Value = %T, want *typ.Array", m.Value)
	}
	if !typ.TypeEquals(arr.Element, elem) {
		t.Fatalf("AdmitMapArrayElementMutation.Value.Element = %v, want %v", arr.Element, elem)
	}
}

func TestAdmitMapArrayElementMutation_StableMapIsIdempotent(t *testing.T) {
	base := typ.NewMap(typ.String, typ.NewArray(typ.Integer))
	got := value.AdmitMapArrayElementMutation(base, typ.String, typ.Integer)
	if got != base {
		t.Fatalf("stable map value array widening rebuilt type: got %p, want %p", got, base)
	}
}

func TestAdmitArrayElementMutation_PreservesAlias(t *testing.T) {
	base := typ.NewAlias("Items", typ.NewArray(typ.String))
	got := value.AdmitArrayElementMutation(base, typ.Integer, typ.JoinPreferNonSoft)
	alias, ok := got.(*typ.Alias)
	if !ok {
		t.Fatalf("value.AdmitArrayElementMutation(alias) = %T, want *typ.Alias", got)
	}
	if alias.Name != "Items" {
		t.Fatalf("alias name = %q, want Items", alias.Name)
	}
	arr, ok := alias.Target.(*typ.Array)
	if !ok {
		t.Fatalf("alias target = %T, want *typ.Array", alias.Target)
	}
	if !typ.TypeEquals(arr.Element, typ.NewUnion(typ.String, typ.Integer)) {
		t.Fatalf("alias array element = %v, want string|integer", arr.Element)
	}
}

func TestTableMutatorAssignment_WidensNestedFieldPathForLaterPoint(t *testing.T) {
	c := cfg.New()
	mut := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "table.insert")
	use := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "use")
	c.AddEdge(c.Entry(), mut, true)
	c.AddEdge(mut, use, true)
	c.AddEdge(use, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), mut, use, c.Exit()}
	symCtx := setupSymbol(g, "ctx", points)
	verCtx := cfg.Version{Root: "ctx", Symbol: symCtx, ID: 1}
	for _, p := range points {
		setVersion(g, p, symCtx, verCtx)
	}

	itemType := typ.NewRecord().Field("name", typ.String).Build()
	ctxType := typ.NewRecord().
		Field("items", typ.NewRecord().SetOpen(true).Build()).
		Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symCtx] = ctxType
	inputs.TableMutatorAssignments = []TableMutatorAssignment{
		{
			Point: mut,
			Target: constraint.Path{
				Root:   "ctx",
				Symbol: symCtx,
				Segments: []constraint.Segment{
					{Kind: constraint.SegmentField, Name: "items"},
				},
			},
			ValueType: itemType,
		},
	}

	s := Solve(inputs, testResolver())
	itemsPath := constraint.Path{
		Root:   "ctx",
		Symbol: symCtx,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "items"},
		},
	}
	got := s.NarrowedTypeAt(use, itemsPath)
	want := typ.NewArray(itemType)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("NarrowedTypeAt(use, ctx.items) = %v, want %v", got, want)
	}
}

func TestTableMutatorAssignment_RefinesSoftAnnotatedMapValue(t *testing.T) {
	c := cfg.New()
	init := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "init")
	mut := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "table.insert")
	use := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "use")
	c.AddEdge(c.Entry(), init, true)
	c.AddEdge(init, mut, true)
	c.AddEdge(mut, use, true)
	c.AddEdge(use, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), init, mut, use, c.Exit()}
	symSuites := setupSymbol(g, "suites", points)
	verSuites := cfg.Version{Root: "suites", Symbol: symSuites, ID: 1}
	for _, p := range points {
		setVersion(g, p, symSuites, verSuites)
	}

	entryType := typ.NewRecord().Field("id", typ.String).Build()
	suitesPath := constraint.Path{Root: "suites", Symbol: symSuites}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symSuites] = typ.NewMap(typ.String, typ.NewArray(typ.Any))
	inputs.AnnotatedVars = map[cfg.SymbolID]bool{}
	inputs.AnnotatedVars[symSuites] = true
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      init,
			TargetPath: suitesPath,
			Type:       typ.NewRecord().Build(),
		},
	}
	inputs.TableMutatorAssignments = []TableMutatorAssignment{
		{
			Point:     mut,
			Target:    suitesPath,
			KeyType:   typ.String,
			ValueType: entryType,
		},
	}

	s := Solve(inputs, testResolver())
	want := typ.NewMap(typ.String, typ.NewArray(entryType))
	if got := s.NarrowedTypeAt(use, suitesPath); !typ.TypeEquals(got, want) {
		t.Fatalf("NarrowedTypeAt(use, suites) = %v, want %v", got, want)
	}
}

func TestIndexerAndTableMutator_RefineLazySoftArraySlot(t *testing.T) {
	c := cfg.New()
	init := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "init")
	slot := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "slot")
	mut := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "table.insert")
	use := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "use")
	c.AddEdge(c.Entry(), init, true)
	c.AddEdge(init, slot, true)
	c.AddEdge(slot, mut, true)
	c.AddEdge(mut, use, true)
	c.AddEdge(use, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), init, slot, mut, use, c.Exit()}
	symSuites := setupSymbol(g, "suites", points)
	verSuites := cfg.Version{Root: "suites", Symbol: symSuites, ID: 1}
	for _, p := range points {
		setVersion(g, p, symSuites, verSuites)
	}

	entryType := typ.NewRecord().Field("id", typ.String).Build()
	suitesPath := constraint.Path{Root: "suites", Symbol: symSuites}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symSuites] = typ.NewMap(typ.String, typ.NewArray(typ.Any))
	inputs.AnnotatedVars = map[cfg.SymbolID]bool{symSuites: true}
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      init,
			TargetPath: suitesPath,
			Type:       typ.NewRecord().Build(),
		},
	}
	inputs.MapMutatorAssignments = []MapMutatorAssignment{
		{
			Point:     slot,
			Target:    suitesPath,
			KeyType:   typ.String,
			ValueType: typ.NewUnion(typ.NewArray(typ.Any), typ.NewRecord().Build()),
		},
	}
	inputs.TableMutatorAssignments = []TableMutatorAssignment{
		{
			Point:     mut,
			Target:    suitesPath,
			KeyType:   typ.String,
			ValueType: entryType,
		},
	}

	s := Solve(inputs, testResolver())
	want := typ.NewMap(typ.String, typ.NewArray(entryType))
	if got := s.NarrowedTypeAt(use, suitesPath); !typ.TypeEquals(got, want) {
		t.Fatalf("NarrowedTypeAt(use, suites) = %v, want %v", got, want)
	}
}

func TestMapMutatorAssignment_PreservesSealedAnnotatedMapValue(t *testing.T) {
	c := cfg.New()
	assign := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "set")
	use := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "use")
	c.AddEdge(c.Entry(), assign, true)
	c.AddEdge(assign, use, true)
	c.AddEdge(use, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), assign, use, c.Exit()}
	symScores := setupSymbol(g, "scores", points)
	verScores := cfg.Version{Root: "scores", Symbol: symScores, ID: 1}
	for _, p := range points {
		setVersion(g, p, symScores, verScores)
	}

	scoresPath := constraint.Path{Root: "scores", Symbol: symScores}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symScores] = typ.NewMap(typ.String, typ.Integer)
	inputs.AnnotatedVars = map[cfg.SymbolID]bool{}
	inputs.AnnotatedVars[symScores] = true
	inputs.MapMutatorAssignments = []MapMutatorAssignment{
		{
			Point:     assign,
			Target:    scoresPath,
			KeyType:   typ.String,
			ValueType: typ.String,
		},
	}

	s := Solve(inputs, testResolver())
	want := typ.NewMap(typ.String, typ.Integer)
	if got := s.NarrowedTypeAt(use, scoresPath); !typ.TypeEquals(got, want) {
		t.Fatalf("NarrowedTypeAt(use, scores) = %v, want sealed annotation %v", got, want)
	}
}

func TestIndexWriteAdmission_UsesMapMutatorProduct(t *testing.T) {
	c := cfg.New()
	assign := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "out[key] = value")
	c.AddEdge(c.Entry(), assign, true)
	c.AddEdge(assign, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), assign, c.Exit()}
	symOut := setupSymbol(g, "out", points)
	symKey := setupSymbol(g, "key", points)
	for _, p := range points {
		setVersion(g, p, symOut, cfg.Version{Root: "out", Symbol: symOut, ID: 1})
		setVersion(g, p, symKey, cfg.Version{Root: "key", Symbol: symKey, ID: 1})
	}

	outPath := constraint.Path{Root: "out", Symbol: symOut}
	valueType := typ.NewRecord().Field("kind", typ.String).Build()
	inputs := newInputs(g)
	inputs.MapMutatorAssignments = []MapMutatorAssignment{
		{
			Point:     assign,
			Target:    outPath,
			KeySymbol: symKey,
			KeyVar:    "key",
			KeyType:   typ.String,
			ValueType: valueType,
		},
	}

	s := Solve(inputs, testResolver())
	got, ok := s.IndexWriteAdmission(IndexWriteQuery{
		Point:     assign,
		Target:    constraint.Path{Root: "out", Symbol: symOut, Version: 1},
		KeySymbol: symKey,
		KeyType:   typ.String,
	})
	if !ok {
		t.Fatal("IndexWriteAdmission did not find map-mutator proof")
	}
	if !typ.TypeEquals(got, valueType) {
		t.Fatalf("IndexWriteAdmission = %v, want %v", got, valueType)
	}
}

func TestIndexWriteAdmission_RejectsSealedAnnotatedTarget(t *testing.T) {
	c := cfg.New()
	assign := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "scores[key] = value")
	c.AddEdge(c.Entry(), assign, true)
	c.AddEdge(assign, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), assign, c.Exit()}
	symScores := setupSymbol(g, "scores", points)
	symKey := setupSymbol(g, "key", points)
	for _, p := range points {
		setVersion(g, p, symScores, cfg.Version{Root: "scores", Symbol: symScores, ID: 1})
		setVersion(g, p, symKey, cfg.Version{Root: "key", Symbol: symKey, ID: 1})
	}

	scoresPath := constraint.Path{Root: "scores", Symbol: symScores}
	inputs := newInputs(g)
	inputs.DeclaredTypes[symScores] = typ.NewMap(typ.String, typ.Integer)
	inputs.AnnotatedVars = map[cfg.SymbolID]bool{symScores: true}
	inputs.MapMutatorAssignments = []MapMutatorAssignment{
		{
			Point:     assign,
			Target:    scoresPath,
			KeySymbol: symKey,
			KeyVar:    "key",
			KeyType:   typ.String,
			ValueType: typ.String,
		},
	}

	s := Solve(inputs, testResolver())
	if got, ok := s.IndexWriteAdmission(IndexWriteQuery{
		Point:     assign,
		Target:    scoresPath,
		KeySymbol: symKey,
		KeyType:   typ.String,
	}); ok {
		t.Fatalf("IndexWriteAdmission admitted sealed write with %v", got)
	}
}

func TestAssignmentSourceMapElement_UsesKeyOfPresenceProof(t *testing.T) {
	c := cfg.New()
	assign := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "local v = suites[name]")
	c.AddEdge(c.Entry(), assign, true)
	c.AddEdge(assign, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), assign, c.Exit()}
	symSuites := setupSymbol(g, "suites", points)
	symName := setupSymbol(g, "name", points)
	symV := setupSymbol(g, "v", points)
	for _, p := range points {
		setVersion(g, p, symSuites, cfg.Version{Root: "suites", Symbol: symSuites, ID: 1})
		setVersion(g, p, symName, cfg.Version{Root: "name", Symbol: symName, ID: 1})
		setVersion(g, p, symV, cfg.Version{Root: "v", Symbol: symV, ID: 1})
	}

	suitesPath := constraint.Path{Root: "suites", Symbol: symSuites}
	namePath := constraint.Path{Root: "name", Symbol: symName}
	vPath := constraint.Path{Root: "v", Symbol: symV}
	elemType := typ.NewArray(typ.Any)
	inputs := newInputs(g)
	inputs.Decomposer = testTypeDecomposer{}
	inputs.DeclaredTypes[symSuites] = typ.NewMap(typ.String, elemType)
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      c.Entry(),
			To:        assign,
			Condition: constraint.FromConstraints(constraint.KeyOf{Table: suitesPath, Key: namePath}),
		},
	}
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      assign,
			TargetPath: vPath,
			Type:       typ.Unknown,
			Source: AssignmentSource{
				Kind:      AssignmentSourceMapElement,
				MapPath:   suitesPath,
				KeySymbol: symName,
				KeyVar:    "name",
			},
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.TypeAt(assign, vPath); !typ.TypeEquals(got, elemType) {
		t.Fatalf("TypeAt(assign, v) = %v, want non-optional %v", got, elemType)
	}
}

func TestMapMutatorAssignment_ReplayedCapturedMapWritePreservesNestedRecordValue(t *testing.T) {
	c := cfg.New()
	readBefore := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "read_before")
	mutate := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "captured_map_write")
	readAfter := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "read_after")
	c.AddEdge(c.Entry(), readBefore, true)
	c.AddEdge(readBefore, mutate, true)
	c.AddEdge(mutate, readAfter, true)
	c.AddEdge(readAfter, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), readBefore, mutate, readAfter, c.Exit()}
	symCtx := setupSymbol(g, "ctx", points)
	verCtx := cfg.Version{Root: "ctx", Symbol: symCtx, ID: 1}
	for _, p := range points {
		setVersion(g, p, symCtx, verCtx)
	}

	ctxPath := constraint.Path{Root: "ctx", Symbol: symCtx}
	mapPath := ctxPath.Field("activity")
	timeType := typ.NewRecord().
		Field("start", typ.Integer).
		Field("finish", typ.Integer).
		Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symCtx] = typ.NewRecord().
		Field("activity", typ.NewRecord().Build()).
		Build()
	inputs.MapMutatorAssignments = []MapMutatorAssignment{
		{
			Point:     mutate,
			Target:    mapPath,
			KeyType:   typ.String,
			ValueType: timeType,
		},
	}

	s := Solve(inputs, testResolver())
	want := typ.NewMap(typ.String, timeType)
	if got := s.NarrowedTypeAt(readAfter, mapPath); !typ.TypeEquals(got, want) {
		t.Fatalf("NarrowedTypeAt(readAfter, ctx.activity) = %v, want %v", got, want)
	}
}

func TestMapMutatorAssignment_ValueUpdateMergesExistingMapValueShape(t *testing.T) {
	c := cfg.New()
	mutate := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "captured_map_value_update")
	readAfter := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "read_after")
	c.AddEdge(c.Entry(), mutate, true)
	c.AddEdge(mutate, readAfter, true)
	c.AddEdge(readAfter, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), mutate, readAfter, c.Exit()}
	symSessions := setupSymbol(g, "sessions", points)
	verSessions := cfg.Version{Root: "sessions", Symbol: symSessions, ID: 1}
	for _, p := range points {
		setVersion(g, p, symSessions, verSessions)
	}

	baseValue := typ.NewRecord().
		Field("created_at", typ.String).
		Field("pid", typ.String).
		Build()
	updateValue := typ.NewRecord().
		Field("last_activity", typ.String).
		Build()
	sessionsPath := constraint.Path{Root: "sessions", Symbol: symSessions}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symSessions] = typ.NewMap(typ.String, baseValue)
	inputs.MapMutatorAssignments = []MapMutatorAssignment{
		{
			Point:     mutate,
			Target:    sessionsPath,
			ValueMode: MapMutationValueUpdate,
			KeyType:   typ.String,
			ValueType: updateValue,
		},
	}

	s := Solve(inputs, testResolver())
	wantValue := typ.NewRecord().
		OptField("created_at", typ.String).
		OptField("last_activity", typ.String).
		OptField("pid", typ.String).
		Build()
	want := typ.NewMap(typ.String, wantValue)
	if got := s.NarrowedTypeAt(readAfter, sessionsPath); !typ.TypeEquals(got, want) {
		t.Fatalf("NarrowedTypeAt(readAfter, sessions) = %v, want %v", got, want)
	}
}

func TestMutablePathState_LaterResetDoesNotRewriteEarlierPoint(t *testing.T) {
	c := cfg.New()
	insert := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "table.insert")
	read := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "read")
	reset := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "reset")
	afterReset := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "after_reset")
	c.AddEdge(c.Entry(), insert, true)
	c.AddEdge(insert, read, true)
	c.AddEdge(read, reset, true)
	c.AddEdge(reset, afterReset, true)
	c.AddEdge(afterReset, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), insert, read, reset, afterReset, c.Exit()}
	symCtx := setupSymbol(g, "ctx", points)
	verCtx := cfg.Version{Root: "ctx", Symbol: symCtx, ID: 1}
	for _, p := range points {
		setVersion(g, p, symCtx, verCtx)
	}

	itemType := typ.NewRecord().Field("name", typ.String).Build()
	ctxItemsPath := constraint.Path{
		Root:   "ctx",
		Symbol: symCtx,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "items"},
		},
	}
	emptyItems := typ.NewRecord().Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symCtx] = typ.NewRecord().
		Field("items", emptyItems).
		Build()
	inputs.TableMutatorAssignments = []TableMutatorAssignment{
		{
			Point:     insert,
			Target:    ctxItemsPath,
			ValueType: itemType,
		},
	}
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      reset,
			TargetPath: ctxItemsPath,
			Type:       emptyItems,
		},
	}

	s := Solve(inputs, testResolver())
	wantInserted := typ.NewArray(itemType)
	if got := s.NarrowedTypeAt(read, ctxItemsPath); !typ.TypeEquals(got, wantInserted) {
		t.Fatalf("NarrowedTypeAt(read, ctx.items) = %v, want %v", got, wantInserted)
	}
	if got := s.PreStateTypeAt(reset, ctxItemsPath); !typ.TypeEquals(got, wantInserted) {
		t.Fatalf("PreStateTypeAt(reset, ctx.items) = %v, want %v", got, wantInserted)
	}
	if got := s.NarrowedTypeAt(afterReset, ctxItemsPath); !typ.TypeEquals(got, emptyItems) {
		t.Fatalf("NarrowedTypeAt(afterReset, ctx.items) = %v, want %v", got, emptyItems)
	}
}

func TestMutablePathState_ChildResetClearsDescendantFields(t *testing.T) {
	c := cfg.New()
	writeField := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "write_field")
	read := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "read")
	resetChild := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "reset_child")
	afterReset := c.AddNode(cfg.NodeCall, cfg.SymbolID(0), "after_reset")
	c.AddEdge(c.Entry(), writeField, true)
	c.AddEdge(writeField, read, true)
	c.AddEdge(read, resetChild, true)
	c.AddEdge(resetChild, afterReset, true)
	c.AddEdge(afterReset, c.Exit(), true)

	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), writeField, read, resetChild, afterReset, c.Exit()}
	symCtx := setupSymbol(g, "ctx", points)
	verCtx := cfg.Version{Root: "ctx", Symbol: symCtx, ID: 1}
	for _, p := range points {
		setVersion(g, p, symCtx, verCtx)
	}

	itemsPath := constraint.Path{
		Root:   "ctx",
		Symbol: symCtx,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "items"},
		},
	}
	itemsNamePath := constraint.Path{
		Root:   "ctx",
		Symbol: symCtx,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "items"},
			{Kind: constraint.SegmentField, Name: "name"},
		},
	}
	emptyItems := typ.NewRecord().Build()

	inputs := newInputs(g)
	inputs.DeclaredTypes[symCtx] = typ.NewRecord().
		Field("items", typ.NewRecord().SetOpen(true).Build()).
		Build()
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      writeField,
			TargetPath: itemsNamePath,
			Type:       typ.String,
		},
		{
			Point:      resetChild,
			TargetPath: itemsPath,
			Type:       emptyItems,
		},
	}

	s := Solve(inputs, testResolver())
	if got := s.NarrowedTypeAt(read, itemsNamePath); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("NarrowedTypeAt(read, ctx.items.name) = %v, want string", got)
	}
	if got := s.NarrowedTypeAt(afterReset, itemsPath); !typ.TypeEquals(got, emptyItems) {
		t.Fatalf("NarrowedTypeAt(afterReset, ctx.items) = %v, want %v", got, emptyItems)
	}
	if got := s.NarrowedTypeAt(afterReset, itemsNamePath); typ.TypeEquals(got, typ.String) {
		t.Fatalf("NarrowedTypeAt(afterReset, ctx.items.name) retained stale descendant %v", got)
	}
}

func TestMergeFieldAssignments_PreservesNilAlternative(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)

	symCtx := setupSymbol(g, "ctx", []cfg.Point{c.Entry()})
	setVersion(g, c.Entry(), symCtx, cfg.Version{Root: "ctx", Symbol: symCtx, ID: 1})

	inputs := newInputs(g)
	inputs.DeclaredTypes[symCtx] = typ.NewUnion(
		typ.Nil,
		typ.NewRecord().Field("id", typ.Integer).Build(),
	)
	s := Solve(inputs, testResolver())
	s.setValue("sym1@1.name", typ.String)

	got := s.TypeAt(c.Entry(), constraint.Path{Root: "ctx", Symbol: symCtx})
	if !subtype.IsSubtype(typ.Nil, got) {
		t.Fatalf("merged optional/union root dropped nil alternative: %v", got)
	}
	namePath := constraint.Path{
		Root:   "ctx",
		Symbol: symCtx,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "name"},
		},
	}
	if gotName := s.TypeAt(c.Entry(), namePath); !typ.TypeEquals(gotName, typ.String) {
		t.Fatalf("TypeAt(ctx.name) = %v, want string", gotName)
	}
}

func TestPhiOperandSuffixIndexesInvalidateOnStateWrites(t *testing.T) {
	assertSuffixes := func(t *testing.T, got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("suffixes = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("suffixes = %v, want %v", got, want)
			}
		}
	}

	s := &Solution{
		values: liftFlowValues(map[string]typ.Type{
			"sym1@1.child.id": typ.Integer,
			"sym1@1.name":     typ.String,
			"sym10@1.name":    typ.Boolean,
		}),
		mutableValues: liftPointFlowValues(map[cfg.Point]map[string]typ.Type{
			3: {
				"sym1@1.child.active": typ.Boolean,
				"sym1@1.score":        typ.Number,
				"sym10@1.score":       typ.String,
			},
		}),
	}

	assertSuffixes(t, s.valueSuffixesForRoot("sym1@1"), ".child.id", ".name")
	if len(s.valueSuffixIndex) != 2 {
		t.Fatalf("value suffix index entries = %d, want 2", len(s.valueSuffixIndex))
	}
	assertSuffixes(t, s.valueSuffixesForRoot("sym10@1"), ".name")
	s.setValue("sym1@1.age", typ.Integer)
	assertSuffixes(t, s.valueSuffixesForRoot("sym1@1"), ".age", ".child.id", ".name")

	assertSuffixes(t, s.mutableSuffixesForRoot(3, "sym1@1"), ".child.active", ".score")
	if len(s.mutableSuffixIndex) != 2 {
		t.Fatalf("mutable suffix index entries = %d, want 2", len(s.mutableSuffixIndex))
	}
	assertSuffixes(t, s.mutableSuffixesForRoot(3, "sym10@1"), ".score")
	s.setMutableValue(3, "sym1@1.email", typ.String)
	assertSuffixes(t, s.mutableSuffixesForRoot(3, "sym1@1"), ".child.active", ".email", ".score")
}

func TestMutableSuffixReplacementIsPointLocal(t *testing.T) {
	assertSuffixes := func(t *testing.T, got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("suffixes = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("suffixes = %v, want %v", got, want)
			}
		}
	}

	old := liftFlowValues(map[string]typ.Type{
		"sym1@1.old":       typ.String,
		"sym1@1.child.old": typ.Integer,
	})
	next := liftFlowValues(map[string]typ.Type{
		"sym1@1.next": typ.Boolean,
	})
	s := &Solution{
		mutableValues: map[cfg.Point]map[string]product.AbstractValue{
			3: old,
			4: liftFlowValues(map[string]typ.Type{"sym1@1.other": typ.Number}),
		},
	}
	assertSuffixes(t, s.mutableSuffixesForRoot(3, "sym1@1"), ".child.old", ".old")
	assertSuffixes(t, s.mutableSuffixesForRoot(4, "sym1@1"), ".other")

	s.mutableValues[3] = next
	s.replaceMutableSuffixesForPoint(3, old, next)

	assertSuffixes(t, s.mutableSuffixesForRoot(3, "sym1@1"), ".next")
	assertSuffixes(t, s.mutableSuffixesForRoot(4, "sym1@1"), ".other")
	if _, ok := s.mutableSuffixIndex[pointRootKey{point: 3, root: "sym1@1"}]; !ok {
		t.Fatalf("replacement should retain point 3 index for current suffixes: %#v", s.mutableSuffixIndex)
	}
}

func TestPhiEquationStateReusesStableJoin(t *testing.T) {
	s := &Solution{}

	first := s.joinPhiEquation(7, "sym1@2", []typ.Type{typ.String, typ.Integer})
	second := s.joinPhiEquation(7, "sym1@2", []typ.Type{typ.String, typ.Integer})
	if first != second {
		t.Fatalf("stable phi input vector recomputed join: first=%p second=%p", first, second)
	}

	changed := s.joinPhiEquation(7, "sym1@2", []typ.Type{typ.String, typ.Boolean})
	if typ.TypeEquals(changed, first) {
		t.Fatalf("changed phi input vector reused stale join: %v", changed)
	}
}

func TestPhiEquationStateReusesRecursiveConvergedJoin(t *testing.T) {
	s := &Solution{}
	left := typ.NewRecursive("Query", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("where", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})
	right := typ.NewRecursive("Query", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("where", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})

	first := s.joinPhiEquation(7, "sym1@2", []typ.Type{typ.Nil, left})
	second := s.joinPhiEquation(7, "sym1@2", []typ.Type{typ.Nil, right})
	if first != second {
		t.Fatalf("equivalent recursive phi inputs recomputed join: first=%p second=%p", first, second)
	}
}

// TestStabilizePhiJoinKeepsProductEqualRecursiveFact verifies the native
// stabilization contract: stabilizePhiJoin keeps the existing stored fact when a
// re-derived recursive observation is the same point in the product lattice (a
// distinct *typ.Recursive instance that interns to the same canonical product
// node), so the worklist reports no change and drains. A product-equal recursive
// observation must therefore return the existing instance.
func TestStabilizePhiJoinKeepsProductEqualRecursiveFact(t *testing.T) {
	existing := typ.NewRecursive("Query", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("where", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})
	observation := typ.NewRecursive("Query", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("where", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})

	if !product.Equal(liftFlowValue(existing), liftFlowValue(observation)) {
		t.Fatalf("precondition: equivalent recursive facts must be product-equal")
	}
	got := stabilizePhiJoin(existing, observation)
	if got != existing {
		t.Fatalf("stabilizePhiJoin product-equal recursive observation = %v, want existing instance", got)
	}
}

// TestStabilizePhiJoinTakesStrictlyExtendingObservation verifies that when a
// recursive observation strictly extends the existing fact (an added field), it
// is not product-equal, so stabilizePhiJoin takes the more precise observation.
// Worklist termination is owned by product.PhiJoin folding growth and the
// product.Equal no-op check, not by a typ.Type precision keep-existing rule.
func TestStabilizePhiJoinTakesStrictlyExtendingObservation(t *testing.T) {
	existing := typ.NewRecursive("Query", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("where", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})
	observation := typ.NewRecursive("Query", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("from", typ.Func().Param("self", self).Returns(self).Build()).
			Field("where", typ.Func().Param("self", self).Returns(self).Build()).
			Build()
	})

	if product.Equal(liftFlowValue(existing), liftFlowValue(observation)) {
		t.Fatalf("precondition: strictly-extending recursive fact must not be product-equal")
	}
	got := stabilizePhiJoin(existing, observation)
	if got != observation {
		t.Fatalf("stabilizePhiJoin strictly-extending observation = %v, want the observation", got)
	}
}

func TestMutablePathState_BranchesKeepLocalPostStateAndJoin(t *testing.T) {
	c, _, thenNode, elseNode, joinPoint := buildBranchJoinCFG()
	g := newMockSSAGraph(c)
	points := []cfg.Point{c.Entry(), thenNode, elseNode, joinPoint, c.Exit()}
	symCtx := setupSymbol(g, "ctx", points)
	verCtx := cfg.Version{Root: "ctx", Symbol: symCtx, ID: 1}
	for _, p := range points {
		setVersion(g, p, symCtx, verCtx)
	}

	itemType := typ.NewRecord().Field("name", typ.String).Build()
	emptyItems := typ.NewRecord().Build()
	ctxItemsPath := constraint.Path{
		Root:   "ctx",
		Symbol: symCtx,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "items"},
		},
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symCtx] = typ.NewRecord().
		Field("items", emptyItems).
		Build()
	inputs.TableMutatorAssignments = []TableMutatorAssignment{
		{
			Point:     thenNode,
			Target:    ctxItemsPath,
			ValueType: itemType,
		},
	}
	inputs.Assignments = []UnifiedAssignment{
		{
			Point:      elseNode,
			TargetPath: ctxItemsPath,
			Type:       emptyItems,
		},
	}

	s := Solve(inputs, testResolver())
	insertedItems := typ.NewArray(itemType)
	if got := s.NarrowedTypeAt(thenNode, ctxItemsPath); !typ.TypeEquals(got, insertedItems) {
		t.Fatalf("then branch ctx.items = %v, want %v", got, insertedItems)
	}
	if got := s.NarrowedTypeAt(elseNode, ctxItemsPath); !typ.TypeEquals(got, emptyItems) {
		t.Fatalf("else branch ctx.items = %v, want %v", got, emptyItems)
	}

	joined := s.NarrowedTypeAt(joinPoint, ctxItemsPath)
	if joined == nil {
		t.Fatal("join ctx.items = nil, want joined branch state")
	}
	if !subtype.IsSubtype(insertedItems, joined) || !subtype.IsSubtype(emptyItems, joined) {
		t.Fatalf("join ctx.items = %v, want to include %v and %v", joined, insertedItems, emptyItems)
	}
}

func TestAdmitMapArrayElementMutation_PreservesAlias(t *testing.T) {
	base := typ.NewAlias("Registry", typ.NewMap(typ.String, typ.NewArray(typ.String)))
	got := value.AdmitMapArrayElementMutation(base, typ.String, typ.Integer)
	alias, ok := got.(*typ.Alias)
	if !ok {
		t.Fatalf("value.AdmitMapArrayElementMutation(alias) = %T, want *typ.Alias", got)
	}
	if alias.Name != "Registry" {
		t.Fatalf("alias name = %q, want Registry", alias.Name)
	}
	mp, ok := alias.Target.(*typ.Map)
	if !ok {
		t.Fatalf("alias target = %T, want *typ.Map", alias.Target)
	}
	arr, ok := mp.Value.(*typ.Array)
	if !ok {
		t.Fatalf("map value = %T, want *typ.Array", mp.Value)
	}
	if !typ.TypeEquals(arr.Element, typ.NewUnion(typ.String, typ.Integer)) {
		t.Fatalf("map alias array element = %v, want string|integer", arr.Element)
	}
}

func TestProcessJoinReturnChangedKeys_NoPhi(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	inputs := newInputs(g)
	s := Solve(inputs, testResolver())

	keys := s.processJoinReturnChangedKeys(c.Entry())
	if len(keys) != 0 {
		t.Errorf("processJoinReturnChangedKeys = %v, want empty", keys)
	}
}

func TestProcessJoinReturnChangedKeys_WithPhi(t *testing.T) {
	c, branch, then1, join, _, _ := buildPhiTruthyCFG()
	g := newMockSSAGraph(c)

	symX := setupSymbol(g, "x", []cfg.Point{c.Entry(), branch, then1, join})
	ver1 := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	ver2 := cfg.Version{Root: "x", Symbol: symX, ID: 2}
	ver3 := cfg.Version{Root: "x", Symbol: symX, ID: 3}

	setVersion(g, c.Entry(), symX, ver1)
	setVersion(g, branch, symX, ver1)
	setVersion(g, then1, symX, ver2)
	setVersion(g, join, symX, ver3)

	g.addPhiNode(cfg.PhiNode{
		Point:  join,
		Target: ver3,
		Operands: []cfg.PhiOperand{
			{From: branch, Version: ver1},
			{From: then1, Version: ver2},
		},
	})

	inputs := newInputs(g)
	inputs.Assignments = []UnifiedAssignment{
		{Point: c.Entry(), TargetPath: constraint.Path{Root: "x", Symbol: symX}, Type: typ.String},
		{Point: then1, TargetPath: constraint.Path{Root: "x", Symbol: symX}, Type: typ.Integer},
	}

	s := Solve(inputs, testResolver())

	// At join, x should be union of string and integer
	path := constraint.Path{Root: "x", Symbol: symX}
	result := s.TypeAt(join, path)
	if result == nil {
		t.Fatal("TypeAt(join, x) = nil, want union")
	}

	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("TypeAt(join, x) = %T (%v), want union", result, result)
	}
	if len(union.Members) != 2 {
		t.Errorf("union members = %d, want 2", len(union.Members))
	}
}
