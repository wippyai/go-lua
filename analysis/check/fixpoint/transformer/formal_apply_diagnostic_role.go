package transformer

// boundaryCallDiagnosticRole selects the one formal diagnostic transport law
// for an Apply terminal.
type boundaryCallDiagnosticRole uint8

const (
	boundaryCallDiagnosticInvalid boundaryCallDiagnosticRole = iota
	boundaryCallDiagnosticCompose
	boundaryCallDiagnosticCalleeCarry
	boundaryCallDiagnosticKnown
)
