package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFuncDefRootWritesFunctionRefAxis(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	sym := cfg.SymbolID(101)
	ref := flow.FunctionRef{GraphID: 202, ParentHash: 303}
	sig := typ.Func().Returns(typ.String).Build()
	tr := New(input.Inputs{}, Config{FuncTyper: functionRefTestTyper{sig: sig, ref: ref}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	tr.applyFuncDef(&out, &cfg.FuncDefInfo{
		Symbol:     sym,
		FuncExpr:   fn,
		TargetPath: constraint.NewPath(sym, "f"),
	})

	got, ok := out.Env[flow.SymbolValueKey(sym)]
	if !ok || got.IsZero() {
		t.Fatalf("root function definition did not write Env[%s]", flow.SymbolValueKey(sym))
	}
	if !typ.TypeEquals(got.ProjectValue(), sig) {
		t.Fatalf("root function value = %v, want %v", got.ProjectValue(), sig)
	}
	refs, ok := flow.FunctionRefAt(out.FunctionRefs, constraint.NewPath(sym, "f").Key())
	if !ok {
		t.Fatal("root function definition did not write FunctionRefs")
	}
	gotRef, singleton := refs.Singleton()
	if !singleton || gotRef != ref {
		t.Fatalf("root function refs = %s, want singleton %v", refs.Format(), ref)
	}
}

func TestFuncDefRootWritesClosureRefEnvironment(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	targetSym := cfg.SymbolID(101)
	captureSym := cfg.SymbolID(102)
	envCaptureSym := cfg.SymbolID(103)
	ref := flow.FunctionRef{GraphID: 202, ParentHash: 303}
	capturedFn := flow.FunctionRef{GraphID: 404}
	capturedClosure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 505}, flow.CaptureCellsDomain.Bottom(), nil)
	sig := typ.Func().Returns(typ.String).Build()
	tr := New(input.Inputs{}, Config{FuncTyper: functionRefTestTyper{
		sig:      sig,
		ref:      ref,
		captured: []cfg.SymbolID{captureSym, envCaptureSym},
	}})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(envCaptureSym): product.FromType(typ.Boolean),
		},
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{
			Symbol: captureSym,
			Value:  product.FromType(typ.String),
		}}),
		FunctionRefs: flow.WithFunctionRef(nil, constraint.NewPath(captureSym, "captured").Key(), flow.FunctionRefSetOf(capturedFn)),
		ClosureRefs:  flow.WithClosureRef(nil, constraint.NewPath(captureSym, "captured").Field("factory").Key(), flow.ClosureRefSetOf(capturedClosure)),
	}

	tr.applyFuncDef(&out, &cfg.FuncDefInfo{
		Symbol:     targetSym,
		FuncExpr:   fn,
		TargetPath: constraint.NewPath(targetSym, "f"),
	})

	set, ok := flow.ClosureRefAt(out.ClosureRefs, constraint.NewPath(targetSym, "f").Key())
	if !ok {
		t.Fatalf("root function definition did not write ClosureRefs: %#v", out.ClosureRefs)
	}
	got, singleton := set.Singleton()
	if !singleton || got.Ref != ref {
		t.Fatalf("root closure refs = %s, want singleton %v", set.Format(), ref)
	}
	if av, ok := got.EntryCells().Value(captureSym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("closure captured cells = %s, want %d:string", got.EntryCells().Format(), captureSym)
	}
	if av, ok := got.EntryCells().Value(envCaptureSym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Boolean) {
		t.Fatalf("closure env-captured cell = %s, want %d:boolean", got.EntryCells().Format(), envCaptureSym)
	}
	if refs, ok := flow.FunctionRefAt(got.EntryFunctionRefs(), constraint.NewPath(captureSym, "captured").Key()); !ok {
		t.Fatalf("closure captured function refs missing: %#v", got.EntryFunctionRefs())
	} else if gotRef, singleton := refs.Singleton(); !singleton || gotRef != capturedFn {
		t.Fatalf("closure captured function refs = %s, want %v", refs.Format(), capturedFn)
	}
	if refs, ok := flow.ClosureRefAt(got.EntryClosureRefs(), constraint.NewPath(captureSym, "captured").Field("factory").Key()); !ok {
		t.Fatalf("closure captured closure refs missing: %#v", got.EntryClosureRefs())
	} else if gotClosure, singleton := refs.Singleton(); !singleton || gotClosure.Ref != capturedClosure.Ref {
		t.Fatalf("closure captured closure refs = %s, want %v", refs.Format(), capturedClosure.Ref)
	}
}

func TestAssignFunctionExprWritesClosureRefEnvironment(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	targetSym := cfg.SymbolID(1101)
	captureSym := cfg.SymbolID(1102)
	ref := flow.FunctionRef{GraphID: 1103, ParentHash: 1104}
	sig := typ.Func().Returns(typ.Number).Build()
	tr := New(input.Inputs{}, Config{FuncTyper: functionRefTestTyper{
		sig:      sig,
		ref:      ref,
		captured: []cfg.SymbolID{captureSym},
	}})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{},
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{
			Symbol: captureSym,
			Value:  product.FromType(typ.Number),
		}}),
	}

	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		IsLocal: true,
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "f",
			Symbol: targetSym,
		}},
		Sources: []ast.Expr{fn},
	}, nil)

	set, ok := flow.ClosureRefAt(out.ClosureRefs, constraint.NewPath(targetSym, "f").Key())
	if !ok {
		t.Fatalf("function expression assignment did not write ClosureRefs: %#v", out.ClosureRefs)
	}
	got, singleton := set.Singleton()
	if !singleton || got.Ref != ref {
		t.Fatalf("assigned closure refs = %s, want singleton %v", set.Format(), ref)
	}
	if av, ok := got.EntryCells().Value(captureSym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("assigned closure captured cells = %s, want %d:number", got.EntryCells().Format(), captureSym)
	}
}

func TestEvalIdentUsesFunctionRefAxisForSolvedSignature(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	sym := cfg.SymbolID(404)
	ident := &ast.IdentExpr{Value: "callee"}
	in.Graph.Bindings().Bind(ident, sym)
	in.Graph.Bindings().SetName(sym, "callee")

	ref := flow.FunctionRef{GraphID: 505, ParentHash: 606}
	raw := typ.Func().Build()
	solved := typ.Func().Returns(typ.Number).Build()
	tr := New(in, Config{CallTyper: functionValueTestTyper{ref: ref, sig: solved}})
	out := flow.PointState{
		Env:          map[flow.ValueKey]product.AbstractValue{flow.SymbolValueKey(sym): product.FromType(raw)},
		FunctionRefs: flow.WithFunctionRef(nil, constraint.NewPath(sym, "callee").Key(), flow.FunctionRefSetOf(ref)),
	}

	got, ok := tr.evalExpr(&out, ident, nil)
	if !ok {
		t.Fatal("identifier did not resolve")
	}
	if !typ.TypeEquals(got.ProjectValue(), solved) {
		t.Fatalf("identifier function value = %v, want solved signature %v", got.ProjectValue(), solved)
	}
}

func TestCapturedIdentUsesCallableProjectionWhenCellValueAbsent(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	sym := cfg.SymbolID(406)
	ident := &ast.IdentExpr{Value: "captured"}
	in.Graph.Bindings().Bind(ident, sym)
	in.Graph.Bindings().SetName(sym, "captured")

	path := constraint.NewPath(sym, "")
	sig := typ.Func().Returns(typ.Number).Build()
	tr := New(in, Config{CallTyper: pathFunctionValueTyper{path: path, sig: sig}})
	if got := tr.symbolStorage.class(sym); got != symbolStorageCapturedCell {
		t.Fatalf("symbol storage class = %v, want captured cell", got)
	}
	out := flow.PointState{}

	got, ok := tr.evalExpr(&out, ident, nil)
	if !ok || !typ.TypeEquals(got.ProjectValue(), sig) {
		t.Fatalf("captured eval ident = %v/%v, want %v/true", got.ProjectValue(), ok, sig)
	}
	projected, ok := tr.projectExprValue(&out, ident)
	if !ok || !typ.TypeEquals(projected.ProjectValue(), sig) {
		t.Fatalf("captured projected ident = %v/%v, want %v/true", projected.ProjectValue(), ok, sig)
	}
}

func TestFunctionValueForPathUsesPathCallableProjectionWhenFunctionRefsAbsent(t *testing.T) {
	sym := cfg.SymbolID(405)
	sig := typ.Func().Returns(typ.Number).Build()
	path := constraint.NewPath(sym, "")
	tr := New(input.Inputs{}, Config{CallTyper: pathFunctionValueTyper{path: path, sig: sig}})
	out := flow.PointState{}

	got, ok := tr.functionValueForPath(&out, path)
	if !ok {
		t.Fatal("function path did not resolve")
	}
	if !typ.TypeEquals(got, sig) {
		t.Fatalf("function path type = %v, want %v", got, sig)
	}
}

func TestFunctionValueForPathPassesClosureRefsAxis(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	sym := cfg.SymbolID(414)
	ref := flow.FunctionRef{GraphID: 515, ParentHash: 616}
	path := constraint.NewPath(sym, "callee")
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 717}, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom())
	closures := flow.WithClosureRef(nil, path.Field("factory").Key(), flow.ClosureRefSetOf(closure))
	provider := &capturingFunctionValueTyper{ref: ref, sig: typ.Func().Returns(typ.Boolean).Build()}
	tr := New(in, Config{CallTyper: provider})
	out := flow.PointState{
		FunctionRefs: flow.WithFunctionRef(nil, path.Key(), flow.FunctionRefSetOf(ref)),
		ClosureRefs:  closures,
	}

	got, ok := tr.functionValueForPath(&out, path)
	if !ok || !typ.TypeEquals(got, provider.sig) {
		t.Fatalf("function value = %v, %v; want %v, true", got, ok, provider.sig)
	}
	if !flow.ClosureRefsDomain.Equal(provider.closures, closures) {
		t.Fatalf("provider closure refs = %#v, want %#v", provider.closures, closures)
	}
}

func TestAssignCallRebasesReturnFunctionRefsToTarget(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	targetSym := cfg.SymbolID(707)
	callee := &ast.IdentExpr{Value: "make"}
	call := &ast.FuncCallExpr{Func: callee}
	ref := flow.FunctionRef{GraphID: 808, ParentHash: 909}
	tr := New(in, Config{CallTyper: returnFunctionRefsTestTyper{ref: ref}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		IsLocal: true,
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "chain",
			Symbol: targetSym,
		}},
		Sources: []ast.Expr{call},
	}, nil)

	refs, ok := flow.FunctionRefAt(out.FunctionRefs, constraint.NewPath(targetSym, "chain").Field("with_options").Key())
	if !ok {
		t.Fatalf("call assignment did not rebase return function refs: %#v", out.FunctionRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got != ref {
		t.Fatalf("rebased function refs = %s, want singleton %v", refs.Format(), ref)
	}
}

func TestSetMetatableAssignmentPublishesPrototypeMethodFunctionRefs(t *testing.T) {
	const (
		targetSym = cfg.SymbolID(730)
		metaSym   = cfg.SymbolID(731)
		protoSym  = cfg.SymbolID(732)
		setSym    = cfg.SymbolID(733)
	)
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil, "setmetatable")
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	set := &ast.IdentExpr{Value: "setmetatable"}
	meta := &ast.IdentExpr{Value: "mt"}
	in.Graph.Bindings().Bind(set, setSym)
	in.Graph.Bindings().SetKind(setSym, cfg.SymbolGlobal)
	in.Graph.Bindings().SetName(setSym, "setmetatable")
	in.Graph.Bindings().Bind(meta, metaSym)
	in.Graph.Bindings().SetName(metaSym, "mt")

	methodRef := flow.FunctionRef{GraphID: 734, ParentHash: 735}
	method := constraint.Segment{Kind: constraint.SegmentField, Name: "run"}
	tr := New(in, Config{
		MetatableIndexes: []metatable.Index{{
			MetatableSym: metaSym,
			PrototypeSym: protoSym,
		}},
		PrototypeMethods: []metatable.PrototypeMethod{{
			PrototypeSym: protoSym,
			Field:        method,
			FuncRef:      methodRef,
		}},
	})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	call := &ast.FuncCallExpr{
		Func: set,
		Args: []ast.Expr{&ast.TableExpr{}, meta},
	}

	if !tr.applySetMetatableInstanceBinding(&out, call, targetSym) {
		t.Fatal("setmetatable source did not publish prototype instance")
	}
	refs, ok := flow.FunctionRefAt(out.FunctionRefs, constraint.NewPath(targetSym, "obj").Field("run").Key())
	if !ok {
		t.Fatalf("prototype method FunctionRefs missing: %#v", out.FunctionRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got != methodRef {
		t.Fatalf("prototype method refs = %s, want singleton %v", refs.Format(), methodRef)
	}
}

func TestReturnSetMetatablePublishesPrototypeMethodFunctionRefs(t *testing.T) {
	const (
		metaSym  = cfg.SymbolID(740)
		protoSym = cfg.SymbolID(741)
		setSym   = cfg.SymbolID(742)
	)
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil, "setmetatable")
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	set := &ast.IdentExpr{Value: "setmetatable"}
	meta := &ast.IdentExpr{Value: "mt"}
	in.Graph.Bindings().Bind(set, setSym)
	in.Graph.Bindings().SetKind(setSym, cfg.SymbolGlobal)
	in.Graph.Bindings().SetName(setSym, "setmetatable")
	in.Graph.Bindings().Bind(meta, metaSym)
	in.Graph.Bindings().SetName(metaSym, "mt")

	methodRef := flow.FunctionRef{GraphID: 743, ParentHash: 744}
	tr := New(in, Config{
		MetatableIndexes: []metatable.Index{{
			MetatableSym: metaSym,
			PrototypeSym: protoSym,
		}},
		PrototypeMethods: []metatable.PrototypeMethod{{
			PrototypeSym: protoSym,
			Field:        constraint.Segment{Kind: constraint.SegmentField, Name: "run"},
			FuncRef:      methodRef,
		}},
	})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	call := &ast.FuncCallExpr{
		Func: set,
		Args: []ast.Expr{&ast.TableExpr{}, meta},
	}

	tr.applyReturn(&out, 0, &cfg.ReturnInfo{Exprs: []ast.Expr{call}}, nil)

	refs, ok := flow.FunctionRefAt(out.FunctionRefs, constraint.NewPlaceholder(0).Field("run").Key())
	if !ok {
		t.Fatalf("return prototype method FunctionRefs missing: %#v", out.FunctionRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got != methodRef {
		t.Fatalf("return prototype method refs = %s, want singleton %v", refs.Format(), methodRef)
	}
}

func TestPrototypeMetatableValueMaterializesMethodSurface(t *testing.T) {
	const protoSym = cfg.SymbolID(744)
	methodRef := flow.FunctionRef{GraphID: 745, ParentHash: 746}
	methodSig := typ.Func().Returns(typ.String).Build()
	tr := New(input.Inputs{}, Config{
		CallTyper: functionValueTestTyper{ref: methodRef, sig: methodSig},
		PrototypeMethods: []metatable.PrototypeMethod{{
			PrototypeSym: protoSym,
			Field:        constraint.Segment{Kind: constraint.SegmentField, Name: "run"},
			FuncRef:      methodRef,
		}},
	})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(protoSym): product.FromType(typ.NewRecord().Build()),
	}}

	meta, ok := tr.prototypeMetatableValue(&out, protoSym)
	if !ok {
		t.Fatal("prototype metatable value did not resolve")
	}
	instance, ok := product.WithMetatable(product.FromType(typ.NewRecord().Build()), meta)
	if !ok {
		t.Fatal("instance with prototype metatable did not resolve")
	}
	got, ok := querycore.Method(instance.ProjectValue(), "run")
	if !ok {
		t.Fatalf("prototype method not visible through metatable: %v", instance.ProjectValue())
	}
	if !typ.TypeEquals(got, methodSig) {
		t.Fatalf("prototype method type = %v, want %v", got, methodSig)
	}
}

func TestEvalSetMetatableCallUsesPrototypeMethodSurface(t *testing.T) {
	const (
		protoSym = cfg.SymbolID(747)
		setSym   = cfg.SymbolID(748)
	)
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil, "setmetatable")
	set := &ast.IdentExpr{Value: "setmetatable"}
	proto := &ast.IdentExpr{Value: "Store"}
	in.Graph.Bindings().Bind(set, setSym)
	in.Graph.Bindings().SetKind(setSym, cfg.SymbolGlobal)
	in.Graph.Bindings().SetName(setSym, "setmetatable")
	in.Graph.Bindings().Bind(proto, protoSym)
	in.Graph.Bindings().SetName(protoSym, "Store")

	methodRef := flow.FunctionRef{GraphID: 749, ParentHash: 750}
	methodSig := typ.Func().Returns(typ.Number).Build()
	tr := New(in, Config{
		CallTyper: functionValueTestTyper{ref: methodRef, sig: methodSig},
		MetatableIndexes: []metatable.Index{{
			MetatableSym: protoSym,
			PrototypeSym: protoSym,
		}},
		PrototypeMethods: []metatable.PrototypeMethod{{
			PrototypeSym: protoSym,
			Field:        constraint.Segment{Kind: constraint.SegmentField, Name: "get"},
			FuncRef:      methodRef,
		}},
	})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(protoSym): product.FromType(typ.NewRecord().Build()),
	}}
	call := &ast.FuncCallExpr{
		Func: set,
		Args: []ast.Expr{&ast.TableExpr{}, proto},
	}

	instance, ok := tr.evalSetMetatableCall(&out, call, nil)
	if !ok {
		t.Fatal("setmetatable call did not resolve")
	}
	got, ok := querycore.Method(instance.ProjectValue(), "get")
	if !ok {
		t.Fatalf("method get not visible on setmetatable result: %v", instance.ProjectValue())
	}
	if !typ.TypeEquals(got, methodSig) {
		t.Fatalf("method get type = %v, want %v", got, methodSig)
	}
}

func TestEvalSetMetatableCallUsesPrototypeMethodsWhenMetatableValueIsAbsent(t *testing.T) {
	const (
		metaSym  = cfg.SymbolID(753)
		protoSym = cfg.SymbolID(754)
		setSym   = cfg.SymbolID(755)
	)
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil, "setmetatable")
	set := &ast.IdentExpr{Value: "setmetatable"}
	meta := &ast.IdentExpr{Value: "class_mt"}
	in.Graph.Bindings().Bind(set, setSym)
	in.Graph.Bindings().SetKind(setSym, cfg.SymbolGlobal)
	in.Graph.Bindings().SetName(setSym, "setmetatable")
	in.Graph.Bindings().Bind(meta, metaSym)
	in.Graph.Bindings().SetName(metaSym, "class_mt")

	methodRef := flow.FunctionRef{GraphID: 756, ParentHash: 757}
	methodSig := typ.Func().Returns(typ.Boolean).Build()
	tr := New(in, Config{
		CallTyper: functionValueTestTyper{ref: methodRef, sig: methodSig},
		MetatableIndexes: []metatable.Index{{
			MetatableSym: metaSym,
			PrototypeSym: protoSym,
		}},
		PrototypeMethods: []metatable.PrototypeMethod{{
			PrototypeSym: protoSym,
			Field:        constraint.Segment{Kind: constraint.SegmentField, Name: "is_empty"},
			FuncRef:      methodRef,
		}},
	})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	call := &ast.FuncCallExpr{
		Func: set,
		Args: []ast.Expr{&ast.TableExpr{}, meta},
	}

	instance, ok := tr.evalSetMetatableCall(&out, call, nil)
	if !ok {
		t.Fatal("setmetatable call did not resolve from prototype methods")
	}
	got, ok := querycore.Method(instance.ProjectValue(), "is_empty")
	if !ok {
		t.Fatalf("method is_empty not visible on setmetatable result without metatable Env value: %v", instance.ProjectValue())
	}
	if !typ.TypeEquals(got, methodSig) {
		t.Fatalf("method is_empty type = %v, want %v", got, methodSig)
	}
}

func TestEvalAttrGetUsesFunctionRefsWhenStructuralMemberMissing(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	objSym := cfg.SymbolID(750)
	obj := &ast.IdentExpr{Value: "obj"}
	in.Graph.Bindings().Bind(obj, objSym)
	in.Graph.Bindings().SetName(objSym, "obj")

	methodRef := flow.FunctionRef{GraphID: 751, ParentHash: 752}
	solved := typ.Func().Returns(typ.String).Build()
	tr := New(in, Config{CallTyper: functionValueTestTyper{ref: methodRef, sig: solved}})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(objSym): product.FromType(typ.NewRecord().Build()),
		},
		FunctionRefs: flow.WithFunctionRef(nil, constraint.NewPath(objSym, "obj").Field("run").Key(), flow.FunctionRefSetOf(methodRef)),
	}
	attr := &ast.AttrGetExpr{
		Object:    obj,
		Key:       &ast.StringExpr{Value: "run"},
		KeySyntax: ast.AttrKeyDot,
	}

	got, ok := tr.evalExpr(&out, attr, nil)
	if !ok {
		t.Fatal("attr get did not resolve through FunctionRefs")
	}
	inner, optional := typ.SplitNilableFieldType(got.ProjectValue())
	if !optional || !typ.TypeEquals(inner, solved) {
		t.Fatalf("attr get value = %v, want optional solved signature %v?", got.ProjectValue(), solved)
	}
}

func TestContainerFunctionRefWriteUsesPlaceForMixedStaticPath(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	root := &ast.IdentExpr{Value: "registry"}
	rootSym := cfg.SymbolID(709)
	in.Graph.Bindings().Bind(root, rootSym)
	in.Graph.Bindings().SetName(rootSym, root.Value)

	ref := flow.FunctionRef{GraphID: 810, ParentHash: 911}
	sig := typ.Func().Returns(typ.Boolean).Build()
	tr := New(in, Config{FuncTyper: functionRefTestTyper{sig: sig, ref: ref}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(rootSym): product.FromType(typ.NewRecord().Build()),
	}}
	target := cfg.AssignTarget{
		Kind: cfg.TargetIndex,
		Expr: &ast.AttrGetExpr{
			Object: &ast.AttrGetExpr{
				Object:    root,
				Key:       &ast.StringExpr{Value: "handlers"},
				KeySyntax: ast.AttrKeyIndex,
			},
			Key:       &ast.StringExpr{Value: "make"},
			KeySyntax: ast.AttrKeyDot,
		},
	}

	tr.applyContainerWrite(&out, target, fn, nil)

	path := constraint.NewPath(rootSym, "registry").IndexStr("handlers").Field("make")
	refs, ok := flow.FunctionRefAt(out.FunctionRefs, path.Key())
	if !ok {
		t.Fatalf("mixed static path function ref missing at %s: %#v", path.Key(), out.FunctionRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got != ref {
		t.Fatalf("mixed static path refs = %s, want singleton %v", refs.Format(), ref)
	}
}

func TestCellBackedContainerWritePublishesNestedFunctionRefs(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	root := &ast.IdentExpr{Value: "M"}
	rootSym := cfg.SymbolID(711)
	in.Graph.Bindings().Bind(root, rootSym)
	in.Graph.Bindings().SetName(rootSym, root.Value)
	in.Scope.CellSymbols = []cfg.SymbolID{rootSym}

	getFn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	ref := flow.FunctionRef{GraphID: 812, ParentHash: 913}
	sig := typ.Func().Returns(typ.String).Build()
	tr := New(in, Config{FuncTyper: functionRefTestTyper{sig: sig, ref: ref}})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{},
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{
			Symbol: rootSym,
			Value:  product.FromType(typ.NewRecord().Build()),
		}}),
	}
	target := cfg.AssignTarget{
		Kind:       cfg.TargetField,
		BaseName:   "M",
		BaseSymbol: rootSym,
		FieldPath:  []string{"dep"},
	}
	src := &ast.TableExpr{Fields: []*ast.Field{{
		Key:       &ast.StringExpr{Value: "get"},
		KeySyntax: ast.AttrKeyDot,
		Value:     getFn,
	}}}

	tr.applyContainerWrite(&out, target, src, nil)

	if _, ok := out.Env[flow.SymbolValueKey(rootSym)]; ok {
		t.Fatalf("cell-backed write stored root in Env[%s]", flow.SymbolValueKey(rootSym))
	}
	if got, ok := out.Cells.Value(rootSym); !ok {
		t.Fatal("cell-backed write did not update root cell")
	} else if dep, ok := product.FieldOf(got, "dep"); !ok || dep.IsZero() {
		t.Fatalf("cell-backed write missing dep field in cell: %v", got.ProjectValue())
	}
	path := constraint.NewPath(rootSym, "M").Field("dep").Field("get")
	refs, ok := flow.FunctionRefAt(out.FunctionRefs, path.Key())
	if !ok {
		t.Fatalf("cell-backed nested function ref missing at %s: %#v", path.Key(), out.FunctionRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got != ref {
		t.Fatalf("cell-backed nested refs = %s, want singleton %v", refs.Format(), ref)
	}
}

func TestAssignCallReturnFunctionRefsUsesProductArgEvidence(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"arg"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one parameter")
	}
	arg := &ast.IdentExpr{Value: "arg"}
	in.Graph.Bindings().Bind(arg, in.Scope.ParamSymbols[0])
	in.Graph.Bindings().SetName(in.Scope.ParamSymbols[0], "arg")

	targetSym := cfg.SymbolID(710)
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "make"},
		Args: []ast.Expr{arg},
	}
	ref := flow.FunctionRef{GraphID: 811, ParentHash: 912}
	typer := &productReturnFunctionRefsTestTyper{ref: ref}
	tr := New(in, Config{CallTyper: typer})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		IsLocal: true,
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "chain",
			Symbol: targetSym,
		}},
		Sources: []ast.Expr{call},
	}, nil)

	if len(typer.args) != 1 || typer.args[0].IsZero() || !typer.args[0].IsGradualTop() {
		t.Fatalf("product call-entry args = %#v, want one gradual-top product value", typer.args)
	}
	refs, ok := flow.FunctionRefAt(out.FunctionRefs, constraint.NewPath(targetSym, "chain").Field("with_options").Key())
	if !ok {
		t.Fatalf("call assignment did not rebase product return function refs: %#v", out.FunctionRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got != ref {
		t.Fatalf("rebased product function refs = %s, want singleton %v", refs.Format(), ref)
	}
}

func TestContainerCallRebasesReturnFunctionRefsToStaticPlace(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	rootSym := cfg.SymbolID(715)
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "make"}}
	ref := flow.FunctionRef{GraphID: 816, ParentHash: 917}
	tr := New(in, Config{CallTyper: returnFunctionRefsTestTyper{ref: ref}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(rootSym): product.FromType(typ.NewRecord().SetOpen(true).Build()),
	}}

	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{
			Kind:       cfg.TargetField,
			BaseName:   "registry",
			BaseSymbol: rootSym,
			FieldPath:  []string{"chain"},
		}},
		Sources: []ast.Expr{call},
	}, nil)

	path := constraint.NewPath(rootSym, "registry").Field("chain").Field("with_options")
	refs, ok := flow.FunctionRefAt(out.FunctionRefs, path.Key())
	if !ok {
		t.Fatalf("call assignment did not rebase return function refs to container place: %#v", out.FunctionRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got != ref {
		t.Fatalf("rebased container function refs = %s, want singleton %v", refs.Format(), ref)
	}
}

func TestAssignCallRebasesReturnClosureRefsToTarget(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	targetSym := cfg.SymbolID(720)
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "make"}}
	ref := flow.FunctionRef{GraphID: 821, ParentHash: 922}
	cells := flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(9), Value: product.FromType(typ.Number)}})
	closure := flow.ClosureRefOf(ref, cells, nil)
	tr := New(in, Config{CallTyper: returnClosureRefsTestTyper{closure: closure}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		IsLocal: true,
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Name:   "chain",
			Symbol: targetSym,
		}},
		Sources: []ast.Expr{call},
	}, nil)

	refs, ok := flow.ClosureRefAt(out.ClosureRefs, constraint.NewPath(targetSym, "chain").Field("with_options").Key())
	if !ok {
		t.Fatalf("call assignment did not rebase return closure refs: %#v", out.ClosureRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got.Ref != ref {
		t.Fatalf("rebased closure refs = %s, want singleton %v", refs.Format(), ref)
	}
	if av, ok := got.EntryCells().Value(cfg.SymbolID(9)); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("rebased closure env = %s, want captured number", got.EntryCells().Format())
	}
}

func TestContainerCallRebasesReturnClosureRefsToStaticPlace(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	rootSym := cfg.SymbolID(721)
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "make"}}
	ref := flow.FunctionRef{GraphID: 822, ParentHash: 923}
	cells := flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.String)}})
	closure := flow.ClosureRefOf(ref, cells, nil)
	tr := New(in, Config{CallTyper: returnClosureRefsTestTyper{closure: closure}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(rootSym): product.FromType(typ.NewRecord().SetOpen(true).Build()),
	}}

	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{
			Kind:       cfg.TargetField,
			BaseName:   "registry",
			BaseSymbol: rootSym,
			FieldPath:  []string{"chain"},
		}},
		Sources: []ast.Expr{call},
	}, nil)

	path := constraint.NewPath(rootSym, "registry").Field("chain").Field("with_options")
	refs, ok := flow.ClosureRefAt(out.ClosureRefs, path.Key())
	if !ok {
		t.Fatalf("call assignment did not rebase return closure refs to container place: %#v", out.ClosureRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton || got.Ref != ref {
		t.Fatalf("rebased container closure refs = %s, want singleton %v", refs.Format(), ref)
	}
	if av, ok := got.EntryCells().Value(cfg.SymbolID(10)); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("rebased container closure env = %s, want captured string", got.EntryCells().Format())
	}
}

type functionRefTestTyper struct {
	sig      *typ.Function
	ref      flow.FunctionRef
	captured []cfg.SymbolID
}

func (t functionRefTestTyper) FuncType(*ast.FunctionExpr) *typ.Function {
	return t.sig
}

func (t functionRefTestTyper) MethodFuncType(*cfg.FuncDefInfo) *typ.Function {
	return t.sig
}

func (t functionRefTestTyper) FuncRef(*ast.FunctionExpr) (flow.FunctionRef, bool) {
	return t.ref, true
}

func (t functionRefTestTyper) MethodFuncRef(*cfg.FuncDefInfo) (flow.FunctionRef, bool) {
	return t.ref, true
}

func (t functionRefTestTyper) CapturedSymbols(flow.FunctionRef) []cfg.SymbolID {
	return append([]cfg.SymbolID(nil), t.captured...)
}

type functionValueTestTyper struct {
	captureEffectTyper
	ref flow.FunctionRef
	sig typ.Type
}

func (t functionValueTestTyper) FunctionValueByRef(ref flow.FunctionRef, _ flow.CaptureCells, _ flow.FunctionRefs, _ flow.ClosureRefs) (typ.Type, bool) {
	if ref != t.ref {
		return nil, false
	}
	return t.sig, true
}

type capturingFunctionValueTyper struct {
	captureEffectTyper
	ref      flow.FunctionRef
	sig      typ.Type
	closures flow.ClosureRefs
}

func (t *capturingFunctionValueTyper) FunctionValueByRef(ref flow.FunctionRef, _ flow.CaptureCells, _ flow.FunctionRefs, closures flow.ClosureRefs) (typ.Type, bool) {
	t.closures = closures
	if ref != t.ref {
		return nil, false
	}
	return t.sig, true
}

type pathFunctionValueTyper struct {
	captureEffectTyper
	path constraint.Path
	sig  typ.Type
}

func (t pathFunctionValueTyper) FunctionValueAtPath(path constraint.Path, _ flow.CaptureCells, _ flow.FunctionRefs, _ flow.ClosureRefs) (typ.Type, bool) {
	if path.Key() != t.path.Key() {
		return nil, false
	}
	return t.sig, true
}

type returnFunctionRefsTestTyper struct {
	captureEffectTyper
	ref flow.FunctionRef
}

func (t returnFunctionRefsTestTyper) CallReturns(*ast.FuncCallExpr, []typ.Type, func(ast.Expr) typ.Type, flow.CaptureCells, flow.FunctionRefs) ([]typ.Type, bool) {
	return []typ.Type{typ.NewRecord().Field("with_options", typ.Func().Build()).Build()}, true
}

func (t returnFunctionRefsTestTyper) CallReturnFunctionRefs(*ast.FuncCallExpr, func(ast.Expr) typ.Type, flow.CaptureCells, flow.FunctionRefs) []flow.FunctionRefs {
	return []flow.FunctionRefs{
		flow.WithFunctionRef(nil, constraint.NewPlaceholder(0).Field("with_options").Key(), flow.FunctionRefSetOf(t.ref)),
	}
}

type productReturnFunctionRefsTestTyper struct {
	captureEffectTyper
	ref  flow.FunctionRef
	args []product.AbstractValue
}

func (t *productReturnFunctionRefsTestTyper) CallReturnFunctionRefsFromValues(
	_ *ast.FuncCallExpr,
	ctx ProductCallContext,
) []flow.FunctionRefs {
	t.args = append([]product.AbstractValue(nil), ctx.ArgValues...)
	return []flow.FunctionRefs{
		flow.WithFunctionRef(nil, constraint.NewPlaceholder(0).Field("with_options").Key(), flow.FunctionRefSetOf(t.ref)),
	}
}

var _ productCallReturnFunctionRefsProvider = (*productReturnFunctionRefsTestTyper)(nil)

type returnClosureRefsTestTyper struct {
	captureEffectTyper
	closure flow.ClosureRef
}

func (t returnClosureRefsTestTyper) CallReturns(*ast.FuncCallExpr, []typ.Type, func(ast.Expr) typ.Type, flow.CaptureCells, flow.FunctionRefs) ([]typ.Type, bool) {
	return []typ.Type{typ.NewRecord().Field("with_options", typ.Func().Build()).Build()}, true
}

func (t returnClosureRefsTestTyper) CallReturnClosureRefsFromValues(*ast.FuncCallExpr, ProductCallContext) []flow.ClosureRefs {
	return []flow.ClosureRefs{
		flow.WithClosureRef(nil, constraint.NewPlaceholder(0).Field("with_options").Key(), flow.ClosureRefSetOf(t.closure)),
	}
}

var _ productCallReturnClosureRefsProvider = returnClosureRefsTestTyper{}
