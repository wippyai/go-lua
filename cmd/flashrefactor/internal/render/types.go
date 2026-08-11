package render

import (
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/generate"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

// Input is the complete pure render boundary. Snapshot is pre-cut evidence;
// Registry is a finite set of pure generators. Neither gives the renderer a
// filesystem or process capability.
type Input struct {
	Intent   cutplan.Intent
	Snapshot semantic.Snapshot
	Registry generate.Registry
}

// Output is the exact post-cut source state for the declared write footprint.
// Files is sorted by repository path and includes explicit deletions.
type Output struct {
	Files      []semantic.VirtualFile
	Diffs      []DiffInput
	Provenance []Provenance
	Witnesses  []RouteWitness
	Providers  []cutplan.ProviderEvidence
	Hazards    []cutplan.Hazard
}

// PhysicalWitnessSite identifies the exact pre-render identifier byte in its
// detached source file. Offset is a zero-based byte offset in the preimage.
// Role uses the semantic site's exact vocabulary: the renderer carries
// lineage, while post-render classification remains semantic collection's
// authority.
type PhysicalWitnessSite struct {
	Path   string
	Offset int
	Role   cutplan.SiteRole
}

// StructuralAnchor identifies the same retained *ast.Ident after rendering.
// Identifier is the non-comment identifier preorder index in the final AST;
// it is intentionally an anchor, not a fabricated line/column position.
type StructuralAnchor struct {
	Path       string
	Identifier int
	Role       cutplan.SiteRole
	Name       string
}

type RouteWitnessSite struct {
	Source PhysicalWitnessSite
	Target StructuralAnchor
}

// RouteWitness groups the causal structural lineage for one relocated From/To
// object pair. A workbench combines it with pre/post semantic evidence to
// construct final cutplan.ReferenceRoute positions.
type RouteWitness struct {
	From  cutplan.SymbolRef
	To    cutplan.SymbolRef
	Sites []RouteWitnessSite
}

// DiffInput is the deterministic, write-free input for an external diff
// renderer. Before is nil only for an authored absent destination; Delete
// makes an existing post-state absence explicit.
type DiffInput struct {
	Path   string
	Before []byte
	After  []byte
	Delete bool
}

type ProvenanceKind string

const (
	ProvenanceRelocate ProvenanceKind = "relocate"
	ProvenanceRetire   ProvenanceKind = "retire"
	ProvenanceGenerate ProvenanceKind = "generate"
	ProvenanceBinding  ProvenanceKind = "binding"
	ProvenanceImport   ProvenanceKind = "import"
)

// Provenance is the narrow stable workbench API. It is generated only after a
// handler consumed one exact authored element; it does not claim post-state
// type evidence, which remains semantic collection's authority.
type Provenance struct {
	Operation   string
	Kind        ProvenanceKind
	From        cutplan.SymbolRef
	To          cutplan.SymbolRef
	Objects     []cutplan.SymbolRef
	Paths       []string
	Receiver    []cutplan.ReceiverPathStep
	Containment *cutplan.Containment
	ImportFrom  *cutplan.ImportRef
	ImportTo    *cutplan.ImportRef
	Provider    cutplan.Provider
}
