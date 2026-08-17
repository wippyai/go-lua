package parserproducts

import "testing"

func TestHelperMetadataClassificationKeepsFiniteExceptionsExplicit(t *testing.T) {
	if !helperMetadataOnly("positionAtEnd") || helperMetadataOnly("unknownHelper") {
		t.Fatal("metadata-only helper classification drifted")
	}
	if !helperDiagnosticOnly("annotationError") || helperDiagnosticOnly("splitNameList") {
		t.Fatal("diagnostic-only helper classification drifted")
	}
	if !helperMapOnly("splitTypedNames") || helperMapOnly("annotationError") {
		t.Fatal("map-only helper classification drifted")
	}
}
