package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// Input is the standalone authored Static boundary. Later verticals extend
// this package's private component rather than create a parallel static IR.
type Input struct {
	Counts       [keyspace.FamilyCount]uint32
	Types        statictypes.Input
	References   staticrefs.Input
	Declarations staticdecl.Input
	Signatures   staticsig.Input
	Contracts    staticcontracts.Input
	Operators    staticoperators.Input
	Operands     staticoperands.Input
	Publications staticpubs.Input
}

// Component is the immutable publication of Static's one canonical query
// snapshot. It retains no second table image, census copy, or construction
// ledger: the build-only assembly below is discarded when this value is
// published.
type Component struct {
	snapshot staticquery.Snapshot
}

// assembly is construction-only state. It joins the typed child owners for
// the cross-vertical Static laws, then disappears when the proof-bearing
// query snapshot is published into Component.
type assembly struct {
	census       [keyspace.FamilyCount]uint32
	types        statictypes.Table
	references   staticrefs.Table
	declarations staticdecl.Table
	signatures   staticsig.Table
	contracts    staticcontracts.Table
	operators    staticoperators.Table
	operands     staticoperands.Table
	publications staticpubs.Table
}

// View returns the proof-bearing immutable Static query surface published at
// Build time. Repeated reads reuse the same snapshot and cannot silently drop
// the local containment proof.
func (component *Component) View() staticquery.View {
	if component == nil {
		return staticquery.View{}
	}
	return component.snapshot.View()
}
