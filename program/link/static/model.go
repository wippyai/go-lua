// Package static owns Link's immutable, non-runtime static-resolution
// relation.  It consumes only sealed Program module inputs and Target identity;
// it has no dependency on Link's runtime lookup, values, seeds, or solver.
package static

import (
	"sync"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programstatic "github.com/wippyai/go-lua/program/static"
)

// Input is the complete static build boundary. The finalized Project supplies
// its immutable constituent identity and canonical mounts; no runtime Link
// state crosses this boundary.
type Input struct {
	Project *linkproject.Component
}

// Draft is construction-only static authority. Its only exported projection is
// Cold, used by Link identity/dependency assembly before finalization.
type Draft struct {
	state *draftState
}

// draftState is shared by all value copies of a Draft so construction
// authority is consumed exactly once.
type draftState struct {
	mu        sync.Mutex
	component *Component
	consumed  bool
	fence     *draftFence
}

// draftFence is the only construction state retained by a Cold snapshot. It
// contains no Static authority and no Program pointer.
type draftFence struct{ consumed bool }

// Component is a fully owner-fenced immutable static authority. Finalize
// fences construction access without changing its content or identity.
type Component struct {
	mounts          linkproject.Mounts
	namespaces      []namespaceRow
	resolutions     []resolutionRow
	inputs          []inputRow
	expressionEnds  []uint32
	byCall          map[shardTerm]Resolution
	byAlias         map[shardTerm]Namespace
	qualified       []qualifiedRow
	byQualified     map[shardTerm]uint32
	contentID       keyspace.ContentID
	semanticReceipt SemanticSourceReceipt
}

func (c *Component) program(shard linkproject.Shard) (*program.Program, bool) {
	if c == nil {
		return nil, false
	}
	return c.mounts.Program(shard)
}

type Namespace struct {
	source  *Component
	ordinal uint32
}
type Resolver struct {
	source  *Component
	ordinal uint32
}
type Resolution struct {
	source  *Component
	ordinal uint32
}
type InputRef struct {
	source  *Component
	ordinal uint32
}

// Expression is a zero-materialization view of one existing Program static
// expression under its exact module resolver.
type Expression struct {
	source    *Component
	shard     linkproject.Shard
	reference programstatic.StaticTypeRef
}

// ExpressionRef is the portable cold identity of an Expression. It carries
// only the Static relation digest, the dense Project mount ordinal, and the
// Program static reference. Find rebinds that ordinal through the receiving
// Component's exact Project Mounts authority; no hot Component or raw Link
// Shard is serialized in the reference.
type ExpressionRef struct {
	staticID     keyspace.ContentID
	shardOrdinal uint32
	reference    keyspace.Term
}

func (ref ExpressionRef) StaticID() keyspace.ContentID { return ref.staticID }
func (ref ExpressionRef) ShardOrdinal() uint32         { return ref.shardOrdinal }
func (ref ExpressionRef) Reference() keyspace.Term     { return ref.reference }

type ResolutionDisposition uint8

const (
	ResolutionInvalid ResolutionDisposition = iota
	ResolutionResolved
	ResolutionUnresolved
)

type InputKind uint8

const (
	InputInvalid InputKind = iota
	InputTypeOf
	InputAnnotation
)

type exportRow struct {
	root    keyspace.Term
	path    []keyspace.Key
	typeRef keyspace.Term
}
type namespaceRow struct {
	shard   linkproject.Shard
	content keyspace.ContentID
	exports []exportRow
}
type resolutionRow struct {
	shard       linkproject.Shard
	importTerm  keyspace.Term
	call        keyspace.Term
	literal     keyspace.Term
	alias       keyspace.Term
	disposition ResolutionDisposition
	namespace   Namespace
}
type inputRow struct {
	owner          keyspace.ContentID
	kind           InputKind
	source         keyspace.Term
	target         keyspace.Term
	operand        keyspace.Term
	resolver       Resolver
	frontierBody   keyspace.Term
	frontierCursor uint32
}
type qualifiedRow struct {
	consumerShard linkproject.Shard
	owner         keyspace.ContentID
	reference     keyspace.Term
	providerOwner keyspace.ContentID
	target        keyspace.Term
	resolver      Resolver
}
type shardTerm struct {
	shard linkproject.Shard
	term  keyspace.Term
}

// Namespaces is the vertical namespace/resolver view. It intentionally keeps
// its query vocabulary separate from expressions and resolutions.
type Namespaces struct{ source *Component }

// Expressions is the vertical static-expression view.
type Expressions struct{ source *Component }

// Resolutions is the vertical literal-require view.
type Resolutions struct{ source *Component }

// Inputs is the vertical static operand/frontier view.
type Inputs struct{ source *Component }

// Cold is a detached identity/schema/publication snapshot. It contains no
// pointer into the hot Static component and therefore remains portable across
// finalization and explicit artifact rebinding.
type Cold struct {
	contentID keyspace.ContentID
	schema    []keyspace.ContentID
	// semanticReceipt is the detached owner-bound receipt built while the
	// Component still owns the typed static projections.  It contains no
	// Component, Project, or Program pointer.
	semanticReceipt SemanticSourceReceipt
	// draft is a construction-only lifecycle fence; Component snapshots keep
	// it nil so their scalar identity remains portable after publication.
	fence *draftFence
}

func (c *Component) Namespaces() Namespaces   { return Namespaces{source: c} }
func (c *Component) Expressions() Expressions { return Expressions{source: c} }
func (c *Component) Resolutions() Resolutions { return Resolutions{source: c} }
func (c *Component) Inputs() Inputs           { return Inputs{source: c} }
func (c *Component) Cold() Cold               { return coldSnapshot(c, nil) }
func (d *Draft) Cold() Cold {
	if d == nil || d.state == nil {
		return Cold{}
	}
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if d.state.consumed || d.state.component == nil {
		return Cold{}
	}
	return coldSnapshot(d.state.component, d.state.fence)
}

func coldSnapshot(c *Component, fence *draftFence) Cold {
	if c == nil || !c.contentID.Available() {
		return Cold{}
	}
	schema := make([]keyspace.ContentID, len(c.namespaces))
	for index, namespace := range c.namespaces {
		schema[index] = namespace.content
	}
	receipt := c.semanticReceipt
	return Cold{
		contentID:       c.contentID,
		schema:          schema,
		semanticReceipt: receipt,
		fence:           fence,
	}
}
