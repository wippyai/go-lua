// Package boundary owns the factorized, Link-local Program/Target topology.
//
// Boundary deliberately stores no Application x Operation product.  The
// application and Target operation remain the canonical constituents; the
// membership predicate and the cold cardinality are derived directly from
// those two sealed authorities.
package boundary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link/internal/radix"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// Input is the complete construction boundary for the topology component.
// Project and Target must be the exact authorities from which the component
// is built; equivalent reseals are intentionally not interchangeable.
type Input struct {
	Project          *linkproject.Component
	Target           *target.Contract
	EndpointRequests []EndpointRequest
}

// EndpointRequest is one named direct provider ingress.  It deliberately
// contains only Target vocabulary: Host and its selector geometry are later
// consumers of this Boundary-owned admission relation.
type EndpointRequest struct {
	Identity string
	Binding  vocabulary.BindingSpec
}

// Draft is the linear construction capability for a Boundary component.
// Copies share the same lifecycle fence and cannot finalize twice.
type Draft struct{ state *draftState }

// Component is the immutable owner-fenced Boundary topology.
type Component struct{ authority *authority }

// Value is an opaque Boundary-issued identity for one existing Program value
// occurrence.  The dense ordinal is meaningful only to the exact Boundary
// component that issued it; a same-ordinal value from another component is
// rejected by every Values query.
type Value struct {
	component *Component
	ordinal   uint32
}

// Values is Boundary's complete canonical Program-value universe.  It is a
// typed view rather than a root Link forwarding surface: Project Shards are
// reissued only after validation against the retained exact Project owner.
type Values struct{ component *Component }

// Calls is Boundary's exact ordinary-Call view.  Applications remain owned by
// Project; Boundary owns the value-bearing call operand relation.
type Calls struct{ component *Component }

// Seed is an opaque Boundary-issued external value identity.  Its ordinal is
// meaningful only to the exact component that issued it.
type Seed struct {
	component *Component
	ordinal   uint32
}

// Seeds is the complete finite Boundary external-value universe.
type Seeds struct{ component *Component }

// Endpoint is the nominal identity of one requested provider endpoint. Two
// endpoint identities selecting the same operation remain distinct.
type Endpoint struct {
	component *Component
	ordinal   uint32
}

// Endpoints is Boundary's canonical provider-endpoint admission view.
type Endpoints struct{ component *Component }

// EndpointRequests is the canonical cold replay projection of the authored
// endpoint contract. It is intentionally separate from Endpoints: identities
// and bindings replay the input, while Endpoints issue nominal runtime handles.
type EndpointRequests struct{ component *Component }

// CallableDisposition is structural provenance only.  It never asserts an
// outcome, effect, or mutable value fact.
type CallableDisposition uint8

const (
	CallableInvalid CallableDisposition = iota
	CallableAdmittedOperation
	CallableDeniedTarget
)

type authority struct {
	component      *Component
	project        *linkproject.Component
	target         *target.Contract
	require        vocabulary.Operation
	valueTable     *valueTable
	seedTable      *seedTable
	moduleRelation identity.ContentID
	content        identity.ContentID
	countRows      denominator.CountRows
}

type valueTable struct {
	rows      []valueRow
	ids       []valueIDRow
	index     radix.Store
	spans     map[valueSpanKey]uint32
	semantic  map[valueSemanticKey]uint32
	mounts    map[identity.ContentID]uint32
	relations []identity.ContentID
	content   identity.ContentID
}

type valueSpanKey struct {
	mount   uint32
	context identity.ContentID
}

// valueSemanticKey is the construction-only inverse from a reusable Program
// semantic occurrence identity to one existing mounted Boundary Value.  It
// is not an authored-term index: the Link builder alone joins the live proof
// once, and consumers later supply only ModuleKey plus the opaque identity.
type valueSemanticKey struct {
	mount uint32
	id    identity.ContentID
}

type valueRow struct {
	shard uint32
	term  keyspace.Term
}

type valueIDRow struct {
	id      identity.ContentID
	ordinal uint32
}

type seedKind uint8

const (
	seedOperation seedKind = iota + 1
	seedLoader
	seedDeniedBootstrap
	seedEndpoint
)

type seedTable struct {
	rows             []seedRow
	operation        []uint32 // zero means scoped require has no global seed
	loaderByMount    []uint32 // zero iff no scoped require
	endpoints        []endpointRow
	requests         []endpointRequestRow
	endpointIDs      []endpointIDRow
	deniedStart      uint32
	deniedCount      uint32
	relation         identity.ContentID
	endpointRelation identity.ContentID
}

type seedRow struct {
	kind     seedKind
	op       vocabulary.Operation
	mount    uint32
	denied   vocabulary.InitialValue
	endpoint uint32 // one-based endpoint row only for seedEndpoint
}

type endpointRow struct {
	seed uint32 // seed row ordinal
	op   vocabulary.Operation
}

type endpointRequestRow struct {
	identity string
	binding  vocabulary.BindingSpec
}

type endpointIDRow struct {
	id      identity.ContentID
	ordinal uint32
}

type draftState struct {
	authority *authority
	consumed  bool
}
