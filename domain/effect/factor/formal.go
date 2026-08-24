package factor

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/effect/internal/valuecore"
	"github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/static"
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
	operation  identity.ContentID
	descriptor identity.ContentID
	types      identity.ContentID
	id         identity.ContentID
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
func (atom FormalAtom) ContentID() (identity.ContentID, bool) {
	if !atom.Valid() {
		return identity.ContentID{}, false
	}
	return atom.id, true
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

func (binding AtomBinding) valid() bool {
	return binding.sealed && binding.owner != nil && binding.formal.Valid() && binding.owner.ownsRoot(binding.root) && atomValidFor(binding.atom, binding.owner) && binding.atom.Root() == binding.root.slot && binding.atom.ID() == binding.formal.id
}

// MatchesCertificate reports whether id is this binding's already-issued
// atom certificate. It deliberately admits no inverse construction: callers
// can prove membership in an observed Effect value, but cannot mint an Atom
// or a beta binding from a portable ID.
func (binding AtomBinding) MatchesCertificate(id identity.ContentID) bool {
	return binding.valid() && id.Available() && binding.atom.ID() == id
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
func (a *Algebra) FormalCallEffectAtom(mounted MountedCall, owner vocabulary.Operation, effect int) (FormalAtom, bool) {
	callRoot, typeArguments, applicationID, ok := a.formalCallRoot(mounted, owner)
	if !ok {
		return FormalAtom{}, false
	}
	tail, _, tailOK := a.contract.Operations.EffectTail(owner)
	if !tailOK || (tail != vocabulary.RowClosed && tail != vocabulary.RowUnknownOpen) || effect < 0 || effect >= a.contract.Operations.EffectCount(owner) || a.contract.Operations.EffectRowArgumentCount(owner, effect) != 0 {
		return FormalAtom{}, false
	}
	targetOperation, targetOK := a.contract.Operations.EffectTarget(owner, effect)
	if !targetOK || !a.validateOrdinaryInputs(owner, effect) {
		return FormalAtom{}, false
	}
	descriptor, descriptorOK := a.contract.Operations.EffectDescriptorID(owner, effect)
	operation, operationOK := a.contract.Operations.EffectOperationID(targetOperation)
	types, typesOK := a.ordinaryTypeFormalDescriptor(applicationID, typeArguments, owner, effect)
	if !descriptorOK || !operationOK || !typesOK {
		return FormalAtom{}, false
	}
	return newFormalAtom(formalAtomOrdinary, callRoot, operation, descriptor, types)
}

// FormalCallbackEffectAtom derives one reusable callback effect template from
// the same exact mounted-call proofs.
func (a *Algebra) FormalCallbackEffectAtom(mounted MountedCall, owner vocabulary.Operation, callback vocabulary.CallbackID, effect int) (FormalAtom, bool) {
	callRoot, typeArguments, applicationID, ok := a.formalCallRoot(mounted, owner)
	if !ok || effect < 0 || effect >= a.contract.Operations.CallbackEffectCount(callback) {
		return FormalAtom{}, false
	}
	callbackOwner, ownerOK := a.contract.Operations.CallbackOwner(callback)
	tail, _, tailOK := a.contract.Operations.CallbackEffectTail(callback)
	if !ownerOK || callbackOwner != owner || !tailOK || (tail != vocabulary.RowClosed && tail != vocabulary.RowUnknownOpen) || a.contract.Operations.CallbackEffectRowArgumentCount(callback, effect) != 0 {
		return FormalAtom{}, false
	}
	targetOperation, targetOK := a.contract.Operations.CallbackEffectTarget(callback, effect)
	if !targetOK || !a.validateCallbackInputs(owner, callback, effect) {
		return FormalAtom{}, false
	}
	descriptor, descriptorOK := a.contract.Operations.CallbackEffectDescriptorID(callback, effect)
	operation, operationOK := a.contract.Operations.EffectOperationID(targetOperation)
	types, typesOK := a.callbackTypeFormalDescriptor(applicationID, typeArguments, owner, callback, effect)
	if !descriptorOK || !operationOK || !typesOK {
		return FormalAtom{}, false
	}
	return newFormalAtom(formalAtomCallback, callRoot, operation, descriptor, types)
}

// BindFormalCallEffectAtom beta-freshens an ordinary formal template into one
// exact mounted Effect root without allocating a new vocabulary coordinate.
func (a *Algebra) BindFormalCallEffectAtom(root Root, mounted MountedCall, owner vocabulary.Operation, effect int, formal FormalAtom) (AtomBinding, bool) {
	expected, ok := a.FormalCallEffectAtom(mounted, owner, effect)
	if !ok || formal.role != formalAtomOrdinary {
		return AtomBinding{}, false
	}
	return a.bindFormalAtom(root, mounted, formal, expected)
}

// BindFormalCallbackEffectAtom is the callback-typed beta binding.
func (a *Algebra) BindFormalCallbackEffectAtom(root Root, mounted MountedCall, owner vocabulary.Operation, callback vocabulary.CallbackID, effect int, formal FormalAtom) (AtomBinding, bool) {
	expected, ok := a.FormalCallbackEffectAtom(mounted, owner, callback, effect)
	if !ok || formal.role != formalAtomCallback {
		return AtomBinding{}, false
	}
	return a.bindFormalAtom(root, mounted, formal, expected)
}

func (a *Algebra) publicationCallEffectOccurrence(root Root, mounted MountedCall, owner vocabulary.Operation, effect int, atomBinding AtomBinding) (identity.ContentID, bool) {
	formal, formalOK := a.FormalCallEffectAtom(mounted, owner, effect)
	expectedBinding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
	descriptor, descriptorOK := a.contract.Operations.EffectPublication(owner, effect)
	descriptorID, descriptorIDOK := a.contract.Operations.PublicationEffectDescriptorID(owner, effect)
	occurrenceID, occurrenceOK := a.contract.Operations.PublicationEffectOccurrenceID(owner, effect)
	application, module, occurrence, mountedOK := a.MountedCallIdentity(mounted)
	if !formalOK || !bindingOK || atomBinding != expectedBinding || !descriptorOK || !descriptorIDOK || !descriptorID.Available() || !occurrenceOK || !occurrenceID.Available() || !mountedOK || formal.descriptor != descriptorID {
		return identity.ContentID{}, false
	}
	if _, selected := a.applicationOperation(application, owner); !selected {
		return identity.ContentID{}, false
	}
	_, _, _, inputOK := a.resolvePublicationInputs(owner, 0, effect, descriptor, module, occurrence)
	if !inputOK {
		return identity.ContentID{}, false
	}
	return occurrenceID, true
}

func (a *Algebra) publicationCallbackEffectOccurrence(root Root, mounted MountedCall, owner vocabulary.Operation, callback vocabulary.CallbackID, effect int, atomBinding AtomBinding) (identity.ContentID, bool) {
	formal, formalOK := a.FormalCallbackEffectAtom(mounted, owner, callback, effect)
	expectedBinding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
	descriptor, descriptorOK := a.contract.Operations.CallbackEffectPublication(callback, effect)
	descriptorID, descriptorIDOK := a.contract.Operations.CallbackPublicationEffectDescriptorID(callback, effect)
	occurrenceID, occurrenceOK := a.contract.Operations.CallbackPublicationEffectOccurrenceID(callback, effect)
	application, module, occurrence, mountedOK := a.MountedCallIdentity(mounted)
	callbackOwner, ownerOK := a.contract.Operations.CallbackOwner(callback)
	if !formalOK || !bindingOK || atomBinding != expectedBinding || !descriptorOK || !descriptorIDOK || !descriptorID.Available() || !occurrenceOK || !occurrenceID.Available() || !mountedOK || !ownerOK || callbackOwner != owner || formal.descriptor != descriptorID {
		return identity.ContentID{}, false
	}
	if _, selected := a.applicationOperation(application, owner); !selected {
		return identity.ContentID{}, false
	}
	_, _, _, inputOK := a.resolvePublicationInputs(owner, callback, effect, descriptor, module, occurrence)
	if !inputOK {
		return identity.ContentID{}, false
	}
	return occurrenceID, true
}

// SelectedCallPublication reports whether one selected call has explicitly
// authored publication effects. All descriptor, occurrence, provenance,
// selector, and uniqueness checks remain private to Effect; callers receive
// no publication receipt or payload.
func (a *Algebra) SelectedCallPublication(root Root, mounted MountedCall, owner vocabulary.Operation) (present bool, ok bool) {
	resolved, resolvedOK := a.RootForMountedCall(mounted)
	application, _, _, identityOK := a.MountedCallIdentity(mounted)
	_, selected := a.applicationOperation(application, owner)
	if !a.ownsRoot(root) || !mounted.Valid() || mounted.owner != a || !resolvedOK || resolved != root || !identityOK || !selected {
		return false, false
	}
	bindings, ok := a.SelectedCallEffectBindings(root, mounted, owner)
	if !ok {
		return false, false
	}
	occurrences := make(map[identity.ContentID]struct{}, len(bindings))
	index := 0
	for effect := 0; effect < a.contract.Operations.EffectCount(owner); effect++ {
		if index >= len(bindings) {
			return false, false
		}
		if _, published := a.contract.Operations.EffectPublication(owner, effect); published {
			occurrenceID, valid := a.publicationCallEffectOccurrence(root, mounted, owner, effect, bindings[index])
			if !valid {
				return false, false
			}
			if _, duplicate := occurrences[occurrenceID]; duplicate {
				return false, false
			}
			occurrences[occurrenceID] = struct{}{}
			present = true
		}
		index++
	}
	for callbackIndex := 0; callbackIndex < a.contract.Operations.CallbackCount(owner); callbackIndex++ {
		callback, callbackOK := a.contract.Operations.CallbackAt(owner, callbackIndex)
		if !callbackOK {
			return false, false
		}
		for effect := 0; effect < a.contract.Operations.CallbackEffectCount(callback); effect++ {
			if index >= len(bindings) {
				return false, false
			}
			if _, published := a.contract.Operations.CallbackEffectPublication(callback, effect); published {
				occurrenceID, valid := a.publicationCallbackEffectOccurrence(root, mounted, owner, callback, effect, bindings[index])
				if !valid {
					return false, false
				}
				if _, duplicate := occurrences[occurrenceID]; duplicate {
					return false, false
				}
				occurrences[occurrenceID] = struct{}{}
				present = true
			}
			index++
		}
	}
	return present, index == len(bindings)
}

// SelectedCallEffectBindings issues the complete explicit-effect beta receipt
// vector for one exact mounted selected operation. It is a cold constructor:
// hot Rules retain the returned AtomBindings and project their atoms in O(1)
// without reopening Boundary, Target, Pack, or Program.
func (a *Algebra) SelectedCallEffectBindings(root Root, mounted MountedCall, owner vocabulary.Operation) ([]AtomBinding, bool) {
	if !a.ownsRoot(root) {
		return nil, false
	}
	tail, _, tailOK := a.contract.Operations.EffectTail(owner)
	if !tailOK || (tail != vocabulary.RowClosed && tail != vocabulary.RowUnknownOpen) {
		return nil, false
	}
	count := a.contract.Operations.EffectCount(owner)
	callbackCount := a.contract.Operations.CallbackCount(owner)
	for callbackIndex := 0; callbackIndex < callbackCount; callbackIndex++ {
		callback, callbackOK := a.contract.Operations.CallbackAt(owner, callbackIndex)
		if !callbackOK {
			return nil, false
		}
		callbackTail, _, callbackTailOK := a.contract.Operations.CallbackEffectTail(callback)
		if !callbackTailOK || (callbackTail != vocabulary.RowClosed && callbackTail != vocabulary.RowUnknownOpen) {
			return nil, false
		}
		var added bool
		count, added = checkedIntAdd(count, a.contract.Operations.CallbackEffectCount(callback))
		if !added {
			return nil, false
		}
	}
	bindings := make([]AtomBinding, 0, count)
	for effect := 0; effect < a.contract.Operations.EffectCount(owner); effect++ {
		formal, formalOK := a.FormalCallEffectAtom(mounted, owner, effect)
		binding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
		if !formalOK || !bindingOK {
			return nil, false
		}
		bindings = append(bindings, binding)
	}
	for callbackIndex := 0; callbackIndex < callbackCount; callbackIndex++ {
		callback, callbackOK := a.contract.Operations.CallbackAt(owner, callbackIndex)
		if !callbackOK {
			return nil, false
		}
		for effect := 0; effect < a.contract.Operations.CallbackEffectCount(callback); effect++ {
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
	atom := valuecore.NewAtom(a, root.slot, formal.id)
	binding := AtomBinding{owner: a, formal: formal, root: root, atom: atom, sealed: true}
	return binding, binding.valid()
}

func (a *Algebra) formalCallRoot(mounted MountedCall, owner vocabulary.Operation) (pack.FormalCallRoot, static.TypeArgumentSequence, identity.ContentID, bool) {
	application, module, occurrence, ok := a.MountedCallIdentity(mounted)
	_, rootOK := a.RootForMountedCall(mounted)
	if !ok || !a.Valid() || !rootOK {
		return pack.FormalCallRoot{}, static.TypeArgumentSequence{}, identity.ContentID{}, false
	}
	if _, available := a.applicationOperation(application, owner); !available {
		return pack.FormalCallRoot{}, static.TypeArgumentSequence{}, identity.ContentID{}, false
	}
	formal, formalOK := a.packs.FormalCallRootForMountedSemantic(module, occurrence)
	types, typesOK := a.packs.TypeArgumentSequenceForMountedSemantic(module, occurrence)
	return formal, types, application, formalOK && typesOK
}

func (a *Algebra) ordinaryTypeFormalDescriptor(applicationID identity.ContentID, arguments static.TypeArgumentSequence, owner vocabulary.Operation, effect int) (identity.ContentID, bool) {
	count := a.contract.Operations.EffectTypeArgumentCount(owner, effect)
	positions := make([]vocabulary.TypeFormal, count)
	for index := range positions {
		formal, ok := a.contract.Operations.EffectTypeArgumentAt(owner, effect, index)
		if !ok {
			return identity.ContentID{}, false
		}
		positions[index] = formal
	}
	return a.selectedTypeFormalDescriptor(applicationID, arguments, owner, positions)
}

func (a *Algebra) callbackTypeFormalDescriptor(applicationID identity.ContentID, arguments static.TypeArgumentSequence, owner vocabulary.Operation, callback vocabulary.CallbackID, effect int) (identity.ContentID, bool) {
	count := a.contract.Operations.CallbackEffectTypeArgumentCount(callback, effect)
	positions := make([]vocabulary.TypeFormal, count)
	for index := range positions {
		formal, ok := a.contract.Operations.CallbackEffectTypeArgumentAt(callback, effect, index)
		if !ok {
			return identity.ContentID{}, false
		}
		positions[index] = formal
	}
	return a.selectedTypeFormalDescriptor(applicationID, arguments, owner, positions)
}

// selectedTypeFormalDescriptor uses Boundary only as the exact live
// call/Target-ABI admission proof. Reusable bytes contain Static's canonical
// semantic sequence identity plus Target formal positions; Boundary's
// Program-fenced correspondence ID and raw Terms never enter them.
func (a *Algebra) selectedTypeFormalDescriptor(applicationID identity.ContentID, formalArguments static.TypeArgumentSequence, owner vocabulary.Operation, positions []vocabulary.TypeFormal) (identity.ContentID, bool) {
	if len(positions) == 0 {
		return identity.ContentID{}, true
	}
	argumentCount, ok := a.applicationOperation(applicationID, owner)
	if !ok {
		return identity.ContentID{}, false
	}
	semanticTypes, typesOK := formalArguments.ContentID()
	if !typesOK || formalArguments.Count() != argumentCount {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(typeFormalDescriptorDomain))
	_, _ = hash.Write(semanticTypes[:])
	var word [4]byte
	binary.BigEndian.PutUint32(word[:], uint32(len(positions)))
	_, _ = hash.Write(word[:])
	for _, formal := range positions {
		if int(formal) < 0 || int(formal) >= argumentCount {
			return identity.ContentID{}, false
		}
		binary.BigEndian.PutUint32(word[:], uint32(formal))
		_, _ = hash.Write(word[:])
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func newFormalAtom(role formalAtomRole, call pack.FormalCallRoot, operation, descriptor, types identity.ContentID) (FormalAtom, bool) {
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

func formalAtomID(operation, descriptor, call, types identity.ContentID) identity.ContentID {
	if !operation.Available() || !descriptor.Available() || !call.Available() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(formalAtomDomain))
	_, _ = hash.Write(operation[:])
	_, _ = hash.Write(descriptor[:])
	_, _ = hash.Write(call[:])
	_, _ = hash.Write(types[:])
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
