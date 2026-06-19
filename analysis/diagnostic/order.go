package diagnostic

import "sort"

// Sort orders diagnostics the way users read source: by file and source
// position first, with deterministic tie-breakers for diagnostics that share a
// span.
func Sort(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		return diagnosticLess(diags[i], diags[j])
	})
}

func diagnosticLess(a, b Diagnostic) bool {
	aValid := a.Position.Valid()
	bValid := b.Position.Valid()
	if aValid != bValid {
		return aValid
	}
	if a.Position.File != b.Position.File {
		return a.Position.File < b.Position.File
	}
	if a.Position.Line != b.Position.Line {
		return a.Position.Line < b.Position.Line
	}
	if a.Position.Column != b.Position.Column {
		return a.Position.Column < b.Position.Column
	}
	if a.Position.EndLine != b.Position.EndLine {
		return a.Position.EndLine < b.Position.EndLine
	}
	if a.Position.EndColumn != b.Position.EndColumn {
		return a.Position.EndColumn < b.Position.EndColumn
	}
	if a.Severity != b.Severity {
		return a.Severity < b.Severity
	}
	if a.Code != b.Code {
		return a.Code < b.Code
	}
	return a.Message < b.Message
}
