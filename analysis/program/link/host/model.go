// Package host owns the immutable host/provider and bootstrap projection.
//
// It is a child of Project, Boundary, and Module.  It deliberately has no
// dependency on the Link composition root: every live coordinate it issues is
// fenced by this Component or by the child which originally issued it.
package host

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

type ProviderCapability struct {
	component *Component
	ordinal   uint32
}
type ProviderCapabilitySeed struct {
	component *Component
	ordinal   uint32
}

// ProviderCapabilitySeedRef is detached replay identity.  It cannot mint a
// seed: FindProviderCapabilitySeed reissues only from this exact Host seal.
type ProviderCapabilitySeedRef struct {
	hostID  identity.ContentID
	ordinal uint32
}

func (r ProviderCapabilitySeedRef) HostID() identity.ContentID { return r.hostID }

type BootRoot struct {
	component *Component
	ordinal   uint32
}
type BootMetatableAttachment struct {
	component *Component
	ordinal   uint32
}
type GlobalBinding struct {
	component *Component
	ordinal   uint32
}

type HostDispatch uint8

const (
	HostDispatchInvalid HostDispatch = iota
	HostDispatchLookup
)

type ProviderCapabilitySpec struct{ Identity string }
type ProviderCapabilitySource uint8

const (
	ProviderCapabilitySourceInvalid ProviderCapabilitySource = iota
	ProviderCapabilitySourceInitialRoot
	ProviderCapabilitySourceABIInput
	ProviderCapabilitySourceResult
	ProviderCapabilitySourceExposure
)

type ProviderCapabilitySeedSpec struct {
	Capability      string
	Source          ProviderCapabilitySource
	InitialRoot     string
	Binding         vocabulary.BindingSpec
	Formal          vocabulary.ValueFormal
	Outcome, Result uint32
	Module          string
	Access          keyspace.Term
}
type HostExposureSpec struct {
	Module   string
	Access   keyspace.Term
	Endpoint string
	Dispatch HostDispatch
}
type HostMemberSpec struct {
	Module               string
	Access               keyspace.Term
	Capability, Endpoint string
	Dispatch             HostDispatch
}
type Spec struct {
	ProviderCapabilities    []ProviderCapabilitySpec
	ProviderCapabilitySeeds []ProviderCapabilitySeedSpec
	Exposures               []HostExposureSpec
	Members                 []HostMemberSpec
}
type Input struct {
	Project  *linkproject.Component
	Boundary *linkboundary.Component
	Module   *linkmodule.Component
	Spec     Spec
}

// ReplaySpec is the detached portable Host input.  It deliberately contains
// no Program Term, Target ordinal, or hot child handle.
type ReplaySpec struct {
	Capabilities []string
	Seeds        []ReplayCapabilitySeed
	Exposures    []ReplaySelector
	Members      []ReplayMemberSelector
}
type ReplayCapabilitySeed struct {
	Capability                        string
	Source                            ProviderCapabilitySource
	InitialRoot                       string
	InputFormal, OutcomeResult, Value identity.ContentID
}
type ReplaySelector struct {
	Value, Endpoint identity.ContentID
	Dispatch        HostDispatch
}
type ReplayMemberSelector struct {
	Capability      string
	Value, Endpoint identity.ContentID
	Dispatch        HostDispatch
}
type Draft struct{ state *draftState }
type Component struct{ authority *authority }
type Cold struct {
	content identity.ContentID
	replay  ReplaySpec
	counts  denominator.CountRows
	fence   *coldFence
}
type coldFence struct{ sealed bool }
type draftState struct {
	authority *authority
	consumed  bool
}

type selectorRow struct {
	shard      linkproject.Shard
	access     keyspace.Term
	output     linkboundary.Value
	capability uint32
	key        linkproject.Key
	endpoint   linkboundary.Endpoint
	dispatch   HostDispatch
}
type capabilitySeedRow struct {
	capability      uint32
	source          ProviderCapabilitySource
	root            vocabulary.InitialRoot
	operation       vocabulary.Operation
	formal          vocabulary.ValueFormal
	outcome, result uint32
	value           linkboundary.Value
}
type globalBindingRow struct {
	analysis linkmodule.AnalysisRoot
	boot     uint32
	cell     keyspace.Term
	key      keyspace.Key
	class    vocabulary.InitialBindingClass
	value    vocabulary.InitialValue
}

// globalLookupKey is the owner-local inverse key for one canonical Program
// global occurrence. Its shard field is the sealed Project mount slot, not a
// portable coordinate; query admission still requires the exact opaque Shard
// and mounted Program handles. A Host may issue at most one GlobalBinding for
// a (Shard, Cell) pair.
type globalLookupKey struct {
	// shard is the canonical zero-based Project mount slot.  It is an
	// owner-local index coordinate, never a portable identity; query admission
	// must still present the exact Shard and mounted Program handles.
	shard uint32
	cell  keyspace.Term
}
type bootAttachmentRow struct {
	base vocabulary.InitialValueKind
	boot uint32
}
type edgeRange struct{ start, end uint32 }

// resolvedHost is the single admitted Host relation.  Both authored syntax and
// portable replay are reduced to these owner-fenced handles before the shared
// finalizer derives ReplaySpec and ContentID.
type resolvedHost struct {
	capabilities       []ProviderCapabilitySpec
	seeds              []capabilitySeedRow
	activeSeeds        []uint32
	exposures, members []selectorRow
}
type authority struct {
	component          *Component
	project            *linkproject.Component
	boundary           *linkboundary.Component
	module             *linkmodule.Component
	target             *target.Contract // issued only by Boundary's exact-owner query.
	capabilities       []ProviderCapabilitySpec
	seeds              []capabilitySeedRow
	activeSeeds        []uint32 // compact active ordinals; query At is O(1)
	exposures, members []selectorRow
	bootRoots          uint32
	attachments        []bootAttachmentRow
	globals            []globalBindingRow
	globalRanges       []edgeRange
	// globalByShardCell is a sealed derived inverse. Its values are Host-local
	// dense ordinals (not portable identities) and its keys are canonical
	// Project mount slots; query-time Shard and Program fences prevent equivalent
	// foreign authorities from matching.
	globalByShardCell map[globalLookupKey]uint32
	content           identity.ContentID
	// replay is the sole portable construction contract retained after sealing.
	// Authored coordinates are reduced to this relation before content is made.
	replay ReplaySpec
	counts denominator.CountRows
	spec   Spec // transient authored input; never exposed by Cold.
	fence  *coldFence
}

func (c *Component) ContentID() identity.ContentID {
	if c == nil || c.authority == nil {
		return identity.ContentID{}
	}
	return c.authority.content
}
func (c *Component) Cold() Cold {
	if c == nil || c.authority == nil {
		return Cold{}
	}
	if !c.authority.content.Available() {
		return Cold{}
	}
	return Cold{content: c.authority.content, replay: cloneReplaySpec(c.authority.replay), counts: c.authority.counts, fence: c.authority.fence}
}
func (c Cold) ContentID() identity.ContentID { return c.content }
func (c Cold) ReplaySpec() (ReplaySpec, bool) {
	if c.fence == nil || !c.fence.sealed || !c.content.Available() {
		return ReplaySpec{}, false
	}
	return cloneReplaySpec(c.replay), true
}
func (d *Draft) Cold() Cold {
	if d == nil || d.state == nil || d.state.consumed || d.state.authority == nil {
		return Cold{}
	}
	a := d.state.authority
	if !a.content.Available() {
		return Cold{}
	}
	return Cold{content: a.content, replay: cloneReplaySpec(a.replay), counts: a.counts, fence: a.fence}
}
func (d *Draft) Finalize() (*Component, error) {
	if d == nil || d.state == nil || d.state.consumed || d.state.authority == nil {
		return nil, errUnavailable
	}
	d.state.consumed = true
	a := d.state.authority
	if a.component == nil || a.component.authority != a {
		return nil, errUnavailable
	}
	a.fence.sealed = true
	return a.component, nil
}
