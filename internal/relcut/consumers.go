// Consumer map for the Wave 4 read-path restatement that precedes the Wave 5
// atomic cut (docs/architecture/relation-engine.md §11 Wave 5 steps 1 and 8).
//
// The deletion manifest states what comes out. This states who is still
// reading it. Every non-test file outside the relation fence that names the
// old analyzer engine's read surfaces appears here once, with the symbols it
// names today and the declared surface that answers it after the cut.
//
// Where no declared owner column carries a distinction a consumer reads, the
// entry carries a gap identity instead of a column. A gap is a lossy-parent
// finding: the consumer needs a distinction no owner publishes, and the answer
// is a published column, not a rediscovery in the consumer. Naming the gap is
// the whole point of the map; inventing a column to close it is not.
//
// The map is read-only until the cut. Nothing here rewrites anything.
package relcut

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:embed consumers.json
var consumersJSON []byte

// ReadClass is the kind of old-surface access a consumer makes. The class
// decides which declared surface answers it, so it is stated per read rather
// than inferred from the symbol at cut time.
type ReadClass string

const (
	// ReadClassQuery is the published-answer surface: the consumer asks for a
	// fact some family decided.
	ReadClassQuery ReadClass = "query"
	// ReadClassResult is the solved-result surface: the consumer reads the
	// outcome of a solve rather than one family's answer.
	ReadClassResult ReadClass = "result"
	// ReadClassDeclaration is the rule-wiring surface: the consumer declares a
	// rule against the engine instead of reading from it.
	ReadClassDeclaration ReadClass = "declaration"
	// ReadClassAdmission is the program assembly surface: the consumer builds
	// or admits a program.
	ReadClassAdmission ReadClass = "admission"
	// ReadClassRowABI is the hot execution ABI a rule body runs against.
	ReadClassRowABI ReadClass = "row-abi"
)

// TargetKind is the shape of the declared surface that answers a consumer.
type TargetKind string

const (
	// TargetOwnerColumn is one declared relation column redeemed from the
	// canonical snapshot at an owner-issued codec.
	TargetOwnerColumn TargetKind = "owner-column"
	// TargetCertificate is a sealed plan row read through the certificate
	// checker rather than through a family answer.
	TargetCertificate TargetKind = "certificate"
	// TargetDeclaredSurface is the declarative rule surface that replaces a
	// hand-wired rule declaration.
	TargetDeclaredSurface TargetKind = "declared-surface"
	// TargetRuntimeComposition is the relational runtime that composes and
	// solves a target.
	TargetRuntimeComposition TargetKind = "runtime-composition"
	// TargetRowABI is the relational runtime's own row ABI.
	TargetRowABI TargetKind = "row-abi"
	// TargetGap is no declared surface at all: the consumer's distinction is
	// unpublished and the entry names a gap.
	TargetGap TargetKind = "gap"
)

// Read is one symbol a consumer names on an old surface.
type Read struct {
	Package string    `json:"package"`
	Symbol  string    `json:"symbol"`
	Uses    int       `json:"uses"`
	Class   ReadClass `json:"class"`
}

// Target is the declared surface that answers a consumer after the cut.
type Target struct {
	Kind     TargetKind `json:"kind"`
	Package  string     `json:"package"`
	Relation string     `json:"relation"`
	Column   string     `json:"column"`
	Note     string     `json:"note"`
}

// Consumer is one non-test reader of an old surface outside the fence.
type Consumer struct {
	Path          string `json:"path"`
	ManifestEntry string `json:"manifest_entry"`
	Reads         []Read `json:"reads"`
	Target        Target `json:"target"`
	Gap           string `json:"gap"`
	// WaitsOn is the prerequisite whose landing makes this entry's move
	// possible: a manifest entry, or a declared gap. It is empty exactly when
	// the target package already declares every symbol the entry attributes to
	// the consumer, which is the record of a read that is at its post-cut home.
	// A target naming a package that answers none of the entry's reads is where
	// a bridge enters, so the map states the prerequisite rather than leaving
	// the move to look available.
	WaitsOn string `json:"waits_on"`
}

// Gap is a distinction a consumer reads that no declared owner publishes.
type Gap struct {
	ID          string   `json:"id"`
	Distinction string   `json:"distinction"`
	Owner       string   `json:"owner"`
	Consumers   []string `json:"consumers"`
	Detail      string   `json:"detail"`
}

// Surface is one old package the map covers, with the manifest entry that
// removes it.
type Surface struct {
	Package       string `json:"package"`
	ManifestEntry string `json:"manifest_entry"`
	Role          string `json:"role"`
}

// ConsumerMap is the whole read-side of the cut.
type ConsumerMap struct {
	SchemaVersion int        `json:"schema_version"`
	PinnedRef     string     `json:"pinned_ref"`
	Authority     []string   `json:"authority"`
	Rule          string     `json:"rule"`
	Fence         []string   `json:"fence"`
	Surfaces      []Surface  `json:"surfaces"`
	Consumers     []Consumer `json:"consumers"`
	Gaps          []Gap      `json:"gaps"`
}

// LoadConsumers returns the checked-in consumer map.
func LoadConsumers() (ConsumerMap, error) {
	var consumers ConsumerMap
	if err := json.Unmarshal(consumersJSON, &consumers); err != nil {
		return ConsumerMap{}, fmt.Errorf("relcut: decode consumer map: %w", err)
	}
	return consumers, nil
}

// Gap returns one declared gap.
func (consumers ConsumerMap) Gap(id string) (Gap, bool) {
	for _, gap := range consumers.Gaps {
		if gap.ID == id {
			return gap, true
		}
	}
	return Gap{}, false
}

// ByClass returns the consumers whose reads include a class, in path order.
func (consumers ConsumerMap) ByClass(class ReadClass) []Consumer {
	var selected []Consumer
	for _, consumer := range consumers.Consumers {
		for _, read := range consumer.Reads {
			if read.Class == class {
				selected = append(selected, consumer)
				break
			}
		}
	}
	sort.Slice(selected, func(left, right int) bool {
		return selected[left].Path < selected[right].Path
	})
	return selected
}

// CutOrder returns the consumers grouped into the order Wave 4 restates them:
// the surfaces a consumer only declares against come out with the wiring flip,
// the answer readers move once their column is published, and the solved-result
// readers move last because they read what the runtime composes.
//
// The order is over classes, not over commits. Within a class the path decides,
// so the order is total and does not depend on authoring order.
func (consumers ConsumerMap) CutOrder() []ReadClass {
	return []ReadClass{
		ReadClassDeclaration,
		ReadClassRowABI,
		ReadClassQuery,
		ReadClassAdmission,
		ReadClassResult,
	}
}
