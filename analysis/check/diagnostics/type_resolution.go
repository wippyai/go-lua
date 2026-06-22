package diagnostics

import "github.com/wippyai/go-lua/analysis/type/typ"

type diagnosticTypeResolutionSource string

type diagnosticTypeResolution struct {
	Type             typ.Type
	Source           diagnosticTypeResolutionSource
	UntrustedTopLike bool
	OK               bool
}

type diagnosticTypeResolutionAttempt struct {
	Source           diagnosticTypeResolutionSource
	UntrustedTopLike bool
	Resolve          func() (typ.Type, bool)
}

func firstDiagnosticTypeResolution(
	fallback diagnosticTypeResolution,
	attempts ...diagnosticTypeResolutionAttempt,
) diagnosticTypeResolution {
	for _, attempt := range attempts {
		if attempt.Resolve == nil {
			continue
		}
		t, ok := attempt.Resolve()
		if !ok {
			continue
		}
		return diagnosticTypeResolution{
			Type:             t,
			Source:           attempt.Source,
			UntrustedTopLike: attempt.UntrustedTopLike,
			OK:               true,
		}
	}
	return fallback
}
