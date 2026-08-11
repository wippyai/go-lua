package typestate

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

type resourceSourceKind uint8

const (
	resourceSourceInvalid resourceSourceKind = iota
	resourceSourceAcquisition
	resourceSourceInput
	resourceSourceUnknown
)

// ResourceSource is Typestate's opaque structural source coordinate. This
// package owns source eligibility, enumeration, validation, identity, and
// rebinding.
type ResourceSource struct {
	source      *proglink.Link
	kind        resourceSourceKind
	application linkproject.Application
	protocol    target.Protocol
	row         uint32
	input       target.InputSource
	id          keyspace.ContentID
}

func (s ResourceSource) validFor(source *proglink.Link) bool {
	if source == nil || s.source != source || !source.ContentID().Available() || s.protocol == 0 || !s.id.Available() {
		return false
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil || contract.StateCount(s.protocol) == 0 {
		return false
	}
	switch s.kind {
	case resourceSourceAcquisition:
		if _, _, _, _, ok := contract.ProtocolAcquisitionAt(s.protocol, int(s.row)); !ok || !resourceAcquisitionEligible(source, s.application, s.protocol, s.row) {
			return false
		}
	case resourceSourceInput:
		if s.input.Kind == target.InputSourceInvalid || !resourceInputEligible(source, s.application, s.protocol, s.input) {
			return false
		}
	case resourceSourceUnknown:
		if s.application != (linkproject.Application{}) || s.row != 0 || s.input != (target.InputSource{}) {
			return false
		}
	default:
		return false
	}
	return s.id == resourceSourceID(source, s.kind, s.application, s.protocol, s.row, s.input)
}

func (s ResourceSource) contentID() keyspace.ContentID {
	if !s.validFor(s.source) {
		return keyspace.ContentID{}
	}
	return s.id
}

// ResourceOrigin is one Typestate-owned structural source plus its nominal
// recurrence role. Callers cannot construct a protocol/input product: every
// source was enumerated under Typestate's direct structural eligibility
// predicate.
type ResourceOrigin struct {
	source *proglink.Link
	raw    ResourceSource
	role   materialization.Role
	id     keyspace.ContentID
	span   uint32
}

func enumerateResourceSources(source *proglink.Link) []ResourceSource {
	if source == nil || !source.ContentID().Available() {
		return nil
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil {
		return nil
	}
	result := make([]ResourceSource, 0)
	for applicationIndex := 0; applicationIndex < source.Project().Applications().Calls().Count(); applicationIndex++ {
		application, ok := source.Project().Applications().Calls().At(applicationIndex)
		if !ok {
			return nil
		}
		for protocolIndex := 0; protocolIndex < contract.ProtocolCount(); protocolIndex++ {
			protocol, ok := contract.ProtocolAt(protocolIndex)
			if !ok {
				return nil
			}
			for row := 0; row < contract.ProtocolAcquisitionCount(protocol); row++ {
				if !resourceAcquisitionEligible(source, application, protocol, uint32(row)) {
					continue
				}
				candidate := ResourceSource{source: source, kind: resourceSourceAcquisition, application: application, protocol: protocol, row: uint32(row)}
				candidate.id = resourceSourceID(source, candidate.kind, application, protocol, candidate.row, target.InputSource{})
				if !candidate.validFor(source) {
					return nil
				}
				result = append(result, candidate)
			}
		}
		for protocolIndex := 0; protocolIndex < contract.ProtocolCount(); protocolIndex++ {
			protocol, ok := contract.ProtocolAt(protocolIndex)
			if !ok {
				return nil
			}
			inputs := make(map[target.InputSource]struct{})
			for row := 0; row < contract.TransitionCount(protocol); row++ {
				_, kind, ordinal, _, ok := contract.TransitionAt(protocol, row)
				if !ok {
					return nil
				}
				inputs[target.InputSource{Kind: kind, Ordinal: ordinal}] = struct{}{}
			}
			for row := 0; row < contract.EscapeCount(protocol); row++ {
				_, kind, ordinal, ok := contract.EscapeAt(protocol, row)
				if !ok {
					return nil
				}
				inputs[target.InputSource{Kind: kind, Ordinal: ordinal}] = struct{}{}
			}
			for row := 0; row < contract.ProtocolCallbackHolderCount(protocol); row++ {
				_, input, _, ok := contract.ProtocolCallbackHolderAt(protocol, row)
				if !ok {
					return nil
				}
				inputs[input] = struct{}{}
			}
			ordered := make([]target.InputSource, 0, len(inputs))
			for input := range inputs {
				ordered = append(ordered, input)
			}
			sort.Slice(ordered, func(i, j int) bool {
				if ordered[i].Kind != ordered[j].Kind {
					return ordered[i].Kind < ordered[j].Kind
				}
				return ordered[i].Ordinal < ordered[j].Ordinal
			})
			for _, input := range ordered {
				if !resourceInputEligible(source, application, protocol, input) {
					continue
				}
				candidate := ResourceSource{source: source, kind: resourceSourceInput, application: application, protocol: protocol, input: input}
				candidate.id = resourceSourceID(source, candidate.kind, application, protocol, 0, input)
				if !candidate.validFor(source) {
					return nil
				}
				result = append(result, candidate)
			}
		}
	}
	for protocolIndex := 0; protocolIndex < contract.ProtocolCount(); protocolIndex++ {
		protocol, ok := contract.ProtocolAt(protocolIndex)
		if !ok {
			return nil
		}
		candidate := ResourceSource{source: source, kind: resourceSourceUnknown, protocol: protocol}
		candidate.id = resourceSourceID(source, candidate.kind, linkproject.Application{}, protocol, 0, target.InputSource{})
		if !candidate.validFor(source) {
			return nil
		}
		result = append(result, candidate)
	}
	return result
}

// resourceAcquisitionEligible reports whether an existing Call application
// names the operation that owns this exact Target acquisition declaration.
func resourceAcquisitionEligible(source *proglink.Link, application linkproject.Application, protocol target.Protocol, row uint32) bool {
	if source == nil || uint64(row) > uint64(^uint(0)>>1) || !resourceCall(source, application) {
		return false
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil {
		return false
	}
	operation, _, _, _, ok := contract.ProtocolAcquisitionAt(protocol, int(row))
	return ok && source.Boundary().ApplicationOperationAvailable(contract, application, operation)
}

// resourceInputEligible reports whether an existing Call application has a
// matching operation for this exact protocol/input source.
func resourceInputEligible(source *proglink.Link, application linkproject.Application, protocol target.Protocol, input target.InputSource) bool {
	if source == nil || input.Kind == target.InputSourceInvalid || !resourceCall(source, application) {
		return false
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil {
		return false
	}
	for row := 0; row < contract.TransitionCount(protocol); row++ {
		operation, kind, ordinal, _, ok := contract.TransitionAt(protocol, row)
		if !ok {
			return false
		}
		if input.Kind == kind && input.Ordinal == ordinal && source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
			return true
		}
	}
	for row := 0; row < contract.EscapeCount(protocol); row++ {
		operation, kind, ordinal, ok := contract.EscapeAt(protocol, row)
		if !ok {
			return false
		}
		if input.Kind == kind && input.Ordinal == ordinal && source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
			return true
		}
	}
	for row := 0; row < contract.ProtocolCallbackHolderCount(protocol); row++ {
		operation, candidate, _, ok := contract.ProtocolCallbackHolderAt(protocol, row)
		if !ok {
			return false
		}
		if input == candidate && source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
			return true
		}
	}
	return false
}

func resourceCall(source *proglink.Link, application linkproject.Application) bool {
	_, _, ok := source.Project().Applications().Call(application)
	return ok
}

func materializeResource(raw ResourceSource, role materialization.Role) (ResourceOrigin, bool) {
	if !raw.validFor(raw.source) || !role.Valid() {
		return ResourceOrigin{}, false
	}
	contract, contractOK := raw.source.Boundary().Target()
	if !contractOK || contract == nil {
		return ResourceOrigin{}, false
	}
	states := contract.StateCount(raw.protocol)
	if states <= 0 || uint64(states)*uint64(DutyUnknown) > uint64(^uint32(0)) {
		return ResourceOrigin{}, false
	}
	if raw.kind == resourceSourceUnknown {
		if role != materialization.Summary {
			return ResourceOrigin{}, false
		}
	}
	origin := ResourceOrigin{source: raw.source, raw: raw, role: role, span: uint32(states * int(DutyUnknown))}
	origin.id = resourceOriginID(raw.source.ContentID(), raw.id, role)
	return origin, origin.validFor(raw.source)
}

func (o ResourceOrigin) ContentID() keyspace.ContentID {
	if !o.valid() {
		return keyspace.ContentID{}
	}
	return o.id
}

func (o ResourceOrigin) valid() bool {
	if o.source == nil || !o.role.Valid() || !o.id.Available() || o.span == 0 || !o.source.ContentID().Available() {
		return false
	}
	if !o.raw.validFor(o.source) || (o.raw.kind == resourceSourceUnknown && o.role != materialization.Summary) {
		return false
	}
	return o.id == resourceOriginID(o.source.ContentID(), o.raw.id, o.role)
}

func (o ResourceOrigin) validFor(source *proglink.Link) bool {
	return o.valid() && source == o.source
}

// defaultHolder is the D0 holder coordinate implied by a sealed source.  A
// call-backed acquisition/formal begins local to that call; an explicit
// unknown source is opaque.  Later transport rules rewrite this exact holder
// through Substitution rather than admitting a second origin source.
func (o ResourceOrigin) defaultHolder() (HolderOrigin, bool) {
	if !o.valid() {
		return HolderOrigin{}, false
	}
	if o.raw.kind == resourceSourceUnknown {
		return OpaqueHolder(o.source)
	}
	return LocalHolder(o.source, o.raw.application)
}

func resourceSourceID(source *proglink.Link, kind resourceSourceKind, application linkproject.Application, protocol target.Protocol, row uint32, input target.InputSource) keyspace.ContentID {
	if source == nil || !source.ContentID().Available() || protocol == 0 {
		return keyspace.ContentID{}
	}
	var applicationID keyspace.ContentID
	if kind != resourceSourceUnknown {
		var ok bool
		project := source.Project()
		if project == nil {
			return keyspace.ContentID{}
		}
		applicationID, ok = project.ApplicationID(application)
		if !ok {
			return keyspace.ContentID{}
		}
	}
	linkID := source.ContentID()
	protocolID := protocolCoordinateID(linkID, protocol)
	var payload [32 + 32 + 32 + 8*8]byte
	copy(payload[:32], linkID[:])
	copy(payload[32:64], applicationID[:])
	copy(payload[64:96], protocolID[:])
	words := payload[96:]
	binary.BigEndian.PutUint64(words[0:8], 0x6c696e6b2d726f72) // "link-ror"
	binary.BigEndian.PutUint64(words[8:16], 2)
	// Preserve the historical raw source preimage. The identity is now
	// computed and validated exclusively by Typestate.
	sourceKind, templateKind := uint64(3), uint64(0)
	switch kind {
	case resourceSourceAcquisition:
		sourceKind, templateKind = 1, 1
	case resourceSourceInput:
		sourceKind, templateKind = 1, 2
	case resourceSourceUnknown:
		sourceKind = 2
	default:
		return keyspace.ContentID{}
	}
	binary.BigEndian.PutUint64(words[16:24], sourceKind)
	binary.BigEndian.PutUint64(words[24:32], templateKind)
	binary.BigEndian.PutUint64(words[32:40], uint64(protocol))
	binary.BigEndian.PutUint64(words[40:48], uint64(row))
	binary.BigEndian.PutUint64(words[48:56], uint64(input.Kind))
	binary.BigEndian.PutUint64(words[56:64], uint64(input.Ordinal))
	return sha256.Sum256(payload[:])
}

type holderKind uint8

const (
	holderInvalid holderKind = iota
	holderLocal
	holderCallback
	holderSuspension
	holderExternal
	holderOpaque
)

// HolderOrigin is one exact finite holder role. Each non-opaque alternative
// is backed directly by an existing typed Link relation.
type HolderOrigin struct {
	source      *proglink.Link
	kind        holderKind
	application keyspace.ContentID
	operation   target.Operation
	port        uint32
	id          keyspace.ContentID
}

func LocalHolder(source *proglink.Link, application linkproject.Application) (HolderOrigin, bool) {
	if source == nil {
		return HolderOrigin{}, false
	}
	project := source.Project()
	if project == nil {
		return HolderOrigin{}, false
	}
	applicationID, ok := project.ApplicationID(application)
	if !ok {
		return HolderOrigin{}, false
	}
	holder := HolderOrigin{source: source, kind: holderLocal, application: applicationID}
	holder.id = holderOriginID(source.ContentID(), holder.kind, applicationID, 0)
	return holder, holder.validFor(source)
}

func CallbackHolder(source *proglink.Link, application linkproject.Application, operation target.Operation, callback target.CallbackID) (HolderOrigin, bool) {
	return applicationHolder(source, holderCallback, application, operation, uint32(callback))
}

func SuspensionHolder(source *proglink.Link, application linkproject.Application, operation target.Operation, suspension uint32) (HolderOrigin, bool) {
	return applicationHolder(source, holderSuspension, application, operation, suspension)
}

func ExternalHolder(source *proglink.Link, application linkproject.Application, operation target.Operation, transfer target.TransferID) (HolderOrigin, bool) {
	return applicationHolder(source, holderExternal, application, operation, uint32(transfer))
}

func applicationHolder(source *proglink.Link, kind holderKind, application linkproject.Application, operation target.Operation, port uint32) (HolderOrigin, bool) {
	if source == nil {
		return HolderOrigin{}, false
	}
	contract, contractOK := source.Boundary().Target()
	if !contractOK || contract == nil || !source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
		return HolderOrigin{}, false
	}
	project := source.Project()
	if project == nil {
		return HolderOrigin{}, false
	}
	applicationID, ok := project.ApplicationID(application)
	if !ok {
		return HolderOrigin{}, false
	}
	holder := HolderOrigin{source: source, kind: kind, application: applicationID, operation: operation, port: port}
	holder.id = holderOriginID(source.ContentID(), holder.kind, applicationID, uint64(operation)<<32|uint64(port))
	return holder, holder.validFor(source)
}

func OpaqueHolder(source *proglink.Link) (HolderOrigin, bool) {
	if source == nil || !source.ContentID().Available() {
		return HolderOrigin{}, false
	}
	holder := HolderOrigin{source: source, kind: holderOpaque}
	holder.id = holderOriginID(source.ContentID(), holder.kind, keyspace.ContentID{}, 0)
	return holder, holder.validFor(source)
}

func (o HolderOrigin) ContentID() keyspace.ContentID {
	if !o.valid() {
		return keyspace.ContentID{}
	}
	return o.id
}

func (o HolderOrigin) valid() bool {
	return o.source != nil && o.kind >= holderLocal && o.kind <= holderOpaque &&
		o.source.ContentID().Available() && o.id.Available()
}

func (o HolderOrigin) validFor(source *proglink.Link) bool {
	return o.valid() && source == o.source
}

func resourceOriginID(linkID, rawID keyspace.ContentID, role materialization.Role) keyspace.ContentID {
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.typestate.resource"))
	_, _ = hash.Write(linkID[:])
	_, _ = hash.Write(rawID[:])
	writeOriginWord(hash, uint64(role))
	return sumOriginID(hash)
}

const holderOriginIdentityDomain = "wippy.analysis.typestate.holder"

func holderOriginID(linkID keyspace.ContentID, kind holderKind, sourceID keyspace.ContentID, ordinal uint64) keyspace.ContentID {
	var payload [len(holderOriginIdentityDomain) + 32 + 32 + 8 + 8]byte
	offset := copy(payload[:], holderOriginIdentityDomain)
	offset += copy(payload[offset:], linkID[:])
	offset += copy(payload[offset:], sourceID[:])
	binary.BigEndian.PutUint64(payload[offset:offset+8], uint64(kind))
	binary.BigEndian.PutUint64(payload[offset+8:], ordinal)
	return keyspace.ContentID(sha256.Sum256(payload[:]))
}

func writeOriginWord(dst interface{ Write([]byte) (int, error) }, value uint64) {
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], value)
	_, _ = dst.Write(word[:])
}

func sumOriginID(hash interface{ Sum([]byte) []byte }) keyspace.ContentID {
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
