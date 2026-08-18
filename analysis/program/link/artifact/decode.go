package artifact

import (
	"bytes"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

const artifactModuleWireMin = 40

func decode(data []byte, contract *target.Contract, programs map[identity.ContentID]*program.Program) (*link.Link, error) {
	measure, err := framing.Scan(data, artifactMaxBytes)
	if err != nil {
		return nil, err
	}
	if !measureAllowed(measure) {
		return nil, ErrLimit
	}
	reader, err := framing.NewReader(data, artifactMaxBytes)
	if err != nil {
		return nil, err
	}
	if err := reader.Header(artifactDomain, artifactCodecVersion); err != nil {
		return nil, err
	}
	decoder := artifactDecoder{r: reader, programs: programs, stringBytes: measure.StringBytes}
	modules, replay, cache, claimed, err := decoder.root(contract)
	if err != nil {
		return nil, err
	}
	if err := reader.Finish(); err != nil {
		return nil, err
	}
	sealed, err := link.SealReplay(&link.Spec{Target: contract, Modules: modules, EndpointRequests: decoder.endpointRequests, Module: cache}, replay)
	if err != nil {
		return nil, fmt.Errorf("link artifact: module replay: %w", err)
	}
	if sealed.ContentID() != claimed {
		return nil, fmt.Errorf("%w: replay ContentID differs", ErrCanonical)
	}
	canonical, err := encodeBounded(sealed, artifactMaxBytes)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(data, canonical) {
		return nil, ErrCanonical
	}
	return sealed, nil
}

type artifactDecoder struct {
	r                *framing.Reader
	programs         map[identity.ContentID]*program.Program
	stringBytes      uint64
	budget           *artifactBudget
	endpointRequests []linkboundary.EndpointRequest
}

func (decoder *artifactDecoder) root(contract *target.Contract) ([]linkproject.Module, linkhost.ReplaySpec, linkmodule.Spec, identity.ContentID, error) {
	if err := decoder.record(artifactRecordRoot); err != nil {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
	}
	storedTarget, err := decoder.id()
	if err != nil {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
	}
	if storedTarget != contract.ContentID() {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrTarget
	}
	claimed, err := decoder.id()
	if err != nil || !claimed.Available() {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrCanonical
	}
	count, err := decoder.moduleCount()
	if err != nil {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
	}
	budget, ok := newBudget(contract)
	if !ok {
		return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrLimit
	}
	decoder.budget = &budget
	// Never let an untrusted arity size a Go allocation. The portable
	// reconstruction budget admits each row before append/map growth.
	var modules []linkproject.Module
	seenNames := make(map[string]struct{})
	var priorID identity.ContentID
	priorName := ""
	for index := 0; index < count; index++ {
		if err := decoder.record(artifactRecordModule); err != nil {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
		}
		id, err := decoder.id()
		if err != nil || !id.Available() {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrCanonical
		}
		name, err := decoder.string()
		if err != nil {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, err
		}
		if name == "" || (index != 0 && compareModule(priorID, priorName, id, name) >= 0) {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrCanonical
		}
		if _, duplicate := seenNames[name]; duplicate {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrCanonical
		}
		sealed := decoder.programs[id]
		if sealed == nil || sealed.ContentID() != id {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, fmt.Errorf("%w: %x", ErrProgram, id)
		}
		if !budget.module(name, sealed) {
			return nil, linkhost.ReplaySpec{}, linkmodule.Spec{}, identity.ContentID{}, ErrLimit
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

func (decoder *artifactDecoder) host() (linkhost.ReplaySpec, error) {
	var result linkhost.ReplaySpec
	count, err := decoder.cacheCount()
	if err != nil {
		return result, err
	}
	if !decoder.reserve(count, artifactHostEndpointBytes) {
		return result, ErrLimit
	}
	decoder.endpointRequests = make([]linkboundary.EndpointRequest, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(artifactRecordHostEndpoint); err != nil {
			return result, err
		}
		identity, err := decoder.string()
		if err != nil {
			return result, err
		}
		namespace, err := decoder.r.Count()
		if err != nil || namespace < uint64(vocabulary.BindingBuiltin) || namespace > uint64(vocabulary.BindingProvider) {
			return result, ErrCanonical
		}
		owner, err := decoder.hostParts()
		if err != nil {
			return result, err
		}
		member, err := decoder.hostParts()
		if err != nil {
			return result, err
		}
		decoder.endpointRequests = append(decoder.endpointRequests, linkboundary.EndpointRequest{Identity: identity, Binding: vocabulary.BindingSpec{Namespace: vocabulary.BindingNamespace(namespace), Owner: owner, Member: member}})
	}
	result.Capabilities, err = decoder.providerCapabilities()
	if err != nil {
		return result, err
	}
	result.Seeds, err = decoder.providerCapabilitySeeds()
	if err != nil {
		return result, err
	}
	result.Exposures, err = decoder.hostSelectors(artifactRecordHostExposure)
	if err != nil {
		return result, err
	}
	result.Members, err = decoder.hostMembers()
	return result, err
}

func (decoder *artifactDecoder) providerCapabilities() ([]string, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, artifactHostSelectorBytes) {
		return nil, ErrLimit
	}
	result := make([]string, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(artifactRecordProviderCapability); err != nil {
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

func (decoder *artifactDecoder) providerCapabilitySeeds() ([]linkhost.ReplayCapabilitySeed, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, artifactHostSelectorBytes) {
		return nil, ErrLimit
	}
	result := make([]linkhost.ReplayCapabilitySeed, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(artifactRecordProviderCapabilitySeed); err != nil {
			return nil, err
		}
		capability, err := decoder.string()
		if err != nil {
			return nil, err
		}
		source, err := decoder.r.Count()
		if err != nil || source > uint64(linkhost.ProviderCapabilitySourceExposure) {
			return nil, ErrCanonical
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
func (decoder *artifactDecoder) hostParts() ([]string, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, artifactHostSelectorBytes) {
		return nil, ErrLimit
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

func (decoder *artifactDecoder) hostSelectors(record uint64) ([]linkhost.ReplaySelector, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, artifactHostSelectorBytes) {
		return nil, ErrLimit
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
			return nil, ErrCanonical
		}
		result = append(result, linkhost.ReplaySelector{Value: value, Endpoint: endpoint, Dispatch: linkhost.HostDispatch(dispatch)})
	}
	return result, nil
}

func (decoder *artifactDecoder) hostMembers() ([]linkhost.ReplayMemberSelector, error) {
	count, err := decoder.cacheCount()
	if err != nil {
		return nil, err
	}
	if !decoder.reserve(count, artifactHostSelectorBytes) {
		return nil, ErrLimit
	}
	result := make([]linkhost.ReplayMemberSelector, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(artifactRecordHostMember); err != nil {
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
			return nil, ErrCanonical
		}
		result = append(result, linkhost.ReplayMemberSelector{Capability: capability, Value: value, Endpoint: endpoint, Dispatch: linkhost.HostDispatch(dispatch)})
	}
	return result, nil
}

func (decoder *artifactDecoder) cache() (linkmodule.Spec, error) {
	var result linkmodule.Spec
	count, err := decoder.cacheCount()
	if err != nil {
		return result, err
	}
	if !decoder.reserve(count, artifactActorSpecBytes) {
		return result, ErrLimit
	}
	result.Actors = make([]linkmodule.ActorSpec, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(artifactRecordActor); err != nil {
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
	if !decoder.reserve(count, artifactAliasClassBytes) {
		return result, ErrLimit
	}
	result.ModuleCacheAliases = make([]linkmodule.ModuleCacheAliasClassSpec, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(artifactRecordAliasClass); err != nil {
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
		if !decoder.reserve(members, artifactAliasMemberBytes) {
			return result, ErrLimit
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
	if !decoder.reserve(count, artifactAnalysisRootSpecBytes) {
		return result, ErrLimit
	}
	result.AnalysisRoots = make([]linkmodule.AnalysisRootSpec, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(artifactRecordAnalysisRoot); err != nil {
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
	if !decoder.reserve(count, artifactModuleCacheEntrySpecBytes) {
		return result, ErrLimit
	}
	result.ModuleCacheEntries = make([]linkmodule.ModuleCacheEntrySpec, 0, count)
	for index := 0; index < count; index++ {
		if err := decoder.record(artifactRecordModuleCacheEntry); err != nil {
			return result, err
		}
		module, err := decoder.string()
		if err != nil {
			return result, err
		}
		occurrence, err := decoder.r.Count()
		if err != nil || occurrence == 0 || occurrence > uint64(^uint32(0)) {
			return result, ErrCanonical
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

func (decoder *artifactDecoder) cacheCount() (int, error) {
	value, err := decoder.r.Count()
	if err != nil || value > artifactMaxModules || value > uint64(decoder.r.Remaining()) {
		return 0, ErrLimit
	}
	return int(value), nil
}

func (decoder *artifactDecoder) record(want uint64) error {
	got, err := decoder.r.Record()
	if err != nil || got != want {
		return ErrCanonical
	}
	return nil
}

func (decoder *artifactDecoder) id() (identity.ContentID, error) {
	payload, err := decoder.r.Bytes(len(identity.ContentID{}))
	var id identity.ContentID
	if err != nil || len(payload) != len(id) {
		return id, ErrCanonical
	}
	copy(id[:], payload)
	return id, nil
}

func (decoder *artifactDecoder) string() (string, error) {
	payload, err := decoder.r.StringBytes(artifactMaxBytes)
	if err != nil {
		return "", err
	}
	if uint64(len(payload)) > decoder.stringBytes {
		return "", ErrLimit
	}
	if decoder.budget == nil || !decoder.budget.string(uint64(len(payload))) {
		return "", ErrLimit
	}
	decoder.stringBytes -= uint64(len(payload))
	return string(payload), nil
}

func (decoder *artifactDecoder) reserve(count int, width uint64) bool {
	return decoder != nil && decoder.budget != nil && count >= 0 && decoder.budget.reserve(uint64(count), width)
}

func (decoder *artifactDecoder) moduleCount() (int, error) {
	value, err := decoder.r.Count()
	if err != nil {
		return 0, err
	}
	if value > artifactMaxModules || value > uint64(decoder.r.Remaining())/artifactModuleWireMin {
		return 0, ErrLimit
	}
	return int(value), nil
}
