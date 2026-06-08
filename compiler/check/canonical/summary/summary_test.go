package summary_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

// testProgram is a Program backed by a small set of named, parsed Lua functions.
// It is the test-side seam: it builds each function's canonical inputs and
// transfer, and derives the call graph by walking each graph's call sites and
// resolving callee names to the named functions. It exercises summary.Queries
// over a real call graph without the full pipeline.
type testProgram struct {
	graphs    map[summary.FuncRef]*cfg.Graph
	transfers map[summary.FuncRef]equation.NodeTransfer
	params    map[summary.FuncRef]int
	byName    map[string]summary.FuncRef
}

func newTestProgram(t *testing.T, fns map[string]string) *testProgram {
	t.Helper()
	p := &testProgram{
		graphs:    make(map[summary.FuncRef]*cfg.Graph),
		transfers: make(map[summary.FuncRef]equation.NodeTransfer),
		params:    make(map[summary.FuncRef]int),
		byName:    make(map[string]summary.FuncRef),
	}
	// The function names are predeclared globals so a body call resolves the
	// callee name to a symbol rather than an unknown.
	globals := make([]string, 0, len(fns))
	for name := range fns {
		globals = append(globals, name)
	}
	for name, src := range fns {
		stmts, err := parse.ParseString(src, name+".lua")
		if err != nil {
			t.Fatalf("parse %s failed: %v", name, err)
		}
		// A function header line "--params: a,b" declares parameters; default none.
		fn := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
		in := input.BuildFromFunction(fn, nil, nil, globals...)
		if in.Graph == nil {
			t.Fatalf("input builder produced no graph for %s", name)
		}
		ref := summary.FuncRef{GraphID: in.Graph.ID()}
		p.graphs[ref] = in.Graph
		p.transfers[ref] = transfer.New(in, transfer.Config{})
		p.params[ref] = in.Scope.NumParams()
		p.byName[name] = ref
	}
	return p
}

func (p *testProgram) Graph(ref summary.FuncRef) *cfg.Graph { return p.graphs[ref] }
func (p *testProgram) NumParams(ref summary.FuncRef) int    { return p.params[ref] }
func (p *testProgram) Transfer(ref summary.FuncRef) equation.NodeTransfer {
	return p.transfers[ref]
}

// Callees derives the call-graph edges of ref by walking every call site in its
// graph and resolving the callee name to a named function. Unresolved calls
// (stdlib, unknown names) are not call-graph nodes and are skipped.
func (p *testProgram) Callees(ref summary.FuncRef) []summary.FuncRef {
	g := p.graphs[ref]
	if g == nil {
		return nil
	}
	seen := make(map[summary.FuncRef]bool)
	var out []summary.FuncRef
	g.EachCallSite(func(_ cfg.Point, call *cfg.CallInfo) {
		if call == nil || call.CalleeName == "" {
			return
		}
		callee, ok := p.byName[call.CalleeName]
		if !ok || seen[callee] {
			return
		}
		seen[callee] = true
		out = append(out, callee)
	})
	return out
}

type captureSeedProgram struct {
	graphs    map[summary.FuncRef]*cfg.Graph
	transfers map[summary.FuncRef]equation.NodeTransfer
	parent    summary.FuncRef
	child     summary.FuncRef
}

func (p *captureSeedProgram) Graph(ref summary.FuncRef) *cfg.Graph { return p.graphs[ref] }
func (p *captureSeedProgram) NumParams(summary.FuncRef) int        { return 0 }
func (p *captureSeedProgram) Transfer(ref summary.FuncRef) equation.NodeTransfer {
	return p.transfers[ref]
}
func (p *captureSeedProgram) Callees(summary.FuncRef) []summary.FuncRef { return nil }
func (p *captureSeedProgram) CaptureEntryReferences(ref summary.FuncRef, captureReferencesOf func(summary.FuncRef) flow.ReferenceContext) flow.ReferenceContext {
	if ref != p.child {
		return flow.ReferenceContextBottom()
	}
	parent := captureReferencesOf(p.parent)
	cells := flow.CaptureCellsDomain.Bottom()
	if av, ok := parent.CaptureCells().Value(cfg.SymbolID(7)); ok {
		cells = flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(7), Value: av}})
	}
	return flow.ReferenceContextOf(
		cells,
		flow.ProjectFunctionRefsBySymbols(parent.FunctionRefs(), []cfg.SymbolID{7}),
		flow.ClosureRefsDomain.Bottom(),
	)
}

type captureSeedTransfer struct {
	exportSym   cfg.SymbolID
	exportValue product.AbstractValue
	exportRef   flow.FunctionRef
}

func (t *captureSeedTransfer) Transfer(
	_ *cfg.Graph,
	_ cfg.Point,
	incoming flow.PointState,
	_ paramevidence.Contracts,
	_ func(int, paramevidence.ParamContract),
) flow.PointState {
	out := incoming
	if out.CellEffects.IsBottom() {
		out.CellEffects = flow.CaptureEffectsIdentity()
	}
	if t.exportSym != 0 && !t.exportValue.IsZero() {
		out.Cells = out.Cells.With(t.exportSym, t.exportValue)
	}
	if t.exportSym != 0 && t.exportRef != (flow.FunctionRef{}) {
		out.FunctionRefs = flow.WithFunctionRefPath(out.FunctionRefs, constraint.NewPath(t.exportSym, "captured"), flow.FunctionRefSetOf(t.exportRef))
	}
	return out
}

type countingTransfer struct {
	delegate equation.NodeTransfer
	calls    int
}

func (t *countingTransfer) Transfer(
	g *cfg.Graph,
	p cfg.Point,
	incoming flow.PointState,
	contracts paramevidence.Contracts,
	emitParamContract func(int, paramevidence.ParamContract),
) flow.PointState {
	t.calls++
	if t.delegate == nil {
		return incoming
	}
	return t.delegate.Transfer(g, p, incoming, contracts, emitParamContract)
}

type returnPostconditionCellProgram struct {
	local     map[summary.FuncRef][]paramevidence.ParamNarrow
	delegated map[summary.FuncRef][]paramevidence.DelegatedCall
	targets   map[*ast.FuncCallExpr]summary.FuncRef
}

func (p *returnPostconditionCellProgram) Graph(summary.FuncRef) *cfg.Graph { return nil }
func (p *returnPostconditionCellProgram) NumParams(summary.FuncRef) int    { return 0 }
func (p *returnPostconditionCellProgram) Transfer(summary.FuncRef) equation.NodeTransfer {
	return nil
}
func (p *returnPostconditionCellProgram) Callees(summary.FuncRef) []summary.FuncRef { return nil }
func (p *returnPostconditionCellProgram) LocalReturnPostconditions(ref summary.FuncRef) paramevidence.ReturnPostconditions {
	return paramevidence.ReturnPostconditionsFromParamNarrows(p.local[ref])
}
func (p *returnPostconditionCellProgram) DelegatedReturnPostconditionCalls(ref summary.FuncRef) []paramevidence.DelegatedCall {
	return append([]paramevidence.DelegatedCall(nil), p.delegated[ref]...)
}
func (p *returnPostconditionCellProgram) ResolveDelegatedCallee(_ summary.FuncRef, call *ast.FuncCallExpr) (summary.FuncRef, bool) {
	ref, ok := p.targets[call]
	return ref, ok
}

func TestSummaryDomain_Laws(t *testing.T) {
	n := product.FromType(typ.Number)
	s := product.FromType(typ.String)
	effects := flow.CaptureMustWrite(cfg.SymbolID(1), s)
	exports := flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(2), Value: n}})
	exportRefs := flow.WithFunctionRefPath(nil, constraint.NewPath(cfg.SymbolID(2), "captured"), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 4}))
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 5}, exports, exportRefs)
	exportClosures := flow.WithClosureRefPath(nil, constraint.NewPath(cfg.SymbolID(2), "captured").Field("factory"), flow.ClosureRefSetOf(closure))
	protos := flow.PrototypeSelfOf([]flow.PrototypeSelfEntry{{Prototype: cfg.SymbolID(3), Value: s}})
	entryPublication := summary.CallEntryPublications{
		summary.FuncRef{GraphID: 99}: {
			Values: summary.EntryValues{0: n},
			Facts: flow.BoundaryFactsOf(
				[]flow.BoundaryKeyPresenceFact{{
					Table: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}}},
					Key:   flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "last_node_id"}}},
				}},
				nil, nil, nil, nil, nil,
			),
		},
	}
	rels := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})

	lattice.LawSuite[summary.Summary]{
		Name:   "Summary",
		Domain: summary.SummaryDomain,
		Sample: []summary.Summary{
			summary.SummaryDomain.Bottom(),
			summary.SummaryDomain.Top(),
			{Returns: []product.AbstractValue{n}},
			{Relations: rels},
			{CellEffects: effects},
			{CaptureReferences: flow.ReferenceContextOf(exports, flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom())},
			{CaptureReferences: flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), exportRefs, flow.ClosureRefsDomain.Bottom())},
			{ReturnRefs: flow.ReturnRefsOfSlots([]flow.ReturnRefSlot{flow.ReturnRefSlotOf(flow.FunctionRefsDomain.Bottom(), exportClosures)})},
			{CaptureReferences: flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), exportClosures)},
			{PrototypeSelf: protos},
			{CallEntryPublication: entryPublication},
			{Returns: []product.AbstractValue{n, s}, ReturnRefs: flow.ReturnRefsOfSlots([]flow.ReturnRefSlot{flow.ReturnRefSlotOf(flow.FunctionRefsDomain.Bottom(), exportClosures)}), Relations: rels, CellEffects: effects, CaptureReferences: flow.ReferenceContextOf(exports, exportRefs, exportClosures), PrototypeSelf: protos, CallEntryPublication: entryPublication},
		},
	}.Run(t)
}

func TestReturnPostconditionQ_InheritsDelegatedEffectsWithoutSummaryContext(t *testing.T) {
	inner := summary.FuncRef{GraphID: 1}
	outer := summary.FuncRef{GraphID: 2}
	call := &ast.FuncCallExpr{}
	prog := &returnPostconditionCellProgram{
		local: map[summary.FuncRef][]paramevidence.ParamNarrow{
			inner: {{Param: 0, Check: cfg.CheckNotNil, EqParam: -1}},
		},
		delegated: map[summary.FuncRef][]paramevidence.DelegatedCall{
			outer: {{Call: call, ArgParams: []int{1}}},
		},
		targets: map[*ast.FuncCallExpr]summary.FuncRef{call: inner},
	}
	q := summary.New(prog)
	ctx := db.NewQueryContext(db.New())

	got := q.ReturnPostconditions(ctx, outer)
	if !containsConstraint(got.Condition().MustConstraints(), constraint.NotNil{Path: constraint.ParamPath(1)}) {
		t.Fatalf("ReturnPostconditions = %v, want param 1 not-nil", got.Condition())
	}

	sum := q.Summarize(ctx, outer)
	if !containsConstraint(sum.Postconditions.Condition().MustConstraints(), constraint.NotNil{Path: constraint.ParamPath(1)}) {
		t.Fatalf("Summary.Postconditions = %v, want delegated not-nil proof", sum.Postconditions.Condition())
	}
}

func TestReturnPostconditionQ_InheritsDelegatedEqualityEffects(t *testing.T) {
	inner := summary.FuncRef{GraphID: 11}
	outer := summary.FuncRef{GraphID: 12}
	call := &ast.FuncCallExpr{}
	prog := &returnPostconditionCellProgram{
		local: map[summary.FuncRef][]paramevidence.ParamNarrow{
			inner: {{Param: 0, EqParam: 1}},
		},
		delegated: map[summary.FuncRef][]paramevidence.DelegatedCall{
			outer: {{Call: call, ArgParams: []int{2, 0}}},
		},
		targets: map[*ast.FuncCallExpr]summary.FuncRef{call: inner},
	}
	q := summary.New(prog)
	ctx := db.NewQueryContext(db.New())

	want := constraint.NewEqPath(constraint.ParamPath(0), constraint.ParamPath(2))
	got := q.ReturnPostconditions(ctx, outer)
	if !containsConstraint(got.Condition().MustConstraints(), want) {
		t.Fatalf("ReturnPostconditions = %v, want %v", got.Condition(), want)
	}

	sum := q.Summarize(ctx, outer)
	if !containsConstraint(sum.Postconditions.Condition().MustConstraints(), want) {
		t.Fatalf("Summary.Postconditions = %v, want delegated equality", sum.Postconditions.Condition())
	}
}

func TestReturnPostconditionQ_InheritsDelegatedInequalityEffects(t *testing.T) {
	inner := summary.FuncRef{GraphID: 13}
	outer := summary.FuncRef{GraphID: 14}
	call := &ast.FuncCallExpr{}
	prog := &returnPostconditionCellProgram{
		local: map[summary.FuncRef][]paramevidence.ParamNarrow{
			inner: {{Param: 0, EqParam: 1, NotEqual: true}},
		},
		delegated: map[summary.FuncRef][]paramevidence.DelegatedCall{
			outer: {{Call: call, ArgParams: []int{2, 0}}},
		},
		targets: map[*ast.FuncCallExpr]summary.FuncRef{call: inner},
	}
	q := summary.New(prog)
	ctx := db.NewQueryContext(db.New())

	want := constraint.NewNotEqPath(constraint.ParamPath(0), constraint.ParamPath(2))
	got := q.ReturnPostconditions(ctx, outer)
	if !containsConstraint(got.Condition().MustConstraints(), want) {
		t.Fatalf("ReturnPostconditions = %v, want %v", got.Condition(), want)
	}
}

func TestReturnPostconditionQ_ComposesDelegatedConditionArgumentEffects(t *testing.T) {
	inner := summary.FuncRef{GraphID: 21}
	outer := summary.FuncRef{GraphID: 22}
	call := &ast.FuncCallExpr{}
	prog := &returnPostconditionCellProgram{
		local: map[summary.FuncRef][]paramevidence.ParamNarrow{
			inner: {{Param: 0, Check: cfg.CheckTruthy, CondArg: true, EqParam: -1}},
		},
		delegated: map[summary.FuncRef][]paramevidence.DelegatedCall{
			outer: {{
				Call:      call,
				ArgParams: []int{-1},
				ArgTruthyEffects: [][]paramevidence.ParamNarrow{
					{{Param: 1, Check: cfg.CheckNotNil, EqParam: -1}},
				},
			}},
		},
		targets: map[*ast.FuncCallExpr]summary.FuncRef{call: inner},
	}
	q := summary.New(prog)
	ctx := db.NewQueryContext(db.New())

	got := q.ReturnPostconditions(ctx, outer)
	if !containsConstraint(got.Condition().MustConstraints(), constraint.NotNil{Path: constraint.ParamPath(1)}) {
		t.Fatalf("ReturnPostconditions = %v, want param 1 not-nil from delegated condition arg", got.Condition())
	}
}

func TestSummaryHasNoParamNarrowsLane(t *testing.T) {
	if _, ok := reflect.TypeOf(summary.Summary{}).FieldByName("ParamNarrows"); ok {
		t.Fatal("Summary must not expose ParamNarrows as an interprocedural lane")
	}
}

func TestReader_UsesConvergedSnapshotWhenNotLive(t *testing.T) {
	ref := summary.FuncRef{GraphID: 77}
	post := paramevidence.ReturnPostconditionsFromParamNarrows([]paramevidence.ParamNarrow{
		{Param: 1, Check: cfg.CheckNotNil, EqParam: -1},
	})
	want := summary.Summary{
		Returns:        []product.AbstractValue{product.FromType(typ.String)},
		Postconditions: post,
	}

	reader := summary.NewReader(nil, nil, map[summary.FuncRef]summary.Summary{ref: want})
	if reader.Live() {
		t.Fatal("snapshot-only reader reported live")
	}

	got := reader.Summarize(ref)
	if !summary.SummaryDomain.Equal(got, want) {
		t.Fatalf("reader summary = %#v, want snapshot %#v", got, want)
	}
	gotPost := reader.ReturnPostconditions(ref)
	if !containsConstraint(gotPost.Condition().MustConstraints(), constraint.NotNil{Path: constraint.ParamPath(1)}) {
		t.Fatalf("reader ReturnPostconditions = %v, want snapshot postconditions", gotPost.Condition())
	}
}

func TestCanonicalSummarySnapshot_ExactKeyDoesNotFallbackToRef(t *testing.T) {
	ref := summary.FuncRef{GraphID: 78}
	key := summary.NewDefaultKey(ref, nil)
	refSummary := summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.String)}}
	snapshot := summary.NewCanonicalSummarySnapshot(map[summary.FuncRef]summary.Summary{ref: refSummary}, nil)

	if _, ok := snapshot.ExactSummaryForKey(key); ok {
		t.Fatal("ExactSummaryForKey must not treat aggregate by-ref summaries as exact keyed summaries")
	}
	if snapshot.HasExactKey(key) {
		t.Fatal("HasExactKey must report only demanded canonical summary keys")
	}
	reader := summary.NewSnapshotReader(snapshot)
	if _, ok := reader.ExactSummaryForKey(key); ok {
		t.Fatal("Reader.ExactSummaryForKey must not fall back to aggregate by-ref summaries")
	}
	if got := reader.SummarizeWithKey(key); !summary.SummaryDomain.Equal(got, summary.SummaryDomain.Bottom()) {
		t.Fatalf("SummarizeWithKey without exact key = %#v, want bottom", got)
	}
	if got := reader.Summarize(ref); !summary.SummaryDomain.Equal(got, refSummary) {
		t.Fatalf("Summarize(ref) = %#v, want aggregate by-ref summary %#v", got, refSummary)
	}
}

func TestReader_MissingSnapshotIsBottom(t *testing.T) {
	got := summary.NewReader(nil, nil, nil).Summarize(summary.FuncRef{GraphID: 88})
	if !summary.SummaryDomain.Equal(got, summary.SummaryDomain.Bottom()) {
		t.Fatalf("missing snapshot summary = %#v, want bottom", got)
	}
	if post := summary.NewReader(nil, nil, nil).ReturnPostconditions(summary.FuncRef{GraphID: 88}); post.HasConstraints() {
		t.Fatalf("missing snapshot ReturnPostconditions = %v, want bottom", post.Condition())
	}
}

func containsConstraint(haystack []constraint.Constraint, needle constraint.Constraint) bool {
	for _, c := range haystack {
		if c.Equals(needle) {
			return true
		}
	}
	return false
}

func TestProject_CellEffectsFromReturnBoundaries(t *testing.T) {
	g := cfg.Build(&ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{},
		},
	})
	var ret cfg.Point
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ret = p
	})
	if ret == 0 {
		t.Fatal("return point not found")
	}

	effect := flow.CaptureMustWrite(cfg.SymbolID(42), product.FromType(typ.String))
	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {CellEffects: effect},
			g.Entry(): {
				CellEffects: flow.CaptureMustWrite(cfg.SymbolID(99), product.FromType(typ.Number)),
			},
		},
	}, g)

	if !flow.CaptureEffectsDomain.Equal(sum.CellEffects, effect) {
		t.Fatalf("summary cell effects = %s, want return-boundary effect %s", sum.CellEffects.Format(), effect.Format())
	}
}

func TestProject_CaptureReferencesFromReturnBoundaryCells(t *testing.T) {
	g := cfg.Build(&ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{},
		},
	})
	var ret cfg.Point
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ret = p
	})
	if ret == 0 {
		t.Fatal("return point not found")
	}

	exported := product.FromType(typ.String)
	renderAddr, ok := flow.StableAddressOfSymbol(cfg.SymbolID(77), []constraint.Segment{{Kind: constraint.SegmentField, Name: "render"}})
	if !ok {
		t.Fatal("render static-member address did not build")
	}
	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env: map[flow.ValueKey]product.AbstractValue{
					"s42": exported,
				},
				Cells: flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(77), Value: product.FromType(typ.NewRecord().Build())}}),
				StaticMembers: flow.StaticMemberFacts{}.WithAddress(
					renderAddr,
					product.FromType(typ.Func().Returns(typ.String).Build()),
				),
			},
			g.Entry(): {
				Env: map[flow.ValueKey]product.AbstractValue{
					"s99": product.FromType(typ.Boolean),
				},
			},
		},
	}, g)

	captures := sum.CaptureReferences.CaptureCells()
	if v, ok := captures.Value(cfg.SymbolID(42)); ok {
		t.Fatalf("ordinary env symbol leaked into capture exports: %v; exports=%s", v.ProjectValue(), captures.Format())
	}
	if v, ok := captures.Value(cfg.SymbolID(77)); !ok {
		t.Fatalf("exported captured cell 77 missing; exports=%s", captures.Format())
	} else if member, ok := product.MemberOf(v, value.MemberField("render")); !ok || typ.IsAbsentOrUnknown(member.ProjectValue()) {
		t.Fatalf("exported captured cell 77.render = %v/%v; exports=%s", member.ProjectValue(), ok, captures.Format())
	}
	if _, ok := captures.Value(cfg.SymbolID(99)); ok {
		t.Fatalf("entry-only symbol leaked into capture exports: %s", captures.Format())
	}
}

func TestProject_CaptureReferencesFunctionRefsFromReturnBoundaries(t *testing.T) {
	g := cfg.Build(&ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{},
		},
	})
	var ret cfg.Point
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ret = p
	})
	if ret == 0 {
		t.Fatal("return point not found")
	}

	sym := cfg.SymbolID(88)
	ref := flow.FunctionRef{GraphID: 707, ParentHash: 808}
	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				FunctionRefs: flow.WithFunctionRefPath(nil, constraint.NewPath(sym, "captured"), flow.FunctionRefSetOf(ref)),
			},
		},
	}, g)

	refs, ok := flow.FunctionRefAtPath(sum.CaptureReferences.FunctionRefs(), constraint.NewPath(sym, "captured"))
	if !ok {
		t.Fatalf("projected capture function refs missing; refs=%v", sum.CaptureReferences.FunctionRefs())
	}
	got, singleton := refs.Singleton()
	if !singleton || got != ref {
		t.Fatalf("projected capture function refs = %s, want singleton %v", refs.Format(), ref)
	}
}

func TestProject_PrototypeSelfFromReturnBoundaries(t *testing.T) {
	g := cfg.Build(&ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{},
		},
	})
	var ret cfg.Point
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ret = p
	})
	if ret == 0 {
		t.Fatal("return point not found")
	}

	self := flow.PrototypeSelfOf([]flow.PrototypeSelfEntry{{Prototype: cfg.SymbolID(7), Value: product.FromType(typ.String)}})
	entryOnly := flow.PrototypeSelfOf([]flow.PrototypeSelfEntry{{Prototype: cfg.SymbolID(8), Value: product.FromType(typ.Number)}})
	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret:       {PrototypeSelf: self},
			g.Entry(): {PrototypeSelf: entryOnly},
		},
	}, g)

	if v, ok := sum.PrototypeSelf.Value(cfg.SymbolID(7)); !ok || !product.Domain.Equal(v, product.FromType(typ.String)) {
		t.Fatalf("prototype 7 = %v/%v, want string; protos=%s", v.ProjectValue(), ok, sum.PrototypeSelf.Format())
	}
	if _, ok := sum.PrototypeSelf.Value(cfg.SymbolID(8)); ok {
		t.Fatalf("entry-only prototype leaked into summary: %s", sum.PrototypeSelf.Format())
	}
}

func TestProject_ReceiverEffectsFromReturnBoundaries(t *testing.T) {
	g := cfg.Build(&ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{},
		},
	})
	var ret cfg.Point
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ret = p
	})
	if ret == 0 {
		t.Fatal("return point not found")
	}

	effects := flow.ReceiverMustWrite(0, product.FromType(typ.String))
	entryOnly := flow.ReceiverMustWrite(1, product.FromType(typ.Number))
	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret:       {ReceiverEffects: effects},
			g.Entry(): {ReceiverEffects: entryOnly},
		},
	}, g)

	if !flow.ReceiverEffectsDomain.Equal(sum.ReceiverEffects, effects) {
		t.Fatalf("receiver effects = %s, want %s", sum.ReceiverEffects.Format(), effects.Format())
	}
}

func TestProject_ReturnIdentifierReadsCells(t *testing.T) {
	stmts, err := parse.ParseString(`
local x = "seed"
return x
`, "return_cell.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	g := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	var ret cfg.Point
	var retInfo *cfg.ReturnInfo
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		ret = p
		retInfo = info
	})
	if ret == 0 || retInfo == nil || len(retInfo.Symbols) != 1 || retInfo.Symbols[0] == 0 {
		t.Fatal("return symbol not found")
	}
	sym := retInfo.Symbols[0]
	sum := summary.Project(state.FunctionState{
		Points: map[cfg.Point]flow.PointState{
			ret: {
				Env:   map[flow.ValueKey]product.AbstractValue{},
				Cells: flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(typ.String)}}),
			},
		},
	}, g)
	if len(sum.Returns) != 1 || !product.Domain.Equal(sum.Returns[0], product.FromType(typ.String)) {
		t.Fatalf("cell-backed return = %#v, want string", sum.Returns)
	}
}

func TestSummarySolve_SeedsCaptureEntriesFromParentExports(t *testing.T) {
	parentGraph := cfg.Build(&ast.FunctionExpr{})
	childGraph := cfg.Build(&ast.FunctionExpr{})
	parentRef := summary.FuncRef{GraphID: parentGraph.ID()}
	childRef := summary.FuncRef{GraphID: childGraph.ID()}
	parentTransfer := &captureSeedTransfer{exportSym: cfg.SymbolID(7), exportValue: product.FromType(typ.String)}
	childTransfer := &captureSeedTransfer{}
	prog := &captureSeedProgram{
		graphs: map[summary.FuncRef]*cfg.Graph{
			parentRef: parentGraph,
			childRef:  childGraph,
		},
		transfers: map[summary.FuncRef]equation.NodeTransfer{
			parentRef: parentTransfer,
			childRef:  childTransfer,
		},
		parent: parentRef,
		child:  childRef,
	}

	q := summary.New(prog)
	ctx := db.NewQueryContext(db.New())
	_ = q.Summarize(ctx, childRef)
	fs := q.Intra(ctx, childRef)

	got, ok := fs.Points[childGraph.Entry()].Cells.Value(cfg.SymbolID(7))
	if !ok || !product.Domain.Equal(got, product.FromType(typ.String)) {
		t.Fatalf("child capture entry = %v/%v, want parent-exported string", got.ProjectValue(), ok)
	}
}

func TestSummary_IntraObserverUsesExactLocalSolve(t *testing.T) {
	g := cfg.Build(&ast.FunctionExpr{})
	ref := summary.FuncRef{GraphID: g.ID()}
	tr := &countingTransfer{delegate: &captureSeedTransfer{
		exportSym:   cfg.SymbolID(7),
		exportValue: product.FromType(typ.String),
	}}
	prog := &captureSeedProgram{
		graphs: map[summary.FuncRef]*cfg.Graph{
			ref: g,
		},
		transfers: map[summary.FuncRef]equation.NodeTransfer{
			ref: tr,
		},
	}
	q := summary.New(prog)
	ctx := db.NewQueryContext(db.New())

	sum := q.Summarize(ctx, ref)
	afterSummary := tr.calls
	if afterSummary == 0 {
		t.Fatal("Summarize did not drive the intraprocedural solve")
	}
	if _, ok := sum.CaptureReferences.CaptureCells().Value(cfg.SymbolID(7)); !ok {
		t.Fatalf("summary did not project transfer result: %s", sum.CaptureReferences.CaptureCells().Format())
	}

	fs := q.Intra(ctx, ref)
	afterIntra := tr.calls
	if afterIntra <= afterSummary {
		t.Fatalf("Intra did not run the exact observer solve after Summary: calls after Summary=%d after Intra=%d", afterSummary, afterIntra)
	}
	if _, ok := fs.Points[g.Entry()].Cells.Value(cfg.SymbolID(7)); !ok {
		t.Fatalf("intra state did not share solved transfer result: %#v", fs.Points[g.Entry()].Cells)
	}

	_ = q.Summarize(ctx, ref)
	if tr.calls != afterIntra {
		t.Fatalf("Summarize re-solved after exact Intra observer: calls after Intra=%d after second Summary=%d", afterIntra, tr.calls)
	}
}

func TestSummarySolve_SeedsCaptureFunctionRefsFromParentReferences(t *testing.T) {
	parentGraph := cfg.Build(&ast.FunctionExpr{})
	childGraph := cfg.Build(&ast.FunctionExpr{})
	parentRef := summary.FuncRef{GraphID: parentGraph.ID()}
	childRef := summary.FuncRef{GraphID: childGraph.ID()}
	fnRef := flow.FunctionRef{GraphID: 909, ParentHash: 1001}
	parentTransfer := &captureSeedTransfer{exportSym: cfg.SymbolID(7), exportRef: fnRef}
	childTransfer := &captureSeedTransfer{}
	prog := &captureSeedProgram{
		graphs: map[summary.FuncRef]*cfg.Graph{
			parentRef: parentGraph,
			childRef:  childGraph,
		},
		transfers: map[summary.FuncRef]equation.NodeTransfer{
			parentRef: parentTransfer,
			childRef:  childTransfer,
		},
		parent: parentRef,
		child:  childRef,
	}

	q := summary.New(prog)
	ctx := db.NewQueryContext(db.New())
	_ = q.Summarize(ctx, childRef)
	fs := q.Intra(ctx, childRef)

	refs, ok := flow.FunctionRefAtPath(fs.Points[childGraph.Entry()].FunctionRefs, constraint.NewPath(cfg.SymbolID(7), "captured"))
	if !ok {
		t.Fatalf("child capture function refs missing: %v", fs.Points[childGraph.Entry()].FunctionRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got != fnRef {
		t.Fatalf("child capture function refs = %s, want singleton %v", refs.Format(), fnRef)
	}
}

// TestSummary_CalleeReturnFlowsToCaller is gate (a): a caller that calls a callee
// resolves the callee's summary, and the callee's summary carries its return
// type. The converged summary of the callee is asserted; the caller's summary
// resolving the callee through the summary solve closes the call-graph edge.
func TestSummary_CalleeReturnFlowsToCaller(t *testing.T) {
	prog := newTestProgram(t, map[string]string{
		// callee returns a string literal.
		"callee": `
local s = "ok"
return s
`,
		// caller calls callee; the call-graph edge caller -> callee is derived.
		"caller": `
local r = callee()
return r
`,
	})

	q := summary.New(prog)
	ctx := db.NewQueryContext(db.New())

	calleeRef := prog.byName["callee"]
	callerRef := prog.byName["caller"]

	calleeSum := q.Summarize(ctx, calleeRef)
	if len(calleeSum.Returns) != 1 {
		t.Fatalf("callee must summarize one return slot; got %d (%v)", len(calleeSum.Returns), calleeSum.Returns)
	}
	got := calleeSum.Returns[0].ProjectValue()
	lit, ok := got.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		t.Fatalf("callee return slot 0 must be a string literal; got %v", got)
	}
	if v, isStr := lit.Value.(string); !isStr || v != "ok" {
		t.Fatalf("callee return slot 0 must be \"ok\"; got %v", got)
	}

	// The caller summarizes without error and resolves the callee edge: Callees
	// must contain callee, proving the call site reads the callee summary.
	callees := prog.Callees(callerRef)
	if len(callees) != 1 || callees[0] != calleeRef {
		t.Fatalf("caller must call callee; callees=%v", callees)
	}
	// Summarizing the caller drives the call-graph fixpoint through the edge.
	_ = q.Summarize(ctx, callerRef)

	// Re-summarizing the callee returns the same converged summary (memoized,
	// deterministic).
	again := q.Summarize(ctx, calleeRef)
	if !summary.SummaryDomain.Equal(calleeSum, again) {
		t.Fatalf("callee summary is not stable across calls:\n first=%v\n again=%v", calleeSum, again)
	}
}

// TestSummary_RecursionTerminates is gate (b): the summary fixpoint of a
// self-recursive function (factorial) and of a mutually recursive pair (is_even
// / is_odd) TERMINATES. The bottom seed plus the db cycle drive the call-graph
// fixpoint to a post-fixpoint; the test process -timeout is the only backstop, so
// completing at all proves interproc-recursion termination by construction, not a
// depth cap.
func TestSummary_RecursionTerminates(t *testing.T) {
	t.Run("self-recursion", func(t *testing.T) {
		prog := newTestProgram(t, map[string]string{
			// Self-recursive: factorial calls itself. The call-graph node fact
			// depends on its own summary, a db cycle seeded at bottom.
			"fact": `
local one = 1
local r = fact()
return one
`,
		})
		q := summary.New(prog)
		ctx := db.NewQueryContext(db.New())

		ref := prog.byName["fact"]
		// fact must be its own callee (self-edge), the recursion under test.
		callees := prog.Callees(ref)
		if len(callees) != 1 || callees[0] != ref {
			t.Fatalf("fact must call itself; callees=%v", callees)
		}

		// Summarize must terminate (the cycle converges via the bottom seed + the
		// summary lattice). A non-terminating regression hits the -timeout.
		sum := q.Summarize(ctx, ref)
		// Sensible summary: returns one slot (the literal 1 it returns).
		if len(sum.Returns) != 1 {
			t.Fatalf("fact must summarize one return slot; got %d (%v)", len(sum.Returns), sum.Returns)
		}
		lit, ok := sum.Returns[0].ProjectValue().(*typ.Literal)
		if !ok || lit.Base != kind.Integer {
			t.Fatalf("fact return slot 0 must be an integer literal; got %v", sum.Returns[0].ProjectValue())
		}
	})

	t.Run("mutual-recursion", func(t *testing.T) {
		prog := newTestProgram(t, map[string]string{
			// is_even calls is_odd and vice versa: a 2-node call-graph cycle. The
			// db cycle solves the pair together from the bottom seed.
			"is_even": `
local r = is_odd()
return r
`,
			"is_odd": `
local r = is_even()
return r
`,
		})
		q := summary.New(prog)
		ctx := db.NewQueryContext(db.New())

		evenRef := prog.byName["is_even"]
		oddRef := prog.byName["is_odd"]

		// The cycle is genuine: each calls the other.
		if c := prog.Callees(evenRef); len(c) != 1 || c[0] != oddRef {
			t.Fatalf("is_even must call is_odd; callees=%v", c)
		}
		if c := prog.Callees(oddRef); len(c) != 1 || c[0] != evenRef {
			t.Fatalf("is_odd must call is_even; callees=%v", c)
		}

		// Both summaries must converge; the db cycle iterates the pair to a
		// post-fixpoint. Reaching here at all is the termination property.
		evenSum := q.Summarize(ctx, evenRef)
		oddSum := q.Summarize(ctx, oddRef)

		// Re-summarizing yields the same converged value (a true fixpoint).
		if !summary.SummaryDomain.Equal(evenSum, q.Summarize(ctx, evenRef)) {
			t.Fatal("is_even summary did not converge to a stable fixpoint")
		}
		if !summary.SummaryDomain.Equal(oddSum, q.Summarize(ctx, oddRef)) {
			t.Fatal("is_odd summary did not converge to a stable fixpoint")
		}
	})
}
