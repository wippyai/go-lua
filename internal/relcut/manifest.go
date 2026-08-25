// Package relcut holds the deletion manifest for the Wave 5 atomic cut of the
// old analyzer execution protocol (docs/architecture/relation-engine.md §11
// Wave 5 and §12).
//
// The manifest is data, not prose: the paths, the order they come out in, the
// laws that die with each entry and the laws that must be restated instead are
// all fields a lane reads. Validate keeps it from rotting - an entry naming a
// path the repository no longer holds is a finding the moment anyone runs the
// checker, rather than a surprise on the day of the cut.
//
// The manifest is read-only until the cut. Nothing here deletes anything.
package relcut

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

//go:embed manifest.json
var manifestJSON []byte

// Disposition is what the cut does with an entry.
type Disposition string

const (
	// DispositionDelete removes the entry from the tree.
	DispositionDelete Disposition = "delete"
	// DispositionRestate keeps the subject and rewrites it against the sealed
	// relational artifacts.
	DispositionRestate Disposition = "restate"
	// DispositionKeepIfGeneric retains the subject only once its proof
	// obligation is discharged. An undischarged obligation is a delete.
	DispositionKeepIfGeneric Disposition = "keep-if-generic"
)

// Restatement names one law that does not die with its entry and where it
// lands instead.
type Restatement struct {
	Law    string `json:"law"`
	Target string `json:"target"`
}

// Measurement is an entry's size at the manifest's pinned ref.
type Measurement struct {
	Files      int `json:"files"`
	NonTestLOC int `json:"non_test_loc"`
}

// Layer is one dependency stratum. Entries in a lower order stratum come out
// first.
type Layer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Order       int    `json:"order"`
	Description string `json:"description"`
}

// Entry is one subject of the cut.
type Entry struct {
	ID              string        `json:"id"`
	Paths           []string      `json:"paths"`
	Kind            string        `json:"kind"`
	Disposition     Disposition   `json:"disposition"`
	CutStep         int           `json:"cut_step"`
	ResidueLayer    string        `json:"residue_layer"`
	Authority       string        `json:"authority"`
	Measured        Measurement   `json:"measured"`
	BlockedBy       []string      `json:"blocked_by"`
	LawsDie         []string      `json:"laws_die"`
	LawsRestated    []Restatement `json:"laws_restated"`
	ProofObligation string        `json:"proof_obligation"`
	ExpectPresent   bool          `json:"expect_present"`
	Note            string        `json:"note"`
}

// Manifest is the whole read-only delete-set.
type Manifest struct {
	SchemaVersion int      `json:"schema_version"`
	PinnedRef     string   `json:"pinned_ref"`
	Authority     []string `json:"authority"`
	Rule          string   `json:"rule"`
	Layers        []Layer  `json:"layers"`
	Entries       []Entry  `json:"entries"`
}

// Load returns the checked-in manifest.
func Load() (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("relcut: decode manifest: %w", err)
	}
	return manifest, nil
}

// Layer returns one declared stratum.
func (manifest Manifest) Layer(id string) (Layer, bool) {
	for _, layer := range manifest.Layers {
		if layer.ID == id {
			return layer, true
		}
	}
	return Layer{}, false
}

// Order returns the entries in execution order: every entry follows the
// entries that block it, and among entries whose blockers are all satisfied
// the dependency stratum comes first, then the Wave 5 step, then the entry
// identity. The order is therefore total and does not depend on the order the
// entries were authored in.
//
// The order is a reading order, not a landing order. Wave 5 is one landing;
// the strata say which work cannot start before which, not which commits.
//
// A manifest whose dependencies do not close is refused by Validate. Order
// still terminates on one: the entries left in the cycle are appended in
// priority order so the caller sees them rather than losing them.
func (manifest Manifest) Order() []Entry {
	layerOrder := map[string]int{}
	for _, layer := range manifest.Layers {
		layerOrder[layer.ID] = layer.Order
	}
	byID := make(map[string]Entry, len(manifest.Entries))
	remaining := make([]string, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		byID[entry.ID] = entry
		remaining = append(remaining, entry.ID)
	}
	precedes := func(left, right string) bool {
		leftEntry, rightEntry := byID[left], byID[right]
		leftLayer, rightLayer := layerOrder[leftEntry.ResidueLayer], layerOrder[rightEntry.ResidueLayer]
		if leftLayer != rightLayer {
			return leftLayer < rightLayer
		}
		if leftEntry.CutStep != rightEntry.CutStep {
			return leftEntry.CutStep < rightEntry.CutStep
		}
		return leftEntry.ID < rightEntry.ID
	}
	sort.Slice(remaining, func(left, right int) bool { return precedes(remaining[left], remaining[right]) })

	emitted := make(map[string]struct{}, len(remaining))
	ordered := make([]Entry, 0, len(remaining))
	for len(remaining) > 0 {
		chosen := -1
		for index, id := range remaining {
			ready := true
			for _, blocker := range byID[id].BlockedBy {
				if _, known := byID[blocker]; !known {
					continue
				}
				if _, done := emitted[blocker]; !done {
					ready = false
					break
				}
			}
			if ready {
				chosen = index
				break
			}
		}
		if chosen < 0 {
			// The remainder does not close. Emit it in priority order rather
			// than dropping it; Validate names the cycle.
			for _, id := range remaining {
				ordered = append(ordered, byID[id])
			}
			return ordered
		}
		id := remaining[chosen]
		emitted[id] = struct{}{}
		ordered = append(ordered, byID[id])
		remaining = append(remaining[:chosen], remaining[chosen+1:]...)
	}
	return ordered
}

// Select returns the entries matching every non-empty filter.
type Select struct {
	Layer       string
	Step        int
	Disposition Disposition
}

// Entries applies a selection to the manifest in reading order.
func (manifest Manifest) Select(selection Select) []Entry {
	var selected []Entry
	for _, entry := range manifest.Order() {
		if selection.Layer != "" && entry.ResidueLayer != selection.Layer {
			continue
		}
		if selection.Step != 0 && entry.CutStep != selection.Step {
			continue
		}
		if selection.Disposition != "" && entry.Disposition != selection.Disposition {
			continue
		}
		selected = append(selected, entry)
	}
	return selected
}

// RepositoryRoot walks up from a directory to the module root that holds
// go.mod.
func RepositoryRoot(from string) (string, error) {
	directory, err := filepath.Abs(from)
	if err != nil {
		return "", fmt.Errorf("relcut: resolve %q: %w", from, err)
	}
	for {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && !info.IsDir() {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("relcut: no module root above %q", from)
		}
		directory = parent
	}
}
