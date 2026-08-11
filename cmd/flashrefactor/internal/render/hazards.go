package render

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/gorewrite"
)

// collectHazards visits only exact read-footprint Go files. It does not walk a
// directory or classify a string as authority. An authority-bearing construct
// blocks the entire pure render before any output is returned.
func (state *renderState) collectHazards(paths []string) error {
	for _, path := range paths {
		file, _, _, err := state.existingFile(path)
		if err != nil {
			// A generator may consume an opaque artifact. The renderer has no
			// parser for it and therefore leaves its semantics to that registered
			// provider; non-Go input is never guessed at as Go source.
			continue
		}
		for _, hazard := range gorewrite.FindHazards(file.file, file.fset, file.info) {
			severity := "warning"
			if hazard.Authority {
				severity = "error"
			}
			state.hazards = append(state.hazards, cutplanHazard{
				code: "go-" + hazard.Kind, severity: severity,
				detail: hazard.Kind + ": " + hazard.Detail, path: path,
			})
			if hazard.Authority {
				return fmt.Errorf("authority hazard %s in %s", hazard.Kind, path)
			}
		}
	}
	return nil
}

func canonicalHazards(values []cutplanHazard) []cutplan.Hazard {
	seen := map[string]cutplan.Hazard{}
	for _, value := range values {
		// A hazard's identity is its observed construct, not an individual
		// occurrence. Paths are its complete support set and must therefore be
		// retained together under the one lock-record identity.
		key := value.code + "\x00" + value.severity + "\x00" + value.detail
		hazard, exists := seen[key]
		if !exists {
			hazard = cutplan.Hazard{Code: value.code, Severity: value.severity, Detail: value.detail}
		}
		if !containsPath(hazard.Paths, value.path) {
			hazard.Paths = append(hazard.Paths, value.path)
		}
		seen[key] = hazard
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]cutplan.Hazard, 0, len(keys))
	for _, key := range keys {
		hazard := seen[key]
		sort.Strings(hazard.Paths)
		result = append(result, hazard)
	}
	return result
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
