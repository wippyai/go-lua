package factor

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	packtransfer "github.com/wippyai/go-lua/domain/pack/transfer"
)

const mountedPublicationDomain = "wippy.analysis.effect.mounted-publication.v1\x00"

// MountedPublicationRole preserves the owner of a publication consequence.
// An ordinary selected effect and a callback selected effect may have equal
// semantic atoms, but they are different typed publication capabilities and
// must not be collapsed by a consumer.
type MountedPublicationRole uint8

const (
	MountedPublicationInvalid MountedPublicationRole = iota
	MountedPublicationOrdinary
	MountedPublicationCallback
)

// MountedPublication is Effect's sealed receipt for one explicitly authored
// Target publication effect selected on one exact mounted call.  It retains
// only already-authenticated scalar/typed consequences:
//
//   - the ordinary/callback provenance and operation-local effect coordinate;
//   - Target's typed descriptor and its descriptor/occurrence IDs;
//   - Pack's exact mounted subject row and optional destination-context row;
//   - the existing beta-issued AtomBinding; and
//   - the exact mounted-call identity.
//
// It is not a solver result, allocation proof, runtime placement decision, or
// permission to infer one.  Placement consumes this receipt later.
type MountedPublication struct {
	owner       *Algebra
	mounted     MountedCall
	application identity.ContentID
	module      identity.ContentID
	occurrence  identity.ContentID
	binding     AtomBinding
	role        MountedPublicationRole
	operation   vocabulary.Operation
	callback    vocabulary.CallbackID
	effect      uint32
	descriptor  operationvalue.PublicationEffectDescriptor
	descriptorID,
	occurrenceID identity.ContentID
	subject      packtransfer.MountedInput
	context      packtransfer.MountedInput
	hasContext   bool
	id           identity.ContentID
	sealed       bool
	sealedScalar uint64
}

// mountedPublicationID is a scalar seal over the receipt's retained
// provenance, typed coordinates, and mounted Pack-input identities.  The
// mounted-input rows are already owner-fenced capabilities; their semantic
// source is reconstructed by Valid from the descriptor rather than exposing
// Pack's private selector state.
func mountedPublicationID(owner, application, module, occurrence, binding, descriptorID, occurrenceID, subjectID, contextID identity.ContentID, role MountedPublicationRole, operation vocabulary.Operation, callback vocabulary.CallbackID, effect uint32, descriptor operationvalue.PublicationEffectDescriptor, hasContext bool) identity.ContentID {
	if !owner.Available() || !application.Available() || !module.Available() || !occurrence.Available() || !binding.Available() || !descriptorID.Available() || !occurrenceID.Available() || role < MountedPublicationOrdinary || role > MountedPublicationCallback || operation == 0 {
		return identity.ContentID{}
	}
	if !subjectID.Available() || hasContext && !contextID.Available() || !hasContext && contextID.Available() || !descriptor.Valid() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(mountedPublicationDomain))
	for _, value := range [...]identity.ContentID{owner, application, module, occurrence, binding, descriptorID, occurrenceID, subjectID, contextID} {
		_, _ = hash.Write(value[:])
	}
	var scalars [12]byte
	binary.BigEndian.PutUint32(scalars[0:4], uint32(role))
	binary.BigEndian.PutUint32(scalars[4:8], uint32(operation))
	binary.BigEndian.PutUint32(scalars[8:12], uint32(callback))
	_, _ = hash.Write(scalars[:])
	var index [4]byte
	binary.BigEndian.PutUint32(index[:], effect)
	_, _ = hash.Write(index[:])
	if hasContext {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
	var consequences [5]byte
	consequences[0] = byte(descriptor.Kind())
	consequences[1] = byte(descriptor.DestinationRole())
	consequences[2] = byte(descriptor.Escape())
	consequences[3] = byte(descriptor.Mutability())
	consequences[4] = byte(descriptor.Lifetime())
	_, _ = hash.Write(consequences[:])
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func mountedPublicationScalar(id identity.ContentID) uint64 {
	if !id.Available() {
		return 0
	}
	return binary.BigEndian.Uint64(id[:8]) | 1
}

func (publication MountedPublication) available() bool {
	return publication.sealed && publication.sealedScalar != 0 && publication.owner != nil && publication.owner.Valid() && publication.mounted.Valid() && publication.mounted.owner == publication.owner && publication.application.Available() && publication.module.Available() && publication.occurrence.Available() && publication.binding.valid() && publication.binding.owner == publication.owner && publication.descriptor.Valid() && publication.descriptorID.Available() && publication.occurrenceID.Available() && publication.id.Available() && mountedPublicationScalar(publication.id) == publication.sealedScalar
}

func (publication MountedPublication) valid() bool {
	if !publication.available() {
		return false
	}
	application, module, occurrence, mountedOK := publication.owner.MountedCallIdentity(publication.mounted)
	if !mountedOK || application != publication.application || module != publication.module || occurrence != publication.occurrence {
		return false
	}
	root, rootOK := publication.owner.RootForMountedCall(publication.mounted)
	boundRoot, boundRootOK := publication.binding.Root()
	if !rootOK || !boundRootOK || root != boundRoot {
		return false
	}
	ownerID := publication.owner.LinkID()
	subjectID, subjectIDOK := publication.subject.ContentID()
	contextID := identity.ContentID{}
	if publication.hasContext {
		contextID, subjectIDOK = publication.context.ContentID()
	}
	if !ownerID.Available() || !subjectIDOK || publication.id != mountedPublicationID(ownerID, publication.application, publication.module, publication.occurrence, publication.binding.formal.id, publication.descriptorID, publication.occurrenceID, subjectID, contextID, publication.role, publication.operation, publication.callback, publication.effect, publication.descriptor, publication.hasContext) {
		return false
	}

	var expected operationvalue.PublicationEffectDescriptor
	var expectedDescriptorID, expectedOccurrenceID identity.ContentID
	switch publication.role {
	case MountedPublicationOrdinary:
		if publication.callback != 0 || publication.binding.formal.role != formalAtomOrdinary || publication.operation == 0 || uint64(publication.effect) >= uint64(publication.owner.contract.Operations.EffectCount(publication.operation)) {
			return false
		}
		effect := int(publication.effect)
		var ok bool
		expected, ok = publication.owner.contract.Operations.EffectPublication(publication.operation, effect)
		if !ok {
			return false
		}
		expectedDescriptorID, ok = publication.owner.contract.Operations.PublicationEffectDescriptorID(publication.operation, effect)
		if !ok {
			return false
		}
		expectedOccurrenceID, ok = publication.owner.contract.Operations.PublicationEffectOccurrenceID(publication.operation, effect)
		if !ok || publication.binding.formal.descriptor != expectedDescriptorID {
			return false
		}
	case MountedPublicationCallback:
		if publication.callback == 0 || publication.binding.formal.role != formalAtomCallback || publication.operation == 0 || uint64(publication.effect) >= uint64(publication.owner.contract.Operations.CallbackEffectCount(publication.callback)) {
			return false
		}
		callbackOwner, ownerOK := publication.owner.contract.Operations.CallbackOwner(publication.callback)
		if !ownerOK || callbackOwner != publication.operation {
			return false
		}
		effect := int(publication.effect)
		var ok bool
		expected, ok = publication.owner.contract.Operations.CallbackEffectPublication(publication.callback, effect)
		if !ok {
			return false
		}
		expectedDescriptorID, ok = publication.owner.contract.Operations.CallbackPublicationEffectDescriptorID(publication.callback, effect)
		if !ok {
			return false
		}
		expectedOccurrenceID, ok = publication.owner.contract.Operations.CallbackPublicationEffectOccurrenceID(publication.callback, effect)
		if !ok || publication.binding.formal.descriptor != expectedDescriptorID {
			return false
		}
	default:
		return false
	}
	applicationOperation, selected := publication.owner.applicationOperation(application, publication.operation)
	if !selected || applicationOperation < 0 {
		return false
	}
	if publication.descriptor != expected || publication.descriptorID != expectedDescriptorID || publication.occurrenceID != expectedOccurrenceID {
		return false
	}
	expectedSubject, expectedContext, expectedHasContext, inputsOK := publication.owner.resolvePublicationInputs(publication.operation, publication.callback, int(publication.effect), expected, publication.module, publication.occurrence)
	if !inputsOK || !publication.subject.Valid() || !publication.subject.Equal(expectedSubject) || publication.hasContext != expectedHasContext {
		return false
	}
	if !expectedHasContext {
		return !publication.context.Valid()
	}
	return publication.context.Valid() && publication.context.Equal(expectedContext)
}

// Valid reports whether this receipt is still authenticated by its issuing
// Effect algebra, exact mounted call, Target descriptor, and mounted Pack
// input rows.
func (publication MountedPublication) Valid() bool { return publication.valid() }

// ContentID returns the scalar seal of the mounted publication receipt.
func (publication MountedPublication) ContentID() (identity.ContentID, bool) {
	return publication.id, publication.available()
}

func (publication MountedPublication) Role() MountedPublicationRole {
	if !publication.available() {
		return MountedPublicationInvalid
	}
	return publication.role
}

// MountedCall returns Effect's exact opaque mounted-call receipt. It does not
// expose the underlying Project application or any mutable mount state.
func (publication MountedPublication) MountedCall() (MountedCall, bool) {
	return publication.mounted, publication.available()
}

// CallProvenance returns the exact mounted module and call-occurrence IDs.
// The second ID is the mounted Program occurrence/context identity, not an
// inferred runtime destination.
func (publication MountedPublication) CallProvenance() (module, call identity.ContentID, ok bool) {
	if !publication.available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	return publication.module, publication.occurrence, true
}

// MountID and CallOccurrenceID are explicit aliases used by mounted joins.
func (publication MountedPublication) MountID() identity.ContentID {
	if !publication.available() {
		return identity.ContentID{}
	}
	return publication.module
}

func (publication MountedPublication) CallOccurrenceID() identity.ContentID {
	if !publication.available() {
		return identity.ContentID{}
	}
	return publication.occurrence
}

func (publication MountedPublication) ApplicationID() (identity.ContentID, bool) {
	return publication.application, publication.available()
}

func (publication MountedPublication) AtomBinding() (AtomBinding, bool) {
	return publication.binding, publication.available()
}

func (publication MountedPublication) Descriptor() (operationvalue.PublicationEffectDescriptor, bool) {
	return publication.descriptor, publication.available()
}

func (publication MountedPublication) DescriptorID() (identity.ContentID, bool) {
	return publication.descriptorID, publication.available()
}

func (publication MountedPublication) OccurrenceID() (identity.ContentID, bool) {
	return publication.occurrenceID, publication.available()
}

func (publication MountedPublication) Operation() vocabulary.Operation {
	if !publication.available() {
		return 0
	}
	return publication.operation
}

func (publication MountedPublication) Callback() vocabulary.CallbackID {
	if !publication.available() || publication.role != MountedPublicationCallback {
		return 0
	}
	return publication.callback
}

func (publication MountedPublication) EffectIndex() int {
	if !publication.available() {
		return -1
	}
	return int(publication.effect)
}

func (publication MountedPublication) Kind() vocabulary.PublicationEffectKind {
	if !publication.available() {
		return vocabulary.PublicationEffectInvalid
	}
	return publication.descriptor.Kind()
}

func (publication MountedPublication) Escape() vocabulary.PublicationEscapeDisposition {
	if !publication.available() {
		return vocabulary.PublicationEscapeInvalid
	}
	return publication.descriptor.Escape()
}

func (publication MountedPublication) Mutability() vocabulary.PublicationMutabilityDisposition {
	if !publication.available() {
		return vocabulary.PublicationMutabilityInvalid
	}
	return publication.descriptor.Mutability()
}

func (publication MountedPublication) Lifetime() vocabulary.PublicationLifetimeDisposition {
	if !publication.available() {
		return vocabulary.PublicationLifetimeInvalid
	}
	return publication.descriptor.Lifetime()
}

func (publication MountedPublication) ContextInput() (packtransfer.MountedInput, bool) {
	return publication.context, publication.available() && publication.hasContext
}

func (publication MountedPublication) SubjectInput() (packtransfer.MountedInput, bool) {
	return publication.subject, publication.available()
}

func (a *Algebra) newMountedPublication(root Root, mounted MountedCall, operation vocabulary.Operation, callback vocabulary.CallbackID, effect int, binding AtomBinding, role MountedPublicationRole, descriptor operationvalue.PublicationEffectDescriptor, descriptorID, occurrenceID identity.ContentID, subject, context packtransfer.MountedInput, hasContext bool) (MountedPublication, bool) {
	if a == nil || !a.Valid() || !a.ownsRoot(root) || !mounted.Valid() || mounted.owner != a || !binding.valid() || binding.owner != a || !descriptor.Valid() || !descriptorID.Available() || !occurrenceID.Available() || !subject.Valid() || role < MountedPublicationOrdinary || role > MountedPublicationCallback || operation == 0 || effect < 0 {
		return MountedPublication{}, false
	}
	if hasContext != (descriptor.DestinationRole() == vocabulary.PublicationDestinationValueFormal) || hasContext && !context.Valid() || !hasContext && context.Valid() {
		return MountedPublication{}, false
	}
	application, module, occurrence, identityOK := a.MountedCallIdentity(mounted)
	if !identityOK {
		return MountedPublication{}, false
	}
	publication := MountedPublication{
		owner: a, mounted: mounted, application: application, module: module, occurrence: occurrence,
		binding: binding, role: role, operation: operation, callback: callback, effect: uint32(effect),
		descriptor: descriptor, descriptorID: descriptorID, occurrenceID: occurrenceID,
		subject: subject, context: context, hasContext: hasContext, sealed: true,
	}
	subjectID, subjectIDOK := subject.ContentID()
	contextID := identity.ContentID{}
	if hasContext {
		contextID, subjectIDOK = context.ContentID()
	}
	if !subjectIDOK {
		return MountedPublication{}, false
	}
	publication.id = mountedPublicationID(a.LinkID(), application, module, occurrence, binding.formal.id, descriptorID, occurrenceID, subjectID, contextID, role, operation, callback, uint32(effect), descriptor, hasContext)
	publication.sealedScalar = mountedPublicationScalar(publication.id)
	return publication, publication.valid()
}

// PublicationCallEffectBinding joins one exact ordinary AtomBinding to one
// authored publication descriptor. Generic ordinary effects cannot acquire
// publication meaning through this constructor.
func (a *Algebra) PublicationCallEffectBinding(root Root, mounted MountedCall, owner vocabulary.Operation, effect int, atomBinding AtomBinding) (MountedPublication, bool) {
	if a == nil || !a.Valid() {
		return MountedPublication{}, false
	}
	formal, formalOK := a.FormalCallEffectAtom(mounted, owner, effect)
	expectedBinding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
	descriptor, descriptorOK := a.contract.Operations.EffectPublication(owner, effect)
	descriptorID, descriptorIDOK := a.contract.Operations.PublicationEffectDescriptorID(owner, effect)
	occurrenceID, occurrenceOK := a.contract.Operations.PublicationEffectOccurrenceID(owner, effect)
	if !formalOK || !bindingOK || atomBinding != expectedBinding || !descriptorOK || !descriptorIDOK || !occurrenceOK {
		return MountedPublication{}, false
	}
	_, module, occurrence, provenanceOK := a.MountedCallIdentity(mounted)
	subject, context, hasContext, inputsOK := a.resolvePublicationInputs(owner, 0, effect, descriptor, module, occurrence)
	if !inputsOK || !provenanceOK {
		return MountedPublication{}, false
	}
	return a.newMountedPublication(root, mounted, owner, 0, effect, atomBinding, MountedPublicationOrdinary, descriptor, descriptorID, occurrenceID, subject, context, hasContext)
}

// PublicationCallbackEffectBinding is the callback-typed counterpart. Its
// role and callback coordinate remain explicit even when its AtomBinding ID
// is equal to an ordinary effect's semantic atom.
func (a *Algebra) PublicationCallbackEffectBinding(root Root, mounted MountedCall, owner vocabulary.Operation, callback vocabulary.CallbackID, effect int, atomBinding AtomBinding) (MountedPublication, bool) {
	if a == nil || !a.Valid() {
		return MountedPublication{}, false
	}
	formal, formalOK := a.FormalCallbackEffectAtom(mounted, owner, callback, effect)
	expectedBinding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
	descriptor, descriptorOK := a.contract.Operations.CallbackEffectPublication(callback, effect)
	descriptorID, descriptorIDOK := a.contract.Operations.CallbackPublicationEffectDescriptorID(callback, effect)
	occurrenceID, occurrenceOK := a.contract.Operations.CallbackPublicationEffectOccurrenceID(callback, effect)
	if !formalOK || !bindingOK || atomBinding != expectedBinding || !descriptorOK || !descriptorIDOK || !occurrenceOK {
		return MountedPublication{}, false
	}
	_, module, occurrence, provenanceOK := a.MountedCallIdentity(mounted)
	subject, context, hasContext, inputsOK := a.resolvePublicationInputs(owner, callback, effect, descriptor, module, occurrence)
	if !inputsOK || !provenanceOK {
		return MountedPublication{}, false
	}
	return a.newMountedPublication(root, mounted, owner, callback, effect, atomBinding, MountedPublicationCallback, descriptor, descriptorID, occurrenceID, subject, context, hasContext)
}

// SelectedCallMountedPublications issues every typed publication receipt for
// one exact selected mounted operation. Effects without authored publication
// descriptors remain ordinary AtomBindings and are omitted.
func (a *Algebra) SelectedCallMountedPublications(root Root, mounted MountedCall, owner vocabulary.Operation) ([]MountedPublication, bool) {
	if a == nil || !a.Valid() {
		return nil, false
	}
	bindings, ok := a.SelectedCallEffectBindings(root, mounted, owner)
	if !ok {
		return nil, false
	}
	publications := make([]MountedPublication, 0, len(bindings))
	index := 0
	for effect := 0; effect < a.contract.Operations.EffectCount(owner); effect++ {
		if index >= len(bindings) {
			return nil, false
		}
		if _, published := a.contract.Operations.EffectPublication(owner, effect); published {
			if _, occurrenceOK := a.publicationCallEffectOccurrence(root, mounted, owner, effect, bindings[index]); !occurrenceOK {
				return nil, false
			}
			publication, publicationOK := a.PublicationCallEffectBinding(root, mounted, owner, effect, bindings[index])
			if !publicationOK {
				return nil, false
			}
			publications = append(publications, publication)
		}
		index++
	}
	for callbackIndex := 0; callbackIndex < a.contract.Operations.CallbackCount(owner); callbackIndex++ {
		callback, callbackOK := a.contract.Operations.CallbackAt(owner, callbackIndex)
		if !callbackOK {
			return nil, false
		}
		for effect := 0; effect < a.contract.Operations.CallbackEffectCount(callback); effect++ {
			if index >= len(bindings) {
				return nil, false
			}
			if _, published := a.contract.Operations.CallbackEffectPublication(callback, effect); published {
				if _, occurrenceOK := a.publicationCallbackEffectOccurrence(root, mounted, owner, callback, effect, bindings[index]); !occurrenceOK {
					return nil, false
				}
				publication, publicationOK := a.PublicationCallbackEffectBinding(root, mounted, owner, callback, effect, bindings[index])
				if !publicationOK {
					return nil, false
				}
				publications = append(publications, publication)
			}
			index++
		}
	}
	if index != len(bindings) {
		return nil, false
	}
	// The Target contract owns occurrence identity. Equal occurrences in one
	// selected call are malformed and must not produce ambiguous receipts.
	seen := make(map[identity.ContentID]struct{}, len(publications))
	for _, publication := range publications {
		occurrenceID, occurrenceOK := publication.OccurrenceID()
		if !occurrenceOK {
			return nil, false
		}
		if _, duplicate := seen[occurrenceID]; duplicate {
			return nil, false
		}
		seen[occurrenceID] = struct{}{}
	}
	return publications, true
}
