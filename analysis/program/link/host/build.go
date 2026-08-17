package host

import (
	"crypto/sha256"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

var errUnavailable = errors.New("link/host: unavailable authority")

func Build(input Input) (*Draft, error) {
	a, err := newAuthority(input)
	if err != nil {
		return nil, err
	}
	a.spec = cloneSpec(input.Spec)
	if err := a.build(); err != nil {
		return nil, err
	}
	return &Draft{state: &draftState{authority: a}}, nil
}

// BuildReplay admits only detached identities issued by the exact prerequisite
// authorities.  It deliberately does not accept a second target/project
// coordinate dialect: replay rows are rebound through Target and Boundary.
func BuildReplay(input Input, replay ReplaySpec) (*Draft, error) {
	if len(input.Spec.ProviderCapabilities) != 0 || len(input.Spec.ProviderCapabilitySeeds) != 0 || len(input.Spec.Exposures) != 0 || len(input.Spec.Members) != 0 {
		return nil, errUnavailable
	}
	a, err := newAuthority(input)
	if err != nil {
		return nil, err
	}
	if err := a.buildReplay(replay); err != nil {
		return nil, err
	}
	return &Draft{state: &draftState{authority: a}}, nil
}

func newAuthority(input Input) (*authority, error) {
	if input.Project == nil || input.Boundary == nil || input.Module == nil || !input.Boundary.MatchesProject(input.Project) {
		return nil, errUnavailable
	}
	target, ok := input.Boundary.Target()
	if !ok || target == nil || !input.Project.MatchesTarget(target) {
		return nil, errUnavailable
	}
	// Module already verifies both exact prerequisites.  Its ContentID is not a
	// substitute for this pointer check: host rows consume its Actor/Root handles.
	if !input.Module.MatchesProject(input.Project) || !input.Module.MatchesBoundary(input.Boundary) {
		return nil, errUnavailable
	}
	a := &authority{project: input.Project, boundary: input.Boundary, module: input.Module, target: target, fence: &coldFence{}}
	c := &Component{authority: a}
	a.component = c
	return a, nil
}

func (a *authority) build() error {
	index, err := newHostBuildIndex(a.project.Mounts())
	if err != nil {
		return err
	}
	if err := a.capabilityRows(index); err != nil {
		return err
	}
	if err := a.selectorRows(index); err != nil {
		return err
	}
	if err := a.bootRows(); err != nil {
		return err
	}
	return a.sealResolved()
}

// buildReplay follows the same row builder as authored admission after first
// resolving every portable reference through its owning authority.  In
// particular, no Project term or Target ordinal is part of ReplaySpec.
func (a *authority) buildReplay(input ReplaySpec) error {
	if err := a.replayCapabilities(input.Capabilities); err != nil {
		return err
	}
	if err := a.replaySeedRows(input.Seeds); err != nil {
		return err
	}
	if err := a.replaySelectorRows(input.Exposures, input.Members); err != nil {
		return err
	}
	if err := a.bootRows(); err != nil {
		return err
	}
	return a.sealResolved()
}

func (a *authority) sealResolved() error {
	if a == nil {
		return errUnavailable
	}
	// This snapshot is deliberately the only point where either admission path
	// becomes Host state. It excludes authored Spec and replay transport bytes.
	resolved := resolvedHost{
		capabilities: a.capabilities, seeds: a.seeds, activeSeeds: a.activeSeeds,
		exposures: a.exposures, members: a.members,
	}
	a.capabilities, a.seeds, a.activeSeeds = resolved.capabilities, resolved.seeds, resolved.activeSeeds
	a.exposures, a.members = resolved.exposures, resolved.members
	var ok bool
	if a.replay, ok = makeReplaySpec(a); !ok {
		return errUnavailable
	}
	// The authored coordinate dialect is builder-local.  Once the exact rows
	// have produced the detached replay relation it must not survive into the
	// finalized authority alongside that portable representation.
	a.spec = Spec{}
	a.content = contentID(a)
	if !a.content.Available() {
		return errUnavailable
	}
	var viewsOK bool
	if a.sourceViews, viewsOK = a.component.buildSourceViews(); !viewsOK {
		return errUnavailable
	}
	return nil
}

func (a *authority) replayCapabilities(items []string) error {
	if len(items) > int(^uint32(0)) {
		return errUnavailable
	}
	a.capabilities = make([]ProviderCapabilitySpec, len(items))
	for i, identity := range items {
		if identity == "" {
			return errUnavailable
		}
		a.capabilities[i] = ProviderCapabilitySpec{Identity: identity}
	}
	sort.Slice(a.capabilities, func(i, j int) bool { return a.capabilities[i].Identity < a.capabilities[j].Identity })
	for i := 1; i < len(a.capabilities); i++ {
		if a.capabilities[i-1].Identity == a.capabilities[i].Identity {
			return errUnavailable
		}
	}
	return nil
}

func (a *authority) replayCapability(identity string) (uint32, bool) {
	i := sort.Search(len(a.capabilities), func(i int) bool { return a.capabilities[i].Identity >= identity })
	if i >= len(a.capabilities) || a.capabilities[i].Identity != identity {
		return 0, false
	}
	return uint32(i + 1), true
}

func (a *authority) replayRootIndex() (map[string]target.InitialRoot, error) {
	roots := make(map[string]target.InitialRoot, a.target.InitialRootCount())
	for i := 0; i < a.target.InitialRootCount(); i++ {
		root, ok := a.target.InitialRootAt(i)
		name, nok := a.target.InitialRootIdentity(root)
		if !ok || !nok || name == "" {
			return nil, errUnavailable
		}
		if _, duplicate := roots[name]; duplicate {
			return nil, errUnavailable
		}
		roots[name] = root
	}
	return roots, nil
}

func (a *authority) replaySeedRows(items []ReplayCapabilitySeed) error {
	roots, err := a.replayRootIndex()
	if err != nil {
		return err
	}
	a.seeds = make([]capabilitySeedRow, len(items))
	seeded := make([]bool, len(a.capabilities))
	for i, raw := range items {
		capability, ok := a.replayCapability(raw.Capability)
		if !ok {
			return errUnavailable
		}
		r := capabilitySeedRow{capability: capability, source: raw.Source}
		switch raw.Source {
		case ProviderCapabilitySourceInitialRoot:
			if raw.InputFormal.Available() || raw.OutcomeResult.Available() || raw.Value.Available() {
				return errUnavailable
			}
			r.root, ok = roots[raw.InitialRoot]
		case ProviderCapabilitySourceABIInput:
			if raw.InitialRoot != "" || raw.OutcomeResult.Available() || raw.Value.Available() {
				return errUnavailable
			}
			r.operation, r.formal, ok = a.target.FindInputFormalID(raw.InputFormal)
		case ProviderCapabilitySourceResult:
			if raw.InitialRoot != "" || raw.InputFormal.Available() || raw.Value.Available() {
				return errUnavailable
			}
			var outcome, result int
			r.operation, outcome, result, ok = a.target.FindOutcomeResultID(raw.OutcomeResult)
			r.outcome, r.result = uint32(outcome), uint32(result)
		case ProviderCapabilitySourceExposure:
			if raw.InitialRoot != "" || raw.InputFormal.Available() || raw.OutcomeResult.Available() {
				return errUnavailable
			}
			r.value, ok = a.boundary.Values().FindID(raw.Value)
		default:
			return errUnavailable
		}
		if !ok {
			return errUnavailable
		}
		a.seeds[i] = r
		seeded[capability-1] = true
	}
	for _, present := range seeded {
		if !present {
			return errUnavailable
		}
	}
	sort.Slice(a.seeds, func(i, j int) bool { return a.compareSeed(a.seeds[i], a.seeds[j]) < 0 })
	for i := 1; i < len(a.seeds); i++ {
		if a.compareSeed(a.seeds[i-1], a.seeds[i]) == 0 {
			return errUnavailable
		}
	}
	a.activeSeeds = make([]uint32, len(a.seeds))
	for i := range a.activeSeeds {
		a.activeSeeds[i] = uint32(i + 1)
	}
	return nil
}

func (a *authority) replaySelectorRows(exposures []ReplaySelector, members []ReplayMemberSelector) error {
	values, endpoints := a.boundary.Values(), a.boundary.Endpoints()
	build := func(valueID, endpointID identity.ContentID, capability string, dispatch HostDispatch, member bool) (selectorRow, error) {
		if dispatch != HostDispatchLookup {
			return selectorRow{}, errUnavailable
		}
		value, ok := values.FindID(valueID)
		if !ok {
			return selectorRow{}, errUnavailable
		}
		endpoint, ok := endpoints.FindID(endpointID)
		if !ok {
			return selectorRow{}, errUnavailable
		}
		shard, access, ok := values.Origin(value)
		if !ok {
			return selectorRow{}, errUnavailable
		}
		p, ok := a.project.Mounts().Program(shard)
		if !ok || !p.Flow().Executable().Contains(access) || !hostAccess(p, access, member) {
			return selectorRow{}, errUnavailable
		}
		r := selectorRow{shard: shard, access: access, output: value, endpoint: endpoint, dispatch: dispatch}
		if member {
			cap, ok := a.replayCapability(capability)
			if !ok {
				return selectorRow{}, errUnavailable
			}
			_, source, _, ok := p.Flow().Authored().Storage().Reads().Get(access)
			if !ok {
				return selectorRow{}, errUnavailable
			}
			sourceKey, ok := memberKey(p, source)
			if !ok {
				return selectorRow{}, errUnavailable
			}
			key, ok := a.project.Keys().ForProgram(shard, p, sourceKey)
			if !ok {
				return selectorRow{}, errUnavailable
			}
			r.capability, r.key = cap, key
		} else if capability != "" {
			return selectorRow{}, errUnavailable
		}
		return r, nil
	}
	for _, raw := range exposures {
		r, err := build(raw.Value, raw.Endpoint, "", raw.Dispatch, false)
		if err != nil {
			return err
		}
		a.exposures = append(a.exposures, r)
	}
	for _, raw := range members {
		r, err := build(raw.Value, raw.Endpoint, raw.Capability, raw.Dispatch, true)
		if err != nil {
			return err
		}
		a.members = append(a.members, r)
	}
	sort.Slice(a.exposures, func(i, j int) bool { return a.compareSelector(a.exposures[i], a.exposures[j]) < 0 })
	sort.Slice(a.members, func(i, j int) bool { return a.compareSelector(a.members[i], a.members[j]) < 0 })
	for _, rows := range [][]selectorRow{a.exposures, a.members} {
		for i := 1; i < len(rows); i++ {
			if a.compareSelector(rows[i-1], rows[i]) == 0 {
				return errUnavailable
			}
		}
	}
	return nil
}

type buildMount struct {
	shard   linkproject.Shard
	program *program.Program
}
type hostBuildIndex struct{ mounts map[string]buildMount }

func newHostBuildIndex(mounts linkproject.Mounts) (hostBuildIndex, error) {
	result := hostBuildIndex{mounts: make(map[string]buildMount, mounts.Count())}
	for i := 0; i < mounts.Count(); i++ {
		shard, ok := mounts.At(i)
		name, nok := mounts.Name(shard)
		p, pok := mounts.Program(shard)
		if !ok || !nok || !pok || name == "" || p == nil {
			return hostBuildIndex{}, errUnavailable
		}
		// Literal values are not necessarily access keys (for example a returned
		// string).  Key resolution is therefore deferred to a selected member
		// FieldExact row, instead of rejecting an otherwise empty Host merely for
		// mounting ordinary literal values.
		result.mounts[name] = buildMount{shard: shard, program: p}
	}
	return result, nil
}
func cloneSpec(in Spec) Spec {
	out := Spec{ProviderCapabilities: append([]ProviderCapabilitySpec(nil), in.ProviderCapabilities...), ProviderCapabilitySeeds: append([]ProviderCapabilitySeedSpec(nil), in.ProviderCapabilitySeeds...), Exposures: append([]HostExposureSpec(nil), in.Exposures...), Members: append([]HostMemberSpec(nil), in.Members...)}
	for i := range out.ProviderCapabilitySeeds {
		out.ProviderCapabilitySeeds[i].Binding.Owner = append([]string(nil), out.ProviderCapabilitySeeds[i].Binding.Owner...)
		out.ProviderCapabilitySeeds[i].Binding.Member = append([]string(nil), out.ProviderCapabilitySeeds[i].Binding.Member...)
	}
	return out
}
func cloneReplaySpec(in ReplaySpec) ReplaySpec {
	return ReplaySpec{Capabilities: append([]string(nil), in.Capabilities...), Seeds: append([]ReplayCapabilitySeed(nil), in.Seeds...), Exposures: append([]ReplaySelector(nil), in.Exposures...), Members: append([]ReplayMemberSelector(nil), in.Members...)}
}

// makeReplaySpec is the one authored-coordinate reduction.  It is used only
// while building; Cold subsequently returns the detached retained contract.
func makeReplaySpec(a *authority) (ReplaySpec, bool) {
	if a == nil || a.target == nil || a.boundary == nil {
		return ReplaySpec{}, false
	}
	out := ReplaySpec{Capabilities: make([]string, len(a.capabilities)), Seeds: make([]ReplayCapabilitySeed, len(a.seeds)), Exposures: make([]ReplaySelector, len(a.exposures)), Members: make([]ReplayMemberSelector, len(a.members))}
	for i, c := range a.capabilities {
		if c.Identity == "" {
			return ReplaySpec{}, false
		}
		out.Capabilities[i] = c.Identity
	}
	for i, r := range a.seeds {
		s := ReplayCapabilitySeed{Capability: a.capabilities[r.capability-1].Identity, Source: r.source}
		var ok bool
		switch r.source {
		case ProviderCapabilitySourceInitialRoot:
			s.InitialRoot, ok = a.target.InitialRootIdentity(r.root)
		case ProviderCapabilitySourceABIInput:
			s.InputFormal, ok = a.target.InputFormalID(r.operation, r.formal)
		case ProviderCapabilitySourceResult:
			s.OutcomeResult, ok = a.target.OutcomeResultID(r.operation, int(r.outcome), int(r.result))
		case ProviderCapabilitySourceExposure:
			s.Value, ok = a.boundary.Values().ID(r.value)
		default:
			return ReplaySpec{}, false
		}
		if !ok {
			return ReplaySpec{}, false
		}
		out.Seeds[i] = s
	}
	for i, r := range a.exposures {
		var ok bool
		out.Exposures[i].Value, ok = a.boundary.Values().ID(r.output)
		if !ok {
			return ReplaySpec{}, false
		}
		out.Exposures[i].Endpoint, ok = a.boundary.Endpoints().ID(r.endpoint)
		if !ok {
			return ReplaySpec{}, false
		}
		out.Exposures[i].Dispatch = r.dispatch
	}
	for i, r := range a.members {
		var ok bool
		out.Members[i].Capability = a.capabilities[r.capability-1].Identity
		out.Members[i].Value, ok = a.boundary.Values().ID(r.output)
		if !ok {
			return ReplaySpec{}, false
		}
		out.Members[i].Endpoint, ok = a.boundary.Endpoints().ID(r.endpoint)
		if !ok {
			return ReplaySpec{}, false
		}
		out.Members[i].Dispatch = r.dispatch
	}
	return out, true
}
func canonicalSpec(in Spec) Spec {
	out := cloneSpec(in)
	sort.Slice(out.ProviderCapabilities, func(i, j int) bool {
		return out.ProviderCapabilities[i].Identity < out.ProviderCapabilities[j].Identity
	})
	sort.Slice(out.ProviderCapabilitySeeds, func(i, j int) bool {
		return compareSeedSpec(out.ProviderCapabilitySeeds[i], out.ProviderCapabilitySeeds[j]) < 0
	})
	sort.Slice(out.Exposures, func(i, j int) bool {
		a, b := out.Exposures[i], out.Exposures[j]
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		if a.Access != b.Access {
			return a.Access < b.Access
		}
		if a.Endpoint != b.Endpoint {
			return a.Endpoint < b.Endpoint
		}
		return a.Dispatch < b.Dispatch
	})
	sort.Slice(out.Members, func(i, j int) bool {
		a, b := out.Members[i], out.Members[j]
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		if a.Access != b.Access {
			return a.Access < b.Access
		}
		if a.Capability != b.Capability {
			return a.Capability < b.Capability
		}
		if a.Endpoint != b.Endpoint {
			return a.Endpoint < b.Endpoint
		}
		return a.Dispatch < b.Dispatch
	})
	return out
}
func compareSeedSpec(a, b ProviderCapabilitySeedSpec) int {
	if a.Capability != b.Capability {
		if a.Capability < b.Capability {
			return -1
		}
		return 1
	}
	if a.Source != b.Source {
		if a.Source < b.Source {
			return -1
		}
		return 1
	}
	if a.InitialRoot != b.InitialRoot {
		if a.InitialRoot < b.InitialRoot {
			return -1
		}
		return 1
	}
	if o := compareBinding(a.Binding, b.Binding); o != 0 {
		return o
	}
	if a.Formal != b.Formal {
		if a.Formal < b.Formal {
			return -1
		}
		return 1
	}
	if a.Outcome != b.Outcome {
		if a.Outcome < b.Outcome {
			return -1
		}
		return 1
	}
	if a.Result != b.Result {
		if a.Result < b.Result {
			return -1
		}
		return 1
	}
	if a.Module != b.Module {
		if a.Module < b.Module {
			return -1
		}
		return 1
	}
	if a.Access < b.Access {
		return -1
	}
	if a.Access > b.Access {
		return 1
	}
	return 0
}
func compareBinding(a, b target.BindingSpec) int {
	if a.Namespace != b.Namespace {
		if a.Namespace < b.Namespace {
			return -1
		}
		return 1
	}
	for n := 0; n < 2; n++ {
		var x, y []string
		if n == 0 {
			x, y = a.Owner, b.Owner
		} else {
			x, y = a.Member, b.Member
		}
		for i := 0; i < len(x) && i < len(y); i++ {
			if x[i] < y[i] {
				return -1
			}
			if x[i] > y[i] {
				return 1
			}
		}
		if len(x) < len(y) {
			return -1
		}
		if len(x) > len(y) {
			return 1
		}
	}
	return 0
}
func (a *authority) capabilityRows(index hostBuildIndex) error {
	a.spec = canonicalSpec(a.spec)
	if len(a.spec.ProviderCapabilities) > int(^uint32(0)) {
		return errUnavailable
	}
	byName := make(map[string]uint32, len(a.spec.ProviderCapabilities))
	a.capabilities = a.spec.ProviderCapabilities
	for i, c := range a.capabilities {
		if c.Identity == "" || (i > 0 && a.capabilities[i-1].Identity == c.Identity) {
			return errUnavailable
		}
		byName[c.Identity] = uint32(i + 1)
	}
	a.seeds = make([]capabilitySeedRow, len(a.spec.ProviderCapabilitySeeds))
	seeded := make([]bool, len(a.capabilities))
	for i, s := range a.spec.ProviderCapabilitySeeds {
		cap, ok := byName[s.Capability]
		if !ok {
			return errUnavailable
		}
		r := capabilitySeedRow{capability: cap, source: s.Source}
		empty := s.Binding.Namespace == 0 && len(s.Binding.Owner) == 0 && len(s.Binding.Member) == 0
		switch s.Source {
		case ProviderCapabilitySourceInitialRoot:
			if s.InitialRoot == "" || !empty || s.Formal != 0 || s.Outcome != 0 || s.Result != 0 || s.Module != "" || s.Access != 0 {
				return errUnavailable
			}
			for n := 0; n < a.target.InitialRootCount(); n++ {
				root, ok := a.target.InitialRootAt(n)
				name, nameOK := a.target.InitialRootIdentity(root)
				if ok && nameOK && name == s.InitialRoot {
					r.root = root
					break
				}
			}
			if r.root == 0 {
				return errUnavailable
			}
		case ProviderCapabilitySourceABIInput:
			if s.InitialRoot != "" || !providerBinding(s.Binding) || s.Outcome != 0 || s.Result != 0 || s.Module != "" || s.Access != 0 {
				return errUnavailable
			}
			op, ok := a.target.Lookup(s.Binding)
			if !ok || int(s.Formal) >= a.target.ValueFormalCount(op) {
				return errUnavailable
			}
			r.operation, r.formal = op, s.Formal
		case ProviderCapabilitySourceResult:
			if s.InitialRoot != "" || !providerBinding(s.Binding) || s.Formal != 0 || s.Module != "" || s.Access != 0 {
				return errUnavailable
			}
			op, ok := a.target.Lookup(s.Binding)
			if !ok || int(s.Outcome) >= a.target.OutcomeCount(op) {
				return errUnavailable
			}
			_, values, ok := a.target.OutcomeAt(op, int(s.Outcome))
			if !ok || int(s.Result) >= a.target.ValuesCount(values) {
				return errUnavailable
			}
			r.operation, r.outcome, r.result = op, s.Outcome, s.Result
		case ProviderCapabilitySourceExposure:
			if s.InitialRoot != "" || !empty || s.Formal != 0 || s.Outcome != 0 || s.Result != 0 || s.Module == "" || s.Access == 0 {
				return errUnavailable
			}
			entry, ok := index.mounts[s.Module]
			shard, p := entry.shard, entry.program
			if !ok || !hostAccess(p, s.Access, false) || !p.Flow().Executable().Contains(s.Access) {
				return errUnavailable
			}
			value, ok := a.boundary.Values().Of(shard, s.Access)
			if !ok {
				return errUnavailable
			}
			r.value = value
		default:
			return errUnavailable
		}
		a.seeds[i] = r
		seeded[cap-1] = true
	}
	for _, ok := range seeded {
		if !ok {
			return errUnavailable
		}
	}
	sort.Slice(a.seeds, func(i, j int) bool { return a.compareSeed(a.seeds[i], a.seeds[j]) < 0 })
	for i := 1; i < len(a.seeds); i++ {
		if a.compareSeed(a.seeds[i-1], a.seeds[i]) == 0 {
			return errUnavailable
		}
	}
	// Admission above proves every retained source is active.  Preserve that
	// compact plane so Count/At never rescan the complete seed table.
	a.activeSeeds = make([]uint32, len(a.seeds))
	for i := range a.activeSeeds {
		a.activeSeeds[i] = uint32(i + 1)
	}
	return nil
}
func providerBinding(b target.BindingSpec) bool {
	return b.Namespace >= target.BindingBuiltin && b.Namespace <= target.BindingProvider && len(b.Member) != 0
}
func (a *authority) compareSeed(x, y capabilitySeedRow) int {
	if x.capability != y.capability {
		if x.capability < y.capability {
			return -1
		}
		return 1
	}
	if x.source != y.source {
		if x.source < y.source {
			return -1
		}
		return 1
	}
	for _, p := range [][2]uint32{{uint32(x.root), uint32(y.root)}, {uint32(x.operation), uint32(y.operation)}, {uint32(x.formal), uint32(y.formal)}, {x.outcome, y.outcome}, {x.result, y.result}} {
		if p[0] < p[1] {
			return -1
		}
		if p[0] > p[1] {
			return 1
		}
	}
	o, ok := a.boundary.Values().Compare(x.value, y.value)
	if !ok {
		return 0
	}
	return o
}
func (a *authority) selectorRows(index hostBuildIndex) error {
	endpoints := a.boundary.Endpoints()
	requests := a.boundary.EndpointRequests()
	byEndpoint := make(map[string]linkboundary.Endpoint, requests.Count())
	if requests.Count() != endpoints.Count() {
		return errUnavailable
	}
	for i := 0; i < requests.Count(); i++ {
		r, ok := requests.At(i)
		e, eok := endpoints.At(i)
		if !ok || !eok || r.Identity == "" {
			return errUnavailable
		}
		if _, seen := byEndpoint[r.Identity]; seen {
			return errUnavailable
		}
		byEndpoint[r.Identity] = e
	}
	byCap := make(map[string]uint32, len(a.capabilities))
	for i, c := range a.capabilities {
		byCap[c.Identity] = uint32(i + 1)
	}
	keys := a.project.Keys()
	values := a.boundary.Values()
	for _, raw := range a.spec.Exposures {
		entry, ok := index.mounts[raw.Module]
		s, p := entry.shard, entry.program
		e, eok := byEndpoint[raw.Endpoint]
		if !ok || !eok || raw.Access == 0 || raw.Dispatch != HostDispatchLookup || !hostAccess(p, raw.Access, false) {
			return errUnavailable
		}
		if !p.Flow().Executable().Contains(raw.Access) {
			return errUnavailable
		}
		v, ok := values.Of(s, raw.Access)
		if !ok {
			return errUnavailable
		}
		a.exposures = append(a.exposures, selectorRow{shard: s, access: raw.Access, output: v, endpoint: e, dispatch: raw.Dispatch})
	}
	for _, raw := range a.spec.Members {
		entry, ok := index.mounts[raw.Module]
		s, p := entry.shard, entry.program
		e, eok := byEndpoint[raw.Endpoint]
		cap, cok := byCap[raw.Capability]
		if !ok || !eok || !cok || raw.Access == 0 || raw.Dispatch != HostDispatchLookup || !hostAccess(p, raw.Access, true) {
			return errUnavailable
		}
		if !p.Flow().Executable().Contains(raw.Access) {
			return errUnavailable
		}
		_, source, _, rok := p.Flow().Authored().Storage().Reads().Get(raw.Access)
		sourceKey, kok := memberKey(p, source)
		key, kok2 := keys.ForProgram(s, p, sourceKey)
		v, vok := values.Of(s, raw.Access)
		if !rok || !kok || !kok2 || !vok {
			return errUnavailable
		}
		a.members = append(a.members, selectorRow{shard: s, access: raw.Access, capability: cap, key: key, output: v, endpoint: e, dispatch: raw.Dispatch})
	}
	sort.Slice(a.exposures, func(i, j int) bool { return a.compareSelector(a.exposures[i], a.exposures[j]) < 0 })
	sort.Slice(a.members, func(i, j int) bool { return a.compareSelector(a.members[i], a.members[j]) < 0 })
	for _, rows := range [][]selectorRow{a.exposures, a.members} {
		for i := 1; i < len(rows); i++ {
			if a.compareSelector(rows[i-1], rows[i]) == 0 {
				return errUnavailable
			}
		}
	}
	return nil
}
func hostAccess(p *program.Program, access keyspace.Term, member bool) bool {
	if p == nil {
		return false
	}
	_, source, _, ok := p.Flow().Authored().Storage().Reads().Get(access)
	if !ok || source == 0 {
		return false
	}
	if !member {
		cellKind, body, key, ok := p.Flow().Authored().Storage().Cells().Get(source)
		return ok && cellKind == flow.CellGlobal && body == 0 && key != 0
	}
	_, base, _, kind, ok := p.Flow().Authored().Access().Exact().Get(source)
	return ok && base != 0 && kind != flowkind.FieldList && kind != flowkind.FieldKey
}
func memberKey(p *program.Program, lens keyspace.Term) (keyspace.Key, bool) {
	_, _, term, kind, ok := p.Flow().Authored().Access().Exact().Get(lens)
	if !ok {
		return 0, false
	}
	keys := p.Source().Keys()
	if kind == flowkind.FieldName {
		_, _, key, ok := keys.Name(term)
		v, vok := keys.Exact(key)
		return key, ok && vok && v.Kind == keyspace.LiteralString
	}
	if kind == flowkind.FieldExact {
		if term == 0 {
			return 0, false
		}
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 {
			return 0, false
		}
		_, _, value, found := p.Source().Literals().Strings().At(int(ordinal - 1))
		if !found {
			return 0, false
		}
		key, ok := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value})
		return key, ok && key != 0
	}
	return 0, false
}
func (a *authority) compareSelector(x, y selectorRow) int {
	xi, _ := a.project.Mounts().Index(x.shard)
	yi, _ := a.project.Mounts().Index(y.shard)
	if xi < yi {
		return -1
	}
	if xi > yi {
		return 1
	}
	if x.access < y.access {
		return -1
	}
	if x.access > y.access {
		return 1
	}
	if x.capability < y.capability {
		return -1
	}
	if x.capability > y.capability {
		return 1
	}
	if o, ok := a.project.Keys().Compare(x.key, y.key); ok && o != 0 {
		return o
	}
	xid, xok := a.boundary.Endpoints().ID(x.endpoint)
	yid, yok := a.boundary.Endpoints().ID(y.endpoint)
	if !xok || !yok {
		return 0
	}
	for i := range xid {
		if xid[i] < yid[i] {
			return -1
		}
		if xid[i] > yid[i] {
			return 1
		}
	}
	if o, ok := a.boundary.Values().Compare(x.output, y.output); ok && o != 0 {
		return o
	}
	if x.dispatch < y.dispatch {
		return -1
	}
	if x.dispatch > y.dispatch {
		return 1
	}
	return 0
}

func (a *authority) bootRows() error {
	actors := a.module.Actors()
	roots := a.module.Roots()
	nRoots := a.target.InitialRootCount()
	if uint64(actors.Count())*uint64(nRoots) > uint64(^uint32(0)) {
		return errUnavailable
	}
	a.bootRoots = uint32(actors.Count() * nRoots)
	global, hasGlobal := a.target.GlobalEnvRoot()
	for i := 0; i < roots.Count(); i++ {
		analysis, ok := roots.At(i)
		if !ok {
			return errUnavailable
		}
		shard, actor, _, ok := roots.Mapping(analysis)
		if !ok {
			return errUnavailable
		}
		p, ok := a.project.Mounts().Program(shard)
		if !ok {
			return errUnavailable
		}
		for j := 0; j < p.Flow().Authored().Storage().Cells().Count(); j++ {
			cell, ok := p.Flow().Authored().Storage().Cells().At(j)
			if !ok {
				return errUnavailable
			}
			kind, body, key, ok := p.Flow().Authored().Storage().Cells().Get(cell)
			if !ok || kind != flow.CellGlobal || body != 0 || key == 0 {
				continue
			}
			if !hasGlobal {
				return errUnavailable
			}
			boot, ok := a.bootFor(actor, global)
			if !ok {
				return errUnavailable
			}
			lit, lok := p.Source().Keys().Exact(key)
			if !lok || lit.Kind != keyspace.LiteralString || lit.String == "" {
				return errUnavailable
			}
			class, value, initial, bindingKey, found := a.target.InitialBinding(lit.String)
			if !found {
				absent, ok := a.target.InitialAbsent()
				if !ok {
					return errUnavailable
				}
				class, value, initial = target.InitialBindingOrdinary, absent, global
			}
			if initial != global || !validGlobal(a.target, class, value, found) || found && !targetKeyName(a.target, bindingKey, lit.String) {
				return errUnavailable
			}
			a.globals = append(a.globals, globalBindingRow{analysis: analysis, boot: boot.ordinal, cell: cell, key: key, class: class, value: value})
		}
	}
	sort.Slice(a.globals, func(i, j int) bool {
		if o, ok := roots.Compare(a.globals[i].analysis, a.globals[j].analysis); ok && o != 0 {
			return o < 0
		}
		return a.globals[i].key < a.globals[j].key
	})
	a.globalRanges = make([]edgeRange, roots.Count())
	at := 0
	for i := 0; i < roots.Count(); i++ {
		r, ok := roots.At(i)
		if !ok {
			return errUnavailable
		}
		start := at
		for at < len(a.globals) && a.globals[at].analysis == r {
			at++
		}
		a.globalRanges[i] = edgeRange{uint32(start), uint32(at)}
	}
	// Build the sole Host-owned inverse after the canonical Global rows have
	// reached their final dense order.  The key retains the exact Project Shard
	// handle, while the value is only a Host-local row ordinal.  Rejecting a
	// collision here is part of sealing: a lookup by (Shard, Cell) must never
	// silently select one of multiple actor/root rows.
	a.globalByShardCell = make(map[globalLookupKey]uint32, len(a.globals))
	for index, row := range a.globals {
		shard, _, _, ok := roots.Mapping(row.analysis)
		if !ok {
			return errUnavailable
		}
		shardIndex, ok := a.project.Mounts().Index(shard)
		if !ok {
			return errUnavailable
		}
		lookup := globalLookupKey{shard: uint32(shardIndex), cell: row.cell}
		if _, duplicate := a.globalByShardCell[lookup]; duplicate {
			return errUnavailable
		}
		a.globalByShardCell[lookup] = uint32(index + 1)
	}
	attachments := a.target.InitialMetatableAttachmentCount()
	if uint64(actors.Count())*uint64(attachments) > uint64(^uint32(0)) {
		return errUnavailable
	}
	for i := 0; i < actors.Count(); i++ {
		actor, ok := actors.At(i)
		if !ok {
			return errUnavailable
		}
		for j := 0; j < attachments; j++ {
			base, root, ok := a.target.InitialMetatableAttachmentAt(j)
			if !ok || base != target.InitialValueString {
				return errUnavailable
			}
			boot, ok := a.bootFor(actor, root)
			if !ok {
				return errUnavailable
			}
			a.attachments = append(a.attachments, bootAttachmentRow{base: base, boot: boot.ordinal})
		}
	}
	return nil
}
func validGlobal(t *target.Contract, class target.InitialBindingClass, value target.InitialValue, found bool) bool {
	kind, ok := t.InitialValueKind(value)
	if !ok {
		return false
	}
	if !found {
		return kind == target.InitialValueAbsent
	}
	switch class {
	case target.InitialBindingAdmitted:
		op, ok := t.InitialValueOperation(value)
		return kind == target.InitialValueOperation && ok && op != 0
	case target.InitialBindingDenied:
		return kind == target.InitialValueDeniedOperation
	case target.InitialBindingOrdinary:
		return kind == target.InitialValueRoot || kind == target.InitialValueNil || kind == target.InitialValueBoolean || kind == target.InitialValueInteger || kind == target.InitialValueFloat || kind == target.InitialValueString || kind == target.InitialValueAbsent
	}
	return false
}
func targetKeyName(t *target.Contract, key target.ExactKey, name string) bool {
	v, ok := t.ExactKeyValue(key)
	return ok && v.Kind == keyspace.LiteralString && v.String == name
}
func (a *authority) bootFor(actor linkmodule.Actor, root target.InitialRoot) (BootRoot, bool) {
	if root == 0 || int(root) > a.target.InitialRootCount() {
		return BootRoot{}, false
	}
	i, ok := a.module.Actors().Index(actor)
	if !ok {
		return BootRoot{}, false
	}
	return BootRoot{a.component, uint32(i*a.target.InitialRootCount() + int(root))}, true
}

func contentID(a *authority) (id identity.ContentID) {
	if a == nil || a.target == nil {
		return
	}
	module, moduleOK := a.module.HostRelationID()
	boot, bootOK := a.target.BootRelationID()
	if !moduleOK || !bootOK {
		return
	}
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, "program/link/host", 3) != nil || w.Record(1) != nil || w.Bytes(module[:]) != nil || w.Bytes(boot[:]) != nil {
		return
	}
	if w.Count(uint64(len(a.replay.Capabilities))) != nil {
		return
	}
	for _, x := range a.replay.Capabilities {
		if w.String(x) != nil {
			return
		}
	}
	if w.Count(uint64(len(a.replay.Seeds))) != nil {
		return
	}
	for _, x := range a.replay.Seeds {
		if w.String(x.Capability) != nil || w.Uint(uint64(x.Source)) != nil || w.String(x.InitialRoot) != nil || w.Bytes(x.InputFormal[:]) != nil || w.Bytes(x.OutcomeResult[:]) != nil || w.Bytes(x.Value[:]) != nil {
			return
		}
	}
	if w.Count(uint64(len(a.replay.Exposures))) != nil {
		return
	}
	for _, x := range a.replay.Exposures {
		if w.Bytes(x.Value[:]) != nil || w.Bytes(x.Endpoint[:]) != nil || w.Uint(uint64(x.Dispatch)) != nil {
			return
		}
	}
	if w.Count(uint64(len(a.replay.Members))) != nil {
		return
	}
	for _, x := range a.replay.Members {
		if w.String(x.Capability) != nil || w.Bytes(x.Value[:]) != nil || w.Bytes(x.Endpoint[:]) != nil || w.Uint(uint64(x.Dispatch)) != nil {
			return
		}
	}
	// BootRelationID covers target boot topology.  Globals additionally commit
	// only their selected Module root, exact literal key, binding class, and
	// InitialValueContentID; no whole Project/Boundary digest leaks upward.
	if w.Count(uint64(len(a.globals))) != nil {
		return
	}
	for _, row := range a.globals {
		root, ok := a.module.Roots().ID(row.analysis)
		if !ok || w.Bytes(root[:]) != nil || w.Uint(uint64(row.class)) != nil {
			return identity.ContentID{}
		}
		value, ok := a.target.InitialValueContentID(row.value)
		if !ok || w.Bytes(value[:]) != nil {
			return identity.ContentID{}
		}
		shard, _, _, mapped := a.module.Roots().Mapping(row.analysis)
		p, programOK := a.project.Mounts().Program(shard)
		projectKey, keyOK := a.project.Keys().ForProgram(shard, p, row.key)
		key, ok := a.project.Keys().Exact(projectKey)
		if !mapped || !programOK || !keyOK || !ok || encodeHostLiteral(&w, key) != nil {
			return identity.ContentID{}
		}
	}
	if w.Finish() != nil {
		return
	}
	sum := h.Sum(id[:0])
	if len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func encodeHostLiteral(w *framing.Writer, value keyspace.LiteralValue) error {
	if err := w.Uint(uint64(value.Kind)); err != nil {
		return err
	}
	switch value.Kind {
	case keyspace.LiteralBool:
		return w.Bool(value.Bool)
	case keyspace.LiteralInteger:
		return w.Uint(uint64(value.Integer))
	case keyspace.LiteralFloat:
		return w.Uint(value.FloatBits)
	case keyspace.LiteralString:
		return w.String(value.String)
	default:
		return errUnavailable
	}
}
