package programdiagnostic

import (
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
)

// The diagnostic plane owns the three canonical manifest definitions. Each
// accessor binds directly to the child catalog so the root package cannot
// manufacture a second slot/name authority.
func DiagnosticObservationFamily() programfamily.Family[DiagnosticObservation] {
	return programfamily.New[DiagnosticObservation](programcatalog.DiagnosticObservation())
}

func DiagnosticEvidenceFamily() programfamily.Family[DiagnosticEvidence] {
	return programfamily.New[DiagnosticEvidence](programcatalog.DiagnosticEvidence())
}

func DiagnosticPathFamily() programfamily.Family[DiagnosticPath] {
	return programfamily.New[DiagnosticPath](programcatalog.DiagnosticPath())
}
