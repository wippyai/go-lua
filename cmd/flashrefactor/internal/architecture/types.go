package architecture

import (
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

// Declaration is the small authored architectural boundary.  There is no
// authored route, import, footprint, gate, output, or executor vocabulary.
// Those are derived from the resolver-backed Survey exactly once.
type Declaration struct {
	Name        string
	Boundary    Boundary
	Parent      cutplan.SymbolRef
	Fields      []cutplan.SymbolRef
	Destination ContainmentDestination
	Laws        []cutplan.Law
}

// Boundary identifies the semantic authority being moved.  It deliberately
// has no execution action: one containment extraction is the only operation
// this compiler can emit.
type Boundary struct {
	ID   string
	From string
	To   string
}

// ContainmentDestination names every post-cut identity that is not already a
// resolved source object. ImportPath and Package are both explicit because a
// Go package clause must never be inferred from a filesystem path.
type ContainmentDestination struct {
	Path       string
	ImportPath string
	Package    string
	Child      string
	Through    string
}

// Survey is read-only resolver evidence for one Declaration. Its fields are
// intentionally private: callers obtain it only through CollectSurvey and it
// cannot become a Lock or apply authority.
type Survey struct {
	snapshot    semantic.Snapshot
	declaration Declaration
}
