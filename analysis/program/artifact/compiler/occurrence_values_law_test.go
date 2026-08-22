package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

// A Values occurrence has one owner-issued geometry row. This pins the
// path-bearing append used for executable Values terms and rejects a second
// append for the same canonical occurrence identity.
func TestValuesOccurrencePublishesOneIssuedGeometry(t *testing.T) {
	input, err := lower.Lower(lower.Source{Name: "values-occurrence-geometry.lua", Text: []byte(`return {}`)})
	if err != nil {
		t.Fatal(err)
	}
	transaction := compiler{input: input, key: testCompileKey(t, input), issuanceRows: programissuance.NewBuilder()}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		t.Fatalf("index point attachments: %v", failure)
	}
	if failure := transaction.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %v", failure)
	}
	authored := input.Flow().Authored().Values()
	term, termOK := authored.At(0)
	if !termOK {
		t.Fatal("Values denominator omitted its first term")
	}
	row := transaction.publication.Values[0]
	span, spanOK := input.Span(term)
	finish, finishOK := span.Finish()
	paths := input.Flow().LocalWTO().PointPathsForSite(finish)
	if !spanOK || !finishOK || !paths.Available() {
		t.Fatal("Values term did not retain its owner-issued finish geometry")
	}
	baselineOccurrences := len(transaction.publication.Occurrences)
	if !transaction.appendOccurrencePaths(programschema.OccurrenceValues, row.ID(), row.BodyPathID(), causal.SitePointPaths{}, paths, nil, 0) {
		t.Fatal("first Values occurrence append refused")
	}
	if got := len(transaction.publication.Occurrences) - baselineOccurrences; got != 1 {
		t.Fatalf("Values occurrence increment = %d, want one", got)
	}
	entry, finishPoints, geometryOK := transaction.issuanceRows.Geometry(programschema.OccurrenceValues, row.ID())
	if !geometryOK || len(entry) != 0 || len(finishPoints) != paths.Count() {
		t.Fatalf("issued Values geometry = entry:%d finish:%d/%d available:%v", len(entry), len(finishPoints), paths.Count(), geometryOK)
	}
	if transaction.appendOccurrencePaths(programschema.OccurrenceValues, row.ID(), row.BodyPathID(), causal.SitePointPaths{}, paths, nil, 0) {
		t.Fatal("duplicate Values occurrence geometry was accepted")
	}
}
