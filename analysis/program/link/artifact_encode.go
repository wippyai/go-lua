package link

import (
	"io"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
)

const (
	linkArtifactRecordRoot uint64 = iota + 1
	linkArtifactRecordModule
	linkArtifactRecordActor
	linkArtifactRecordAliasClass
	linkArtifactRecordAnalysisRoot
	linkArtifactRecordModuleCacheEntry
	linkArtifactRecordHostEndpoint
	linkArtifactRecordProviderCapability
	linkArtifactRecordProviderCapabilitySeed
	linkArtifactRecordHostExposure
	linkArtifactRecordHostMember
)

func encodeLinkArtifactBounded(link *Link, limit int) ([]byte, error) {
	dst := &linkArtifactBuffer{limit: limit}
	if err := encodeLinkArtifact(dst, link); err != nil {
		return nil, err
	}
	return dst.data.Bytes(), nil
}

func encodeLinkArtifact(dst io.Writer, link *Link) error {
	var writer framing.Writer
	if err := writer.Reset(dst, linkArtifactDomain, linkArtifactCodecVersion); err != nil {
		return err
	}
	encoder := linkArtifactEncoder{
		link: link,
		w:    &writer,
		measure: framing.StreamMeasure{
			Events: 2, // domain and version
		},
	}
	encoder.root()
	if encoder.err != nil {
		return encoder.err
	}
	return writer.Finish()
}

type linkArtifactEncoder struct {
	link    *Link
	w       *framing.Writer
	err     error
	measure framing.StreamMeasure
}

func (encoder *linkArtifactEncoder) frame(stringBytes int) bool {
	if encoder == nil || encoder.err != nil || stringBytes < 0 ||
		encoder.measure.Events == ^uint64(0) || uint64(stringBytes) > ^uint64(0)-encoder.measure.StringBytes {
		if encoder != nil && encoder.err == nil {
			encoder.err = ErrArtifactLimit
		}
		return false
	}
	next := encoder.measure
	next.Events++
	next.StringBytes += uint64(stringBytes)
	if !artifactMeasureAllowed(next) {
		encoder.err = ErrArtifactLimit
		return false
	}
	encoder.measure = next
	return true
}

func (encoder *linkArtifactEncoder) call(err error) {
	if encoder.err == nil && err != nil {
		encoder.err = err
	}
}

func (encoder *linkArtifactEncoder) record(value uint64) {
	if encoder.frame(0) {
		encoder.call(encoder.w.Record(value))
	}
}

func (encoder *linkArtifactEncoder) count(value uint64) {
	if encoder.frame(0) {
		encoder.call(encoder.w.Count(value))
	}
}

func (encoder *linkArtifactEncoder) id(value identity.ContentID) {
	if encoder.frame(0) {
		encoder.call(encoder.w.Bytes(value[:]))
	}
}

func (encoder *linkArtifactEncoder) string(value string) {
	if encoder.frame(len(value)) {
		encoder.call(encoder.w.String(value))
	}
}

func (encoder *linkArtifactEncoder) root() {
	encoder.record(linkArtifactRecordRoot)
	contract, ok := encoder.link.boundary.Target()
	if !ok || contract == nil {
		encoder.err = ErrArtifactUnavailable
		return
	}
	encoder.id(contract.ContentID())
	encoder.id(encoder.link.id)
	mounts := encoder.link.project.Mounts()
	encoder.count(uint64(mounts.Count()))
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		if !ok {
			encoder.err = ErrArtifactUnavailable
			return
		}
		program, programOK := mounts.Program(shard)
		name, nameOK := mounts.Name(shard)
		if !programOK || !nameOK || program == nil {
			encoder.err = ErrArtifactUnavailable
			return
		}
		encoder.record(linkArtifactRecordModule)
		encoder.id(program.ContentID())
		encoder.string(name)
	}
	requests := encoder.link.boundary.EndpointRequests()
	encoder.count(uint64(requests.Count()))
	for index := 0; index < requests.Count(); index++ {
		endpoint, ok := requests.At(index)
		if !ok {
			encoder.err = ErrArtifactUnavailable
			return
		}
		encoder.record(linkArtifactRecordHostEndpoint)
		encoder.string(endpoint.Identity)
		encoder.count(uint64(endpoint.Binding.Namespace))
		encoder.count(uint64(len(endpoint.Binding.Owner)))
		for _, item := range endpoint.Binding.Owner {
			encoder.string(item)
		}
		encoder.count(uint64(len(endpoint.Binding.Member)))
		for _, item := range endpoint.Binding.Member {
			encoder.string(item)
		}
	}
	host, ok := encoder.link.host.Cold().ReplaySpec()
	if !ok {
		encoder.err = ErrArtifactUnavailable
		return
	}
	encoder.count(uint64(len(host.Capabilities)))
	for _, capability := range host.Capabilities {
		encoder.record(linkArtifactRecordProviderCapability)
		encoder.string(capability)
	}
	encoder.count(uint64(len(host.Seeds)))
	for _, seed := range host.Seeds {
		encoder.record(linkArtifactRecordProviderCapabilitySeed)
		encoder.string(seed.Capability)
		encoder.count(uint64(seed.Source))
		encoder.string(seed.InitialRoot)
		encoder.id(seed.InputFormal)
		encoder.id(seed.OutcomeResult)
		encoder.id(seed.Value)
	}
	encoder.count(uint64(len(host.Exposures)))
	for _, exposure := range host.Exposures {
		encoder.record(linkArtifactRecordHostExposure)
		encoder.id(exposure.Value)
		encoder.id(exposure.Endpoint)
		encoder.count(uint64(exposure.Dispatch))
	}
	encoder.count(uint64(len(host.Members)))
	for _, member := range host.Members {
		encoder.record(linkArtifactRecordHostMember)
		encoder.string(member.Capability)
		encoder.id(member.Value)
		encoder.id(member.Endpoint)
		encoder.count(uint64(member.Dispatch))
	}
	cache, ok := encoder.link.module.Cold().Spec()
	if !ok {
		encoder.err = ErrArtifactUnavailable
		return
	}
	encoder.count(uint64(len(cache.Actors)))
	for _, actor := range cache.Actors {
		encoder.record(linkArtifactRecordActor)
		encoder.string(actor.Name)
	}
	encoder.count(uint64(len(cache.ModuleCacheAliases)))
	for _, alias := range cache.ModuleCacheAliases {
		encoder.record(linkArtifactRecordAliasClass)
		encoder.string(alias.Actor)
		encoder.string(alias.Representative)
		encoder.count(uint64(len(alias.Instances)))
		for _, instance := range alias.Instances {
			encoder.string(instance)
		}
	}
	encoder.count(uint64(len(cache.AnalysisRoots)))
	for _, root := range cache.AnalysisRoots {
		encoder.record(linkArtifactRecordAnalysisRoot)
		encoder.string(root.Name)
		encoder.string(root.Module)
		encoder.string(root.Actor)
		encoder.string(root.Instance)
	}
	encoder.count(uint64(len(cache.ModuleCacheEntries)))
	for _, entry := range cache.ModuleCacheEntries {
		encoder.record(linkArtifactRecordModuleCacheEntry)
		encoder.string(entry.Module)
		encoder.count(uint64(entry.Import))
		encoder.string(entry.FromRoot)
		encoder.string(entry.ToRoot)
	}
}
