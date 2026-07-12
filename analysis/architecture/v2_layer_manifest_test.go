package architecture

import "testing"

// v2PackagePrefix is an exact package-tree boundary. A prefix matches either
// the package itself or a child separated by '/', never a merely textual
// neighbor (for example, check/projection does not match check/projectionx).
type v2PackagePrefix string

type v2ImportBoundary struct {
	name string

	// subjects is optional by construction: a rule has no subjects until its
	// future package is introduced, and is enforced immediately thereafter.
	subjects  []v2PackagePrefix
	forbidden []v2PackagePrefix

	// allowedRepository is a repository-import allowlist. An empty list means
	// the boundary uses only forbidden. Standard-library and third-party imports
	// are outside this architecture gate.
	allowedRepository []v2PackagePrefix

	// exceptSubjects exempts a narrower owner subtree from a broad subject rule.
	exceptSubjects []v2PackagePrefix

	// delegatedTo records an existing gate that remains the sole enforcer. This
	// keeps the v2 manifest complete without running a second, drifting version
	// of an established boundary.
	delegatedTo string
}

type v2ExclusiveBridge struct {
	name string

	// activatedBy keeps a future seam optional until the destination package is
	// introduced. Once present, the bridge owner must import the destination.
	activatedBy v2PackagePrefix
	bridge      v2PackagePrefix
	source      v2PackagePrefix
	destination v2PackagePrefix
}

var v2LayerImportBoundaries = []v2ImportBoundary{
	{
		name:     "only transferfacts lowers Lua into the neutral semantic program",
		subjects: []v2PackagePrefix{"github.com/wippyai/go-lua/analysis/lua"},
		forbidden: []v2PackagePrefix{
			"github.com/wippyai/go-lua/analysis/semantic/program",
		},
		exceptSubjects: []v2PackagePrefix{
			"github.com/wippyai/go-lua/analysis/lua/transferfacts",
		},
	},
	{
		name: "semantic schema and program stay backend neutral",
		subjects: []v2PackagePrefix{
			"github.com/wippyai/go-lua/analysis/semantic/schema",
			"github.com/wippyai/go-lua/analysis/semantic/program",
		},
		forbidden: []v2PackagePrefix{
			"github.com/wippyai/go-lua/analysis/check",
			"github.com/wippyai/go-lua/analysis/engine",
			"github.com/wippyai/go-lua/analysis/lsp",
			"github.com/wippyai/go-lua/analysis/lua",
			"github.com/wippyai/go-lua/compiler",
		},
	},
	{
		name:     "low-level artifact storage is product neutral",
		subjects: []v2PackagePrefix{"github.com/wippyai/go-lua/analysis/artifact"},
		allowedRepository: []v2PackagePrefix{
			"github.com/wippyai/go-lua/analysis/artifact",
			"github.com/wippyai/go-lua/analysis/internal",
		},
	},
	{
		name:     "checker projection cannot recover solver or lexical authority",
		subjects: []v2PackagePrefix{"github.com/wippyai/go-lua/analysis/check/projection"},
		forbidden: []v2PackagePrefix{
			"github.com/wippyai/go-lua/analysis/check/body",
			"github.com/wippyai/go-lua/analysis/check/fixpoint/program",
			"github.com/wippyai/go-lua/analysis/check/fixpoint/query",
			"github.com/wippyai/go-lua/analysis/check/service",
			"github.com/wippyai/go-lua/analysis/engine/factapply",
			"github.com/wippyai/go-lua/analysis/engine/solve",
			"github.com/wippyai/go-lua/analysis/engine/transfer",
			"github.com/wippyai/go-lua/analysis/engine/workplan",
			"github.com/wippyai/go-lua/analysis/lsp",
			"github.com/wippyai/go-lua/analysis/lua",
			"github.com/wippyai/go-lua/compiler",
		},
	},
	{
		name:        "LSP remains a service-only orchestration adapter",
		subjects:    []v2PackagePrefix{"github.com/wippyai/go-lua/analysis/lsp"},
		delegatedTo: "TestLSPAdapterImportBoundaries",
	},
}

var v2ExclusiveBridges = []v2ExclusiveBridge{
	{
		name:        "transferfacts is the sole WIR to semantic-program bridge",
		activatedBy: "github.com/wippyai/go-lua/analysis/semantic/program",
		bridge:      "github.com/wippyai/go-lua/analysis/lua/transferfacts",
		source:      "github.com/wippyai/go-lua/analysis/ir/wir",
		destination: "github.com/wippyai/go-lua/analysis/semantic/program",
	},
}

// v2DelegatedImportGates is a compile-time link to the established owner of a
// delegated rule. Renaming or removing that owner cannot silently disable the
// manifest entry, while the function is still executed only once by go test.
var v2DelegatedImportGates = map[string]func(*testing.T){
	"TestLSPAdapterImportBoundaries": TestLSPAdapterImportBoundaries,
}
