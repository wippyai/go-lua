package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestBoundaryReachabilityProgramPathConeIdentityHeapFixedPoint(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	root := keys.FromPath(pathdom.Path{Symbol: 801, Version: 1})
	child := keys.FromPath(pathdom.Path{Symbol: 801, Version: 1}.Field("child"))
	alias := keys.FromPath(pathdom.Path{Symbol: 802, Version: 1})
	suffix, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	if !ok {
		t.Fatal("heap suffix")
	}
	outer := identity.ID{Kind: "table", Site: "outer", Index: 1}
	inner := identity.ID{Kind: "table", Site: "inner", Index: 2}
	builder := newBoundaryReachabilityProgramBuilder(reg, keys)
	builder.pathCone(false, child, alias)
	builder.addValue(identityvalue.Present(reg, outer))
	builder.identity(identity.ConcreteTerm(outer))
	builder.addHeapSuffix(identity.ConcreteTerm(outer), suffix)
	builder.addValue(identityvalue.Present(reg, inner))
	builder.identity(identity.ConcreteTerm(inner))
	program, err := builder.seal()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := SealBoundaryFactorSelection(keys, []BoundaryFactorRoot{{Path: root}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := program.Close(selection, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !closed.closure.ContainsPath(child) || !closed.closure.ContainsPath(alias) ||
		!closed.closure.ContainsIdentity(outer) || !closed.closure.ContainsIdentity(inner) ||
		!closed.closure.ContainsHeapSuffix(outer, suffix) {
		t.Fatalf("closure = %#v", closed.closure)
	}
}

func TestBoundaryReachabilityProgramVersionInsensitiveAndAllIdentities(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	visible := keys.FromPath(pathdom.Path{Symbol: symbol.ID(811), Version: 7})
	unversioned := keys.FromPath(pathdom.Path{Symbol: symbol.ID(811)})
	companion := keys.FromPath(pathdom.Path{Symbol: symbol.ID(812)})
	selected := identity.ID{Kind: "table", Site: "all", Index: 1}
	builder := newBoundaryReachabilityProgramBuilder(reg, keys)
	builder.pathCone(true, unversioned, companion)
	builder.allIdentities()
	builder.addIdentityTerm(identity.ConcreteTerm(selected))
	program, err := builder.seal()
	if err != nil {
		t.Fatal(err)
	}
	selection, _ := SealBoundaryFactorSelection(keys, []BoundaryFactorRoot{{Path: visible}}, nil, false)
	closed, err := program.Close(selection, []product.Value{product.Top()})
	if err != nil {
		t.Fatal(err)
	}
	if !closed.closure.ContainsPath(unversioned) || !closed.closure.ContainsPath(companion) ||
		!closed.closure.allIdentities || !closed.closure.ContainsIdentity(selected) {
		t.Fatalf("closure = %#v", closed.closure)
	}
}

func TestBoundaryReachabilityProgramSetSharesOneFixedPoint(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	root := keys.FromPath(pathdom.Path{Symbol: 821, Version: 1})
	related := keys.FromPath(pathdom.Path{Symbol: 822, Version: 1})
	object := identity.ID{Kind: "table", Site: "set", Index: 1}

	paths := newBoundaryReachabilityProgramBuilder(reg, keys)
	paths.pathCone(false, root, related)
	paths.addValue(identityvalue.Present(reg, object))
	pathProgram, err := paths.seal()
	if err != nil {
		t.Fatal(err)
	}
	identities := newBoundaryReachabilityProgramBuilder(reg, keys)
	identities.identity(identity.ConcreteTerm(object))
	identities.addPath(root)
	identityProgram, err := identities.seal()
	if err != nil {
		t.Fatal(err)
	}
	programs, err := SealBoundaryReachabilityProgramSet(identityProgram, pathProgram)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := SealBoundaryFactorSelection(keys, []BoundaryFactorRoot{{Path: related}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := programs.Close(selection, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !closed.closure.ContainsPath(root) || !closed.closure.ContainsIdentity(object) {
		t.Fatalf("closure = %#v", closed.closure)
	}
}
