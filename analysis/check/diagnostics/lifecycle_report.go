package diagnostics

import (
	"fmt"
	"strings"
)

type lifecycleResourceReport struct {
	resourceName string
	protocol     string
	current      string
	final        string
}

func newLifecycleResourceReport(resourceName, protocol, current, final string) lifecycleResourceReport {
	return lifecycleResourceReport{
		resourceName: resourceName,
		protocol:     protocol,
		current:      current,
		final:        final,
	}
}

func (r lifecycleResourceReport) Message() string {
	if strings.TrimSpace(r.current) == "" {
		return fmt.Sprintf("resource %s remains in a non-final %s state at function exit; expected %s", codeName(r.resourceName), r.protocol, lifecycleStateName(r.final))
	}
	return fmt.Sprintf("resource %s remains in %s state %s at function exit; expected %s", codeName(r.resourceName), r.protocol, lifecycleStateName(r.current), lifecycleStateName(r.final))
}

func (r lifecycleResourceReport) AcquireEvidence(resourceName, current string) string {
	return fmt.Sprintf("this call acquires %s as %s:%s and requires %s before local ownership ends", codeName(resourceName), r.protocol, lifecycleStateName(current), lifecycleStateName(r.final))
}

func (r lifecycleResourceReport) TransitionEvidence(resourceName, from, to string) string {
	if from == "" {
		return fmt.Sprintf("this call transitions %s in protocol %s to %s on a reachable path", codeName(resourceName), r.protocol, lifecycleStateName(to))
	}
	return fmt.Sprintf("this call transitions %s in protocol %s from %s to %s on a reachable path", codeName(resourceName), r.protocol, lifecycleStateName(from), lifecycleStateName(to))
}

func (r lifecycleResourceReport) EscapeEvidence(resourceName string) string {
	return fmt.Sprintf("this call escapes local ownership of %s in protocol %s on a reachable path", codeName(resourceName), r.protocol)
}

func (r lifecycleResourceReport) ExitObligationEvidence() string {
	return fmt.Sprintf("exit state still has %s in protocol %s at %s; no proof reaches %s or escapes ownership on every path", codeName(r.resourceName), r.protocol, lifecycleStateName(r.current), lifecycleStateName(r.final))
}

func (r lifecycleResourceReport) Help() string {
	return fmt.Sprintf("Transition %s to %s or escape ownership on every return path.", codeName(r.resourceName), lifecycleStateName(r.final))
}

func lifecycleStateName(state string) string {
	if strings.TrimSpace(state) == "" {
		return "a non-final state"
	}
	if strings.Contains(state, "`") {
		return state
	}
	return codeName(state)
}
