package variant

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLiteralDiscriminantDomainsFindsSharedNestedTag(t *testing.T) {
	created := typetable.NewRecord().
		Field("payload", typetable.NewRecord().
			Field("kind", typ.LiteralString("created")).
			Field("id", typ.String).
			Build()).
		Build()
	deleted := typetable.NewRecord().
		Field("payload", typetable.NewRecord().
			Field("kind", typ.LiteralString("deleted")).
			Field("id", typ.String).
			Build()).
		Build()
	family, cases, ok := OriginCasesOfType(typ.MaterializeUnion([]typ.Type{created, deleted}))
	if !ok || family == 0 || len(cases) != 2 {
		t.Fatalf("OriginCasesOfType = %d/%#v/%v, want two-case family", family, cases, ok)
	}
	domains, ok := LiteralDiscriminantDomainsForCases(cases)
	if !ok || len(domains) != 1 {
		t.Fatalf("LiteralDiscriminantDomainsForCases = %#v/%v, want one domain", domains, ok)
	}
	wantSuffix := []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
		{Kind: segment.SegmentField, Name: "kind"},
	}
	if len(domains[0].Suffix) != len(wantSuffix) {
		t.Fatalf("suffix = %#v, want %#v", domains[0].Suffix, wantSuffix)
	}
	for i := range wantSuffix {
		if domains[0].Suffix[i] != wantSuffix[i] {
			t.Fatalf("suffix = %#v, want %#v", domains[0].Suffix, wantSuffix)
		}
	}
	if len(domains[0].Cases) != 2 ||
		domains[0].Cases[0].Literal.Value != "created" ||
		domains[0].Cases[1].Literal.Value != "deleted" {
		t.Fatalf("cases = %#v, want created/deleted literals", domains[0].Cases)
	}
}

func TestLiteralDiscriminantDomainsIgnoresOptionalTagSlot(t *testing.T) {
	ready := typetable.NewRecord().
		OptField("kind", typ.LiteralString("ready")).
		Field("id", typ.String).
		Build()
	failed := typetable.NewRecord().
		OptField("kind", typ.LiteralString("failed")).
		Field("error", typ.String).
		Build()
	_, cases, ok := OriginCasesOfType(typ.MaterializeUnion([]typ.Type{ready, failed}))
	if !ok || len(cases) != 2 {
		t.Fatalf("OriginCasesOfType = %#v/%v, want two-case family", cases, ok)
	}
	if domains, ok := LiteralDiscriminantDomainsForCases(cases); ok {
		t.Fatalf("LiteralDiscriminantDomainsForCases = %#v/%v, want no required literal domain", domains, ok)
	}
}
