package factor

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/pack"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

const (
	formalAtomDomain           = "wippy.analysis.effect.formal-atom.v1\x00"
	typeFormalDescriptorDomain = "wippy.analysis.effect.type-formals.v1\x00"
)

type formalAtomRole uint8

const (
	formalAtomInvalid formalAtomRole = iota
	formalAtomOrdinary
	formalAtomCallback
)

// FormalAtom is a reusable selected-call effect template. Its complete bytes
// contain only a Program formal call root, Target operation/descriptor
// identities, and the canonical type-formal correspondence. It carries no
// Link, Application, ModuleKey, Shard, Effect Root, runtime handle, or raw
// Program term.
type FormalAtom struct {
	call       pack.FormalCallRoot
	operation  keyspace.ContentID
	descriptor keyspace.ContentID
	types      keyspace.ContentID
	id         keyspace.ContentID
	role       formalAtomRole
	sealed     bool
}

// Valid reports whether this value was issued by the closed formal codec.
func (atom FormalAtom) Valid() bool {
	return atom.sealed && (atom.role == formalAtomOrdinary || atom.role == formalAtomCallback) && atom.call.Valid() && atom.operation.Available() && atom.descriptor.Available() && atom.id.Available()
}

// ContentID is the reusable formal atom identity. Ordinary and callback
// occurrences with the same semantic effect substitution deliberately share
// this quotient even though their typed capabilities remain distinct.
func (atom FormalAtom) ContentID() (keyspace.ContentID, bool) {
	if !atom.Valid() {
		return keyspace.ContentID{}, false
	}
	return atom.id, true
}

// CallRoot returns the reusable Pack call-root template consumed by beta.
func (atom FormalAtom) CallRoot() (pack.FormalCallRoot, bool) {
	if !atom.Valid() {
		return pack.FormalCallRoot{}, false
	}
	return atom.call, true
}

// Same compares the complete typed formal capability, not only its semantic
// quotient ID.
func (atom FormalAtom) Same(other FormalAtom) bool {
	return atom.Valid() && other.Valid() && atom == other
}

// AtomBinding is the exact beta receipt from one reusable FormalAtom to an
// existing mounted Effect Atom. It is immutable and its hot projections do
// not reopen Program, Project, Boundary, Pack, or Target.
type AtomBinding struct {
	owner  *Algebra
	formal FormalAtom
	root   Root
	atom   Atom
	sealed bool
}

// PublicationAtomBinding is the Effect-owned cold receipt for one explicitly
// authored Target publication effect. It joins the exact beta-bound effect
// atom to Target's descriptor occurrence and Pack's typed subject/context
// selectors. It is not a solver result, runtime placement decision, or
// allocation proof.
type PublicationAtomBinding struct {
	owner                      *Algebra
	mounted                    MountedCall
	binding                    AtomBinding
	role                       publicationAtomBindingRole
	operation                  target.Operation
	callback                   target.CallbackID
	effect                     uint32
	descriptor                 target.PublicationEffectDescriptor
	descriptorID, occurrenceID keyspace.ContentID
	subject                    pack.InputSelector
	context                    pack.InputSelector
	hasContext                 bool
	sealed                     bool
}

type publicationAtomBindingRole uint8

const (
	publicationAtomBindingInvalid publicationAtomBindingRole = iota
	publicationAtomBindingOrdinary
	publicationAtomBindingCallback
)

// PublicationAtomBindingRole preserves whether the descriptor originated in
// an ordinary selected effect row or an exact callback effect row.
type PublicationAtomBindingRole uint8

const (
	PublicationAtomBindingInvalid PublicationAtomBindingRole = iota
	PublicationAtomBindingOrdinary
	PublicationAtomBindingCallback
)

func (binding PublicationAtomBinding) valid() bool {
	if !binding.sealed || binding.owner == nil || !binding.owner.Valid() || !binding.mounted.Valid() || binding.mounted.owner != binding.owner || !binding.binding.valid() || binding.binding.owner != binding.owner || !binding.descriptorID.Available() || !binding.occurrenceID.Available() || !binding.owner.packs.OwnsInputSelector(binding.subject) {
		return false
	}
	root, rootOK := binding.owner.RootForMountedCall(binding.mounted)
	application, _, _, mountedOK := binding.owner.MountedCallIdentity(binding.mounted)
	if !rootOK || !mountedOK || root != binding.binding.root || !binding.owner.callInRootID(root, application) {
		return false
	}
	var expected target.PublicationEffectDescriptor
	var descriptorID, occurrenceID keyspace.ContentID
	var selectorFormal target.ValueFormal
	switch binding.role {
	case publicationAtomBindingOrdinary:
		if binding.callback != 0 || binding.binding.formal.role != formalAtomOrdinary || uint64(binding.effect) >= uint64(binding.owner.contract.EffectCount(binding.operation)) {
			return false
		}
		effect := int(binding.effect)
		var ok bool
		expected, ok = binding.owner.contract.PublicationEffectDescriptor(binding.operation, effect)
		if !ok {
			return false
		}
		descriptorID, ok = binding.owner.contract.PublicationEffectDescriptorID(binding.operation, effect)
		if !ok {
			return false
		}
		occurrenceID, ok = binding.owner.contract.PublicationEffectOccurrenceID(binding.operation, effect)
		if !ok || binding.binding.formal.descriptor != descriptorID {
			return false
		}
		selectorFormal, ok = binding.owner.contract.EffectValueArgumentAt(binding.operation, effect, int(expected.Subject()))
		if !ok {
			return false
		}
	case publicationAtomBindingCallback:
		if binding.callback == 0 || binding.binding.formal.role != formalAtomCallback || uint64(binding.effect) >= uint64(binding.owner.contract.CallbackEffectCount(binding.callback)) {
			return false
		}
		effect := int(binding.effect)
		callbackOwner, ownerOK := binding.owner.contract.CallbackOwner(binding.callback)
		if !ownerOK || callbackOwner != binding.operation {
			return false
		}
		var ok bool
		expected, ok = binding.owner.contract.CallbackPublicationEffectDescriptor(binding.callback, effect)
		if !ok {
			return false
		}
		descriptorID, ok = binding.owner.contract.CallbackPublicationEffectDescriptorID(binding.callback, effect)
		if !ok {
			return false
		}
		occurrenceID, ok = binding.owner.contract.CallbackPublicationEffectOccurrenceID(binding.callback, effect)
		if !ok || binding.binding.formal.descriptor != descriptorID {
			return false
		}
		selectorFormal, ok = binding.owner.contract.CallbackEffectValueArgumentAt(binding.callback, effect, int(expected.Subject()))
		if !ok {
			return false
		}
	default:
		return false
	}
	if _, selected := binding.owner.applicationOperation(application, binding.operation); !selected {
		return false
	}
	if binding.descriptor != expected || binding.descriptorID != descriptorID || binding.occurrenceID != occurrenceID {
		return false
	}
	expectedSubject, subjectOK := binding.owner.packs.InputSelector(binding.operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(selectorFormal)})
	if !subjectOK || binding.subject != expectedSubject {
		return false
	}
	switch expected.DestinationRole() {
	case target.PublicationDestinationNone:
		return !binding.hasContext && binding.context == (pack.InputSelector{})
	case target.PublicationDestinationValueFormal:
		var contextFormal target.ValueFormal
		var contextOK bool
		if binding.role == publicationAtomBindingOrdinary {
			contextFormal, contextOK = binding.owner.contract.EffectValueArgumentAt(binding.operation, int(binding.effect), int(expected.Context()))
		} else {
			contextFormal, contextOK = binding.owner.contract.CallbackEffectValueArgumentAt(binding.callback, int(binding.effect), int(expected.Context()))
		}
		expectedContext, selectorOK := binding.owner.packs.InputSelector(binding.operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(contextFormal)})
		return binding.hasContext && contextOK && selectorOK && binding.owner.packs.OwnsInputSelector(binding.context) && binding.context == expectedContext
	default:
		return false
	}
}

// Valid reports whether the receipt is still authenticated by its issuing
// Effect algebra, mounted call, Target descriptor occurrence, and Pack ABI.
func (binding PublicationAtomBinding) Valid() bool { return binding.valid() }

func (binding PublicationAtomBinding) Role() PublicationAtomBindingRole {
	if !binding.valid() {
		return PublicationAtomBindingInvalid
	}
	switch binding.role {
	case publicationAtomBindingOrdinary:
		return PublicationAtomBindingOrdinary
	case publicationAtomBindingCallback:
		return PublicationAtomBindingCallback
	default:
		return PublicationAtomBindingInvalid
	}
}

func (binding PublicationAtomBinding) MountedCall() (MountedCall, bool) {
	return binding.mounted, binding.valid()
}
func (binding PublicationAtomBinding) AtomBinding() (AtomBinding, bool) {
	return binding.binding, binding.valid()
}
func (binding PublicationAtomBinding) DescriptorID() (keyspace.ContentID, bool) {
	return binding.descriptorID, binding.valid()
}
func (binding PublicationAtomBinding) OccurrenceID() (keyspace.ContentID, bool) {
	return binding.occurrenceID, binding.valid()
}
func (binding PublicationAtomBinding) Kind() target.PublicationEffectKind {
	if !binding.valid() {
		return target.PublicationEffectInvalid
	}
	return binding.descriptor.Kind()
}
func (binding PublicationAtomBinding) Escape() target.PublicationEscapeDisposition {
	if !binding.valid() {
		return target.PublicationEscapeInvalid
	}
	return binding.descriptor.Escape()
}
func (binding PublicationAtomBinding) Mutability() target.PublicationMutabilityDisposition {
	if !binding.valid() {
		return target.PublicationMutabilityInvalid
	}
	return binding.descriptor.Mutability()
}
func (binding PublicationAtomBinding) Lifetime() target.PublicationLifetimeDisposition {
	if !binding.valid() {
		return target.PublicationLifetimeInvalid
	}
	return binding.descriptor.Lifetime()
}
func (binding PublicationAtomBinding) SubjectSelector() (pack.InputSelector, bool) {
	return binding.subject, binding.valid()
}
func (binding PublicationAtomBinding) ContextSelector() (pack.InputSelector, bool) {
	return binding.context, binding.valid() && binding.hasContext
}

func (binding AtomBinding) valid() bool {
	return binding.sealed && binding.owner != nil && binding.formal.Valid() && binding.owner.ownsRoot(binding.root) && binding.atom.validFor(binding.owner) && binding.atom.root == binding.root.slot && binding.atom.id == binding.formal.id
}

// MatchesCertificate reports whether id is this binding's already-issued
// atom certificate. It deliberately admits no inverse construction: callers
// can prove membership in an observed Effect value, but cannot mint an Atom
// or a beta binding from a portable ID.
func (binding AtomBinding) MatchesCertificate(id keyspace.ContentID) bool {
	return binding.valid() && id.Available() && binding.atom.id == id
}

// Formal returns the reusable source template.
func (binding AtomBinding) Formal() (FormalAtom, bool) {
	if !binding.valid() {
		return FormalAtom{}, false
	}
	return binding.formal, true
}

// Root returns the exact mounted Effect placement.
func (binding AtomBinding) Root() (Root, bool) {
	if !binding.valid() {
		return Root{}, false
	}
	return binding.root, true
}

// Atom returns the already-bound existing Effect atom.
func (binding AtomBinding) Atom() (Atom, bool) {
	if !binding.valid() {
		return Atom{}, false
	}
	return binding.atom, true
}

// FormalCallEffectAtom derives one reusable ordinary effect template from the
// exact Project/Program mounted-call proofs and a Target descriptor.
func (a *Algebra) FormalCallEffectAtom(mounted MountedCall, owner target.Operation, effect int) (FormalAtom, bool) {
	callRoot, typeArguments, applicationID, ok := a.formalCallRoot(mounted, owner)
	if !ok {
		return FormalAtom{}, false
	}
	tail, _, tailOK := a.contract.EffectTail(owner)
	if !tailOK || (tail != target.RowClosed && tail != target.RowUnknownOpen) || effect < 0 || effect >= a.contract.EffectCount(owner) || a.contract.EffectRowArgumentCount(owner, effect) != 0 {
		return FormalAtom{}, false
	}
	targetOperation, targetOK := a.contract.EffectTarget(owner, effect)
	if !targetOK || !a.validateOrdinaryInputs(owner, effect) {
		return FormalAtom{}, false
	}
	descriptor, descriptorOK := a.contract.EffectDescriptorID(owner, effect)
	operation, operationOK := a.contract.EffectOperationID(targetOperation)
	types, typesOK := a.ordinaryTypeFormalDescriptor(applicationID, typeArguments, owner, effect)
	if !descriptorOK || !operationOK || !typesOK {
		return FormalAtom{}, false
	}
	return newFormalAtom(formalAtomOrdinary, callRoot, operation, descriptor, types)
}

// FormalCallbackEffectAtom derives one reusable callback effect template from
// the same exact mounted-call proofs.
func (a *Algebra) FormalCallbackEffectAtom(mounted MountedCall, owner target.Operation, callback target.CallbackID, effect int) (FormalAtom, bool) {
	callRoot, typeArguments, applicationID, ok := a.formalCallRoot(mounted, owner)
	if !ok || effect < 0 || effect >= a.contract.CallbackEffectCount(callback) {
		return FormalAtom{}, false
	}
	callbackOwner, ownerOK := a.contract.CallbackOwner(callback)
	tail, _, tailOK := a.contract.CallbackEffectTail(callback)
	if !ownerOK || callbackOwner != owner || !tailOK || (tail != target.RowClosed && tail != target.RowUnknownOpen) || a.contract.CallbackEffectRowArgumentCount(callback, effect) != 0 {
		return FormalAtom{}, false
	}
	targetOperation, targetOK := a.contract.CallbackEffectTarget(callback, effect)
	if !targetOK || !a.validateCallbackInputs(owner, callback, effect) {
		return FormalAtom{}, false
	}
	descriptor, descriptorOK := a.contract.CallbackEffectDescriptorID(callback, effect)
	operation, operationOK := a.contract.EffectOperationID(targetOperation)
	types, typesOK := a.callbackTypeFormalDescriptor(applicationID, typeArguments, owner, callback, effect)
	if !descriptorOK || !operationOK || !typesOK {
		return FormalAtom{}, false
	}
	return newFormalAtom(formalAtomCallback, callRoot, operation, descriptor, types)
}

// BindFormalCallEffectAtom beta-freshens an ordinary formal template into one
// exact mounted Effect root without allocating a new vocabulary coordinate.
func (a *Algebra) BindFormalCallEffectAtom(root Root, mounted MountedCall, owner target.Operation, effect int, formal FormalAtom) (AtomBinding, bool) {
	expected, ok := a.FormalCallEffectAtom(mounted, owner, effect)
	if !ok || formal.role != formalAtomOrdinary {
		return AtomBinding{}, false
	}
	return a.bindFormalAtom(root, mounted, formal, expected)
}

// BindFormalCallbackEffectAtom is the callback-typed beta binding.
func (a *Algebra) BindFormalCallbackEffectAtom(root Root, mounted MountedCall, owner target.Operation, callback target.CallbackID, effect int, formal FormalAtom) (AtomBinding, bool) {
	expected, ok := a.FormalCallbackEffectAtom(mounted, owner, callback, effect)
	if !ok || formal.role != formalAtomCallback {
		return AtomBinding{}, false
	}
	return a.bindFormalAtom(root, mounted, formal, expected)
}

// PublicationCallEffectBinding joins one already-issued ordinary AtomBinding
// to an explicitly authored Target publication descriptor. Effects without a
// publication descriptor remain ordinary AtomBindings and return absent.
func (a *Algebra) PublicationCallEffectBinding(root Root, mounted MountedCall, owner target.Operation, effect int, atomBinding AtomBinding) (PublicationAtomBinding, bool) {
	formal, formalOK := a.FormalCallEffectAtom(mounted, owner, effect)
	expectedBinding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
	descriptor, descriptorOK := a.contract.PublicationEffectDescriptor(owner, effect)
	descriptorID, descriptorIDOK := a.contract.PublicationEffectDescriptorID(owner, effect)
	occurrenceID, occurrenceOK := a.contract.PublicationEffectOccurrenceID(owner, effect)
	if !formalOK || !bindingOK || atomBinding != expectedBinding || !descriptorOK || !descriptorIDOK || !occurrenceOK {
		return PublicationAtomBinding{}, false
	}
	subjectFormal, subjectOK := a.contract.EffectValueArgumentAt(owner, effect, int(descriptor.Subject()))
	subject, selectorOK := a.packs.InputSelector(owner, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(subjectFormal)})
	if !subjectOK || !selectorOK {
		return PublicationAtomBinding{}, false
	}
	binding := PublicationAtomBinding{owner: a, mounted: mounted, binding: atomBinding, role: publicationAtomBindingOrdinary, operation: owner, effect: uint32(effect), descriptor: descriptor, descriptorID: descriptorID, occurrenceID: occurrenceID, subject: subject, sealed: true}
	if descriptor.DestinationRole() == target.PublicationDestinationValueFormal {
		contextFormal, contextOK := a.contract.EffectValueArgumentAt(owner, effect, int(descriptor.Context()))
		context, contextSelectorOK := a.packs.InputSelector(owner, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(contextFormal)})
		if !contextOK || !contextSelectorOK {
			return PublicationAtomBinding{}, false
		}
		binding.context, binding.hasContext = context, true
	}
	return binding, binding.valid()
}

// PublicationCallbackEffectBinding is the callback-typed counterpart of
// PublicationCallEffectBinding. Callback provenance remains explicit even
// when its ordinary atom quotient is content-equal to another effect row.
func (a *Algebra) PublicationCallbackEffectBinding(root Root, mounted MountedCall, owner target.Operation, callback target.CallbackID, effect int, atomBinding AtomBinding) (PublicationAtomBinding, bool) {
	formal, formalOK := a.FormalCallbackEffectAtom(mounted, owner, callback, effect)
	expectedBinding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
	descriptor, descriptorOK := a.contract.CallbackPublicationEffectDescriptor(callback, effect)
	descriptorID, descriptorIDOK := a.contract.CallbackPublicationEffectDescriptorID(callback, effect)
	occurrenceID, occurrenceOK := a.contract.CallbackPublicationEffectOccurrenceID(callback, effect)
	if !formalOK || !bindingOK || atomBinding != expectedBinding || !descriptorOK || !descriptorIDOK || !occurrenceOK {
		return PublicationAtomBinding{}, false
	}
	subjectFormal, subjectOK := a.contract.CallbackEffectValueArgumentAt(callback, effect, int(descriptor.Subject()))
	subject, selectorOK := a.packs.InputSelector(owner, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(subjectFormal)})
	if !subjectOK || !selectorOK {
		return PublicationAtomBinding{}, false
	}
	binding := PublicationAtomBinding{owner: a, mounted: mounted, binding: atomBinding, role: publicationAtomBindingCallback, operation: owner, callback: callback, effect: uint32(effect), descriptor: descriptor, descriptorID: descriptorID, occurrenceID: occurrenceID, subject: subject, sealed: true}
	if descriptor.DestinationRole() == target.PublicationDestinationValueFormal {
		contextFormal, contextOK := a.contract.CallbackEffectValueArgumentAt(callback, effect, int(descriptor.Context()))
		context, contextSelectorOK := a.packs.InputSelector(owner, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(contextFormal)})
		if !contextOK || !contextSelectorOK {
			return PublicationAtomBinding{}, false
		}
		binding.context, binding.hasContext = context, true
	}
	return binding, binding.valid()
}

// SelectedCallPublicationAtomBindings mints publication receipts alongside
// the existing selected ordinary/callback AtomBindings. It omits generic
// non-publication effects rather than assigning them inferred memory meaning.
func (a *Algebra) SelectedCallPublicationAtomBindings(root Root, mounted MountedCall, owner target.Operation) ([]PublicationAtomBinding, bool) {
	bindings, ok := a.SelectedCallEffectBindings(root, mounted, owner)
	if !ok {
		return nil, false
	}
	publications := make([]PublicationAtomBinding, 0, len(bindings))
	index := 0
	for effect := 0; effect < a.contract.EffectCount(owner); effect++ {
		if index >= len(bindings) {
			return nil, false
		}
		if _, published := a.contract.PublicationEffectDescriptor(owner, effect); published {
			binding, bindingOK := a.PublicationCallEffectBinding(root, mounted, owner, effect, bindings[index])
			if !bindingOK {
				return nil, false
			}
			publications = append(publications, binding)
		}
		index++
	}
	for callbackIndex := 0; callbackIndex < a.contract.CallbackCount(owner); callbackIndex++ {
		callback, callbackOK := a.contract.CallbackAt(owner, callbackIndex)
		if !callbackOK {
			return nil, false
		}
		for effect := 0; effect < a.contract.CallbackEffectCount(callback); effect++ {
			if index >= len(bindings) {
				return nil, false
			}
			if _, published := a.contract.CallbackPublicationEffectDescriptor(callback, effect); published {
				binding, bindingOK := a.PublicationCallbackEffectBinding(root, mounted, owner, callback, effect, bindings[index])
				if !bindingOK {
					return nil, false
				}
				publications = append(publications, binding)
			}
			index++
		}
	}
	return publications, index == len(bindings)
}

// SelectedCallEffectBindings issues the complete explicit-effect beta receipt
// vector for one exact mounted selected operation. It is a cold constructor:
// hot Rules retain the returned AtomBindings and project their atoms in O(1)
// without reopening Boundary, Target, Pack, or Program.
func (a *Algebra) SelectedCallEffectBindings(root Root, mounted MountedCall, owner target.Operation) ([]AtomBinding, bool) {
	if !a.ownsRoot(root) {
		return nil, false
	}
	tail, _, tailOK := a.contract.EffectTail(owner)
	if !tailOK || (tail != target.RowClosed && tail != target.RowUnknownOpen) {
		return nil, false
	}
	count := a.contract.EffectCount(owner)
	callbackCount := a.contract.CallbackCount(owner)
	for callbackIndex := 0; callbackIndex < callbackCount; callbackIndex++ {
		callback, callbackOK := a.contract.CallbackAt(owner, callbackIndex)
		if !callbackOK {
			return nil, false
		}
		callbackTail, _, callbackTailOK := a.contract.CallbackEffectTail(callback)
		if !callbackTailOK || (callbackTail != target.RowClosed && callbackTail != target.RowUnknownOpen) {
			return nil, false
		}
		var added bool
		count, added = checkedIntAdd(count, a.contract.CallbackEffectCount(callback))
		if !added {
			return nil, false
		}
	}
	bindings := make([]AtomBinding, 0, count)
	for effect := 0; effect < a.contract.EffectCount(owner); effect++ {
		formal, formalOK := a.FormalCallEffectAtom(mounted, owner, effect)
		binding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
		if !formalOK || !bindingOK {
			return nil, false
		}
		bindings = append(bindings, binding)
	}
	for callbackIndex := 0; callbackIndex < callbackCount; callbackIndex++ {
		callback, callbackOK := a.contract.CallbackAt(owner, callbackIndex)
		if !callbackOK {
			return nil, false
		}
		for effect := 0; effect < a.contract.CallbackEffectCount(callback); effect++ {
			formal, formalOK := a.FormalCallbackEffectAtom(mounted, owner, callback, effect)
			binding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
			if !formalOK || !bindingOK {
				return nil, false
			}
			bindings = append(bindings, binding)
		}
	}
	return bindings, true
}

func (a *Algebra) bindFormalAtom(root Root, mounted MountedCall, formal, expected FormalAtom) (AtomBinding, bool) {
	resolved, resolvedOK := a.RootForMountedCall(mounted)
	if !a.ownsRoot(root) || !formal.Same(expected) || !resolvedOK || resolved != root {
		return AtomBinding{}, false
	}
	_, module, occurrence, rowOK := a.MountedCallIdentity(mounted)
	issued, issuedOK := a.packs.FormalCallRootForMountedSemantic(module, occurrence)
	packRoot, packRootOK := a.packs.CallRootForMountedSemantic(module, occurrence)
	_, packRootIDOK := a.packs.RootID(packRoot)
	if !rowOK || !issuedOK || !formal.call.Same(issued) || !packRootOK || !packRootIDOK {
		return AtomBinding{}, false
	}
	atom := Atom{owner: a, root: root.slot, id: formal.id}
	binding := AtomBinding{owner: a, formal: formal, root: root, atom: atom, sealed: true}
	return binding, binding.valid()
}

func (a *Algebra) formalCallRoot(mounted MountedCall, owner target.Operation) (pack.FormalCallRoot, pack.FormalCallTypeArguments, keyspace.ContentID, bool) {
	application, module, occurrence, ok := a.MountedCallIdentity(mounted)
	root, rootOK := a.RootForMountedCall(mounted)
	if !ok || !a.Valid() || !rootOK || !a.callInRootID(root, application) {
		return pack.FormalCallRoot{}, pack.FormalCallTypeArguments{}, keyspace.ContentID{}, false
	}
	if _, available := a.applicationOperation(application, owner); !available {
		return pack.FormalCallRoot{}, pack.FormalCallTypeArguments{}, keyspace.ContentID{}, false
	}
	formal, formalOK := a.packs.FormalCallRootForMountedSemantic(module, occurrence)
	types, typesOK := a.packs.FormalTypeArgumentsForMountedSemantic(module, occurrence)
	return formal, types, application, formalOK && typesOK
}

func (a *Algebra) ordinaryTypeFormalDescriptor(applicationID keyspace.ContentID, arguments pack.FormalCallTypeArguments, owner target.Operation, effect int) (keyspace.ContentID, bool) {
	count := a.contract.EffectTypeArgumentCount(owner, effect)
	positions := make([]target.TypeFormal, count)
	for index := range positions {
		formal, ok := a.contract.EffectTypeArgumentAt(owner, effect, index)
		if !ok {
			return keyspace.ContentID{}, false
		}
		positions[index] = formal
	}
	return a.selectedTypeFormalDescriptor(applicationID, arguments, owner, positions)
}

func (a *Algebra) callbackTypeFormalDescriptor(applicationID keyspace.ContentID, arguments pack.FormalCallTypeArguments, owner target.Operation, callback target.CallbackID, effect int) (keyspace.ContentID, bool) {
	count := a.contract.CallbackEffectTypeArgumentCount(callback, effect)
	positions := make([]target.TypeFormal, count)
	for index := range positions {
		formal, ok := a.contract.CallbackEffectTypeArgumentAt(callback, effect, index)
		if !ok {
			return keyspace.ContentID{}, false
		}
		positions[index] = formal
	}
	return a.selectedTypeFormalDescriptor(applicationID, arguments, owner, positions)
}

// selectedTypeFormalDescriptor uses Boundary only as the exact live
// call/Target-ABI admission proof. Reusable bytes contain Pack's ordered
// canonical semantic type descriptor plus Target formal positions; Boundary's
// Program/Static-fenced correspondence ID and raw Terms never enter them.
func (a *Algebra) selectedTypeFormalDescriptor(applicationID keyspace.ContentID, formalArguments pack.FormalCallTypeArguments, owner target.Operation, positions []target.TypeFormal) (keyspace.ContentID, bool) {
	if len(positions) == 0 {
		return keyspace.ContentID{}, true
	}
	argumentCount, ok := a.applicationOperation(applicationID, owner)
	if !ok {
		return keyspace.ContentID{}, false
	}
	semanticTypes, typesOK := formalArguments.ContentID()
	if !typesOK || formalArguments.Count() != argumentCount {
		return keyspace.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(typeFormalDescriptorDomain))
	_, _ = hash.Write(semanticTypes[:])
	var word [4]byte
	binary.BigEndian.PutUint32(word[:], uint32(len(positions)))
	_, _ = hash.Write(word[:])
	for _, formal := range positions {
		if int(formal) < 0 || int(formal) >= argumentCount {
			return keyspace.ContentID{}, false
		}
		binary.BigEndian.PutUint32(word[:], uint32(formal))
		_, _ = hash.Write(word[:])
	}
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func newFormalAtom(role formalAtomRole, call pack.FormalCallRoot, operation, descriptor, types keyspace.ContentID) (FormalAtom, bool) {
	if (role != formalAtomOrdinary && role != formalAtomCallback) || !call.Valid() || !operation.Available() || !descriptor.Available() {
		return FormalAtom{}, false
	}
	callID, ok := call.ContentID()
	if !ok {
		return FormalAtom{}, false
	}
	id := formalAtomID(operation, descriptor, callID, types)
	atom := FormalAtom{call: call, operation: operation, descriptor: descriptor, types: types, id: id, role: role, sealed: true}
	return atom, atom.Valid()
}

func formalAtomID(operation, descriptor, call, types keyspace.ContentID) keyspace.ContentID {
	if !operation.Available() || !descriptor.Available() || !call.Available() {
		return keyspace.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(formalAtomDomain))
	_, _ = hash.Write(operation[:])
	_, _ = hash.Write(descriptor[:])
	_, _ = hash.Write(call[:])
	_, _ = hash.Write(types[:])
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
