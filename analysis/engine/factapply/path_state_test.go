package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestPathStateAdaptersUseResolvedKeysAndRejectMissingVersion(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	sym := symbol.ID(30)
	resolver := resolverWithVisibleVersion(point, sym, "x")
	targetPath := path.NewPath(sym, "x").Field("field")
	targetPath.Version = 99
	pathKey := path.PathKey("sym30@1.field")
	unversionedPathKey := path.PathKey("sym30.field")
	syntaxVersionPathKey := path.PathKey("sym30@99.field")
	value := presentValue(reg)

	s, ok := writePathAt(reg, state.State{}, resolver, point, targetPath, value)
	if !ok {
		t.Fatal("writePathAt rejected visible version")
	}
	assertPathValue(t, reg, s, pathKey, value)
	assertPathValue(t, reg, s, unversionedPathKey, product.Bottom(reg))
	assertPathValue(t, reg, s, syntaxVersionPathKey, product.Bottom(reg))

	missingResolver := visibility.NewResolver(visibility.NewTable(nil))
	unchanged, ok := writePathAt(reg, s, missingResolver, point, targetPath, absentValue(reg))
	if ok {
		t.Fatal("writePathAt accepted missing visible version")
	}
	assertStateEqual(t, reg, unchanged, s)
}

func TestPathStateAdapterUpdateUsesResolvedKeyAndSkipsUnresolvedPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8)
	sym := symbol.ID(31)
	resolver := resolverWithVisibleVersion(point, sym, "x")
	targetPath := path.NewPath(sym, "x").Field("field")
	pathKey := path.PathKey("sym31@1.field")
	present := presentValue(reg)
	bottom := product.Bottom(reg)

	s := state.State{}.WritePathKey(reg, pathKey, present)
	updated, ok := updatePathAt(reg, s, resolver, point, targetPath, func(got product.Value) product.Value {
		if !product.Equal(reg, got, present) {
			t.Fatalf("updatePathAt read %s, want present", formatValue(reg, got))
		}
		return bottom
	})
	if !ok {
		t.Fatal("updatePathAt rejected visible version")
	}
	assertPathValue(t, reg, updated, pathKey, bottom)
	assertPathValue(t, reg, s, pathKey, present)

	called := false
	unchanged, ok := updatePathAt(reg, s, visibility.NewResolver(visibility.NewTable(nil)), point, targetPath, func(product.Value) product.Value {
		called = true
		return absentValue(reg)
	})
	if ok {
		t.Fatal("updatePathAt accepted missing visible version")
	}
	if called {
		t.Fatal("updatePathAt called transform for unresolved path")
	}
	assertStateEqual(t, reg, unchanged, s)
}

func TestPathStateAdapterInvalidateSubtreeUsesResolvedKeyAndRejectsUnresolvedPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	sym := symbol.ID(42)
	resolver := resolverWithVisibleVersion(point, sym, "obj")
	targetPath := path.NewPath(sym, "obj").Field("field")
	childKey := path.PathKey("sym42@1.field.deep")
	otherKey := path.PathKey("sym42@2.field.deep")
	present := presentValue(reg)
	s := state.State{}.
		WritePathKey(reg, childKey, present).
		WritePathKey(reg, otherKey, present)

	out, ok := invalidatePathSubtreeAt(s, resolver, point, targetPath)
	if !ok {
		t.Fatal("invalidatePathSubtreeAt rejected visible version")
	}
	assertPathValue(t, reg, out, childKey, product.Bottom(reg))
	assertPathValue(t, reg, out, otherKey, present)

	unchanged, ok := invalidatePathSubtreeAt(s, visibility.NewResolver(visibility.NewTable(nil)), point, targetPath)
	if ok {
		t.Fatal("invalidatePathSubtreeAt accepted missing visible version")
	}
	assertStateEqual(t, reg, unchanged, s)
}

func TestPathStateAdapterInvalidateSubtreeDropsEquivalentStaticMemberFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	alias := symbol.ID(43)
	original := symbol.ID(44)
	builder := visibility.NewBuilder()
	builder.Define(point, alias, "alias")
	builder.Define(point, original, "original")
	resolver := visibility.NewResolver(builder.Build())
	aliasPath := path.NewPath(alias, "alias").Field("value")
	aliasKey := path.PathKey("sym43@1.value")
	originalKey := path.PathKey("sym44@1.value")
	present := presentValue(reg)
	in := state.State{}.
		WritePathStaticMember(originalKey, present).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  aliasKey,
			Other: originalKey,
		})

	out, ok := invalidatePathSubtreeAt(in, resolver, point, aliasPath)
	if !ok {
		t.Fatal("invalidatePathSubtreeAt rejected visible alias path")
	}
	if got, ok := out.ReadPathStaticMember(originalKey); ok {
		t.Fatalf("static member %s = %s, want removed through alias invalidation", originalKey, formatValue(reg, got))
	}
	if out.HasBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  aliasKey,
		Other: originalKey,
	}) {
		t.Fatalf("equivalent branch proof survived alias invalidation")
	}
}

func resolverWithVisibleVersion(point cfg.Point, sym symbol.ID, root string) *visibility.Resolver {
	builder := visibility.NewBuilder()
	builder.Define(point, sym, root)
	return visibility.NewResolver(builder.Build())
}
