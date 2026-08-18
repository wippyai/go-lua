package artifact

import (
	"io"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	artifactRecordRoot uint64 = iota + 1
	artifactRecordModule
	artifactRecordActor
	artifactRecordAliasClass
	artifactRecordAnalysisRoot
	artifactRecordModuleCacheEntry
	artifactRecordHostEndpoint
	artifactRecordProviderCapability
	artifactRecordProviderCapabilitySeed
	artifactRecordHostExposure
	artifactRecordHostMember
)

func encodeBounded(sealed *link.Link, limit int) ([]byte, error) {
	dst := &artifactBuffer{limit: limit}
	if err := encodeTo(dst, sealed); err != nil {
		return nil, err
	}
	return dst.data.Bytes(), nil
}

func encodeTo(dst io.Writer, sealed *link.Link) error {
	var writer framing.Writer
	if err := writer.Reset(dst, artifactDomain, artifactCodecVersion); err != nil {
		return err
	}
	encoder := artifactEncoder{
		sealed: sealed,
		w:      &writer,
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

type artifactEncoder struct {
	sealed  *link.Link
	w       *framing.Writer
	err     error
	measure framing.StreamMeasure
}

func (encoder *artifactEncoder) frame(stringBytes int) bool {
	if encoder == nil || encoder.err != nil || stringBytes < 0 ||
		encoder.measure.Events == ^uint64(0) || uint64(stringBytes) > ^uint64(0)-encoder.measure.StringBytes {
		if encoder != nil && encoder.err == nil {
			encoder.err = ErrLimit
		}
		return false
	}
	next := encoder.measure
	next.Events++
	next.StringBytes += uint64(stringBytes)
	if !measureAllowed(next) {
		encoder.err = ErrLimit
		return false
	}
	encoder.measure = next
	return true
}

func (encoder *artifactEncoder) call(err error) {
	if encoder.err == nil && err != nil {
		encoder.err = err
	}
}

func (encoder *artifactEncoder) record(value uint64) {
	if encoder.frame(0) {
		encoder.call(encoder.w.Record(value))
	}
}

func (encoder *artifactEncoder) count(value uint64) {
	if encoder.frame(0) {
		encoder.call(encoder.w.Count(value))
	}
}

func (encoder *artifactEncoder) id(value identity.ContentID) {
	if encoder.frame(0) {
		encoder.call(encoder.w.Bytes(value[:]))
	}
}

func (encoder *artifactEncoder) string(value string) {
	if encoder.frame(len(value)) {
		encoder.call(encoder.w.String(value))
	}
}

func (encoder *artifactEncoder) root() {
	encoder.record(artifactRecordRoot)
	contract, ok := encoder.sealed.Boundary().Target()
	if !ok || contract == nil {
		encoder.err = ErrUnavailable
		return
	}
	encoder.id(contract.ContentID())
	encoder.id(encoder.sealed.ContentID())
	mounts := encoder.sealed.Project().Mounts()
	encoder.count(uint64(mounts.Count()))
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		if !ok {
			encoder.err = ErrUnavailable
			return
		}
		program, programOK := mounts.Program(shard)
		name, nameOK := mounts.Name(shard)
		if !programOK || !nameOK || program == nil {
			encoder.err = ErrUnavailable
			return
		}
		encoder.record(artifactRecordModule)
		encoder.id(program.ContentID())
		encoder.string(name)
	}
	requests := encoder.sealed.Boundary().EndpointRequests()
	encoder.count(uint64(requests.Count()))
	for index := 0; index < requests.Count(); index++ {
		endpoint, ok := requests.At(index)
		if !ok {
			encoder.err = ErrUnavailable
			return
		}
		encoder.record(artifactRecordHostEndpoint)
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
	host, ok := encoder.sealed.Host().Cold().ReplaySpec()
	if !ok {
		encoder.err = ErrUnavailable
		return
	}
	encoder.count(uint64(len(host.Capabilities)))
	for _, capability := range host.Capabilities {
		encoder.record(artifactRecordProviderCapability)
		encoder.string(capability)
	}
	encoder.count(uint64(len(host.Seeds)))
	for _, seed := range host.Seeds {
		encoder.record(artifactRecordProviderCapabilitySeed)
		encoder.string(seed.Capability)
		encoder.count(uint64(seed.Source))
		encoder.string(seed.InitialRoot)
		encoder.id(seed.InputFormal)
		encoder.id(seed.OutcomeResult)
		encoder.id(seed.Value)
	}
	encoder.count(uint64(len(host.Exposures)))
	for _, exposure := range host.Exposures {
		encoder.record(artifactRecordHostExposure)
		encoder.id(exposure.Value)
		encoder.id(exposure.Endpoint)
		encoder.count(uint64(exposure.Dispatch))
	}
	encoder.count(uint64(len(host.Members)))
	for _, member := range host.Members {
		encoder.record(artifactRecordHostMember)
		encoder.string(member.Capability)
		encoder.id(member.Value)
		encoder.id(member.Endpoint)
		encoder.count(uint64(member.Dispatch))
	}
	cache, ok := encoder.sealed.Module().Cold().Spec()
	if !ok {
		encoder.err = ErrUnavailable
		return
	}
	encoder.count(uint64(len(cache.Actors)))
	for _, actor := range cache.Actors {
		encoder.record(artifactRecordActor)
		encoder.string(actor.Name)
	}
	encoder.count(uint64(len(cache.ModuleCacheAliases)))
	for _, alias := range cache.ModuleCacheAliases {
		encoder.record(artifactRecordAliasClass)
		encoder.string(alias.Actor)
		encoder.string(alias.Representative)
		encoder.count(uint64(len(alias.Instances)))
		for _, instance := range alias.Instances {
			encoder.string(instance)
		}
	}
	encoder.count(uint64(len(cache.AnalysisRoots)))
	for _, root := range cache.AnalysisRoots {
		encoder.record(artifactRecordAnalysisRoot)
		encoder.string(root.Name)
		encoder.string(root.Module)
		encoder.string(root.Actor)
		encoder.string(root.Instance)
	}
	encoder.count(uint64(len(cache.ModuleCacheEntries)))
	for _, entry := range cache.ModuleCacheEntries {
		encoder.record(artifactRecordModuleCacheEntry)
		encoder.string(entry.Module)
		encoder.count(uint64(entry.Import))
		encoder.string(entry.FromRoot)
		encoder.string(entry.ToRoot)
	}
}
