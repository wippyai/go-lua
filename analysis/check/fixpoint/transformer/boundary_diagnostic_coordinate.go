package transformer

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

// composeBoundaryDiagnostics sequences one reachable diagnostic event after
// the current path value. This differs from lattice Join: obligations and
// exposures accumulate on a path, while suspension certification is composed
// only when the event owns that descriptor. Alternative paths still meet at
// the coordinate lattice Join in guarded_world.go.
func composeBoundaryDiagnostics(reg *axis.Registry, current, event callpayload.DiagnosticOutput, ownsSuspension bool) callpayload.DiagnosticOutput {
	if !diagnosticOutputHasFacts(current) && !diagnosticOutputHasFacts(event) {
		out := current
		if ownsSuspension {
			out.SuspensionKnown = current.SuspensionKnown && event.SuspensionKnown
			out.MaySuspend = current.MaySuspend || event.MaySuspend
		}
		return out
	}
	out := current.Clone()
	if ownsSuspension {
		out.SuspensionKnown = current.SuspensionKnown && event.SuspensionKnown
		out.MaySuspend = current.MaySuspend || event.MaySuspend
	}
	out.ParamObligations = append(out.ParamObligations, event.ParamObligations...)
	out.PathObligations = append(out.PathObligations, event.PathObligations...)
	out.ParamExposures = append(out.ParamExposures, event.ParamExposures...)
	return out.Normalize(reg)
}

func diagnosticOutputHasFacts(value callpayload.DiagnosticOutput) bool {
	return len(value.ParamObligations) != 0 || len(value.PathObligations) != 0 || len(value.ParamExposures) != 0
}

func semanticContributionCarriesRecursiveDiagnostics(contribution semanticContribution) bool {
	return contribution.suspensionKnown || contribution.maySuspend || len(contribution.paramObligations) != 0 ||
		len(contribution.pathObligations) != 0 || len(contribution.paramExposures) != 0
}
