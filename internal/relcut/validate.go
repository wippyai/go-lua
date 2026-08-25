package relcut

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Severity separates a manifest that cannot be executed from one that has
// something to say.
type Severity string

const (
	// SeverityRefused is a manifest defect: the cut cannot be run from it.
	SeverityRefused Severity = "REFUSED"
	// SeverityNote is an observation about the tree the manifest describes.
	SeverityNote Severity = "NOTE"
)

// Finding is one validator result.
type Finding struct {
	Severity Severity
	Entry    string
	Detail   string
}

// String renders one finding.
func (finding Finding) String() string {
	return fmt.Sprintf("%s %s: %s", finding.Severity, finding.Entry, finding.Detail)
}

// Validate checks the manifest against itself and against a repository.
//
// Against itself: identities are unique, dispositions are declared, every
// keep-if-generic carries its proof obligation, every entry cites an
// authority, no path belongs to two entries, every dependency names a real
// entry, and the dependency graph is acyclic.
//
// Against the repository: every path an entry expects to be there is there.
// An entry that expects a path to be absent and finds it reports a note - that
// is how the Layer 0 landmine announces itself instead of waiting to be
// committed by accident.
func Validate(manifest Manifest, repositoryRoot string) []Finding {
	var findings []Finding
	refuse := func(entry, detail string) {
		findings = append(findings, Finding{Severity: SeverityRefused, Entry: entry, Detail: detail})
	}
	note := func(entry, detail string) {
		findings = append(findings, Finding{Severity: SeverityNote, Entry: entry, Detail: detail})
	}

	identities := map[string]struct{}{}
	owners := map[string]string{}
	layers := map[string]struct{}{}
	for _, layer := range manifest.Layers {
		layers[layer.ID] = struct{}{}
	}

	for _, entry := range manifest.Entries {
		if entry.ID == "" {
			refuse("<unnamed>", "entry has no identity")
			continue
		}
		if _, held := identities[entry.ID]; held {
			refuse(entry.ID, "entry identity is used twice")
		}
		identities[entry.ID] = struct{}{}

		switch entry.Disposition {
		case DispositionDelete, DispositionRestate:
		case DispositionKeepIfGeneric:
			if strings.TrimSpace(entry.ProofObligation) == "" {
				refuse(entry.ID, "keep-if-generic carries no proof obligation")
			}
		default:
			refuse(entry.ID, fmt.Sprintf("disposition %q is not declared", entry.Disposition))
		}
		if strings.TrimSpace(entry.Authority) == "" {
			refuse(entry.ID, "entry cites no authority")
		}
		if entry.CutStep < 0 || entry.CutStep > 8 {
			refuse(entry.ID, fmt.Sprintf("cut step %d is outside the Wave 5 sequence", entry.CutStep))
		}
		if entry.ResidueLayer != "" {
			if _, held := layers[entry.ResidueLayer]; !held {
				refuse(entry.ID, fmt.Sprintf("names undeclared dependency layer %q", entry.ResidueLayer))
			}
		}
		if len(entry.Paths) == 0 {
			refuse(entry.ID, "entry names no path")
		}
		for _, path := range entry.Paths {
			if owner, held := owners[path]; held {
				refuse(entry.ID, fmt.Sprintf("path %s is already owned by %s", path, owner))
				continue
			}
			owners[path] = entry.ID
		}
	}

	for _, entry := range manifest.Entries {
		for _, blocker := range entry.BlockedBy {
			if _, held := identities[blocker]; !held {
				refuse(entry.ID, fmt.Sprintf("blocked by unknown entry %q", blocker))
			}
		}
	}
	for _, cycle := range dependencyCycles(manifest) {
		refuse(cycle[0], "dependency cycle: "+strings.Join(cycle, " -> "))
	}

	if repositoryRoot != "" {
		for _, entry := range manifest.Order() {
			for _, path := range entry.Paths {
				_, err := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(path)))
				switch {
				case entry.ExpectPresent && os.IsNotExist(err):
					refuse(entry.ID, fmt.Sprintf("path %s is no longer in the tree; the manifest is stale", path))
				case entry.ExpectPresent && err != nil:
					refuse(entry.ID, fmt.Sprintf("path %s: %v", path, err))
				case !entry.ExpectPresent && err == nil:
					note(entry.ID, fmt.Sprintf("path %s is present although the manifest expects it absent", path))
				}
			}
		}
	}
	return findings
}

// Refused reports whether any finding blocks the cut.
func Refused(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Severity == SeverityRefused {
			return true
		}
	}
	return false
}

// dependencyCycles returns every cycle in the blocked-by graph, each as the
// sequence of entry identities that closes it.
func dependencyCycles(manifest Manifest) [][]string {
	blockers := map[string][]string{}
	identities := make([]string, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		blockers[entry.ID] = entry.BlockedBy
		identities = append(identities, entry.ID)
	}
	sort.Strings(identities)

	const (
		unvisited = 0
		open      = 1
		closed    = 2
	)
	state := map[string]int{}
	var stack []string
	var cycles [][]string
	var walk func(string)
	walk = func(id string) {
		switch state[id] {
		case open:
			for index, entry := range stack {
				if entry == id {
					cycles = append(cycles, append(append([]string(nil), stack[index:]...), id))
					return
				}
			}
			return
		case closed:
			return
		}
		state[id] = open
		stack = append(stack, id)
		for _, blocker := range blockers[id] {
			if _, held := blockers[blocker]; held {
				walk(blocker)
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = closed
	}
	for _, id := range identities {
		walk(id)
	}
	return cycles
}
