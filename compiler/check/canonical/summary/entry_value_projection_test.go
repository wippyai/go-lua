package summary_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDirectCallEntryProductValues_ProjectsAllExactInformativeArgs(t *testing.T) {
	args := []ast.Expr{
		&ast.StringExpr{Value: "s"},
		&ast.NumberExpr{Value: "1"},
		&ast.TrueExpr{},
		&ast.IdentExpr{Value: "unknown"},
	}
	call := &ast.FuncCallExpr{Args: args}
	callee := summary.FuncRef{GraphID: 7}

	projection := summary.CallEntryContextProjection{
		ParamSlot: func(gotCallee summary.FuncRef, _ *ast.FuncCallExpr, argIdx int) (int, int, bool) {
			if gotCallee != callee {
				t.Fatalf("callee = %v, want %v", gotCallee, callee)
			}
			switch argIdx {
			case 0:
				return 0, 0, true
			case 1:
				return 1, 0, true
			case 2:
				return 2, 2, true
			case 3:
				return 3, 3, true
			default:
				return -1, -1, false
			}
		},
	}
	got := projection.DirectProductValues(callee, call, []product.AbstractValue{
		product.FromType(typ.String),
		product.FromType(typ.Number),
		product.FromType(typ.Boolean),
		product.FromType(typ.Unknown),
	})

	wantSlot0 := product.Join(product.FromType(typ.String), product.FromType(typ.Number))
	wantSlot2 := product.FromType(typ.Boolean)
	if len(got) != 2 {
		t.Fatalf("entry values = %#v, want joined slot 0 and exact slot 2", got)
	}
	if !product.Equal(got[0], wantSlot0) {
		t.Fatalf("slot 0 = %s, want %s", got[0].ProjectValue(), wantSlot0.ProjectValue())
	}
	if !product.Equal(got[2], wantSlot2) {
		t.Fatalf("slot 2 = %s, want %s", got[2].ProjectValue(), wantSlot2.ProjectValue())
	}
}

func TestDirectCallEntryProductValues_ProjectsOmittedFixedArgsAsNil(t *testing.T) {
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.StringExpr{Value: "World"}}}
	callee := summary.FuncRef{GraphID: 7}

	projection := summary.CallEntryContextProjection{
		ParamSlot: func(gotCallee summary.FuncRef, _ *ast.FuncCallExpr, runtimeIdx int) (int, int, bool) {
			if gotCallee != callee {
				t.Fatalf("callee = %v, want %v", gotCallee, callee)
			}
			switch runtimeIdx {
			case 0:
				return 0, 0, true
			case 1:
				return 1, 1, true
			default:
				return -1, -1, false
			}
		},
		ParamSlotCount: func(summary.FuncRef, *ast.FuncCallExpr) int {
			return 2
		},
	}
	got := projection.DirectProductValues(callee, call, []product.AbstractValue{product.FromType(typ.String)})

	if len(got) != 2 {
		t.Fatalf("entry values = %#v, want supplied string and omitted nil", got)
	}
	if !product.Equal(got[0], product.FromType(typ.String)) {
		t.Fatalf("slot 0 = %s, want string", got[0].ProjectValue())
	}
	if !product.Equal(got[1], product.FromType(typ.Nil)) {
		t.Fatalf("slot 1 = %s, want nil", got[1].ProjectValue())
	}
}

func TestDirectCallEntryProductValues_ProjectsMethodReceiverRuntimeSlot(t *testing.T) {
	recv := &ast.IdentExpr{Value: "state"}
	arg := &ast.StringExpr{Value: "payload"}
	call := &ast.FuncCallExpr{
		Receiver: recv,
		Method:   "load_state",
		Args:     []ast.Expr{arg},
	}
	callee := summary.FuncRef{GraphID: 8}
	receiverType := typ.NewRecord().Field("nodes", typ.NewRecord().Build()).Build()

	projection := summary.CallEntryContextProjection{
		ParamSlot: func(gotCallee summary.FuncRef, _ *ast.FuncCallExpr, runtimeIdx int) (int, int, bool) {
			if gotCallee != callee {
				t.Fatalf("callee = %v, want %v", gotCallee, callee)
			}
			switch runtimeIdx {
			case 0:
				return -1, 0, true
			case 1:
				return 0, 1, true
			default:
				return 0, 0, false
			}
		},
	}
	got := projection.DirectProductValues(callee, call, []product.AbstractValue{
		product.FromType(receiverType),
		product.FromType(typ.String),
	})

	if gotSlot0, ok := got[0]; !ok || !typ.TypeEquals(gotSlot0.ProjectValue(), receiverType) {
		t.Fatalf("receiver slot = %v/%v, want %v", gotSlot0.ProjectValue(), ok, receiverType)
	}
	if gotSlot1, ok := got[1]; !ok || !typ.TypeEquals(gotSlot1.ProjectValue(), typ.String) {
		t.Fatalf("arg slot = %v/%v, want string", gotSlot1.ProjectValue(), ok)
	}
}

func TestDirectCallEntryReferences_ProjectFunctionRuntimeArgsToParamPaths(t *testing.T) {
	sourceSym := cfg.SymbolID(10)
	param0 := cfg.SymbolID(20)
	param1 := cfg.SymbolID(21)
	sourcePath := constraint.NewPath(sourceSym, "cb")
	param0Path := constraint.NewPath(param0, "fn")
	param1Path := constraint.NewPath(param1, "direct")
	callbackRef := flow.FunctionRef{GraphID: 101}
	nestedRef := flow.FunctionRef{GraphID: 102}
	directRef := flow.FunctionRef{GraphID: 103}
	arg0 := &ast.IdentExpr{Value: "cb"}
	arg1 := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{Args: []ast.Expr{arg0, arg1}}

	callee := summary.FuncRef{GraphID: 7}
	functionRefs := flow.WithFunctionRef(flow.WithFunctionRef(nil, sourcePath.Key(), flow.FunctionRefSetOf(callbackRef)), sourcePath.Field("nested").Key(), flow.FunctionRefSetOf(nestedRef))
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(_ summary.FuncRef, _ *ast.FuncCallExpr, runtimeIdx int) (int, int, bool) {
			switch runtimeIdx {
			case 0:
				return 0, 0, true
			case 1:
				return 1, 1, true
			default:
				return 0, 0, false
			}
		},
		ParamPath: func(_ summary.FuncRef, slot int) (constraint.Path, bool) {
			if slot == 0 {
				return param0Path, true
			}
			if slot == 1 {
				return param1Path, true
			}
			return constraint.Path{}, false
		},
		ArgPath: func(runtimeIdx int, _ ast.Expr) (constraint.Path, bool) {
			if runtimeIdx == 0 {
				return sourcePath, true
			}
			return constraint.Path{}, false
		},
	}
	got := projection.DirectReferences(
		callee,
		call,
		nil,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), functionRefs, flow.ClosureRefsDomain.Bottom()),
		summary.EntryReferenceArgSources{
			FunctionRefs: func(runtimeIdx int, _ ast.Expr, _ *flow.PointState) (flow.FunctionRefSet, bool) {
				if runtimeIdx == 1 {
					return flow.FunctionRefSetOf(directRef), true
				}
				return flow.FunctionRefSet{}, false
			},
		},
	).FunctionRefs()

	if refs, ok := flow.FunctionRefAt(got, param0Path.Key()); !ok {
		t.Fatalf("rebased root refs missing: %#v", got)
	} else if ref, singleton := refs.Singleton(); !singleton || ref != callbackRef {
		t.Fatalf("rebased root refs = %s, want %v", refs.Format(), callbackRef)
	}
	if refs, ok := flow.FunctionRefAt(got, param0Path.Field("nested").Key()); !ok {
		t.Fatalf("rebased nested refs missing: %#v", got)
	} else if ref, singleton := refs.Singleton(); !singleton || ref != nestedRef {
		t.Fatalf("rebased nested refs = %s, want %v", refs.Format(), nestedRef)
	}
	if refs, ok := flow.FunctionRefAt(got, param1Path.Key()); !ok {
		t.Fatalf("direct literal refs missing: %#v", got)
	} else if ref, singleton := refs.Singleton(); !singleton || ref != directRef {
		t.Fatalf("direct literal refs = %s, want %v", refs.Format(), directRef)
	}
}

func TestDirectCallEntryReferences_LimitsRebasedFunctionArgsToCalleeVocabulary(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(24), "receiver")
	param := constraint.NewPath(cfg.SymbolID(25), "self")
	usedRef := flow.FunctionRef{GraphID: 106}
	unusedRef := flow.FunctionRef{GraphID: 107}
	arg := &ast.IdentExpr{Value: "receiver"}
	call := &ast.FuncCallExpr{Args: []ast.Expr{arg}}
	refs := flow.WithFunctionRef(nil, source.Field("used").Key(), flow.FunctionRefSetOf(usedRef))
	refs = flow.WithFunctionRef(refs, source.Field("unused").Key(), flow.FunctionRefSetOf(unusedRef))

	callee := summary.FuncRef{GraphID: 7}
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) { return 0, 0, true },
		ParamPath: func(summary.FuncRef, int) (constraint.Path, bool) { return param, true },
		ArgPath:   func(int, ast.Expr) (constraint.Path, bool) { return source, true },
		ReferencePaths: func(summary.FuncRef) flow.ReferencePathProjection {
			return flow.ReferencePathProjection{Exact: []constraint.Path{param.Field("used")}}
		},
	}
	got := projection.DirectReferences(
		callee,
		call,
		nil,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), refs, flow.ClosureRefsDomain.Bottom()),
		summary.EntryReferenceArgSources{},
	).FunctionRefs()

	if refs, ok := flow.FunctionRefAt(got, param.Field("used").Key()); !ok {
		t.Fatalf("projected used ref missing: %#v", got)
	} else if ref, singleton := refs.Singleton(); !singleton || ref != usedRef {
		t.Fatalf("projected used ref = %s, want %v", refs.Format(), usedRef)
	}
	if _, ok := flow.FunctionRefAt(got, param.Field("unused").Key()); ok {
		t.Fatalf("projected refs retained unused path: %#v", got)
	}
}

func TestDirectCallEntryReferences_SeedsDirectFunctionLiteralWhenParamSlotMapped(t *testing.T) {
	param := cfg.SymbolID(22)
	paramPath := constraint.NewPath(param, "fn")
	directRef := flow.FunctionRef{GraphID: 104}
	arg := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{Args: []ast.Expr{arg}}

	callee := summary.FuncRef{GraphID: 8}
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ParamPath: func(summary.FuncRef, int) (constraint.Path, bool) {
			return paramPath, true
		},
	}
	got := projection.DirectReferences(
		callee,
		call,
		nil,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
		summary.EntryReferenceArgSources{
			FunctionRefs: func(_ int, gotArg ast.Expr, _ *flow.PointState) (flow.FunctionRefSet, bool) {
				if gotArg != arg {
					t.Fatalf("arg = %#v, want direct literal", gotArg)
				}
				return flow.FunctionRefSetOf(directRef), true
			},
		},
	).FunctionRefs()

	if refs, ok := flow.FunctionRefAt(got, paramPath.Key()); !ok {
		t.Fatalf("direct literal refs missing: %#v", got)
	} else if ref, singleton := refs.Singleton(); !singleton || ref != directRef {
		t.Fatalf("direct literal refs = %s, want %v", refs.Format(), directRef)
	}
}

func TestDirectCallEntryReferences_RebasesFunctionCallReturnSubtreeToParamPath(t *testing.T) {
	param := cfg.SymbolID(23)
	paramPath := constraint.NewPath(param, "database")
	queryRef := flow.FunctionRef{GraphID: 105}
	arg := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "mock"}}
	call := &ast.FuncCallExpr{Args: []ast.Expr{arg}}
	returnRefs := flow.WithFunctionRef(nil, constraint.NewPlaceholder(0).Field("query").Key(), flow.FunctionRefSetOf(queryRef))

	callee := summary.FuncRef{GraphID: 9}
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ParamPath: func(summary.FuncRef, int) (constraint.Path, bool) {
			return paramPath, true
		},
	}
	got := projection.DirectReferences(
		callee,
		call,
		nil,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
		summary.EntryReferenceArgSources{
			RefTrees: func(_ int, gotArg ast.Expr, _ *flow.PointState) (flow.ReferenceContext, bool) {
				if gotArg != arg {
					t.Fatalf("arg = %#v, want call expression", gotArg)
				}
				return flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), returnRefs, flow.ClosureRefsDomain.Bottom()), true
			},
		},
	).FunctionRefs()

	if refs, ok := flow.FunctionRefAt(got, paramPath.Field("query").Key()); !ok {
		t.Fatalf("rebased call-return subtree refs missing: %#v", got)
	} else if ref, singleton := refs.Singleton(); !singleton || ref != queryRef {
		t.Fatalf("rebased call-return subtree refs = %s, want %v", refs.Format(), queryRef)
	}
}

func TestDirectCallEntryReferences_ProjectClosureRuntimeArgsToParamPaths(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(30), "cb")
	target := constraint.NewPath(cfg.SymbolID(31), "fn")
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 201}, flow.CaptureCellsDomain.Bottom(), nil)
	arg := &ast.IdentExpr{Value: "cb"}
	call := &ast.FuncCallExpr{Args: []ast.Expr{arg}}

	callee := summary.FuncRef{GraphID: 9}
	closureRefs := flow.WithClosureRef(nil, source.Key(), flow.ClosureRefSetOf(closure))
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ParamPath: func(summary.FuncRef, int) (constraint.Path, bool) {
			return target, true
		},
		ArgPath: func(int, ast.Expr) (constraint.Path, bool) {
			return source, true
		},
	}
	got := projection.DirectReferences(
		callee,
		call,
		nil,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), closureRefs),
		summary.EntryReferenceArgSources{},
	).ClosureRefs()

	if refs, ok := flow.ClosureRefAt(got, target.Key()); !ok {
		t.Fatalf("rebased closure refs missing: %#v", got)
	} else if ref, singleton := refs.Singleton(); !singleton || ref.Ref != closure.Ref {
		t.Fatalf("rebased closure refs = %s, want %v", refs.Format(), closure.Ref)
	}
}

func TestDirectCallEntryReferences_RebasesClosureCallReturnSubtreeToParamPath(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(32), "database")
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 202}, flow.CaptureCellsDomain.Bottom(), nil)
	arg := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "mock"}}
	call := &ast.FuncCallExpr{Args: []ast.Expr{arg}}
	returnRefs := flow.WithClosureRef(nil, constraint.NewPlaceholder(0).Field("query").Key(), flow.ClosureRefSetOf(closure))

	callee := summary.FuncRef{GraphID: 10}
	projection := summary.CallEntryContextProjection{
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ParamPath: func(summary.FuncRef, int) (constraint.Path, bool) {
			return target, true
		},
	}
	got := projection.DirectReferences(
		callee,
		call,
		nil,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
		summary.EntryReferenceArgSources{
			RefTrees: func(_ int, gotArg ast.Expr, _ *flow.PointState) (flow.ReferenceContext, bool) {
				if gotArg != arg {
					t.Fatalf("arg = %#v, want call expression", gotArg)
				}
				return flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), returnRefs), true
			},
		},
	).ClosureRefs()

	if refs, ok := flow.ClosureRefAt(got, target.Field("query").Key()); !ok {
		t.Fatalf("rebased call-return closure subtree missing: %#v", got)
	} else if ref, singleton := refs.Singleton(); !singleton || ref.Ref != closure.Ref {
		t.Fatalf("rebased call-return closure subtree = %s, want %v", refs.Format(), closure.Ref)
	}
}

func TestCallEntryContextProjectionUsesCallEventPostState(t *testing.T) {
	arg := &ast.IdentExpr{Value: "arg"}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "callee"},
		Args: []ast.Expr{arg},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.FuncCallStmt{Expr: call}}}
	graph := cfg.Build(fn, "callee")
	var point cfg.Point
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		point = p
	})
	if point == 0 {
		t.Fatal("test graph did not expose a call site")
	}

	callee := summary.FuncRef{GraphID: 9}
	targetCells := flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(99), Value: product.FromType(typ.String)}})
	targetRefs := flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(8), "ref").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 7}))
	targetClosures := flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(9), "closure").Key(), flow.ClosureRefSetOf(
		flow.ClosureRefOf(
			flow.FunctionRef{GraphID: 9},
			flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(12), Value: product.FromType(typ.Number)}}),
			flow.FunctionRefsDomain.Bottom(),
		),
	))
	keys := summary.CallEntryContextProjection{
		Graph: graph,
		State: state.FunctionState{
			InPoints: map[cfg.Point]flow.PointState{
				point: {Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(1): product.FromType(typ.Number),
				}},
			},
			Points: map[cfg.Point]flow.PointState{
				point: {Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(1): product.FromType(typ.String),
				}},
			},
		},
		ResolveTargets: func(*ast.FuncCallExpr, *flow.PointState) []summary.CallEntryTarget {
			return []summary.CallEntryTarget{{
				Ref:             callee,
				EntryReferences: flow.ReferenceContextOf(targetCells, targetRefs, targetClosures),
			}}
		},
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		EvalArg: func(in *flow.PointState, _ ast.Expr) (product.AbstractValue, bool) {
			return flow.SymbolValue(*in, 1)
		},
	}.ProjectKeys()

	if len(keys) != 1 {
		t.Fatalf("context keys = %d, want 1", len(keys))
	}
	if keys[0].Ref != callee {
		t.Fatalf("callee key = %v, want %v", keys[0].Ref, callee)
	}
	if !flow.CaptureCellsDomain.Equal(keys[0].References.CaptureCells(), targetCells) {
		t.Fatalf("entry cells key = %s, want %s", keys[0].References.CaptureCells().Format(), targetCells.Format())
	}
	if !flow.FunctionRefsDomain.Equal(keys[0].References.FunctionRefs(), targetRefs) {
		t.Fatalf("entry refs = %#v, want %#v", keys[0].References.FunctionRefs(), targetRefs)
	}
	if !flow.ClosureRefsDomain.Equal(keys[0].References.ClosureRefs(), targetClosures) {
		t.Fatalf("entry closures = %s, want %s", flow.ClosureRefsKeyOf(keys[0].References.ClosureRefs()).Format(), flow.ClosureRefsKeyOf(targetClosures).Format())
	}
	values := keys[0].Values.Values()
	got, ok := values[0]
	if !ok || !product.Equal(got, product.FromType(typ.String)) {
		t.Fatalf("entry slot 0 = %v/%v, want post-state string", got.ProjectValue(), ok)
	}
}

func TestCallEntryContextProjectionResolvesTargetsFromPreCallState(t *testing.T) {
	arg := &ast.IdentExpr{Value: "arg"}
	call := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "c"},
		Method:   "with_options",
		Args:     []ast.Expr{arg},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.FuncCallStmt{Expr: call}}}
	graph := cfg.Build(fn)
	var (
		point cfg.Point
		info  *cfg.CallInfo
	)
	graph.EachCallSite(func(p cfg.Point, callInfo *cfg.CallInfo) {
		point = p
		info = callInfo
	})
	if point == 0 || info == nil || info.CalleePath.IsEmpty() {
		t.Fatal("test graph did not expose method call info")
	}

	callee := summary.FuncRef{GraphID: 10}
	methodRef := flow.FunctionRef{GraphID: 10}
	methodPath := info.CalleePath.Field(info.Method)
	postRoot := flow.WithFunctionRef(nil, info.CalleePath.Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 99}))
	preValue := product.FromType(typ.Number)
	postValue := product.FromType(typ.String)
	keys := summary.CallEntryContextProjection{
		Graph: graph,
		State: state.FunctionState{
			InPoints: map[cfg.Point]flow.PointState{
				point: {
					Env: map[flow.ValueKey]product.AbstractValue{
						flow.SymbolValueKey(1): preValue,
					},
					FunctionRefs: flow.WithFunctionRef(nil, methodPath.Key(), flow.FunctionRefSetOf(methodRef)),
				},
			},
			Points: map[cfg.Point]flow.PointState{
				point: {
					Env: map[flow.ValueKey]product.AbstractValue{
						flow.SymbolValueKey(1): postValue,
					},
					FunctionRefs: postRoot,
				},
			},
		},
		ResolveTargets: func(gotCall *ast.FuncCallExpr, in *flow.PointState) []summary.CallEntryTarget {
			if gotCall != call {
				t.Fatalf("call = %#v, want test call", gotCall)
			}
			if set, ok := flow.FunctionRefAt(in.FunctionRefs, methodPath.Key()); ok && !set.IsBottom() {
				return []summary.CallEntryTarget{{Ref: callee}}
			}
			return nil
		},
		ParamSlot: func(_ summary.FuncRef, _ *ast.FuncCallExpr, runtimeIdx int) (int, int, bool) {
			if runtimeIdx == 1 {
				return 0, 0, true
			}
			return 0, 0, false
		},
		EvalArg: func(in *flow.PointState, _ ast.Expr) (product.AbstractValue, bool) {
			return flow.SymbolValue(*in, 1)
		},
	}.ProjectKeys()

	if len(keys) != 1 {
		t.Fatalf("context keys = %d, want 1", len(keys))
	}
	if keys[0].Ref != callee {
		t.Fatalf("callee key = %v, want %v", keys[0].Ref, callee)
	}
	got, ok := keys[0].Values.Values()[0]
	if !ok || !product.Equal(got, preValue) {
		t.Fatalf("entry slot 0 = %v/%v, want pre-state number", got.ProjectValue(), ok)
	}
}

func TestCallEntryContextProjectionProjectsMethodReceiverRuntimeSlot(t *testing.T) {
	recv := &ast.IdentExpr{Value: "state"}
	call := &ast.FuncCallExpr{Receiver: recv, Method: "load_state"}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.FuncCallStmt{Expr: call}}}
	graph := cfg.Build(fn)
	var point cfg.Point
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		point = p
	})
	if point == 0 {
		t.Fatal("test graph did not expose a call site")
	}

	callee := summary.FuncRef{GraphID: 10}
	receiverValue := product.FromType(typ.NewRecord().Field("nodes", typ.NewRecord().Build()).Build())
	keys := summary.CallEntryContextProjection{
		Graph: graph,
		State: state.FunctionState{
			Points: map[cfg.Point]flow.PointState{
				point: {},
			},
		},
		ResolveTargets: func(*ast.FuncCallExpr, *flow.PointState) []summary.CallEntryTarget {
			return []summary.CallEntryTarget{{Ref: callee}}
		},
		ParamSlot: func(_ summary.FuncRef, _ *ast.FuncCallExpr, runtimeIdx int) (int, int, bool) {
			if runtimeIdx == 0 {
				return -1, 0, true
			}
			return 0, 0, false
		},
		EvalArg: func(_ *flow.PointState, expr ast.Expr) (product.AbstractValue, bool) {
			if expr == recv {
				return receiverValue, true
			}
			return product.AbstractValue{}, false
		},
	}.ProjectKeys()

	if len(keys) != 1 {
		t.Fatalf("context keys = %d, want 1", len(keys))
	}
	got, ok := keys[0].Values.Values()[0]
	if !ok || !product.Equal(got, receiverValue) {
		t.Fatalf("receiver entry slot = %v/%v, want %v", got.ProjectValue(), ok, receiverValue.ProjectValue())
	}
}

func TestCallEntryContextProjection_UsesCallEntryTargetResolverAxesForClosureAndDedupesKeys(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "callee"}}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.FuncCallStmt{Expr: call}}}
	graph := cfg.Build(fn, "callee")
	var point cfg.Point
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		point = p
	})
	if point == 0 {
		t.Fatal("test graph did not expose a call site")
	}

	callee := summary.FuncRef{GraphID: 11, ParentHash: 4}
	closureA := summary.CallEntryTarget{
		Ref: callee,
		EntryReferences: flow.ReferenceContextOf(
			flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 1, Value: product.FromType(typ.String)}}),
			flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(2), "a").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 101})),
			flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(3), "a").Key(), flow.ClosureRefSetOf(
				flow.ClosureRefOf(
					flow.FunctionRef{GraphID: 201},
					flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(12), Value: product.FromType(typ.Number)}}),
					flow.FunctionRefsDomain.Bottom(),
				),
			)),
		),
	}
	closureB := summary.CallEntryTarget{
		Ref: callee,
		EntryReferences: flow.ReferenceContextOf(
			flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 2, Value: product.FromType(typ.Boolean)}}),
			flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(4), "b").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 102})),
			flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(5), "b").Key(), flow.ClosureRefSetOf(
				flow.ClosureRefOf(
					flow.FunctionRef{GraphID: 202},
					flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(13), Value: product.FromType(typ.Boolean)}}),
					flow.FunctionRefsDomain.Bottom(),
				),
			)),
		),
	}

	keys := summary.CallEntryContextProjection{
		Graph: graph,
		State: state.FunctionState{
			Points: map[cfg.Point]flow.PointState{
				point: {
					FunctionRefs: flow.FunctionRefsDomain.Bottom(),
					ClosureRefs:  flow.ClosureRefsDomain.Bottom(),
					Cells:        flow.CaptureCellsDomain.Bottom(),
				},
			},
		},
		ResolveTargets: func(_ *ast.FuncCallExpr, _ *flow.PointState) []summary.CallEntryTarget {
			return []summary.CallEntryTarget{closureA, closureA, closureB}
		},
	}.ProjectKeys()

	if len(keys) != 2 {
		t.Fatalf("context keys = %d, want 2", len(keys))
	}
	wantA := summary.NewKeyWithReferenceContext(
		callee,
		closureA.EntryReferences,
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
	wantB := summary.NewKeyWithReferenceContext(
		callee,
		closureB.EntryReferences,
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
	if keys[0] != wantA {
		t.Fatalf("first key = %+v, want closure-A key", keys[0])
	}
	if keys[1] != wantB {
		t.Fatalf("second key = %+v, want closure-B key", keys[1])
	}
}

func TestClosureEntryContextProjection_ProjectsDeclarationClosureContexts(t *testing.T) {
	pathA := constraint.NewPath(cfg.SymbolID(1), "a")
	pathB := constraint.NewPath(cfg.SymbolID(2), "b")
	refA := summary.FuncRef{GraphID: 21, ParentHash: 1}
	refB := summary.FuncRef{GraphID: 22, ParentHash: 2}
	cellsA := flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.Number)}})
	cellsB := flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(11), Value: product.FromType(typ.String)}})
	refsA := flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(12), "helper").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 101}))
	nested := flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(13), "factory").Key(), flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 102}, flow.CaptureCellsDomain.Bottom(), nil),
	))
	closureA := flow.ClosureRefOf(flow.FunctionRef{GraphID: refA.GraphID, ParentHash: refA.ParentHash}, cellsA, refsA, nested)
	closureB := flow.ClosureRefOf(flow.FunctionRef{GraphID: refB.GraphID, ParentHash: refB.ParentHash}, cellsB, nil)

	keys := summary.ClosureEntryContextProjection{
		State: state.FunctionState{
			Points: map[cfg.Point]flow.PointState{
				1: {ClosureRefs: flow.WithClosureRef(nil, pathA.Key(), flow.ClosureRefSetOf(closureA))},
				2: {ClosureRefs: flow.WithClosureRef(nil, pathB.Key(), flow.ClosureRefSetOf(closureB))},
			},
			InPoints: map[cfg.Point]flow.PointState{
				3: {ClosureRefs: flow.WithClosureRef(nil, pathA.Key(), flow.ClosureRefSetOf(closureA))},
			},
		},
	}.ProjectKeys()

	if len(keys) != 2 {
		t.Fatalf("context keys = %d, want 2", len(keys))
	}
	wantA := summary.NewKeyWithReferenceContext(refA, flow.ReferenceContextOf(cellsA, refsA, nested), nil, flow.BoundaryFactsDomain.Top())
	wantB := summary.NewKeyWithReferenceContext(refB, flow.ReferenceContextOf(cellsB, nil, nil), nil, flow.BoundaryFactsDomain.Top())
	if keys[0] != wantA {
		t.Fatalf("first key = %+v, want declaration closure A", keys[0])
	}
	if keys[1] != wantB {
		t.Fatalf("second key = %+v, want declaration closure B", keys[1])
	}
}

func TestCallEntryValueProjection_UsesResolvedTargetsForRefAndDedupesSlots(t *testing.T) {
	arg0 := &ast.IdentExpr{Value: "arg0"}
	arg1 := &ast.IdentExpr{Value: "arg1"}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "callee"},
		Args: []ast.Expr{arg0, arg1},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.FuncCallStmt{Expr: call}}}
	graph := cfg.Build(fn, "caller")
	var point cfg.Point
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		point = p
	})
	if point == 0 {
		t.Fatal("test graph did not expose a call site")
	}

	calleeA := summary.FuncRef{GraphID: 11}
	calleeB := summary.FuncRef{GraphID: 12}
	argValues := map[ast.Expr]product.AbstractValue{
		arg0: product.FromType(typ.String),
		arg1: product.FromType(typ.Number),
	}
	out := summary.CallEntryValueProjection{
		Graph: graph,
		State: state.FunctionState{
			Points: map[cfg.Point]flow.PointState{
				point: {},
			},
		},
		ResolveTargets: func(_ *ast.FuncCallExpr, _ *flow.PointState) []summary.CallEntryTarget {
			return []summary.CallEntryTarget{
				{Ref: calleeA},
				{Ref: calleeA},
				{Ref: calleeB},
			}
		},
		ParamSlot: func(callee summary.FuncRef, _ *ast.FuncCallExpr, argIdx int) (int, int, bool) {
			switch argIdx {
			case 0:
				return 0, 0, true
			case 1:
				if callee == calleeB {
					return 1, 1, true
				}
				return 1, 0, true
			default:
				return -1, -1, false
			}
		},
		ParamAnnotated: func(callee summary.FuncRef, sourceParam int) bool {
			return callee == calleeB && sourceParam == 1
		},
		EvalArg: func(_ *flow.PointState, expr ast.Expr) (product.AbstractValue, bool) {
			av, ok := argValues[expr]
			return av, ok
		},
	}.Project()

	if len(out) != 2 {
		t.Fatalf("projected call-entry values = %#v, want exactly two callees", out)
	}

	wantA := product.Join(argValues[arg0], argValues[arg1])
	if got, ok := out[calleeA][0]; !ok || !product.Equal(got, wantA) {
		t.Fatalf("callee=%+v slot 0 = %#v/%v, want %v", calleeA, got, ok, wantA.ProjectValue())
	}
	if got, ok := out[calleeA][1]; ok || got != (product.AbstractValue{}) {
		t.Fatalf("callee=%+v should not have slot 1 evidence, got %#v", calleeA, got)
	}

	if got, ok := out[calleeB][1]; ok {
		t.Fatalf("callee=%+v slot 1 should be filtered by ParamAnnotated, got %#v", calleeB, got)
	}
	if got, ok := out[calleeB][0]; !ok || !product.Equal(got, argValues[arg0]) {
		t.Fatalf("callee=%+v slot 0 = %#v, want %v", calleeB, got, argValues[arg0].ProjectValue())
	}
}

func TestCallEntryValueProjection_EmptyTargetsYieldNoEvidence(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "callee"}}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.FuncCallStmt{Expr: call}}}
	graph := cfg.Build(fn, "caller")
	var point cfg.Point
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		point = p
	})
	if point == 0 {
		t.Fatal("test graph did not expose a call site")
	}

	got := summary.CallEntryValueProjection{
		Graph: graph,
		State: state.FunctionState{
			Points: map[cfg.Point]flow.PointState{
				point: {},
			},
		},
		ResolveTargets: func(*ast.FuncCallExpr, *flow.PointState) []summary.CallEntryTarget {
			return nil
		},
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		EvalArg: func(*flow.PointState, ast.Expr) (product.AbstractValue, bool) {
			return product.FromType(typ.String), true
		},
	}.Project()
	if len(got) != 0 {
		t.Fatalf("projected call-entry values = %#v, want none", got)
	}
}

func TestCallEntryValueProjection_ProjectsCallbackExpectedParams(t *testing.T) {
	callbackArg := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "up"},
		Args: []ast.Expr{callbackArg},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.FuncCallStmt{Expr: call}}}
	graph := cfg.Build(fn, "up")
	var point cfg.Point
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		point = p
	})
	if point == 0 {
		t.Fatal("test graph did not expose a call site")
	}

	callbackRef := summary.FuncRef{GraphID: 21}
	callbackRef2 := summary.FuncRef{GraphID: 31}
	got := summary.CallEntryValueProjection{
		Graph: graph,
		State: state.FunctionState{
			Points: map[cfg.Point]flow.PointState{
				point: {},
			},
		},
		ResolveTargets: func(*ast.FuncCallExpr, *flow.PointState) []summary.CallEntryTarget {
			return nil
		},
		ResolveCallback: func(arg ast.Expr, _ cfg.SymbolID, _ *flow.PointState) ([]summary.FuncRef, bool) {
			if arg != callbackArg {
				t.Fatalf("ResolveCallback arg = %#v, want callback literal", arg)
			}
			return []summary.FuncRef{callbackRef2, callbackRef}, true
		},
		ExpectedArgType: func(_ cfg.Point, _ *cfg.CallInfo, _ *flow.PointState, argIdx int) typ.Type {
			if argIdx != 0 {
				t.Fatalf("ExpectedArgType index = %d, want 0", argIdx)
			}
			return typ.Func().Param("db", typ.String).Build()
		},
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		EvalArg: func(*flow.PointState, ast.Expr) (product.AbstractValue, bool) {
			return product.AbstractValue{}, false
		},
	}.Project()

	if gotValue, ok := got[callbackRef][0]; !ok || !typ.TypeEquals(gotValue.ProjectValue(), typ.String) {
		t.Fatalf("callback slot 0 = %v/%v, want string", gotValue.ProjectValue(), ok)
	}
	if gotValue, ok := got[callbackRef2][0]; !ok || !typ.TypeEquals(gotValue.ProjectValue(), typ.String) {
		t.Fatalf("second callback slot 0 = %v/%v, want string", gotValue.ProjectValue(), ok)
	}
}

func TestCallEntryContextProjection_ProjectsCallbackExpectedParams(t *testing.T) {
	callbackArg := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "up"},
		Args: []ast.Expr{callbackArg},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.FuncCallStmt{Expr: call}}}
	graph := cfg.Build(fn, "up")
	var point cfg.Point
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		point = p
	})
	if point == 0 {
		t.Fatal("test graph did not expose a call site")
	}

	callbackRef := summary.FuncRef{GraphID: 22}
	keys := summary.CallEntryContextProjection{
		Graph: graph,
		State: state.FunctionState{
			Points: map[cfg.Point]flow.PointState{
				point: {},
			},
		},
		ResolveTargets: func(*ast.FuncCallExpr, *flow.PointState) []summary.CallEntryTarget {
			return nil
		},
		ResolveCallback: func(arg ast.Expr, _ cfg.SymbolID, _ *flow.PointState) ([]summary.FuncRef, bool) {
			if arg != callbackArg {
				t.Fatalf("ResolveCallback arg = %#v, want callback literal", arg)
			}
			return []summary.FuncRef{callbackRef}, true
		},
		ExpectedArgType: func(_ cfg.Point, _ *cfg.CallInfo, _ *flow.PointState, argIdx int) typ.Type {
			if argIdx != 0 {
				t.Fatalf("ExpectedArgType index = %d, want 0", argIdx)
			}
			return typ.Func().Param("db", typ.Number).Build()
		},
	}.ProjectKeys()

	if len(keys) != 1 || keys[0].Ref != callbackRef {
		t.Fatalf("callback context keys = %#v, want one key for callback ref", keys)
	}
	values := keys[0].Values.Values()
	if got, ok := values[0]; !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("callback context slot 0 = %v/%v, want number", got.ProjectValue(), ok)
	}
}

func TestCallEntryContextProjection_ProjectsZeroParamCallbackContext(t *testing.T) {
	callbackArg := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "database"},
		Args: []ast.Expr{&ast.StringExpr{Value: "postgres"}, callbackArg},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.FuncCallStmt{Expr: call}}}
	graph := cfg.Build(fn, "database")
	var point cfg.Point
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		point = p
	})
	if point == 0 {
		t.Fatal("test graph did not expose a call site")
	}

	callbackRef := summary.FuncRef{GraphID: 23}
	keys := summary.CallEntryContextProjection{
		Graph: graph,
		State: state.FunctionState{
			Points: map[cfg.Point]flow.PointState{
				point: {},
			},
		},
		ResolveTargets: func(*ast.FuncCallExpr, *flow.PointState) []summary.CallEntryTarget {
			return nil
		},
		ResolveCallback: func(arg ast.Expr, _ cfg.SymbolID, _ *flow.PointState) ([]summary.FuncRef, bool) {
			if arg != callbackArg {
				return nil, false
			}
			return []summary.FuncRef{callbackRef}, true
		},
		ExpectedArgType: func(_ cfg.Point, _ *cfg.CallInfo, _ *flow.PointState, argIdx int) typ.Type {
			if argIdx != 1 {
				return nil
			}
			return typ.Func().Build()
		},
	}.ProjectKeys()

	if len(keys) != 1 || keys[0].Ref != callbackRef {
		t.Fatalf("zero-param callback context keys = %#v, want one key for callback ref", keys)
	}
	if values := keys[0].Values.Values(); len(values) != 0 {
		t.Fatalf("zero-param callback entry values = %#v, want none", values)
	}
}

func TestAggregateEntryValues_FoldsCallerSummariesAndSkipsDeclaredSlots(t *testing.T) {
	callee := summary.FuncRef{GraphID: 3}
	stringAV := product.FromType(typ.String)
	numberAV := product.FromType(typ.Number)
	boolAV := product.FromType(typ.Boolean)

	got := summary.AggregateEntryValues(summary.EntryValueAggregation{
		Callee:           callee,
		HasInferredSlots: true,
		EachCallerEntryValues: func(yield func(summary.EntryValues)) {
			yield(summary.EntryValues{0: stringAV, 1: boolAV})
			yield(summary.EntryValues{0: numberAV})
		},
		SlotDeclared: func(slot int) bool {
			return slot == 1
		},
	})

	wantSlot0 := product.Join(stringAV, numberAV)
	if len(got) != 1 {
		t.Fatalf("entry values = %#v, want only inferred slot 0", got)
	}
	if !product.Equal(got[0], wantSlot0) {
		t.Fatalf("slot 0 = %s, want %s", got[0].ProjectValue(), wantSlot0.ProjectValue())
	}
}

func TestAggregateEntryValues_ProjectsPrototypeSelfFromPublishingSummaries(t *testing.T) {
	proto := cfg.SymbolID(42)
	unpublishedProto := cfg.SymbolID(99)
	selfString := product.FromType(typ.String)
	selfNumber := product.FromType(typ.Number)

	got := summary.AggregateEntryValues(summary.EntryValueAggregation{
		PrototypeReceivers: []summary.EntryValuePrototypeReceiver{
			{Prototype: proto, Slot: 0},
			{Prototype: unpublishedProto, Slot: 1},
			{Prototype: proto, Slot: 2},
		},
		EachPrototypeSource: func(yield func(summary.EntryValuePrototypeSource)) {
			yield(summary.EntryValuePrototypeSource{
				Prototypes: []cfg.SymbolID{proto},
				Self: flow.PrototypeSelfOf([]flow.PrototypeSelfEntry{
					{Prototype: proto, Value: selfString},
				}),
			})
			yield(summary.EntryValuePrototypeSource{
				Prototypes: []cfg.SymbolID{unpublishedProto},
				Self: flow.PrototypeSelfOf([]flow.PrototypeSelfEntry{
					{Prototype: proto, Value: selfNumber},
				}),
			})
		},
		SlotDeclared: func(slot int) bool {
			return slot == 2
		},
	})

	if len(got) != 1 {
		t.Fatalf("entry values = %#v, want only unpublished-filtered prototype slot 0", got)
	}
	if !product.Equal(got[0], selfString) {
		t.Fatalf("slot 0 = %s, want %s", got[0].ProjectValue(), selfString.ProjectValue())
	}
}

func TestAggregateEntryValues_DoesNotForceUnusedSummaryDependencies(t *testing.T) {
	got := summary.AggregateEntryValues(summary.EntryValueAggregation{
		HasInferredSlots: false,
		EachCallerEntryValues: func(func(summary.EntryValues)) {
			t.Fatal("caller summaries should not be read when every slot is declared")
		},
		PrototypeReceivers: []summary.EntryValuePrototypeReceiver{
			{Prototype: cfg.SymbolID(42), Slot: 0},
		},
		EachPrototypeSource: func(func(summary.EntryValuePrototypeSource)) {
			t.Fatal("prototype summaries should not be read when receiver slot is declared")
		},
		SlotDeclared: func(int) bool { return true },
	})
	if len(got) != 0 {
		t.Fatalf("entry values = %#v, want none", got)
	}
}
