package engine

import (
	"reflect"
	"strings"
	"testing"
)

func TestSolveDiagnosticRowExportsNoImplementationSite(t *testing.T) {
	row := reflect.TypeOf(SolveDiagnosticRow{})
	for index := 0; index < row.NumField(); index++ {
		field := row.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name := field.Name
		if name == "CallSite" || name == "Reason" || name == "Phase" || name == "Region" || name == "Head" {
			t.Fatalf("SolveDiagnosticRow exports implementation site field %s", name)
		}
		rendered := field.Type.String()
		if strings.Contains(rendered, "RestartCallSite") || strings.Contains(rendered, "RestartReason") || strings.Contains(rendered, "RegionPhase") {
			t.Fatalf("SolveDiagnosticRow exports implementation type %s on %s", rendered, name)
		}
	}
	if _, ok := row.FieldByName("Site"); !ok {
		t.Fatal("SolveDiagnosticRow has no opaque site identity")
	}
}

func TestSolveDiagnosticOptionsClassifyPresentationAndResources(t *testing.T) {
	options := reflect.TypeOf(SolveDiagnosticOptions{})
	if _, ok := options.FieldByName("Flags"); ok {
		t.Fatal("SolveDiagnosticOptions exports mixed collection flags")
	}
	if _, ok := options.FieldByName("MaxRows"); ok {
		t.Fatal("SolveDiagnosticOptions exports mixed resource bound")
	}
	presentation, presentationOK := options.FieldByName("Presentation")
	resources, resourcesOK := options.FieldByName("Resources")
	if !presentationOK || presentation.Type != reflect.TypeOf(SolveDiagnosticPresentation{}) {
		t.Fatal("SolveDiagnosticOptions has no presentation setting")
	}
	if !resourcesOK || resources.Type != reflect.TypeOf(SolveDiagnosticResources{}) {
		t.Fatal("SolveDiagnosticOptions has no resource setting")
	}
	if presentation.Type == resources.Type {
		t.Fatal("presentation and resource settings share a type")
	}
	var zeroPresentation SolveDiagnosticPresentation
	var zeroResources SolveDiagnosticResources
	if !zeroPresentation.Valid() || !zeroResources.Valid() {
		t.Fatal("zero presentation or resource setting rejected")
	}
	if (SolveDiagnosticPresentation{Flags: SolveDiagnosticAll << 1}).Valid() {
		t.Fatal("unknown presentation flag admitted")
	}
	if (SolveDiagnosticResources{MaxRows: -1}).Valid() || (SolveDiagnosticResources{MaxRows: maxSolveDiagnosticMaxRows + 1}).Valid() {
		t.Fatal("unbounded resource setting admitted")
	}
}

func TestSolveDiagnosticRowSitesDistinguishInteriorKeys(t *testing.T) {
	left := solveDiagnosticRowKey{
		revision: 1, kind: SolveDiagnosticKindRestart,
		callSite: solveDiagnosticRestartHeadInterface, reason: solveDiagnosticRestartInterfaceChanged,
		phase: solveDiagnosticRegionAscent, region: 2, head: 3,
	}
	right := left
	right.region = 9
	if solveDiagnosticRowSite(left) == solveDiagnosticRowSite(right) {
		t.Fatal("distinct recurrence identities share a public site")
	}
	if !solveDiagnosticRowSite(left).Available() || !solveDiagnosticRowSite(right).Available() {
		t.Fatal("diagnostic row site is unavailable")
	}
}
