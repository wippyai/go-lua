package mounted

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

func TestObservationSiteRejectsUnprovenGeometry(t *testing.T) {
	id := orderLawID("site")
	span := programsource.Span{File: "main.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	if !(ObservationSite{Mount: id, Local: id, Kind: programartifact.DiagnosticObservationTypeReferenceUnresolved, Location: span}).Available() {
		t.Fatal("unresolved reference site with no producers was rejected")
	}
	if (ObservationSite{Mount: id, Local: id, Kind: programartifact.DiagnosticObservationBranchCondition, Location: span}).Available() {
		t.Fatal("branch site without producer geometry was admitted")
	}
	if (ObservationSite{Mount: identity.ContentID{}, Local: id, Kind: programartifact.DiagnosticObservationTypeReferenceUnresolved, Location: span}).Available() {
		t.Fatal("site with unavailable mount identity was admitted")
	}
}
