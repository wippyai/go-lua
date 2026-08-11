// Package semantic collects compiler-structured semantic evidence for one
// flash-refactor transaction.  The authority is go/packages plus go/types,
// never a human-oriented gopls text transcript.
package semantic

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

// Loader is the sole semantic-resolution authority.  Its concrete
// implementation loads a complete Go workspace with syntax and type
// information; tests supply fixed structured results, never synthetic CLI
// output.
type Loader interface {
	Load(context.Context, LoadRequest) (LoadResult, error)
}

// Config describes one isolated collection transaction.
type Config struct {
	Root          string
	Flashrefactor string
	CacheParent   string
	Loader        Loader
}

// Session owns one disposable scratch tree for virtual workspace shadows.
// Go's inherited content-addressed action cache is intentionally outside this
// lifecycle: it changes work only, never semantic evidence.
type Session struct {
	config        Config
	root          string
	scratchParent string
	scratch       string
	environment   []string
	buildFlags    []string
}

// SymbolRequest names a reviewed object in one explicit pre/post role. Source
// coordinates are resolver output, never a second authored intent vocabulary.
type SymbolRequest struct {
	Object cutplan.SymbolRef
	Role   cutplan.ObjectRole
	Impact bool
}

// loadScope names the concrete package roots needed to type one collection
// state. It is query planning only: evidence remains exclusively in Requests.
// Files are relative Go paths in that state. RemovedSurfaceOwners are target
// state package surfaces that may have disappeared with a retired declaration;
// their absence is therefore valid.
type loadScope struct {
	Files                []string
	ExpandFileOwners     bool
	RemovedSurfaceOwners []string
}

// LoadRequest is normalized before it reaches a Loader. All overlay keys are
// canonical absolute workspace paths. Environment and BuildFlags are copied
// from one private Session snapshot for every source/target load.
type LoadRequest struct {
	Root            string
	Scratch         string
	Environment     []string
	BuildFlags      []string
	Patterns        []string
	scope           loadScope
	Requests        []SymbolRequest
	Overlay         map[string][]byte
	DiagnosticPaths []string
}

// LoadResult contains only structured results. WorkspaceFailures are distinct
// from source diagnostics: the former invalidate evidence, while the latter
// are captured into a reviewed before/after diagnostic delta.
type LoadResult struct {
	Workspace         *Workspace
	Objects           []cutplan.ObjectEvidence
	Diagnostics       []Diagnostic
	WorkspaceFailures []string
	Toolchain         ToolchainEvidence
}

// ExecutableIdentity binds the exact Go driver used by the structured loader.
// Digest is SHA-256 of the resolved executable, not a path-only assertion.
type ExecutableIdentity struct {
	Path    string
	SHA256  string
	Version string
}

// ToolchainEvidence records the semantic authority itself.
type ToolchainEvidence struct {
	Go                ExecutableIdentity
	Loader            string
	BuildEnvSHA256    string
	ModuleGraphSHA256 string
}

// Diagnostic is a source-positioned compiler diagnostic. There is no
// unpositioned warning channel because an exact delta could not account for
// one.
type Diagnostic struct {
	Position cutplan.Position
	Message  string
	Kind     string
}

// Snapshot is one immutable evidence collection. Authority retains the Go
// executable identity in addition to the lock's resolver commitment.
type Snapshot struct {
	Toolchain   cutplan.Toolchain
	Authority   ToolchainEvidence
	Workspace   *Workspace
	Objects     []cutplan.ObjectEvidence
	Diagnostics []Diagnostic
	Structure   StructuralSnapshot
}

// StructuralSnapshot is the exact typed workspace shape consumed by the
// structural gates. It is evidence, not another resolver: object ownership
// remains exclusively in cutplan.ResolutionRequirements.
type StructuralSnapshot struct {
	Packages []StructuralPackage
	Files    []StructuralFile
}

// StructuralPackage records one loaded package variant and its direct import
// paths. IDs preserve test-variant distinction.
type StructuralPackage struct {
	ID      string
	Path    string
	Name    string
	Imports []string
}

// StructuralFile records exact imports as resolved by the typed workspace.
// Alias is source spelling; Name is the imported package clause.
type StructuralFile struct {
	Path        string
	PackageID   string
	PackagePath string
	Imports     []cutplan.ImportRef
}

// ResidueQuery asks whether one exact semantic object remains resolved at the
// listed post-cut paths. Empty or duplicate paths are invalid: residue is an
// exact structural assertion, never a workspace-wide text search.
type ResidueQuery struct {
	Object cutplan.SymbolRef
	Paths  []string
}

// ObjectResidue is the compiler-resolved residue of one queried object. A
// zero-length Sites set is the only proof that it is absent from the queried
// paths.
type ObjectResidue struct {
	Object cutplan.SymbolRef
	Sites  []cutplan.Position
}

// Merged is the one source/target evidence view used to build a lock. The
// snapshots remain separate because their structural worlds legitimately
// differ; Objects is their exact role-indexed union.
type Merged struct {
	Source       Snapshot
	Target       Snapshot
	Requirements []cutplan.ResolutionRequirement
	Objects      []cutplan.ObjectEvidence
}

// DiagnosticDelta is the exact normalized change between two snapshots.
type DiagnosticDelta struct {
	Added   []Diagnostic
	Removed []Diagnostic
}

func sortedPositions(values []cutplan.Position) []cutplan.Position {
	result := append([]cutplan.Position(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Offset != right.Offset {
			return left.Offset < right.Offset
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		if left.Role != right.Role {
			return left.Role < right.Role
		}
		return strings.Join(left.PackageIDs, "\x00") < strings.Join(right.PackageIDs, "\x00")
	})
	return result
}

func positionKey(position cutplan.Position) string {
	return fmt.Sprintf("%s:%s:%d:%d:%d:%s", strings.Join(position.PackageIDs, "\x00"), position.Path, position.Offset, position.Line, position.Column, position.Role)
}
