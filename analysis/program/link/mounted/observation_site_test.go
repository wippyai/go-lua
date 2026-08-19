package mounted

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

func TestObservationSiteRejectsUnprovenGeometry(t *testing.T) {
	id := orderLawID("site")
	span := programsource.Span{File: "main.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	if !(ObservationSite{Mount: id, Local: id, Kind: structure.DiagnosticObservationTypeReferenceUnresolved, Location: span}).Available() {
		t.Fatal("unresolved reference site with no producers was rejected")
	}
	if (ObservationSite{Mount: id, Local: id, Kind: structure.DiagnosticObservationTypeReferenceUnresolved, Location: span, ValueID: id}).Available() {
		t.Fatal("unresolved reference site carried a value identity")
	}
	producer := BranchProducer{Key: "value.eq", Occurrence: id, Point: id, Anchor: id, ValueID: id}
	if (ObservationSite{Mount: id, Local: id, Kind: structure.DiagnosticObservationBranchCondition, Location: span, producers: []BranchProducer{producer}}).Available() {
		t.Fatal("branch site without a value identity was admitted")
	}
	if !(ObservationSite{Mount: id, Local: id, Kind: structure.DiagnosticObservationBranchCondition, Location: span, ValueID: id, producers: []BranchProducer{producer}}).Available() {
		t.Fatal("branch site with producer geometry and value identity was rejected")
	}
	if (ObservationSite{Mount: id, Local: id, Kind: structure.DiagnosticObservationTypeConformance, Location: span}).Available() {
		t.Fatal("conformance site without a value identity was admitted")
	}
	if (ObservationSite{Mount: identity.ContentID{}, Local: id, Kind: structure.DiagnosticObservationTypeReferenceUnresolved, Location: span}).Available() {
		t.Fatal("site with unavailable mount identity was admitted")
	}
}
