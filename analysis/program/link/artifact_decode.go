package link

import (
	"bytes"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

const linkArtifactModuleWireMin = 40

func decodeLinkArtifact(data []byte, contract *target.Contract, programs map[identity.ContentID]*program.Program) (*Link, error) {
	measure, err := framing.Scan(data, linkArtifactMaxBytes)
	if err != nil {
		return nil, err
	}
	if !artifactMeasureAllowed(measure) {
		return nil, ErrArtifactLimit
	}
	reader, err := framing.NewReader(data, linkArtifactMaxBytes)
	if err != nil {
		return nil, err
	}
	if err := reader.Header(linkArtifactDomain, linkArtifactCodecVersion); err != nil {
		return nil, err
	}
	decoder := linkArtifactDecoder{r: reader, programs: programs, stringBytes: measure.StringBytes}
	modules, replay, cache, claimed, err := decoder.root(contract)
	if err != nil {
		return nil, err
	}
	if err := reader.Finish(); err != nil {
		return nil, err
	}
	link, err := sealReplay(&Spec{Target: contract, Modules: modules, EndpointRequests: decoder.endpointRequests, Module: cache}, replay)
	if err != nil {
		return nil, fmt.Errorf("link artifact: module replay: %w", err)
	}
	if link.ContentID() != claimed {
		return nil, fmt.Errorf("%w: replay ContentID differs", ErrArtifactCanonical)
	}
	canonical, err := encodeLinkArtifactBounded(link, linkArtifactMaxBytes)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(data, canonical) {
		return nil, ErrArtifactCanonical
	}
	return link, nil
}

type linkArtifactDecoder struct {
	r                *framing.Reader
	programs         map[identity.ContentID]*program.Program
	stringBytes      uint64
	budget           *linkArtifactBudget
	endpointRequests []linkboundary.EndpointRequest
}

func (decoder *linkArtifactDecoder) root(contract *target.Contract) ([]linkproject.Module, linkhost.ReplaySpec, linkmodule.Spec, identity.ContentID, error) {
	if err := decoder.record(linkArtifactRecordRoot); err != nil {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
	}
	storedTarget, err := decoder.id()
	if err != nil {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
	}
	if storedTarget != contract.ContentID() {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrArtifactTarget
	}
	claimed, err := decoder.id()
	if err != nil || !claimed.Available() {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrArtifactCanonical
	}
	count, err := decoder.moduleCount()
	if err != nil {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
	}
	budget, ok := newLinkArtifactBudget(contract)
	if !ok {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrArtifactLimit
	}
	decoder.budget = &budget
	// Never let an untrusted arity size a Go allocation. The portable
	// reconstruction budget admits each row before append/map growth.
	var modules []linkproject.Module
	seenNames := make(map[string]struct{})
	var priorID identity.ContentID
	priorName := ""
	for index := 0; index < count; index++ {
		if err := decoder.record(linkArtifactRecordModule); err != nil {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
		}
		id, err := decoder.id()
		if err != nil || !id.Available() {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrArtifactCanonical
		}
		name, err := decoder.string()
		if err != nil {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
		}
		if name == "" || (index != 0 && compareArtifactModule(priorID, priorName, id, name) >= 0) {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrArtifactCanonical
		}
		if _, duplicate := seenNames[name]; duplicate {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrArtifactCanonical
		}
		sealed := decoder.programs[id]
		if sealed == nil || sealed.ContentID() != id {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, fmt.Errorf("%w: %x", ErrArtifactProgram, id)
		}
		if !budget.module(name, sealed) {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrArtifactLimit
		}
		seenNames[name] = struct{}{}
		modules = append(modules, linkproject.Module{Name: name, Program: sealed})
		priorID, priorName = id, name
	}
	replay, err := decoder.host()
	if err != nil {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
	}
	cache, err := decoder.cache()
	if err != nil {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
	}
	return modules, replay, cache, claimed, nil
}

func (decoder *linkArtifactDecoder) host() (linkhost.ReplaySpec, error) {
	var result linkhost.ReplaySpec
	count, err := decoder.cacheCount()
	if err != nil {
		return result, err
	}
	if !decoder.reserve(count, linkArtifactHostEndpointBytes) {
		return result, ErrArtifactLimit
	}
	decoder.endpointRequests = make([]linkboundary.EndpointRequest, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(linkArtifactRecordHostEndpoint); err != nil {
			return result, err
		}
		identity, err := decoder.string()
		if err != nil {
			return result, err
		}
		namespace, err := decoder.r.Count()
		if err != nil || namespace < uint64(target.BindingBuiltin) || namespace > uint64(target.BindingProvider) {
			return result, ErrArtifactCanonical
		}
		owner, err := decoder.hostParts()
		if err != nil {
			return result, err
		}
		member, err := decoder.hostParts()
		if err != nil {
			return result, err
		}
		decoder.endpointRequests = append(decoder.endpointRequests, linkboundary.EndpointRequest{Identity: identity, Binding: target.BindingSpec{Namespace: target.BindingNamespace(namespace), Owner: owner, Member: member}})
	}
	result.Capabilities, err = decoder.providerCapabilities()
	if err != nil {
		return result, err
	}
	result.Seeds, err = decoder.providerCapabilitySeeds()
	if err != nil {
		return result, err
	}
	result.Exposures, err = decoder.hostSelectors(linkArtifactRecordHostExposure)
	if err != nil {
		return result, err
	}
	result.Members, err = decoder.hostMembers()
	return result, err
}

func (decoder *linkArtifactDecoder) providerCapabilities() ([]string, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, linkArtifactHostSelectorBytes) {
		return nil, ErrArtifactLimit
	}
	result := make([]string, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(linkArtifactRecordProviderCapability); err != nil {
			return nil, err
		}
		identity, err := decoder.string()
		if err != nil {
			return nil, err
		}
		result = append(result, identity)
	}
	return result, nil
}

func (decoder *linkArtifactDecoder) providerCapabilitySeeds() ([]linkhost.ReplayCapabilitySeed, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, linkArtifactHostSelectorBytes) {
		return nil, ErrArtifactLimit
	}
	result := make([]linkhost.ReplayCapabilitySeed, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(linkArtifactRecordProviderCapabilitySeed); err != nil {
			return nil, err
		}
		capability, err := decoder.string()
		if err != nil {
			return nil, err
		}
		source, err := decoder.r.Count()
		if err != nil || source > uint64(linkhost.ProviderCapabilitySourceExposure) {
			return nil, ErrArtifactCanonical
		}
		initialRoot, err := decoder.string()
		if err != nil {
			return nil, err
		}
		inputFormal, err := decoder.id()
		if err != nil {
			return nil, err
		}
		outcomeResult, err := decoder.id()
		if err != nil {
			return nil, err
		}
		value, err := decoder.id()
		if err != nil {
			return nil, err
		}
		result = append(result, linkhost.ReplayCapabilitySeed{Capability: capability, Source: linkhost.ProviderCapabilitySource(source), InitialRoot: initialRoot, InputFormal: inputFormal, OutcomeResult: outcomeResult, Value: value})
	}
	return result, nil
}

// hostParts belongs to the Boundary endpoint-request wire row, whose binding
// is authored text. Host replay rows below deliberately do not use it.
func (decoder *linkArtifactDecoder) hostParts() ([]string, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, linkArtifactHostSelectorBytes) {
		return nil, ErrArtifactLimit
	}
	result := make([]string, 0, count)
	for index := 0; index < count; index++ {
		item, err := decoder.string()
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (decoder *linkArtifactDecoder) hostSelectors(record uint64) ([]linkhost.ReplaySelector, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, linkArtifactHostSelectorBytes) {
		return nil, ErrArtifactLimit
	}
	result := make([]linkhost.ReplaySelector, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(record); err != nil {
			return nil, err
		}
		value, err := decoder.id()
		if err != nil {
			return nil, err
		}
		endpoint, err := decoder.id()
		if err != nil {
			return nil, err
		}
		dispatch, err := decoder.r.Count()
		if err != nil || dispatch != uint64(linkhost.HostDispatchLookup) {
			return nil, ErrArtifactCanonical
		}
		result = append(result, linkhost.ReplaySelector{Value: value, Endpoint: endpoint, Dispatch: linkhost.HostDispatch(dispatch)})
	}
	return result, nil
}

func (decoder *linkArtifactDecoder) hostMembers() ([]linkhost.ReplayMemberSelector, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, linkArtifactHostSelectorBytes) {
		return nil, ErrArtifactLimit
	}
	result := make([]linkhost.ReplayMemberSelector, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(linkArtifactRecordHostMember); err != nil {
			return nil, err
		}
		capability, err := decoder.string()
		if err != nil {
			return nil, err
		}
		value, err := decoder.id()
		if err != nil {
			return nil, err
		}
		endpoint, err := decoder.id()
		if err != nil {
			return nil, err
		}
		dispatch, err := decoder.r.Count()
		if err != nil || dispatch != uint64(linkhost.HostDispatchLookup) {
			return nil, ErrArtifactCanonical
		}
		result = append(result, linkhost.ReplayMemberSelector{Capability: capability, Value: value, Endpoint: endpoint, Dispatch: linkhost.HostDispatch(dispatch)})
	}
	return result, nil
}

func (decoder *linkArtifactDecoder) cache() (linkmodule.Spec, error) {
	var result linkmodule.Spec
	count, err := decoder.cacheCount()
	if err != nil {
		return result, err
	}
	if !decoder.reserve(count, linkArtifactActorSpecBytes) {
		return result, ErrArtifactLimit
	}
	result.Actors = make([]linkmodule.ActorSpec, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(linkArtifactRecordActor); err != nil {
			return result, err
		}
		name, err := decoder.string()
		if err != nil {
			return result, err
		}
		result.Actors = append(result.Actors, linkmodule.ActorSpec{Name: name})
	}
	count, err = decoder.cacheCount()
	if err != nil {
		return result, err
	}
	if !decoder.reserve(count, linkArtifactAliasClassBytes) {
		return result, ErrArtifactLimit
	}
	result.ModuleCacheAliases = make([]linkmodule.ModuleCacheAliasClassSpec, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(linkArtifactRecordAliasClass); err != nil {
			return result, err
		}
		actor, err := decoder.string()
		if err != nil {
			return result, err
		}
		representative, err := decoder.string()
		if err != nil {
			return result, err
		}
		members, err := decoder.cacheCount()
		if err != nil {
			return result, err
		}
		if !decoder.reserve(members, linkArtifactAliasMemberBytes) {
			return result, ErrArtifactLimit
		}
		item := linkmodule.ModuleCacheAliasClassSpec{Actor: actor, Representative: representative, Instances: make([]string, 0, members)}
		for member := 0; member < members; member++ {
			value, err := decoder.string()
			if err != nil {
				return result, err
			}
			item.Instances = append(item.Instances, value)
		}
		result.ModuleCacheAliases = append(result.ModuleCacheAliases, item)
	}
	count, err = decoder.cacheCount()
	if err != nil {
		return result, err
	}
	if !decoder.reserve(count, linkArtifactAnalysisRootSpecBytes) {
		return result, ErrArtifactLimit
	}
	result.AnalysisRoots = make([]linkmodule.AnalysisRootSpec, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(linkArtifactRecordAnalysisRoot); err != nil {
			return result, err
		}
		name, err := decoder.string()
		if err != nil {
			return result, err
		}
		module, err := decoder.string()
		if err != nil {
			return result, err
		}
		actor, err := decoder.string()
		if err != nil {
			return result, err
		}
		instance, err := decoder.string()
		if err != nil {
			return result, err
		}
		result.AnalysisRoots = append(result.AnalysisRoots, linkmodule.AnalysisRootSpec{Name: name, Module: module, Actor: actor, Instance: instance})
	}
	count, err = decoder.cacheCount()
	if err != nil {
		return result, err
	}
	if !decoder.reserve(count, linkArtifactModuleCacheEntrySpecBytes) {
		return result, ErrArtifactLimit
	}
	result.ModuleCacheEntries = make([]linkmodule.ModuleCacheEntrySpec, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(linkArtifactRecordModuleCacheEntry); err != nil {
			return result, err
		}
		module, err := decoder.string()
		if err != nil {
			return result, err
		}
		occurrence, err := decoder.r.Count()
		if err != nil || occurrence == 0 || occurrence > uint64(^uint32(0)) {
			return result, ErrArtifactCanonical
		}
		from, err := decoder.string()
		if err != nil {
			return result, err
		}
		to, err := decoder.string()
		if err != nil {
			return result, err
		}
		result.ModuleCacheEntries = append(result.ModuleCacheEntries, linkmodule.ModuleCacheEntrySpec{Module: module, Import: keyspace.Term(occurrence), FromRoot: from, ToRoot: to})
	}
	return result, nil
}

func (decoder *linkArtifactDecoder) cacheCount() (int, error) {
	value, err := decoder.r.Count()
	if err != nil || value > linkArtifactMaxModules || value > uint64(decoder.r.Remaining()) {
		return 0, ErrArtifactLimit
	}
	return int(value), nil
}

func (decoder *linkArtifactDecoder) record(want uint64) error {
	got, err := decoder.r.Record()
	if err != nil || got != want {
		return ErrArtifactCanonical
	}
	return nil
}

func (decoder *linkArtifactDecoder) id() (identity.ContentID, error) {
	payload, err := decoder.r.Bytes(len(identity.ContentID{}))
	var id identity.ContentID
	if err != nil || len(payload) != len(id) {
		return id, ErrArtifactCanonical
	}
	copy(id[:], payload)
	return id, nil
}

func (decoder *linkArtifactDecoder) string() (string, error) {
	payload, err := decoder.r.StringBytes(linkArtifactMaxBytes)
	if err != nil {
		return "", err
	}
	if uint64(len(payload)) > decoder.stringBytes {
		return "", ErrArtifactLimit
	}
	if decoder.budget == nil || !decoder.budget.string(uint64(len(payload))) {
		return "", ErrArtifactLimit
	}
	decoder.stringBytes -= uint64(len(payload))
	return string(payload), nil
}

func (decoder *linkArtifactDecoder) reserve(count int, width uint64) bool {
	return decoder != nil && decoder.budget != nil && count >= 0 && decoder.budget.reserve(uint64(count), width)
}

func (decoder *linkArtifactDecoder) moduleCount() (int, error) {
	value, err := decoder.r.Count()
	if err != nil {
		return 0, err
	}
	if value > linkArtifactMaxModules || value > uint64(decoder.r.Remaining())/linkArtifactModuleWireMin {
		return 0, ErrArtifactLimit
	}
	return int(value), nil
}
