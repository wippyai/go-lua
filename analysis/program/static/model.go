package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
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

// Component is immutable authored Static syntax with no inferred/domain
// resolution, query index, or construction state.
// census is the sealed per-family cardinality column assigned once by Build;
// it is the sole cardinality authority and retains no duplicate Term stream
// or second graph.
type Component struct {
	contentID    identity.ContentID
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

// View returns the immutable composed Static query surface. The query child
// receives only the sealed canonical values and no root Component capability.
func (component *Component) View() staticquery.View {
	if component == nil {
		return staticquery.View{}
	}
	return component.querySnapshot().View()
}

func (component *Component) querySnapshot() staticquery.Snapshot {
	return component.querySnapshotWithProof(nil)
}

func (component *Component) querySnapshotWithProof(proof *staticquery.Proof) staticquery.Snapshot {
	if component == nil {
		return staticquery.Snapshot{}
	}
	return staticquery.NewSnapshot(
		component.contentID,
		component.census,
		component.types,
		component.references,
		component.declarations,
		component.signatures,
		component.contracts,
		component.operators,
		component.operands,
		component.publications,
		proof,
	)
}
