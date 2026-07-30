package state

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestCoordinateFormalStructuralKeyRekeyPreservesNestedSuffixExactly(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	root := from.FromPath(pathdom.NewPath(symbol.ID(771), "root"))
	nested := from.FromPath(pathdom.NewPath(symbol.ID(771), "root").
		Append(segment.Segment{Kind: segment.SegmentField, Name: "left"}).
		Append(segment.Segment{Kind: segment.SegmentIndexString, Name: "right"}))
	formalRoot := formal.NewRoot(owner, 1, formal.Input)
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{{Source: root, Target: formalRoot}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.RekeyStructuralKeyFormal(plan, nested)
	if err != nil {
		t.Fatal(err)
	}
	described, ok := to.DescribeFormalRoot(got)
	if !ok || described != formalRoot {
		t.Fatalf("nested formal root = %#v/%t, want %#v", described, ok, formalRoot)
	}
	segments, ok := to.SegmentsView(got)
	if !ok || len(segments) != 2 || segments[0].Name != "left" || segments[1].Name != "right" {
		t.Fatalf("nested suffix = %#v/%t", segments, ok)
	}
}

func TestCoordinateFormalStructuralKeyRekeyImportsRootlessSuffix(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, ok := from.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	if !ok {
		t.Fatal("source rootless suffix")
	}
	got, err := domain.RekeyStructuralKeyFormal(plan, source)
	if err != nil {
		t.Fatal(err)
	}
	segments, ok := to.SuffixSegmentsView(got)
	if !ok || len(segments) != 1 || segments[0].Name != "member" {
		t.Fatalf("rootless suffix = %#v/%t", segments, ok)
	}
}

func TestCoordinateFormalStructuralKeyRekeyRejectsForeignAuthority(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to, foreignKeys := keyspace.New(), keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	root := from.FromPath(pathdom.NewPath(symbol.ID(772), "root"))
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{{
		Source: root, Target: formal.NewRoot(owner, 1, formal.Input),
	}})
	if err != nil {
		t.Fatal(err)
	}
	foreignDomain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreignDomain.RekeyStructuralKeyFormal(plan, root); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("foreign ProductDomain error = %v", err)
	}
	foreign := foreignKeys.FromPath(pathdom.NewPath(symbol.ID(772), "root"))
	if _, err := domain.RekeyStructuralKeyFormal(plan, foreign); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("foreign keyspace error = %v", err)
	}
	unmapped := from.FromPath(pathdom.NewPath(symbol.ID(773), "unmapped"))
	if _, err := domain.RekeyStructuralKeyFormal(plan, unmapped); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("unmapped root error = %v", err)
	}
}
