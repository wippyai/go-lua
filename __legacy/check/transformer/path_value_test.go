package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLowerBoundaryPathValueUsesCanonicalDescendantRead(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	root := Root{Kind: RootParam, Index: 0}
	ownerTerm := arena.Root(root)
	base := arena.Path(root)
	lexical := pathdom.NewPath(symbol.ID(41), "self").Field("nodes").IndexStr("wanted")
	valueTerm, pathTerm, err := arena.LowerBoundaryPathValue(lexical, BoundaryPathBinding{
		Symbol: 41, Root: root, Base: base, Owner: ownerTerm,
	})
	if err != nil {
		t.Fatal(err)
	}
	if valueTerm == 0 || pathTerm == 0 {
		t.Fatalf("lowered terms = %d/%d", valueTerm, pathTerm)
	}
	if again, againPath, err := arena.LowerBoundaryPathValue(lexical, BoundaryPathBinding{Symbol: 41, Root: root, Base: base, Owner: ownerTerm}); err != nil || again != valueTerm || againPath != pathTerm {
		t.Fatalf("lowering was not canonical: %d/%d, %v", again, againPath, err)
	}

	basePath := pathdom.NewPath(symbol.ID(901), "caller_graph")
	cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{product.Top()}, []pathdom.Path{basePath})
	if err != nil {
		t.Fatal(err)
	}
	gotPath, ok := arena.evalPath(pathTerm, cursor)
	wantPath := basePath.Field("nodes").IndexStr("wanted")
	if !ok || !gotPath.Equal(wantPath) {
		t.Fatalf("descendant path = %s/%v, want %s", gotPath, ok, wantPath)
	}
	if _, ok := arena.evalValue(valueTerm, cursor, SpecializationContext{}); ok {
		t.Fatal("descendant read bypassed the factor-native guarded resolver")
	}
}

func TestLowerBoundaryPathValueMakesSubstitutableSingleMemberPathMandatory(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	root := Root{Kind: RootParam, Index: 0}
	value, _, err := arena.LowerBoundaryPathValue(
		pathdom.NewPath(symbol.ID(42), "provider").Field("send"),
		BoundaryPathBinding{Symbol: 42, Root: root, Base: arena.Path(root), Owner: arena.Root(root)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if node := arena.values[value]; node.op != valueDynamicRead || node.path == 0 {
		t.Fatalf("formal member read is not path-owning: %s", arena.canonicalValue(value))
	}
}

func TestLowerBoundaryRequiredPathValueUsesMandatoryCarrierReads(t *testing.T) {
	reg := standard.Registry()
	for _, tc := range []struct {
		name    string
		carrier Root
	}{
		{name: "param", carrier: Root{Kind: RootParam, Index: 0}},
		{name: "capture", carrier: Root{Kind: RootCapture, Index: 1}},
		{name: "global", carrier: Root{Kind: RootGlobal, Index: 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			carrier := tc.carrier
			arena := NewArena(reg)
			id := symbol.ID(70 + carrier.Kind)
			owner := arena.bindEnvironmentSymbol(id)
			binding := BoundaryPathBinding{Symbol: id, Base: arena.EnvironmentPath(id), Owner: owner, Point: 9}

			rootPath := pathdom.NewPath(id, "carrier")
			rootValue, rootTerm, err := arena.LowerBoundaryRequiredPathValue(rootPath, binding, carrier)
			if err != nil || rootValue != owner || rootTerm != binding.Base {
				t.Fatalf("root required path = %d/%d/%v, want %d/%d", rootValue, rootTerm, err, owner, binding.Base)
			}
			for _, lexical := range []pathdom.Path{
				rootPath.Field("send"),
				rootPath.Field("client").Field("send"),
			} {
				value, fullPath, err := arena.LowerBoundaryRequiredPathValue(lexical, binding, carrier)
				if err != nil {
					t.Fatal(err)
				}
				node := arena.values[value]
				if node.op != valueDynamicRead || node.path == 0 || node.point != 9 || fullPath == 0 {
					t.Fatalf("required path %s is not a point-owned mandatory read: %s", lexical, arena.canonicalValue(value))
				}
			}
		})
	}
}

func TestLowerBoundaryRequiredPathValueRejectsLocalCarrierWithoutChangingOptionalLowering(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	id := symbol.ID(81)
	owner := arena.bindEnvironmentSymbol(id)
	binding := BoundaryPathBinding{Symbol: id, Base: arena.EnvironmentPath(id), Owner: owner, Point: 4}
	lexical := pathdom.NewPath(id, "local").Field("send")

	if value, path, err := arena.LowerBoundaryRequiredPathValue(lexical, binding, Root{}); err == nil || value != 0 || path != 0 {
		t.Fatalf("local required path = %d/%d/%v, want closed rejection", value, path, err)
	}
	value, _, err := arena.LowerBoundaryPathValue(lexical, binding)
	if err != nil {
		t.Fatal(err)
	}
	if node := arena.values[value]; node.op != valueDynamicTableRead {
		t.Fatalf("ordinary local path lost optional lowering: %s", arena.canonicalValue(value))
	}
}

func TestLowerBoundaryPathValueRejectsNonCanonicalBindings(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	root := Root{Kind: RootParam, Index: 0}
	owner := arena.Root(root)
	base := arena.Path(root)
	tests := []struct {
		name    string
		path    pathdom.Path
		binding BoundaryPathBinding
	}{
		{name: "versioned", path: pathdom.Path{Root: "x", Symbol: 1, Version: 2}, binding: BoundaryPathBinding{Symbol: 1, Root: root, Base: base, Owner: owner}},
		{name: "wrong symbol", path: pathdom.NewPath(symbol.ID(1), "x").Field("y"), binding: BoundaryPathBinding{Symbol: 2, Root: root, Base: base, Owner: owner}},
		{name: "missing owner", path: pathdom.NewPath(symbol.ID(1), "x").Field("y"), binding: BoundaryPathBinding{Symbol: 1, Root: root, Base: base}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if value, path, err := arena.LowerBoundaryPathValue(tc.path, tc.binding); err == nil || value != 0 || path != 0 {
				t.Fatalf("lowering = %d/%d/%v, want closed failure", value, path, err)
			}
		})
	}
}

func TestLowerBoundaryDynamicReadValueRequiresFactorNativeResolver(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	tableRoot := Root{Kind: RootParam, Index: 0}
	keyRoot := Root{Kind: RootParam, Index: 1}
	tableOwner := arena.Root(tableRoot)
	tableBase := arena.Path(tableRoot)
	keyTerm := arena.Root(keyRoot)
	tablePath := pathdom.NewPath(symbol.ID(51), "self").Field("references")
	read, retainedPath, err := arena.LowerBoundaryDynamicReadValue(tablePath, BoundaryPathBinding{
		Symbol: 51, Root: tableRoot, Base: tableBase, Owner: tableOwner,
	}, keyTerm)
	if err != nil {
		t.Fatal(err)
	}

	callerPath := pathdom.NewPath(symbol.ID(951), "graph")
	keyValue := typevalue.LiteralString(reg, "route")
	cursor, err := NewBindingCursor(Shape{Params: 2}, []product.Value{product.Top(), keyValue}, []pathdom.Path{callerPath, pathdom.NewPath(symbol.ID(952), "name")})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := arena.evalValue(read, cursor, SpecializationContext{}); ok {
		t.Fatal("dynamic descendant read bypassed the factor-native guarded resolver")
	}
	gotPath, ok := arena.evalPath(retainedPath, cursor)
	if !ok || !gotPath.Equal(callerPath.Field("references")) {
		t.Fatalf("retained table path = %s/%v", gotPath, ok)
	}
}
