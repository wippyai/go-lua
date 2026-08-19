package static

import (
	"sync"

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

type draftState struct {
	component        *Component
	localContainment *staticquery.Proof
	live             uint32
	phase            draftPhase
	mu               sync.Mutex
}

type draftPhase uint8

const (
	draftOpen draftPhase = iota + 1
	draftClaimed
	draftCommitted
	draftAborted
)

// Draft is a shared construction capability. Copies share state; publication
// is possible only by first claiming the owner-defined Finalizer.
type Draft struct{ state *draftState }

// Finalizer is an owner-defined one-shot publication capability. Copies share
// the Draft state, so exactly one terminal action (Commit or Abort) can win.
// The View is a read-only validation surface for the owner that coordinates
// finalization; it carries no construction state or sibling projection.
type Finalizer struct {
	state *draftState
}

// View returns the immutable composed Static query surface. The query child
// receives only the sealed canonical values and no root Component capability.
func (component *Component) View() staticquery.View {
	if component == nil {
		return staticquery.View{}
	}
	return staticquery.NewView(component.querySnapshot(), nil)
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
