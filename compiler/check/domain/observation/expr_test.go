package observation

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type factsStub map[cfg.SymbolID]typ.Type

func (f factsStub) DeclaredAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return typedValueForTest(f[sym])
}

func (f factsStub) RefinedAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: nil, State: flow.StateUnknown}
}

func (f factsStub) EffectiveTypeAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return typedValueForTest(f[sym])
}

func (f factsStub) IsAnnotated(cfg.SymbolID) bool {
	return false
}

type annotatedFactsStub struct {
	factsStub
	annotated map[cfg.SymbolID]bool
}

func (f annotatedFactsStub) IsAnnotated(sym cfg.SymbolID) bool {
	return f.annotated[sym]
}

func typedValueForTest(t typ.Type) flow.TypedValue {
	if t == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return flow.TypedValue{Type: t, State: flow.StateResolved}
}

type productFactsStub struct {
	factsStub
	values map[cfg.SymbolID]product.AbstractValue
}

func (f productFactsStub) RefinedValueAt(_ cfg.Point, sym cfg.SymbolID) flow.ProductValue {
	if av, ok := f.values[sym]; ok && !av.IsZero() {
		return flow.ProductValue{Value: av, State: flow.StateResolved}
	}
	return flow.ProductValue{State: flow.StateUnknown}
}

func (f productFactsStub) RefinedPathValueAt(_ cfg.Point, path constraint.Path) flow.ProductValue {
	if len(path.Segments) == 0 {
		return f.RefinedValueAt(0, path.Symbol)
	}
	if av, ok := f.values[path.Symbol]; ok && !av.IsZero() {
		for _, seg := range path.Segments {
			member, ok := value.MemberFromSegment(seg)
			if !ok {
				return flow.ProductValue{State: flow.StateUnknown}
			}
			next, ok := product.MemberOf(av, member)
			if !ok || next.IsZero() {
				return flow.ProductValue{State: flow.StateUnknown}
			}
			av = next
		}
		return flow.ProductValue{Value: av, State: flow.StateResolved}
	}
	return flow.ProductValue{State: flow.StateUnknown}
}

type assignmentSourceFactsStub struct {
	factsStub
	value typ.Type
}

func (f assignmentSourceFactsStub) AssignmentSourceValueAt(cfg.Point, constraint.Path, flow.AssignmentSource) typ.Type {
	return f.value
}

type assignmentSelectionFactsStub struct {
	factsStub
	stored   typ.Type
	path     typ.Type
	constSym cfg.SymbolID
	sawPath  constraint.Path
}

func (f *assignmentSelectionFactsStub) AssignmentSourceValueAt(cfg.Point, constraint.Path, flow.AssignmentSource) typ.Type {
	return f.stored
}

func (f *assignmentSelectionFactsStub) RefinedPathAt(_ cfg.Point, path constraint.Path) flow.TypedValue {
	f.sawPath = path
	return typedValueForTest(f.path)
}

func (f *assignmentSelectionFactsStub) ConstValueAtSym(_ cfg.Point, sym cfg.SymbolID) *flow.ConstValue {
	if sym != f.constSym {
		return nil
	}
	return &flow.ConstValue{Kind: flow.ConstString, Str: "p-q"}
}

type indexWriteFactsStub struct {
	factsStub
	value typ.Type
	query flow.IndexWriteQuery
}

func (f *indexWriteFactsStub) IndexWriteAdmission(q flow.IndexWriteQuery) (typ.Type, bool) {
	f.query = q
	return f.value, f.value != nil
}

type conditionProofFactsStub struct {
	factsStub
	cond       constraint.Condition
	provedPath constraint.Path
	provedType typ.Type
}

func (f conditionProofFactsStub) ConditionAt(cfg.Point) constraint.Condition {
	if f.cond.HasConstraints() || f.cond.IsFalse() {
		return f.cond
	}
	return constraint.TrueCondition()
}

func (f conditionProofFactsStub) ProvesTypeAt(_ cfg.Point, path constraint.Path, t typ.Type) bool {
	return path.Equal(f.provedPath) && typ.TypeEquals(t, f.provedType)
}

func (f conditionProofFactsStub) ConditionTypeAt(_ cfg.Point, path constraint.Path) typ.Type {
	if path.Equal(f.provedPath) {
		return f.provedType
	}
	return nil
}

func (f conditionProofFactsStub) ConditionedTypeAt(_ cfg.Point, path constraint.Path, _ constraint.Condition) typ.Type {
	return f.ConditionTypeAt(0, path)
}

func (f conditionProofFactsStub) ConditionedSeedTypeAt(_ cfg.Point, _ constraint.Path, _ typ.Type, queryPath constraint.Path, _ constraint.Condition) typ.Type {
	return f.ConditionTypeAt(0, queryPath)
}

type flowOpsStub struct {
	narrowed typ.Type
	pre      typ.Type
}

func (f flowOpsStub) NarrowedTypeAt(cfg.Point, constraint.Path) typ.Type {
	return f.narrowed
}

func (f flowOpsStub) NarrowedTypeAtWithCondition(cfg.Point, constraint.Path, constraint.Condition) typ.Type {
	return f.narrowed
}

func (f flowOpsStub) PreStateTypeAt(cfg.Point, constraint.Path) typ.Type {
	return f.pre
}

func (f flowOpsStub) ExcludesTypeAt(cfg.Point, constraint.Path, typ.Type) bool {
	return false
}

func (f flowOpsStub) BoundsAt(cfg.Point, string) (int64, int64, bool) {
	return 0, 0, false
}

func (f flowOpsStub) ArrayLenBoundAt(cfg.Point, string) (string, bool) {
	return "", false
}

func (f flowOpsStub) ArrayLenBoundWithOffsetAt(cfg.Point, string) (string, int64, bool) {
	return "", 0, false
}

func (f flowOpsStub) LengthBoundsAt(cfg.Point, constraint.Path) (int64, int64, bool) {
	return 0, 0, false
}

func (f flowOpsStub) IsPointDead(cfg.Point) bool {
	return false
}

func (f flowOpsStub) HasKeyOf(cfg.Point, constraint.Path, constraint.Path) bool {
	return false
}

type pathObservationFactsStub struct {
	factsStub
	observation flow.PathObservation
	query       flow.PathObservationQuery
}

func (f *pathObservationFactsStub) ObservePath(q flow.PathObservationQuery) flow.PathObservation {
	f.query = q
	return f.observation
}

type bodyContractOriginFactsStub struct {
	factsStub
	contracts     paramevidence.Contracts
	origins       flow.ValueOriginFacts
	aliases       flow.PathAliasFacts
	appendOrigins flow.KeyPresenceFacts
	cond          constraint.Condition
	annotated     map[cfg.SymbolID]bool
}

func (f bodyContractOriginFactsStub) BodyContracts() paramevidence.Contracts {
	return f.contracts
}

func (f bodyContractOriginFactsStub) ValueOriginsAt(cfg.Point) flow.ValueOriginFacts {
	return f.origins
}

func (f bodyContractOriginFactsStub) PathAliasesAt(cfg.Point) flow.PathAliasFacts {
	return f.aliases
}

func (f bodyContractOriginFactsStub) AppendElementFieldSourcesAt(_ cfg.Point, array constraint.PathKey, field []constraint.Segment) []flow.AppendElementFieldOriginUse {
	return f.appendOrigins.AppendElementFieldSources(array, field)
}

func (f bodyContractOriginFactsStub) IsAnnotated(sym cfg.SymbolID) bool {
	return f.annotated[sym]
}

func (f bodyContractOriginFactsStub) ConditionAt(cfg.Point) constraint.Condition {
	if f.cond.HasConstraints() || f.cond.IsFalse() {
		return f.cond
	}
	return constraint.TrueCondition()
}

func (f bodyContractOriginFactsStub) ProvesTypeAt(point cfg.Point, path constraint.Path, t typ.Type) bool {
	return f.conditionProjector().ProvesTypeAt(point, path, t)
}

func (f bodyContractOriginFactsStub) ConditionTypeAt(point cfg.Point, path constraint.Path) typ.Type {
	return f.conditionProjector().ConditionTypeAt(point, path)
}

func (f bodyContractOriginFactsStub) ConditionedTypeAt(point cfg.Point, path constraint.Path, extra constraint.Condition) typ.Type {
	return f.conditionProjector().ConditionedTypeAt(point, path, extra)
}

func (f bodyContractOriginFactsStub) ConditionedSeedTypeAt(point cfg.Point, seedPath constraint.Path, seedType typ.Type, queryPath constraint.Path, extra constraint.Condition) typ.Type {
	return f.conditionProjector().ConditionedSeedTypeAt(point, seedPath, seedType, queryPath, extra)
}

func (f bodyContractOriginFactsStub) conditionProjector() flow.ConditionProofProjector {
	return flow.ConditionProofProjector{
		Resolver:    observationResolver{},
		ConditionAt: f.ConditionAt,
		RootTypeAt: func(point cfg.Point, path constraint.Path) typ.Type {
			tv := f.EffectiveTypeAt(point, path.Symbol)
			if tv.State != flow.StateResolved {
				return nil
			}
			return tv.Type
		},
		ResolvePath: func(_ cfg.Point, path constraint.Path) constraint.PathKey {
			return flow.ConditionProofStructuralPathKey(path)
		},
	}
}

func TestProjector_IdentUsesSolvedFacts(t *testing.T) {
	ident := &ast.IdentExpr{Value: "value"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 10
	bindings.Bind(ident, sym)

	observed := New(Config{
		Bindings: bindings,
		Facts:    factsStub{sym: typ.String},
	}).TypeOf(ident, 1)

	if !typ.TypeEquals(observed, typ.String) {
		t.Fatalf("TypeOf(ident) = %v, want string", observed)
	}
}

func TestProjector_CallArgumentProofUsesBodyContractValueOrigin(t *testing.T) {
	op := &ast.IdentExpr{Value: "op"}
	opTemplate := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object:    op,
			Key:       &ast.StringExpr{Value: "config"},
			KeySyntax: ast.AttrKeyDot,
		},
		Key:       &ast.StringExpr{Value: "template"},
		KeySyntax: ast.AttrKeyDot,
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"operations"}, Types: []ast.TypeExpr{nil}},
	}
	g := cfg.Build(fn)
	slots := g.ParamSlotsReadOnly()
	if len(slots) != 1 || slots[0].Symbol == 0 {
		t.Fatalf("ParamSlotsReadOnly() = %#v, want one operations slot", slots)
	}
	const point cfg.Point = 7
	const opSym cfg.SymbolID = 52
	operationsSym := slots[0].Symbol
	bindings := bind.NewBindingTable()
	bindings.Bind(op, opSym)

	valuePath := constraint.NewPath(opSym, "op")
	templatePath := valuePath.Field("config").Field("template")
	sourcePath := constraint.NewPath(operationsSym, "operations")
	localContract := paramevidence.DemandFromPathType(
		[]constraint.Segment{
			{Kind: constraint.SegmentField, Name: "config"},
			{Kind: constraint.SegmentField, Name: "template"},
		},
		typ.NewUnion(typ.String, typ.Nil, typ.False),
	)
	sourceContract := paramevidence.IndexedIteratorContract(1, localContract)
	facts := bodyContractOriginFactsStub{
		factsStub: factsStub{opSym: typ.Any},
		contracts: paramevidence.Contracts{
			0: sourceContract,
		},
		origins: flow.ValueOriginFacts{}.WithPaths(
			valuePath,
			sourcePath,
			flow.ValueOriginIndexedIterator,
			1,
		),
		cond: constraint.FromConstraints(constraint.Truthy{Path: templatePath}),
	}

	projector := New(Config{
		Graph:    g,
		Bindings: bindings,
		Facts:    facts,
	})
	if ordinary := projector.TypeOf(opTemplate, point); !typ.IsAny(ordinary) {
		t.Fatalf("ordinary TypeOf(op.config.template) = %v, want any without call-argument proof", ordinary)
	}

	got := projector.WithCallArgumentProofs().TypeOfWithExpected(opTemplate, point, typ.String)

	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("TypeOfWithExpected(op.config.template) = %v, want string", got)
	}
}

func TestProjector_CallArgumentProofRoutesThroughAppendElementFieldOrigin(t *testing.T) {
	routeEntry := &ast.IdentExpr{Value: "route_entry"}
	routeTarget := &ast.AttrGetExpr{
		Object:    routeEntry,
		Key:       &ast.StringExpr{Value: "target_name"},
		KeySyntax: ast.AttrKeyDot,
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"operations"}, Types: []ast.TypeExpr{nil}},
	}
	g := cfg.Build(fn)
	slots := g.ParamSlotsReadOnly()
	if len(slots) != 1 || slots[0].Symbol == 0 {
		t.Fatalf("ParamSlotsReadOnly() = %#v, want one operations slot", slots)
	}
	const point cfg.Point = 9
	const opSym cfg.SymbolID = 61
	const routeSym cfg.SymbolID = 62
	const routeEntrySym cfg.SymbolID = 63
	const graphSym cfg.SymbolID = 64
	operationsSym := slots[0].Symbol
	bindings := bind.NewBindingTable()
	bindings.Bind(routeEntry, routeEntrySym)

	opPath := constraint.NewPath(opSym, "op")
	operationsPath := constraint.NewPath(operationsSym, "operations")
	routePath := constraint.NewPath(routeSym, "route")
	routeEntryPath := constraint.NewPath(routeEntrySym, "route_entry")
	inputRoutesPath := constraint.NewPath(graphSym, "graph").Field("input_routes")
	targetField := []constraint.Segment{{Kind: constraint.SegmentField, Name: "target_name"}}
	targetContract := paramevidence.DemandFromPathType(
		[]constraint.Segment{
			{Kind: constraint.SegmentField, Name: "config"},
			{Kind: constraint.SegmentField, Name: "target"},
		},
		typ.String,
	)
	facts := bodyContractOriginFactsStub{
		factsStub: factsStub{routeEntrySym: typ.Any},
		contracts: paramevidence.Contracts{
			0: paramevidence.IndexedIteratorContract(1, targetContract),
		},
		origins: flow.ValueOriginFacts{}.
			WithPaths(opPath, operationsPath, flow.ValueOriginIndexedIterator, 1).
			WithPaths(routePath, inputRoutesPath, flow.ValueOriginIndexedIterator, 1),
		aliases: flow.PathAliasFacts{}.WithPaths(routeEntryPath, routePath),
		appendOrigins: flow.KeyPresenceFacts{}.
			WithAppendHistoryBasePath(inputRoutesPath).
			WithAppendElementFieldOriginPaths(inputRoutesPath, targetField, opPath.Field("config").Field("target")),
		cond: constraint.TrueCondition(),
	}

	projector := New(Config{
		Graph:    g,
		Bindings: bindings,
		Facts:    facts,
	})
	if ordinary := projector.TypeOf(routeTarget, point); !typ.IsAny(ordinary) {
		t.Fatalf("ordinary TypeOf(route_entry.target_name) = %v, want any without call-argument proof", ordinary)
	}

	got := projector.WithCallArgumentProofs().TypeOfWithExpected(routeTarget, point, typ.String)

	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("TypeOfWithExpected(route_entry.target_name) = %v, want string", got)
	}
}

func TestProjector_CallArgumentProofRoutesThroughElementRelativeAppendOrigin(t *testing.T) {
	routeEntry := &ast.IdentExpr{Value: "route_entry"}
	routeTarget := &ast.AttrGetExpr{
		Object:    routeEntry,
		Key:       &ast.StringExpr{Value: "target_name"},
		KeySyntax: ast.AttrKeyDot,
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"operations"}, Types: []ast.TypeExpr{nil}},
	}
	g := cfg.Build(fn)
	slots := g.ParamSlotsReadOnly()
	if len(slots) != 1 || slots[0].Symbol == 0 {
		t.Fatalf("ParamSlotsReadOnly() = %#v, want one operations slot", slots)
	}
	const point cfg.Point = 9
	const routeSym cfg.SymbolID = 62
	const routeEntrySym cfg.SymbolID = 63
	const graphSym cfg.SymbolID = 64
	operationsSym := slots[0].Symbol
	bindings := bind.NewBindingTable()
	bindings.Bind(routeEntry, routeEntrySym)

	operationsPath := constraint.NewPath(operationsSym, "operations")
	routePath := constraint.NewPath(routeSym, "route")
	routeEntryPath := constraint.NewPath(routeEntrySym, "route_entry")
	inputRoutesPath := constraint.NewPath(graphSym, "graph").Field("input_routes")
	targetField := []constraint.Segment{{Kind: constraint.SegmentField, Name: "target_name"}}
	sourceField := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "config"},
		{Kind: constraint.SegmentField, Name: "target"},
	}
	targetContract := paramevidence.DemandFromPathType(sourceField, typ.String)
	facts := bodyContractOriginFactsStub{
		factsStub: factsStub{routeEntrySym: typ.Any},
		contracts: paramevidence.Contracts{
			0: paramevidence.IndexedIteratorContract(1, targetContract),
		},
		origins: flow.ValueOriginFacts{}.
			WithPaths(routePath, inputRoutesPath, flow.ValueOriginIndexedIterator, 1),
		aliases: flow.PathAliasFacts{}.WithPaths(routeEntryPath, routePath),
		appendOrigins: flow.KeyPresenceFacts{}.
			WithAppendHistoryBasePath(inputRoutesPath).
			WithAppendElementFieldOriginFromPaths(inputRoutesPath, targetField, operationsPath, sourceField),
		cond: constraint.TrueCondition(),
	}

	projector := New(Config{
		Graph:    g,
		Bindings: bindings,
		Facts:    facts,
	})
	got := projector.WithCallArgumentProofs().TypeOfWithExpected(routeTarget, point, typ.String)

	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("TypeOfWithExpected(route_entry.target_name) = %v, want string", got)
	}
}

func TestProjector_CallArgumentProofUsesRootLocalBodyContractValueOrigin(t *testing.T) {
	nodeID := &ast.IdentExpr{Value: "node_id"}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"self"}, Types: []ast.TypeExpr{nil}},
	}
	g := cfg.Build(fn)
	slots := g.ParamSlotsReadOnly()
	if len(slots) != 1 || slots[0].Symbol == 0 {
		t.Fatalf("ParamSlotsReadOnly() = %#v, want one self slot", slots)
	}
	const point cfg.Point = 11
	const nodeSym cfg.SymbolID = 77
	selfSym := slots[0].Symbol
	bindings := bind.NewBindingTable()
	bindings.Bind(nodeID, nodeSym)

	nodePath := constraint.NewPath(nodeSym, "node_id")
	nodesPath := constraint.NewPath(selfSym, "self").Field("nodes")
	nodesContract := paramevidence.KeyedIteratorContract(0, paramevidence.DemandFromType(typ.String))
	if projected := nodesContract.ProjectValue(); projected == nil || typ.IsNever(projected) {
		t.Fatalf("nodesContract.ProjectValue() = %v, want readonly map", projected)
	}
	selfContract := paramevidence.DemandFromPathContract(
		[]constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}},
		nodesContract,
	)
	if projected := selfContract.ProjectValue(); projected == nil || typ.IsNever(projected) {
		t.Fatalf("selfContract.ProjectValue() = %v, want record with nodes map", projected)
	}
	facts := bodyContractOriginFactsStub{
		factsStub: factsStub{nodeSym: typ.Any},
		contracts: paramevidence.Contracts{
			0: selfContract,
		},
		annotated: map[cfg.SymbolID]bool{selfSym: true},
		origins: flow.ValueOriginFacts{}.WithPaths(
			nodePath,
			nodesPath,
			flow.ValueOriginKeyedIterator,
			0,
		),
		cond: constraint.TrueCondition(),
	}

	projector := New(Config{
		Graph:    g,
		Bindings: bindings,
		Facts:    facts,
	})
	if ordinary := projector.TypeOf(nodeID, point); !typ.IsAny(ordinary) {
		t.Fatalf("ordinary TypeOf(node_id) = %v, want any without call-argument proof", ordinary)
	}
	proofProjector := projector.WithCallArgumentProofs()
	if direct := proofProjector.directBodyContractPathType(nodesPath); direct == nil || typ.IsNever(direct) {
		t.Fatalf("directBodyContractPathType(self.nodes) = %v, want readonly map", direct)
	}
	sourceType := proofProjector.bodyContractPathTypeAtPath(point, nodesPath, nil)
	if keyType := querycore.EntryKeyType(sourceType); !typ.TypeEquals(keyType, typ.String) {
		t.Fatalf("bodyContractPathTypeAtPath(self.nodes) = %v, key %v, want string key", sourceType, keyType)
	}
	if localType := proofProjector.bodyContractPathTypeAtPath(point, nodePath, nil); !typ.TypeEquals(localType, typ.String) {
		t.Fatalf("bodyContractPathTypeAtPath(node_id) = %v, want string", localType)
	}

	got := proofProjector.TypeOfWithExpected(nodeID, point, typ.String)

	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("TypeOfWithExpected(node_id) = %v, want string", got)
	}
}

func TestProjector_AssignmentSourceTypeUsesFactsCarrierWithoutSolution(t *testing.T) {
	source := &ast.IdentExpr{Value: "rhs"}
	const targetSym cfg.SymbolID = 22
	target := constraint.NewPath(targetSym, "target")

	got := New(Config{
		Inputs: &flow.Inputs{
			Assignments: []flow.UnifiedAssignment{
				{
					Point:      7,
					TargetPath: target,
					Source: flow.AssignmentSource{
						Kind: flow.AssignmentSourcePath,
						Path: constraint.NewPath(23, "rhs"),
					},
				},
			},
		},
		Facts: assignmentSourceFactsStub{
			factsStub: factsStub{},
			value:     typ.String,
		},
	}).AssignmentSourceType(source, 7, nil, targetSym)

	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("AssignmentSourceType via facts carrier = %v, want string", got)
	}
}

func TestProjector_AssignmentSourceTypeUsesMorePreciseNormalizedPath(t *testing.T) {
	obj := &ast.IdentExpr{Value: "obj"}
	key := &ast.IdentExpr{Value: "key"}
	source := &ast.AttrGetExpr{Object: obj, Key: key, KeySyntax: ast.AttrKeyIndex}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"obj"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"key"},
				Exprs: []ast.Expr{&ast.StringExpr{Value: "p-q"}},
			},
			&ast.LocalAssignStmt{
				Names: []string{"target"},
				Exprs: []ast.Expr{source},
			},
		},
	}
	g := cfg.Build(fn)
	bindings := g.Bindings()

	var point cfg.Point
	var targetSym cfg.SymbolID
	g.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			if src != source {
				return
			}
			point = p
			targetSym = target.Symbol
		})
	})
	if point == 0 || targetSym == 0 {
		t.Fatalf("target assignment not found: point=%d target=%d", point, targetSym)
	}
	objSym, ok := g.SymbolAt(point, "obj")
	if !ok || objSym == 0 {
		t.Fatalf("obj symbol at point = %d, %v; want visible symbol", objSym, ok)
	}
	keySym, ok := g.SymbolAt(point, "key")
	if !ok || keySym == 0 {
		t.Fatalf("key symbol at point = %d, %v; want visible symbol", keySym, ok)
	}

	pointType := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	target := constraint.NewPath(targetSym, "target")
	facts := &assignmentSelectionFactsStub{
		stored:   typ.Any,
		path:     pointType,
		constSym: keySym,
	}

	got := New(Config{
		Graph:    g,
		Bindings: bindings,
		Inputs: &flow.Inputs{
			Assignments: []flow.UnifiedAssignment{
				{
					Point:      point,
					TargetPath: target,
					Source: flow.AssignmentSource{
						Kind: flow.AssignmentSourcePath,
						Path: constraint.NewPath(objSym, "obj").IndexStr("p-q"),
					},
				},
			},
		},
		Facts: facts,
	}).AssignmentSourceType(source, point, pointType, targetSym)

	if !typ.TypeEquals(got, pointType) {
		t.Fatalf("AssignmentSourceType = %v, want normalized path precision %v", got, pointType)
	}
	if len(facts.sawPath.Segments) != 1 ||
		facts.sawPath.Segments[0].Kind != constraint.SegmentIndexString ||
		facts.sawPath.Segments[0].Name != "p-q" {
		t.Fatalf("observed path = %#v, want const-normalized obj[\"p-q\"]", facts.sawPath)
	}
}

func TestProjector_AssignmentSourceSelfReadUsesStrictPreState(t *testing.T) {
	source := &ast.IdentExpr{Value: "value"}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"value"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{source}},
		},
	}
	g := cfg.Build(fn)
	syms := g.ParamSymbols()
	if len(syms) != 1 || syms[0] == 0 {
		t.Fatalf("ParamSymbols() = %v, want one non-zero symbol", syms)
	}

	got := New(Config{
		Graph:    g,
		Bindings: g.Bindings(),
		Flow:     flowOpsStub{pre: typ.String, narrowed: typ.Number},
	}).AssignmentSourceType(source, g.Entry(), nil, syms[0])

	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("AssignmentSourceType self-read = %v, want pre-state string", got)
	}
}

func TestProjector_AssignmentTargetWriteTypeUsesIndexWriteFactsWithoutSolution(t *testing.T) {
	base := &ast.IdentExpr{Value: "m"}
	key := &ast.IdentExpr{Value: "k"}
	source := &ast.IdentExpr{Value: "v"}
	bindings := bind.NewBindingTable()
	const (
		baseSym cfg.SymbolID = 31
		keySym  cfg.SymbolID = 32
		valSym  cfg.SymbolID = 33
	)
	bindings.Bind(base, baseSym)
	bindings.Bind(key, keySym)
	bindings.Bind(source, valSym)
	facts := &indexWriteFactsStub{value: typ.String}

	got := New(Config{
		Bindings: bindings,
		Facts:    facts,
	}).AssignmentTargetWriteType(cfg.AssignTarget{
		Kind:       cfg.TargetIndex,
		Base:       base,
		BaseName:   "m",
		BaseSymbol: baseSym,
		Key:        key,
	}, source, 9)

	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("AssignmentTargetWriteType via IndexWriteFacts = %v, want string", got)
	}
	if facts.query.Point != 9 || !facts.query.Target.Equal(constraint.NewPath(baseSym, "m")) ||
		facts.query.KeySymbol != keySym || !facts.query.ValuePath.Equal(constraint.NewPath(valSym, "v")) {
		t.Fatalf("IndexWriteAdmission query = %#v", facts.query)
	}
}

func TestProjector_AssignmentTargetFlowWriteTypeIgnoresAnyAdmission(t *testing.T) {
	base := &ast.IdentExpr{Value: "m"}
	key := &ast.IdentExpr{Value: "k"}
	source := &ast.IdentExpr{Value: "v"}
	bindings := bind.NewBindingTable()
	const baseSym cfg.SymbolID = 41
	bindings.Bind(base, baseSym)
	bindings.Bind(key, cfg.SymbolID(42))
	bindings.Bind(source, cfg.SymbolID(43))

	got := New(Config{
		Bindings: bindings,
		Facts:    &indexWriteFactsStub{value: typ.Any},
	}).assignmentTargetFlowWriteType(cfg.AssignTarget{
		Kind:       cfg.TargetIndex,
		Base:       base,
		BaseName:   "m",
		BaseSymbol: baseSym,
		Key:        key,
	}, source, 9)

	if got != nil {
		t.Fatalf("assignmentTargetFlowWriteType(any admission) = %v, want nil", got)
	}
}

func TestProjectorProvesExprTypeUsesConditionProofFactsWithoutSolution(t *testing.T) {
	ident := &ast.IdentExpr{Value: "value"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 44
	bindings.Bind(ident, sym)
	path := constraint.NewPath(sym, "value")

	projector := New(Config{
		Bindings: bindings,
		Facts: conditionProofFactsStub{
			factsStub:  factsStub{sym: typ.Unknown},
			provedPath: path,
			provedType: typ.String,
		},
	})

	if !projector.provesExprType(3, ident, typ.String) {
		t.Fatalf("provesExprType via ConditionProofFacts = false, want true")
	}
}

func TestProjectorPathTypeUsesFlowOpsWithoutSolution(t *testing.T) {
	ident := &ast.IdentExpr{Value: "value"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 45
	bindings.Bind(ident, sym)

	projector := New(Config{
		Bindings: bindings,
		Flow:     flowOpsStub{narrowed: typ.String},
	})

	got := projector.pathType(ident, 4)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("pathType via FlowOps = %v, want string", got)
	}
}

func TestProjectorPathTypeUsesPathObservationFactsWithoutSolution(t *testing.T) {
	ident := &ast.IdentExpr{Value: "value"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 46
	bindings.Bind(ident, sym)
	facts := &pathObservationFactsStub{
		factsStub: factsStub{sym: typ.Unknown},
		observation: flow.PathObservation{
			Type:   typ.String,
			State:  flow.StateResolved,
			Source: flow.PathObservationSolvedFlow,
		},
	}

	got := New(Config{
		Bindings:      bindings,
		Facts:         facts,
		PreserveProof: true,
	}).pathType(ident, 8)

	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("pathType via PathObservationFacts = %v, want string", got)
	}
	if facts.query.Point != 8 || facts.query.View != flow.PathReadCurrent || !facts.query.PreserveProof ||
		!facts.query.Path.Equal(constraint.NewPath(sym, "value")) {
		t.Fatalf("PathObservationQuery = %#v", facts.query)
	}
}

func TestObservedArgumentTypeUsesPathObservationFactsWithoutConcreteSolver(t *testing.T) {
	arg := &ast.IdentExpr{Value: "arg"}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"arg"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{arg}},
		},
	}
	g := cfg.Build(fn)
	bindings := g.Bindings()
	sym, ok := bindings.SymbolOf(arg)
	if !ok || sym == 0 {
		t.Fatal("missing arg symbol")
	}
	facts := &pathObservationFactsStub{
		factsStub: factsStub{sym: typ.Unknown},
		observation: flow.PathObservation{
			Type:   typ.String,
			State:  flow.StateResolved,
			Source: flow.PathObservationFactProjection,
		},
	}

	got := ObservedArgumentType(&api.FuncResult{
		Graph:          g,
		ModuleBindings: bindings,
		Facts:          facts,
	}, g.Entry(), arg, nil, bindings)

	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ObservedArgumentType via PathObservationFacts = %v, want string", got)
	}
	if facts.query.Point != g.Entry() ||
		facts.query.View != flow.PathReadPre ||
		!facts.query.PreserveProof ||
		facts.query.AllowConditionProof ||
		facts.query.Path.Symbol != sym {
		t.Fatalf("PathObservationQuery = %#v", facts.query)
	}
}

func TestObservedArgumentTypeIgnoresDeclaredOnlyPathObservation(t *testing.T) {
	arg := &ast.IdentExpr{Value: "arg"}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"arg"}},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{arg}},
		},
	}
	g := cfg.Build(fn)
	bindings := g.Bindings()
	sym, ok := bindings.SymbolOf(arg)
	if !ok || sym == 0 {
		t.Fatal("missing arg symbol")
	}
	facts := &pathObservationFactsStub{
		factsStub: factsStub{sym: typ.String},
		observation: flow.PathObservation{
			Type:   typ.String,
			State:  flow.StateResolved,
			Source: flow.PathObservationDeclared,
		},
	}

	got := ObservedArgumentType(&api.FuncResult{
		Graph:          g,
		ModuleBindings: bindings,
		Facts:          facts,
	}, g.Entry(), arg, typ.Number, bindings)

	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("ObservedArgumentType declared-only fallback = %v, want current number", got)
	}
}

func TestProjectorConstResolverUsesInputsWithoutSolution(t *testing.T) {
	keyIdent := &ast.IdentExpr{Value: "key"}
	objIdent := &ast.IdentExpr{Value: "obj"}
	read := &ast.AttrGetExpr{
		Object:    objIdent,
		Key:       keyIdent,
		KeySyntax: ast.AttrKeyIndex,
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"obj"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"key"},
				Exprs: []ast.Expr{&ast.StringExpr{Value: "name"}},
			},
			&ast.ReturnStmt{Exprs: []ast.Expr{read}},
		},
	}
	g := cfg.Build(fn)
	bindings := g.Bindings()

	var returnPoint cfg.Point
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		returnPoint = p
	})
	if returnPoint == 0 {
		t.Fatal("return point not found")
	}
	keySym, ok := g.SymbolAt(returnPoint, "key")
	if !ok || keySym == 0 {
		t.Fatalf("key symbol at return point = %d, %v; want visible symbol", keySym, ok)
	}
	objSym, ok := g.SymbolAt(returnPoint, "obj")
	if !ok || objSym == 0 {
		t.Fatalf("obj symbol at return point = %d, %v; want visible symbol", objSym, ok)
	}

	projector := New(Config{
		Graph:    g,
		Bindings: bindings,
		Inputs: &flow.Inputs{
			ConstValues: map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue{
				keySym: {
					returnPoint: {Kind: flow.ConstString, Str: "name"},
				},
			},
		},
	})

	got := projector.pathOfExpr(read, returnPoint)
	if got.Symbol != objSym || len(got.Segments) != 1 ||
		got.Segments[0].Kind != constraint.SegmentIndexString || got.Segments[0].Name != "name" {
		t.Fatalf("pathOfExpr with input const facts = %#v, want obj[\"name\"]", got)
	}
}

func TestProjectorDeclaredPathProofRequiresAnnotation(t *testing.T) {
	ident := &ast.IdentExpr{Value: "value"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 15
	bindings.Bind(ident, sym)

	unannotated := New(Config{
		Bindings: bindings,
		Facts:    factsStub{sym: typ.String},
	})
	if got := unannotated.declaredPathProofType(1, ident); got != nil {
		t.Fatalf("unannotated declaredPathProofType = %v, want nil", got)
	}

	annotated := New(Config{
		Bindings: bindings,
		Facts: annotatedFactsStub{
			factsStub: factsStub{sym: typ.String},
			annotated: map[cfg.SymbolID]bool{
				sym: true,
			},
		},
	})
	if got := annotated.declaredPathProofType(1, ident); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("annotated declaredPathProofType = %v, want string", got)
	}
}

func TestProjector_ExpectedAnyCoercionDoesNotUseGradualEvidenceAsProof(t *testing.T) {
	ident := &ast.IdentExpr{Value: "value"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 12
	bindings.Bind(ident, sym)

	strict := New(Config{
		Bindings: bindings,
		Facts: productFactsStub{
			factsStub: factsStub{sym: typ.Any},
			values:    map[cfg.SymbolID]product.AbstractValue{sym: product.FromType(typ.Any)},
		},
	}).TypeOfWithExpected(ident, 1, typ.String)
	if !typ.TypeEquals(strict, typ.Any) {
		t.Fatalf("strict any with expected string = %v, want any", strict)
	}

	gradual := New(Config{
		Bindings: bindings,
		Facts: productFactsStub{
			factsStub: factsStub{sym: typ.Any},
			values:    map[cfg.SymbolID]product.AbstractValue{sym: product.GradualAny()},
		},
	}).TypeOfWithExpected(ident, 1, typ.String)
	if !typ.TypeEquals(gradual, typ.Any) {
		t.Fatalf("gradual any with expected string = %v, want any", gradual)
	}
}

func TestProjector_ProductGradualEvidenceDoesNotOverrideStrictAnyPolicy(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"value"}}}
	g := cfg.Build(fn)
	syms := g.ParamSymbols()
	if len(syms) != 1 || syms[0] == 0 {
		t.Fatalf("ParamSymbols() = %v, want one non-zero symbol", syms)
	}
	ident := &ast.IdentExpr{Value: "value"}
	bindings := bind.NewBindingTable()
	bindings.Bind(ident, syms[0])

	compatFallback := New(Config{
		Graph:    g,
		Bindings: bindings,
		Facts:    factsStub{syms[0]: typ.Any},
	}).TypeOfWithExpected(ident, g.Entry(), typ.String)
	if !typ.TypeEquals(compatFallback, typ.Any) {
		t.Fatalf("unannotated fallback any with expected string = %v, want any", compatFallback)
	}

	strictProduct := New(Config{
		Graph:    g,
		Bindings: bindings,
		Facts: productFactsStub{
			factsStub: factsStub{syms[0]: typ.Any},
			values:    map[cfg.SymbolID]product.AbstractValue{syms[0]: product.FromType(typ.Any)},
		},
	}).TypeOfWithExpected(ident, g.Entry(), typ.String)
	if !typ.TypeEquals(strictProduct, typ.Any) {
		t.Fatalf("strict product any with expected string = %v, want any", strictProduct)
	}
}

func TestProjector_GlobalIdentUsesSymbolFactsBeforeNameMap(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{
		&ast.ReturnStmt{Exprs: []ast.Expr{&ast.IdentExpr{Value: "print"}}},
	}}
	g := cfg.Build(fn, "print")
	sym, ok := g.GlobalSymbol("print")
	if !ok || sym == 0 {
		t.Fatal("print global symbol not found")
	}
	ident := &ast.IdentExpr{Value: "print"}
	bindings := bind.NewBindingTable()
	bindings.Bind(ident, sym)
	bindings.SetKind(sym, cfg.SymbolGlobal)

	observed := New(Config{
		Graph:    g,
		Bindings: bindings,
		Facts:    factsStub{sym: typ.String},
	}).TypeOf(ident, g.Entry())

	if !typ.TypeEquals(observed, typ.String) {
		t.Fatalf("TypeOf(global ident) = %v, want string", observed)
	}
}

func TestProjector_SelectFromVariadicUsesPointScopeAndGlobalEffect(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "select"},
				Args: []ast.Expr{
					&ast.NumberExpr{Value: "1"},
					&ast.Comma3Expr{},
				},
			},
		}},
	}}
	g := cfg.Build(fn, "select")
	tParam := typ.NewTypeParam("T", nil)
	selectFn := typ.Func().
		Param("index", typ.Integer).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	call := fn.Stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FuncCallExpr)

	nonEntryPoint := cfg.Point(999)
	if nonEntryPoint == g.Entry() {
		t.Fatal("test setup expected a non-entry point")
	}
	observed := New(Config{
		Graph:             g,
		Scopes:            map[cfg.Point]*scope.State{g.Entry(): scope.New().WithVariadic(tParam)},
		Facts:             factsStub{},
		GlobalTypeOverlay: globalenv.TypeOverlayFromMap(map[string]typ.Type{"select": selectFn}),
	}).TypeOf(call, nonEntryPoint)

	if observed != tParam {
		t.Fatalf("TypeOf(select(1, ...)) = %#v (%v), want variadic type param", observed, observed)
	}
}

func TestProjector_NumberLiteralUsesCanonicalPrecision(t *testing.T) {
	observed := New(Config{}).TypeOf(&ast.NumberExpr{Value: "42"}, 1)
	lit, ok := observed.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		t.Fatalf("TypeOf(integer literal) = %T %[1]v, want integer literal", observed)
	}
}

func TestProjector_ExpectedNumberLiteralUsesContext(t *testing.T) {
	observed := New(Config{}).TypeOfWithExpected(&ast.NumberExpr{Value: "42"}, 1, typ.Integer)
	if !typ.TypeEquals(observed, typ.Integer) {
		t.Fatalf("TypeOfWithExpected(integer literal) = %v, want integer", observed)
	}
}

func TestProjector_DynamicIndexIdentifierDoesNotBecomeFieldName(t *testing.T) {
	objExpr := &ast.IdentExpr{Value: "obj"}
	keyExpr := &ast.IdentExpr{Value: "name"}
	indexExpr := &ast.AttrGetExpr{Object: objExpr, Key: keyExpr}
	bindings := bind.NewBindingTable()
	const objSym cfg.SymbolID = 20
	const keySym cfg.SymbolID = 21
	bindings.Bind(objExpr, objSym)
	bindings.Bind(keyExpr, keySym)

	objType := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()
	observed := New(Config{
		Bindings: bindings,
		Facts: factsStub{
			objSym: objType,
			keySym: typ.LiteralString("suite"),
		},
	}).TypeOf(indexExpr, 1)
	want := typ.NewOptional(typ.Number)

	if !typ.TypeEquals(observed, want) {
		t.Fatalf("TypeOf(obj[name]) = %v, want %v", observed, want)
	}
}

func TestProjector_CastExprUsesConfiguredTypeResolver(t *testing.T) {
	cast := &ast.CastExpr{
		Expr: &ast.TableExpr{Fields: []*ast.Field{
			{Value: &ast.StringExpr{Value: "admin"}},
		}},
		Type: &ast.ArrayTypeExpr{Element: &ast.PrimitiveTypeExpr{Name: "string"}},
	}
	expected := typ.NewArray(typ.String)

	observed := New(Config{
		ResolveType: func(expr ast.TypeExpr, _ *scope.State) typ.Type {
			if _, ok := expr.(*ast.ArrayTypeExpr); ok {
				return expected
			}
			return nil
		},
	}).TypeOf(cast, 1)

	if !typ.TypeEquals(observed, expected) {
		t.Fatalf("TypeOf(cast) = %v, want %v", observed, expected)
	}
}

func TestProjector_OperatorsUseCanonicalQueryAlgebra(t *testing.T) {
	p := New(Config{})
	intAdd := p.TypeOf(&ast.ArithmeticOpExpr{
		Operator: "+",
		Lhs:      &ast.NumberExpr{Value: "1"},
		Rhs:      &ast.NumberExpr{Value: "2"},
	}, 1)
	if !typ.TypeEquals(intAdd, typ.Integer) {
		t.Fatalf("integer addition = %v, want integer", intAdd)
	}

	neg := p.TypeOf(&ast.UnaryMinusOpExpr{Expr: &ast.NumberExpr{Value: "1"}}, 1)
	if !typ.TypeEquals(neg, typ.Integer) {
		t.Fatalf("unary minus integer = %v, want integer", neg)
	}

	bnot := p.TypeOf(&ast.UnaryBNotOpExpr{Expr: &ast.NumberExpr{Value: "1"}}, 1)
	if !typ.TypeEquals(bnot, typ.Integer) {
		t.Fatalf("bitwise not integer = %v, want integer", bnot)
	}
}

func TestProjector_TableWithExpectedMapKeepsMapProduct(t *testing.T) {
	expected := typ.NewMap(typ.String, typ.Any)
	observed := New(Config{}).TypeOfWithExpected(&ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.IdentExpr{Value: "query"}, Value: &ast.StringExpr{Value: "term"}},
	}}, 1, expected)

	if !typ.TypeEquals(observed, expected) {
		t.Fatalf("TypeOfWithExpected(map literal) = %v, want %v", observed, expected)
	}
}

func TestProjector_ArrayElementUsesDiscriminatedUnionContext(t *testing.T) {
	content := typ.NewRecord().
		Field("type", typ.LiteralString("content")).
		Field("data", typ.String).
		Build()
	toolCall := typ.NewRecord().
		Field("type", typ.LiteralString("tool_call")).
		Field("id", typ.String).
		Field("name", typ.String).
		Field("arguments", typ.NewMap(typ.String, typ.Any)).
		Build()
	expected := typ.NewArray(typ.NewUnion(content, toolCall))

	observed := New(Config{}).TypeOfWithExpected(&ast.TableExpr{Fields: []*ast.Field{
		{Value: &ast.TableExpr{Fields: []*ast.Field{
			{Key: &ast.IdentExpr{Value: "type"}, Value: &ast.StringExpr{Value: "content"}},
			{Key: &ast.IdentExpr{Value: "data"}, Value: &ast.StringExpr{Value: "hello"}},
		}}},
		{Value: &ast.TableExpr{Fields: []*ast.Field{
			{Key: &ast.IdentExpr{Value: "type"}, Value: &ast.StringExpr{Value: "tool_call"}},
			{Key: &ast.IdentExpr{Value: "id"}, Value: &ast.StringExpr{Value: "t1"}},
			{Key: &ast.IdentExpr{Value: "name"}, Value: &ast.StringExpr{Value: "search"}},
			{Key: &ast.IdentExpr{Value: "arguments"}, Value: &ast.TableExpr{Fields: []*ast.Field{
				{Key: &ast.IdentExpr{Value: "query"}, Value: &ast.StringExpr{Value: "term"}},
			}}},
		}}},
	}}, 1, expected)

	if !typ.TypeEquals(observed, expected) {
		t.Fatalf("TypeOfWithExpected(union array literal) = %v, want %v", observed, expected)
	}
}

func TestProjector_TableUnionContextChecksMembersBeforeUnionFieldContext(t *testing.T) {
	chanInt := typ.NewRecord().Field("__tag", typ.LiteralString("int")).Build()
	chanStr := typ.NewRecord().Field("__tag", typ.LiteralString("str")).Build()
	intResult := typ.NewRecord().
		Field("channel", chanInt).
		Field("value", typ.Number).
		Field("ok", typ.Boolean).
		Build()
	strResult := typ.NewRecord().
		Field("channel", chanStr).
		Field("value", typ.String).
		Field("ok", typ.Boolean).
		Build()
	expected := typ.NewUnion(intResult, strResult)

	channel := &ast.IdentExpr{Value: "a"}
	bindings := bind.NewBindingTable()
	const channelSym cfg.SymbolID = 91
	bindings.Bind(channel, channelSym)

	observed := New(Config{
		Bindings: bindings,
		Facts:    factsStub{channelSym: chanInt},
	}).ReturnSourceType(&ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.IdentExpr{Value: "channel"}, Value: channel},
		{Key: &ast.IdentExpr{Value: "value"}, Value: &ast.NumberExpr{Value: "42"}},
		{Key: &ast.IdentExpr{Value: "ok"}, Value: &ast.TrueExpr{}},
	}}, 1, expected)

	if !typ.TypeEquals(observed, intResult) {
		t.Fatalf("ReturnSourceType(table union) = %v, want %v", observed, intResult)
	}
}

func TestProjector_FunctionLiteralUsesActualBeforeExpected(t *testing.T) {
	fn := &ast.FunctionExpr{}
	actual := typ.Func().Param("n", typ.Number).Returns(typ.Number).Build()
	expected := typ.Func().Param("s", typ.String).Returns(typ.String).Build()

	observed := New(Config{
		LiteralSignatureProvider: api.LiteralSigsLookup{fn: actual},
	}).TypeOfWithExpected(fn, 1, expected)

	if !typ.TypeEquals(observed, actual) {
		t.Fatalf("function literal observation = %v, want actual %v", observed, actual)
	}
}

func TestProjector_CallReturnsFunctionSignatureWithoutCallInference(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 11
	bindings.Bind(callee, sym)
	call := &ast.FuncCallExpr{Func: callee}

	observed := New(Config{
		Bindings: bindings,
		FunctionType: func(candidate cfg.SymbolID) typ.Type {
			if candidate == sym {
				return typ.Func().Returns(typ.String, typ.Integer).Build()
			}
			return nil
		},
	}).MultiTypeOf(call, 1)

	if len(observed) != 2 || !typ.TypeEquals(observed[0], typ.String) || !typ.TypeEquals(observed[1], typ.Integer) {
		t.Fatalf("MultiTypeOf(call) = %v, want string, integer", observed)
	}
}

func TestProjector_MethodReturnEffectsUseRuntimeReceiverSlot(t *testing.T) {
	receiverExpr := &ast.IdentExpr{Value: "context_query"}
	call := &ast.FuncCallExpr{
		Receiver: receiverExpr,
		Method:   "type",
		Args:     []ast.Expr{&ast.StringExpr{Value: "conversation_summary"}},
	}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 14
	bindings.Bind(receiverExpr, sym)

	method := typ.Func().
		Param("self", typ.Unknown).
		Param("kind", typ.String).
		Returns(typ.Unknown).
		Effects(effect.Row{Labels: []effect.Label{
			effect.FlowInto{ParamIndex: 0, ReturnIndex: 0},
		}}).
		Build()
	receiver := typ.NewRecord().
		Field("id", typ.String).
		Field("type", method).
		Build()

	observed := New(Config{
		Bindings: bindings,
		Facts:    factsStub{sym: receiver},
		Ctx:      db.NewQueryContext(db.New()),
		TypeOps:  querycore.NewEngine(),
	}).MultiTypeOf(call, 1)

	if len(observed) != 1 {
		t.Fatalf("MultiTypeOf(method call) = %v, want one return", observed)
	}
	rec, ok := observed[0].(*typ.Record)
	if !ok {
		t.Fatalf("method return = %T %v, want receiver record", observed[0], observed[0])
	}
	if field := rec.GetField("id"); field == nil || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("receiver id field = %#v, want string", field)
	}
}

func TestProjector_SetMetatableCallReturnsMetatabledRecord(t *testing.T) {
	closeFn := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "setmetatable"},
		Args: []ast.Expr{
			&ast.TableExpr{Fields: []*ast.Field{
				{Key: &ast.IdentExpr{Value: "id"}, Value: &ast.StringExpr{Value: "r1"}},
			}},
			&ast.TableExpr{Fields: []*ast.Field{
				{Key: &ast.IdentExpr{Value: "__index"}, Value: &ast.TableExpr{Fields: []*ast.Field{
					{Key: &ast.IdentExpr{Value: "close"}, Value: closeFn},
				}}},
			}},
		},
	}

	observed := New(Config{}).MultiTypeOf(call, 1)
	if len(observed) != 1 {
		t.Fatalf("MultiTypeOf(setmetatable) = %v, want one return", observed)
	}
	if _, ok := querycore.Method(observed[0], "close"); !ok {
		t.Fatalf("expected metatable method on observed return, got %s", typ.FormatShort(observed[0]))
	}
}

func TestProjector_EmptyTableUsesFreshBottom(t *testing.T) {
	observed := New(Config{}).TypeOf(&ast.TableExpr{}, 1)
	rec, ok := observed.(*typ.Record)
	if !ok {
		t.Fatalf("TypeOf(empty table) = %T %[1]v, want record", observed)
	}
	if rec.Open || len(rec.Fields) != 0 || rec.HasMapComponent() || rec.Metatable != nil {
		t.Fatalf("empty table observation should be closed fresh bottom, got %v", rec)
	}
}

func TestProjector_MissingRecordFieldReadsNilForLogicalDefault(t *testing.T) {
	const entrySym cfg.SymbolID = 42
	bindings := bind.NewBindingTable()
	entryForData := &ast.IdentExpr{Value: "entry"}
	entryForMax := &ast.IdentExpr{Value: "entry"}
	bindings.Bind(entryForData, entrySym)
	bindings.Bind(entryForMax, entrySym)

	dataForCondition := &ast.AttrGetExpr{
		Object: entryForData,
		Key:    &ast.StringExpr{Value: "data"},
	}
	dataForValue := &ast.AttrGetExpr{
		Object: entryForMax,
		Key:    &ast.StringExpr{Value: "data"},
	}
	value := &ast.AttrGetExpr{
		Object: dataForValue,
		Key:    &ast.StringExpr{Value: "max_tokens"},
	}
	expr := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs: &ast.LogicalOpExpr{
			Operator: "and",
			Lhs:      dataForCondition,
			Rhs:      value,
		},
		Rhs: &ast.NumberExpr{Value: "0"},
	}

	entryType := typ.NewRecord().
		Field("data", typ.NewRecord().Build()).
		SetOpen(true).
		Build()
	got := New(Config{
		Bindings: bindings,
		Facts:    factsStub{entrySym: entryType},
	}).TypeOf(expr, 1)

	if !typ.TypeEquals(got, typ.LiteralInt(0)) {
		t.Fatalf("TypeOf(logical default) = %v, want 0", got)
	}
}

func TestFromFuncResultUsesModuleBindingsAndFunctionProjection(t *testing.T) {
	callee := &ast.IdentExpr{Value: "f"}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 12
	bindings.Bind(callee, sym)
	call := &ast.FuncCallExpr{Func: callee}

	observed := FromFuncResult(&api.FuncResult{
		ModuleBindings: bindings,
	}, func(candidate cfg.SymbolID) typ.Type {
		if candidate == sym {
			return typ.Func().Returns(typ.Boolean).Build()
		}
		return nil
	}).MultiTypeOf(call, 1)

	if len(observed) != 1 || !typ.TypeEquals(observed[0], typ.Boolean) {
		t.Fatalf("MultiTypeOf(call) = %v, want boolean", observed)
	}
}

func TestProjector_FunctionLiteralUsesCanonicalProjection(t *testing.T) {
	fn := &ast.FunctionExpr{}
	bindings := bind.NewBindingTable()
	const sym cfg.SymbolID = 13
	bindings.SetFuncLitSymbol(fn, sym)
	staleLiteral := typ.Func().Build()
	want := typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()

	observed := New(Config{
		Bindings: bindings,
		LiteralSignatureProvider: api.LiteralSigsLookup{
			fn: staleLiteral,
		},
		FunctionType: func(candidate cfg.SymbolID) typ.Type {
			if candidate == sym {
				return want
			}
			return nil
		},
	}).TypeOf(fn, 1)

	if !typ.TypeEquals(observed, want) {
		t.Fatalf("TypeOf(function literal) = %v, want canonical projection %v", observed, want)
	}
}

func TestProjector_TableUsesDiscriminatedExpectedMember(t *testing.T) {
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.IdentExpr{Value: "kind"}, Value: &ast.StringExpr{Value: "ok"}},
		{Key: &ast.IdentExpr{Value: "value"}, Value: &ast.StringExpr{Value: "payload"}},
	}}
	expected := typ.NewUnion(
		typ.NewRecord().
			Field("kind", typ.LiteralString("ok")).
			Field("value", typ.String).
			Build(),
		typ.NewRecord().
			Field("kind", typ.LiteralString("err")).
			Field("message", typ.String).
			Build(),
	)

	observed := New(Config{}).TypeOfWithExpected(table, 1, expected)
	rec, ok := observed.(*typ.Record)
	if !ok {
		t.Fatalf("observed table = %T %v, want record", observed, observed)
	}
	if field := rec.GetField("kind"); field == nil || !typ.TypeEquals(field.Type, typ.LiteralString("ok")) {
		t.Fatalf("kind field = %#v, want literal ok", field)
	}
	if field := rec.GetField("value"); field == nil || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("value field = %#v, want string", field)
	}
}
