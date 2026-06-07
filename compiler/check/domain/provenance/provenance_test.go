package provenance

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/domain/typepath"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRouteSourceTypeIdentityUsesBodyContractEvidence(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(11), "source")
	var reads []SourceReadKind
	got := RouteSourceType(flow.ProvenanceRoute{
		Kind:   flow.ProvenanceRouteIdentityAlias,
		Source: source,
	}, nil, func(path constraint.Path, read SourceReadKind) typ.Type {
		if !path.Equal(source) {
			t.Fatalf("resolver path = %v, want %v", path, source)
		}
		reads = append(reads, read)
		if read == SourceReadBodyContract {
			return typ.String
		}
		return typ.Number
	}, testProjectSegments)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RouteSourceType(identity) = %v, want string", got)
	}
	if !reflect.DeepEqual(reads, []SourceReadKind{SourceReadBodyContract}) {
		t.Fatalf("reads = %v, want body-contract only", reads)
	}
}

func TestRouteSourceTypeIndexedIteratorComposesRemainder(t *testing.T) {
	sourcePath := constraint.NewPath(cfg.SymbolID(12), "items")
	sourceType := typ.NewArray(typ.NewRecord().Field("payload", typ.String).Build())
	got := RouteSourceType(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteIndexedIterator,
		Source:   sourcePath,
		VarIndex: 1,
		Remainder: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "payload"},
		},
	}, nil, func(path constraint.Path, read SourceReadKind) typ.Type {
		if !path.Equal(sourcePath) || read != SourceReadBodyContract {
			t.Fatalf("resolver got (%v, %v), want body contract for %v", path, read, sourcePath)
		}
		return sourceType
	}, testProjectSegments)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RouteSourceType(indexed remainder) = %v, want string", got)
	}
}

func TestRouteSourceTypeKeyedIteratorKeyAndValue(t *testing.T) {
	sourcePath := constraint.NewPath(cfg.SymbolID(13), "by_id")
	sourceType := typ.NewMap(typ.String, typ.Number)
	resolve := func(path constraint.Path, read SourceReadKind) typ.Type {
		if !path.Equal(sourcePath) || read != SourceReadBodyContract {
			t.Fatalf("resolver got (%v, %v), want body contract for %v", path, read, sourcePath)
		}
		return sourceType
	}
	key := RouteSourceType(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteKeyedIterator,
		Source:   sourcePath,
		VarIndex: 0,
	}, nil, resolve, testProjectSegments)
	if !typ.TypeEquals(key, typ.String) {
		t.Fatalf("RouteSourceType(keyed key) = %v, want string", key)
	}
	value := RouteSourceType(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteKeyedIterator,
		Source:   sourcePath,
		VarIndex: 1,
	}, nil, resolve, testProjectSegments)
	if !typ.TypeEquals(value, typ.Number) {
		t.Fatalf("RouteSourceType(keyed value) = %v, want number", value)
	}
}

func TestRouteSourceTypeRejectsUnsupportedSlots(t *testing.T) {
	got := RouteSourceType(flow.ProvenanceRoute{
		Kind:     flow.ProvenanceRouteIndexedIterator,
		Source:   constraint.NewPath(cfg.SymbolID(14), "items"),
		VarIndex: 0,
	}, nil, func(constraint.Path, SourceReadKind) typ.Type {
		return typ.NewArray(typ.String)
	}, testProjectSegments)
	if got != nil {
		t.Fatalf("RouteSourceType(indexed key slot) = %v, want nil", got)
	}
}

func TestRouteSourceTypeAppendSourceFieldPrefersNonAnyEvidence(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(15), "records")
	record := typ.NewRecord().Field("payload", typ.NewRecord().Field("name", typ.String).Build()).Build()
	got := RouteSourceType(flow.ProvenanceRoute{
		Kind:   flow.ProvenanceRouteAppendElementField,
		Source: source,
		SourceField: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "payload"},
		},
		FieldRemainder: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "name"},
		},
	}, nil, func(path constraint.Path, read SourceReadKind) typ.Type {
		if !path.Equal(source) {
			t.Fatalf("resolver path = %v, want %v", path, source)
		}
		if read == SourceReadPointPath {
			return typ.NewArray(typ.Any)
		}
		return typ.NewArray(record)
	}, testProjectSegments)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RouteSourceType(append source-field) = %v, want string", got)
	}
}

func TestRouteSourceTypeAppendRelativeSourceReadsContractThenPointPath(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(16), "source")
	wantPath := source.Field("payload").Field("name")
	var reads []SourceReadKind
	got := RouteSourceType(flow.ProvenanceRoute{
		Kind:   flow.ProvenanceRouteAppendElementField,
		Source: source,
		FieldRemainder: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "payload"},
			{Kind: constraint.SegmentField, Name: "name"},
		},
	}, nil, func(path constraint.Path, read SourceReadKind) typ.Type {
		if !path.Equal(wantPath) {
			t.Fatalf("resolver path = %v, want %v", path, wantPath)
		}
		reads = append(reads, read)
		if read == SourceReadBodyContract {
			return typ.Number
		}
		return typ.String
	}, testProjectSegments)
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("RouteSourceType(append relative) = %v, want number", got)
	}
	if !reflect.DeepEqual(reads, []SourceReadKind{SourceReadBodyContract}) {
		t.Fatalf("reads = %v, want body-contract first and sufficient", reads)
	}
}

func TestRouteSourceTypeGraphFollowsBodyContractRoutes(t *testing.T) {
	local := constraint.NewPath(cfg.SymbolID(21), "local_id")
	source := constraint.NewPath(cfg.SymbolID(22), "source_id")
	var finalized []constraint.Path
	graph := RouteSourceTypeGraph{
		Routes: func(path constraint.Path) []flow.ProvenanceRoute {
			if path.Equal(local) {
				return []flow.ProvenanceRoute{{
					Kind:   flow.ProvenanceRouteIdentityAlias,
					Source: source,
				}}
			}
			return nil
		},
		BodyContractRead: func(path constraint.Path) typ.Type {
			if path.Equal(source) {
				return typ.String
			}
			return nil
		},
		Finalize: func(path constraint.Path, t typ.Type) typ.Type {
			finalized = append(finalized, path)
			return t
		},
		Project: testProjectSegments,
	}

	got := graph.TypeAt(local)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RouteSourceTypeGraph.TypeAt(local) = %v, want string", got)
	}
	if !reflect.DeepEqual(finalized, []constraint.Path{source, local}) {
		t.Fatalf("finalized paths = %v, want source then local", finalized)
	}
}

func TestRouteSourceTypeGraphStopsIdentityCycles(t *testing.T) {
	a := constraint.NewPath(cfg.SymbolID(31), "a")
	b := constraint.NewPath(cfg.SymbolID(32), "b")
	graph := RouteSourceTypeGraph{
		Routes: func(path constraint.Path) []flow.ProvenanceRoute {
			switch {
			case path.Equal(a):
				return []flow.ProvenanceRoute{{
					Kind:   flow.ProvenanceRouteIdentityAlias,
					Source: b,
				}}
			case path.Equal(b):
				return []flow.ProvenanceRoute{{
					Kind:   flow.ProvenanceRouteIdentityAlias,
					Source: a,
				}}
			default:
				return nil
			}
		},
		Project: testProjectSegments,
	}

	if got := graph.TypeAt(a); got != nil {
		t.Fatalf("RouteSourceTypeGraph.TypeAt(cycle) = %v, want nil", got)
	}
}

func testProjectSegments(base typ.Type, segments []constraint.Segment) typ.Type {
	return typepath.TypeAtSegments(base, segments, typepath.Options{MissingFieldAsNil: true})
}
