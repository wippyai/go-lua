package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

type sourceValueFunc func(cfg.Point, factflow.ValueSource, state.State, func(cfg.Point) state.State) (product.Value, bool)

func (f sourceValueFunc) ValueOfSource(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	return f(point, source, in, read)
}

func TestSourceValueAtBoundaryCachesSolvedReadModelProjection(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(12), HasExpr: true}
	want := typevalue.FromType(reg, typ.String)
	calls := 0
	result := &Result{
		registry: reg,
		flow: transfer.Result{
			point: state.State{}.WriteValue(reg, statekey.SymbolValue(symbol.ID(9001)), typevalue.FromType(reg, typ.Boolean)),
		},
		sources: sourceValueFunc(func(gotPoint cfg.Point, gotSource factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
			calls++
			if gotPoint != point {
				t.Fatalf("point = %d, want %d", gotPoint, point)
			}
			if gotSource != source {
				t.Fatalf("source = %#v, want %#v", gotSource, source)
			}
			return want, true
		}),
	}

	first, ok := result.SourceValueAtBoundary(point, source)
	if !ok {
		t.Fatal("first SourceValueAtBoundary returned !ok")
	}
	second, ok := result.SourceValueAtBoundary(point, source)
	if !ok {
		t.Fatal("second SourceValueAtBoundary returned !ok")
	}
	if !product.Equal(reg, first, second) || !product.Equal(reg, first, want) {
		t.Fatalf("cached values = %v then %v, want %v", first, second, want)
	}
	if calls != 1 {
		t.Fatalf("source resolver calls = %d, want 1", calls)
	}
}

func TestValueSourcePathParsesUnversionedStructuralSymbolKey(t *testing.T) {
	p := pathdom.NewPath(symbol.ID(40), "bindings").Field("checkpoint")
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	source, ok := factflow.NewPathValueSource(p.Key(), 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewPathValueSource returned false")
	}
	got, ok := valueSourcePath(factflow.NewFacts(factflow.FactsInput{}), visibility.NewResolver(visibility.NewBuilder().Build()), source)
	if !ok || got.Symbol != p.Symbol || got.Version != 0 || !got.EqualIgnoringVersion(p) {
		t.Fatalf("valueSourcePath(%q) = %s/%v, want symbol-rooted %s", source.PathKey, got.String(), ok, p.String())
	}
}

func TestSourceValueBeforeBoundarySkipsUnreachablePoint(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(170), HasExpr: true}
	calls := 0
	result := &Result{
		registry: reg,
		flow: transfer.Result{
			point: state.Domain(reg).Bottom(),
		},
		sources: sourceValueFunc(func(cfg.Point, factflow.ValueSource, state.State, func(cfg.Point) state.State) (product.Value, bool) {
			calls++
			return typevalue.FromType(reg, typ.String), true
		}),
	}

	if got, ok := result.SourceValueBeforeBoundary(point, source); ok {
		t.Fatalf("SourceValueBeforeBoundary = %v, want !ok for unreachable point", got)
	}
	if calls != 0 {
		t.Fatalf("source resolver calls = %d, want 0 for unreachable point", calls)
	}
}

func TestPathValueAtBoundaryCachesSolvedReadModelProjection(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8)
	sym := symbol.ID(24)
	p := pathdom.NewPath(sym, "record")
	want := typevalue.FromType(reg, typ.String)
	st := state.State{}.WriteValue(reg, statekey.SymbolValue(sym), want)
	builder := visibility.NewBuilder()
	builder.Define(point, sym, "record")
	result := &Result{
		registry:   reg,
		visibility: visibility.NewResolver(builder.Build()),
		flow: transfer.Result{
			point: st,
		},
	}

	first, ok := result.PathValueAtBoundary(point, p)
	if !ok {
		t.Fatal("first PathValueAtBoundary returned !ok")
	}
	if got := result.queries.pathValueCount(); got != 1 {
		t.Fatalf("path cache size after first read = %d, want 1", got)
	}
	result.queries.forEachPathValueKey(func(key pathValueCacheKey) bool {
		if got := result.visibility.KeySpace().Format(key.path); got != p.Key() {
			t.Fatalf("path cache structural key = %q, want %q", got, p.Key())
		}
		return true
	})
	second, ok := result.PathValueAtBoundary(point, p)
	if !ok {
		t.Fatal("second PathValueAtBoundary returned !ok")
	}
	if got := result.queries.pathValueCount(); got != 1 {
		t.Fatalf("path cache size after second read = %d, want 1", got)
	}
	if !product.Equal(reg, first, second) || !product.Equal(reg, first, want) {
		t.Fatalf("cached path values = %v then %v, want %v", first, second, want)
	}
}

func TestPathValueAtBoundaryUsesFormalObservationWithoutState(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(81)
	sym := symbol.ID(8101)
	p := pathdom.NewPath(sym, "formal")
	want := typevalue.FromType(reg, typ.String)
	builder := visibility.NewBuilder()
	builder.Define(point, sym, "formal")
	calls := 0
	result := &Result{
		registry:   reg,
		visibility: visibility.NewResolver(builder.Build()),
		formalPathValue: func(gotPoint cfg.Point, gotPath pathdom.Path, boundary bool) (product.Value, bool) {
			calls++
			if gotPoint != point || !gotPath.Equal(p) || boundary {
				t.Fatalf("formal observation = (%d, %s, boundary=%t), want (%d, %s, false)", gotPoint, gotPath, boundary, point, p)
			}
			return want, true
		},
	}

	first, ok := result.PathValueAtBoundary(point, p)
	if !ok || !product.Equal(reg, first, want) {
		t.Fatalf("formal PathValueAtBoundary = %v/%t, want %v/true", first, ok, want)
	}
	second, ok := result.PathValueAtBoundary(point, p)
	if !ok || !product.Equal(reg, second, want) {
		t.Fatalf("cached formal PathValueAtBoundary = %v/%t, want %v/true", second, ok, want)
	}
	if calls != 1 {
		t.Fatalf("formal observation calls = %d, want 1", calls)
	}
}

func TestDistinctPathsShareExactIdentityAtBoundary(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	leftSym := symbol.ID(1901)
	rightSym := symbol.ID(1902)
	otherSym := symbol.ID(1903)
	leftPath := pathdom.NewPath(leftSym, "left")
	rightPath := pathdom.NewPath(rightSym, "right")
	otherPath := pathdom.NewPath(otherSym, "other")
	sharedID := testTableIdentity(19, 1)
	otherID := testTableIdentity(19, 2)
	st := state.State{}.
		WriteValue(reg, statekey.SymbolValue(leftSym), identityvalue.Present(reg, sharedID)).
		WriteValue(reg, statekey.SymbolValue(rightSym), identityvalue.Present(reg, sharedID)).
		WriteValue(reg, statekey.SymbolValue(otherSym), identityvalue.Present(reg, otherID))
	builder := visibility.NewBuilder()
	builder.Define(point, leftSym, "left")
	builder.Define(point, rightSym, "right")
	builder.Define(point, otherSym, "other")
	result := &Result{
		registry:   reg,
		visibility: visibility.NewResolver(builder.Build()),
		flow: transfer.Result{
			point: st,
		},
	}

	if !result.DistinctPathsShareExactIdentityAtBoundary(point, leftPath, rightPath) {
		t.Fatal("DistinctPathsShareExactIdentityAtBoundary(shared identities) = false, want true")
	}
	if result.DistinctPathsShareExactIdentityAtBoundary(point, leftPath, leftPath) {
		t.Fatal("DistinctPathsShareExactIdentityAtBoundary(same path) = true, want false")
	}
	if result.DistinctPathsShareExactIdentityAtBoundary(point, leftPath, otherPath) {
		t.Fatal("DistinctPathsShareExactIdentityAtBoundary(distinct identities) = true, want false")
	}
}

func TestPathsAliasWithSameSuffixAtBoundaryUsesRootIdentity(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(20)
	leftSym := symbol.ID(2001)
	rightSym := symbol.ID(2002)
	leftPath := pathdom.NewPath(leftSym, "left").Field("kind")
	rightPath := pathdom.NewPath(rightSym, "right").Field("kind")
	otherSuffixPath := pathdom.NewPath(rightSym, "right").Field("value")
	sharedID := testTableIdentity(20, 1)
	st := state.State{}.
		WriteValue(reg, statekey.SymbolValue(leftSym), identityvalue.Present(reg, sharedID)).
		WriteValue(reg, statekey.SymbolValue(rightSym), identityvalue.Present(reg, sharedID))
	builder := visibility.NewBuilder()
	builder.Define(point, leftSym, "left")
	builder.Define(point, rightSym, "right")
	result := &Result{
		registry:   reg,
		visibility: visibility.NewResolver(builder.Build()),
		flow: transfer.Result{
			point: st,
		},
	}

	if !result.PathsAliasWithSameSuffixAtBoundary(point, leftPath, rightPath) {
		t.Fatal("PathsAliasWithSameSuffixAtBoundary(shared root identity, same suffix) = false, want true")
	}
	if result.PathsAliasWithSameSuffixAtBoundary(point, leftPath, otherSuffixPath) {
		t.Fatal("PathsAliasWithSameSuffixAtBoundary(shared root identity, different suffix) = true, want false")
	}
}

func TestPathProjectionContextsAreScopedByReadMode(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	sym := symbol.ID(25)
	p := pathdom.NewPath(sym, "record")
	before := typevalue.FromType(reg, typ.Integer)
	boundary := typevalue.FromType(reg, typ.String)

	builder := visibility.NewBuilder()
	builder.Define(point, sym, "record")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(21), HasExpr: true}
	result := &Result{
		registry:   reg,
		visibility: visibility.NewResolver(builder.Build()),
		facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, sym, p, source),
			},
		}),
		flow: transfer.Result{
			point: state.State{}.WriteValue(reg, statekey.SymbolValue(sym), before),
		},
		published: PublishedFacts{nodeOutputs: map[cfg.Point]state.State{
			point: state.State{}.WriteValue(reg, statekey.SymbolValue(sym), boundary),
		}},
	}

	gotBoundary, ok := result.PathValueAtBoundary(point, p)
	if !ok {
		t.Fatal("PathValueAtBoundary returned !ok")
	}
	if !product.Equal(reg, gotBoundary, boundary) {
		t.Fatalf("PathValueAtBoundary = %v, want boundary value %v", gotBoundary, boundary)
	}
	gotBefore, ok := result.PathValueBeforeBoundary(point, p)
	if !ok {
		t.Fatal("PathValueBeforeBoundary returned !ok")
	}
	if !product.Equal(reg, gotBefore, before) {
		t.Fatalf("PathValueBeforeBoundary = %v, want before-boundary value %v", gotBefore, before)
	}
}

func TestSourceValueAtBoundaryDoesNotUseExplanationRecovery(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, use, false)
	graph.AddEdge(use, graph.Exit(), false)

	target := symbol.ID(17)
	declExpr := factflow.ExprRef(31)
	useExpr := factflow.ExprRef(32)
	declSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: declExpr, HasExpr: true}
	useSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: useExpr, HasExpr: true}

	weakUse := typevalue.FromType(reg, typ.Any)
	concreteDeclaration := typevalue.FromType(reg, typ.String)
	result := &Result{
		registry: reg,
		cfg:      &cfgbuild.Result{Graph: graph},
		facts: factflow.NewFacts(factflow.FactsInput{
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				useExpr: pathdom.NewPath(target, "x"),
			},
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				decl: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "x"), declSource),
			},
		}),
		flow: transfer.Result{
			decl: state.State{},
			use:  state.State{},
		},
		sources: sourceValueFunc(func(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
			switch source.ExprRef {
			case declExpr:
				return concreteDeclaration, true
			case useExpr:
				return weakUse, true
			default:
				return product.Value{}, false
			}
		}),
	}

	got, ok := result.SourceValueAtBoundary(use, useSource)
	if !ok {
		t.Fatal("SourceValueAtBoundary returned !ok")
	}
	if product.Equal(reg, got, concreteDeclaration) {
		t.Fatal("SourceValueAtBoundary recovered declaration value; semantic projection must stay solved-state only")
	}
	if !product.Equal(reg, got, weakUse) {
		t.Fatalf("SourceValueAtBoundary = %v, want weak solved source %v", got, weakUse)
	}

	recovered, ok := result.SourceValueForExplanationAtBoundary(use, useSource)
	if !ok {
		t.Fatal("SourceValueForExplanationAtBoundary returned !ok")
	}
	if !product.Equal(reg, recovered, concreteDeclaration) {
		t.Fatalf("declaration recovery = %v, want declaration value %v", recovered, concreteDeclaration)
	}
}

func TestSourceValueForExplanationRecoversDeclarationFromPathBackedRootSource(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, use, false)
	graph.AddEdge(use, graph.Exit(), false)

	target := symbol.ID(17)
	declExpr := factflow.ExprRef(31)
	declSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: declExpr, HasExpr: true}
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	usePath := pathdom.NewPath(target, "x")
	useSource, ok := factflow.NewPathValueSource(usePath.Key(), 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewPathValueSource returned false")
	}

	weakUse := typevalue.FromType(reg, typ.Any)
	concreteDeclaration := typevalue.FromType(reg, typ.String)
	builder := visibility.NewBuilder()
	builder.Define(use, target, "x")
	result := &Result{
		registry:   reg,
		cfg:        &cfgbuild.Result{Graph: graph},
		visibility: visibility.NewResolver(builder.Build()),
		facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				decl: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, usePath, declSource),
			},
		}),
		flow: transfer.Result{
			decl: state.State{},
			use:  state.State{},
		},
		sources: sourceValueFunc(func(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
			switch {
			case source.Kind == factflow.ValueSourceExpression && source.ExprRef == declExpr:
				return concreteDeclaration, true
			case source.Kind == factflow.ValueSourcePath && source.PathKey == usePath.Key():
				return weakUse, true
			default:
				return product.Value{}, false
			}
		}),
	}

	got, ok := result.SourceValueAtBoundary(use, useSource)
	if !ok {
		t.Fatal("SourceValueAtBoundary returned !ok")
	}
	if product.Equal(reg, got, concreteDeclaration) {
		t.Fatal("SourceValueAtBoundary recovered declaration value; semantic projection must stay solved-state only")
	}
	if !product.Equal(reg, got, weakUse) {
		t.Fatalf("SourceValueAtBoundary = %v, want weak solved source %v", got, weakUse)
	}

	recovered, ok := result.SourceValueForExplanationAtBoundary(use, useSource)
	if !ok {
		t.Fatal("SourceValueForExplanationAtBoundary returned !ok")
	}
	if !product.Equal(reg, recovered, concreteDeclaration) {
		t.Fatalf("path-backed declaration recovery = %v, want declaration value %v", recovered, concreteDeclaration)
	}
}

func TestBoundaryStateKeyIsCanonicalTypedPathVocabulary(t *testing.T) {
	point := cfg.Point(9)
	target := symbol.ID(42)
	p := pathdom.NewPath(target, "resource").Field("tx")

	builder := visibility.NewBuilder()
	builder.Define(point, target, "resource")
	resolver := visibility.NewResolver(builder.Build())
	result := &Result{visibility: resolver}

	stateKey, ok := result.StateKeyAtBoundary(point, p)
	if !ok {
		t.Fatal("StateKeyAtBoundary returned !ok")
	}
	wantStateKey, ok := visibility.AddressAt(resolver, point, p).VisibleStateKey()
	if !ok {
		t.Fatal("visibility.Address rejected boundary path")
	}
	if stateKey != wantStateKey {
		t.Fatalf("StateKeyAtBoundary = %q, want visibility.Address visible key %q", stateKey, wantStateKey)
	}

	pathKey, ok := result.PathKeyAtBoundary(point, p)
	if !ok {
		t.Fatal("PathKeyAtBoundary returned !ok")
	}
	if pathKey != stateKey.PathKey() {
		t.Fatalf("PathKeyAtBoundary = %q, want typed state key %q", pathKey, stateKey.PathKey())
	}

	resourceKey, ok := result.TypestateResourceKeyAtBoundary(point, p)
	if !ok {
		t.Fatal("TypestateResourceKeyAtBoundary returned !ok")
	}
	if resourceKey != stateKey {
		t.Fatalf("TypestateResourceKeyAtBoundary = %q, want %q without solved aliases", resourceKey, stateKey)
	}

	if _, ok := result.StateKeyAtBoundary(point, pathdom.Path{}); ok {
		t.Fatal("StateKeyAtBoundary accepted an empty path")
	}
}

func TestBoundaryAddressContextCanonicalizesTypestateResourceAliases(t *testing.T) {
	point := cfg.Point(10)
	txSym := symbol.ID(42)
	aliasSym := symbol.ID(43)
	tx := pathdom.NewPath(txSym, "tx")
	alias := pathdom.NewPath(aliasSym, "alias")

	builder := visibility.NewBuilder()
	builder.Define(point, txSym, "tx")
	builder.Define(point, aliasSym, "alias")
	resolver := visibility.NewResolver(builder.Build())
	txStateKey, ok := visibility.AddressAt(resolver, point, tx).VisibleStateKey()
	if !ok {
		t.Fatal("missing tx state key")
	}
	aliasStateKey, ok := visibility.AddressAt(resolver, point, alias).VisibleStateKey()
	if !ok {
		t.Fatal("missing alias state key")
	}
	txKey, ok := resolver.KeySpace().InternStateKey(txStateKey)
	if !ok {
		t.Fatal("missing tx keyspace key")
	}
	aliasKey, ok := resolver.KeySpace().InternStateKey(aliasStateKey)
	if !ok {
		t.Fatal("missing alias keyspace key")
	}
	st := state.State{}.AddBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  txKey,
		Other: aliasKey,
	})
	result := &Result{
		visibility: resolver,
		flow:       transfer.Result{point: st},
	}

	if !result.PathsEquivalentAtBoundary(point, tx, alias) {
		t.Fatal("PathsEquivalentAtBoundary did not use boundary address equivalence")
	}
	got, ok := result.TypestateResourceKeyAtBoundary(point, alias)
	if !ok {
		t.Fatal("TypestateResourceKeyAtBoundary(alias) returned !ok")
	}
	if got != txStateKey {
		t.Fatalf("canonical resource key = %q, want alias folded to %q", got, txStateKey)
	}
	resource, ok := result.TypestateResourceAtBoundary(point, alias, typestate.Protocol("transaction"))
	if !ok {
		t.Fatal("TypestateResourceAtBoundary(alias) returned !ok")
	}
	if resource.ID != typestate.ResourceID(txStateKey.String()) || resource.Protocol != typestate.Protocol("transaction") {
		t.Fatalf("resource = %#v, want canonical tx key/protocol transaction", resource)
	}
}

func TestTypestateResourceAtCallEntryUsesRootOrVisibleRootForPathBackedSource(t *testing.T) {
	point := cfg.Point(10)
	txSym := symbol.ID(42)
	rootlessTX := pathdom.Path{Symbol: txSym}

	builder := visibility.NewBuilder()
	builder.Define(point, txSym, "tx")
	resolver := visibility.NewResolver(builder.Build())
	result := &Result{
		visibility: resolver,
		flow:       transfer.Result{point: state.State{}},
	}

	boundaryResource, ok := result.TypestateResourceAtBoundary(point, rootlessTX, typestate.Protocol("transaction"))
	if !ok {
		t.Fatal("TypestateResourceAtBoundary returned !ok")
	}
	if boundaryResource.ID != typestate.ResourceID("sym42@1") {
		t.Fatalf("boundary resource = %#v, want visible versioned root", boundaryResource)
	}

	callEntryResource, ok := result.TypestateResourceAtCallEntry(point, rootlessTX, typestate.Protocol("transaction"))
	if !ok {
		t.Fatal("TypestateResourceAtCallEntry returned !ok")
	}
	if callEntryResource.ID != typestate.ResourceID("sym42") {
		t.Fatalf("call-entry resource = %#v, want root-or-visible root", callEntryResource)
	}
}

func genericForVariablePoint(t *testing.T, facts map[cfg.Point]GenericForFact, stmt *ast.GenericForStmt) cfg.Point {
	t.Helper()
	for point, fact := range facts {
		if fact.Stmt == stmt && fact.Role == GenericForRoleVariable {
			return point
		}
	}
	t.Fatalf("no generic-for variable point found for statement")
	return 0
}

func genericForPathSource(t *testing.T, sym symbol.ID, name string) factflow.ValueSource {
	t.Helper()
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	source, ok := factflow.NewPathValueSource(pathdom.NewPath(sym, name).Key(), 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewPathValueSource returned false")
	}
	return source
}

// TestUnresolvedGenericForVariableSourceIsPointSpecific proves
// unresolvedGenericForVariableSource resolves against the generic-for fact
// recorded at its own point, not against any generic-for variable anywhere in
// the body. Two independent generic-for loops each bind a distinct variable at
// their own CFG point; the source resolved for loop A's point must not match
// loop B's variable symbol, and vice versa.
func TestUnresolvedGenericForVariableSourceIsPointSpecific(t *testing.T) {
	stmts, err := parse.ParseString(`
for a in pairs(t1) do
end
for b in pairs(t2) do
end
`, "test")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"pairs", "t1", "t2"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	genericFors := genericForFactsFromSource(bindings, built, stmts)

	loopA := stmts[0].(*ast.GenericForStmt)
	loopB := stmts[1].(*ast.GenericForStmt)

	pointA := genericForVariablePoint(t, genericFors, loopA)
	pointB := genericForVariablePoint(t, genericFors, loopB)

	symA := bindings.GenericForSymbols(loopA)[0]
	symB := bindings.GenericForSymbols(loopB)[0]
	if symA == 0 || symB == 0 || symA == symB {
		t.Fatalf("loop variable symbols = %d, %d, want distinct non-zero symbols", symA, symB)
	}

	result := &Result{
		cfg:         built,
		genericFors: genericFors,
		visibility:  visibility.NewResolver(visibility.NewBuilder().Build()),
	}

	sourceA := genericForPathSource(t, symA, "a")
	sourceB := genericForPathSource(t, symB, "b")

	if !result.unresolvedGenericForVariableSource(pointA, sourceA) {
		t.Fatal("point A did not resolve its own generic-for variable source")
	}
	if !result.unresolvedGenericForVariableSource(pointB, sourceB) {
		t.Fatal("point B did not resolve its own generic-for variable source")
	}
	if result.unresolvedGenericForVariableSource(pointA, sourceB) {
		t.Fatal("point A incorrectly resolved point B's generic-for variable source")
	}
	if result.unresolvedGenericForVariableSource(pointB, sourceA) {
		t.Fatal("point B incorrectly resolved point A's generic-for variable source")
	}
}
