package semantic

import (
	"fmt"
	"strings"
)

var semanticBuildFlags = []string{"-buildvcs=false", "-trimpath"}

// freezeSemanticEnvironment captures the sole Go environment used by a
// Session. A physical workfile, modfile, or overlay cannot be rebound safely
// in the disposable target shadow, so this boundary rejects it rather than
// silently letting source and target observe different worlds.
func freezeSemanticEnvironment() ([]string, error) {
	environment := packagesEnvironment()
	values := semanticEnvironmentValues(environment)
	if work := values["GOWORK"]; work != "" && work != "auto" && work != "off" {
		return nil, fmt.Errorf("location-bound GOWORK is not supported: %q", work)
	}
	for _, flag := range strings.Fields(values["GOFLAGS"]) {
		if flag == "-modfile" || strings.HasPrefix(flag, "-modfile=") || flag == "-overlay" || strings.HasPrefix(flag, "-overlay=") {
			return nil, fmt.Errorf("location-bound Go flag is not supported: %q", flag)
		}
	}
	return environment, nil
}

func semanticEnvironmentValues(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}
	return values
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}
