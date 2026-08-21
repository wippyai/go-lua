package factor

import (
	"github.com/wippyai/go-lua/analysis/identity"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	packtransfer "github.com/wippyai/go-lua/domain/pack/transfer"
)

// resolvePublicationInputs is the shared ordinary/callback Effect resolver
// for an authored publication descriptor.  Subject is target-owned and may
// select either a fixed ValueFormal or the target operation's input ValuesVar;
// the resolver maps that coordinate through the exact invocation argument
// vector before issuing the caller-owned Pack MountedInput.  A ValuesVar is
// admitted only when it is the target's input tail: outcome-scoped ValuesVars
// have no caller-owned Pack source and must fail closed.
func (a *Algebra) resolvePublicationInputs(owner vocabulary.Operation, callback vocabulary.CallbackID, effect int, descriptor operationvalue.PublicationEffectDescriptor, module, occurrence identity.ContentID) (subject, context packtransfer.MountedInput, hasContext, ok bool) {
	if a == nil || !a.Valid() || !descriptor.Valid() || !module.Available() || !occurrence.Available() || owner == 0 || effect < 0 {
		return packtransfer.MountedInput{}, packtransfer.MountedInput{}, false, false
	}
	var target vocabulary.Operation
	if callback == 0 {
		if effect >= a.contract.Operations.EffectCount(owner) {
			return packtransfer.MountedInput{}, packtransfer.MountedInput{}, false, false
		}
		var targetOK bool
		target, targetOK = a.contract.Operations.EffectTarget(owner, effect)
		if !targetOK {
			return packtransfer.MountedInput{}, packtransfer.MountedInput{}, false, false
		}
	} else {
		callbackOwner, ownerOK := a.contract.Operations.CallbackOwner(callback)
		if !ownerOK || callbackOwner != owner || effect >= a.contract.Operations.CallbackEffectCount(callback) {
			return packtransfer.MountedInput{}, packtransfer.MountedInput{}, false, false
		}
		var targetOK bool
		target, targetOK = a.contract.Operations.CallbackEffectTarget(callback, effect)
		if !targetOK {
			return packtransfer.MountedInput{}, packtransfer.MountedInput{}, false, false
		}
	}

	subject, subjectOK := a.resolvePublicationInput(owner, callback, effect, target, descriptor.Subject(), module, occurrence)
	if !subjectOK {
		return packtransfer.MountedInput{}, packtransfer.MountedInput{}, false, false
	}
	switch descriptor.DestinationRole() {
	case vocabulary.PublicationDestinationNone:
		return subject, packtransfer.MountedInput{}, false, true
	case vocabulary.PublicationDestinationValueFormal:
		var contextOK bool
		context, contextOK = a.resolvePublicationInput(owner, callback, effect, target,
			vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(descriptor.Context())}, module, occurrence)
		if !contextOK {
			return packtransfer.MountedInput{}, packtransfer.MountedInput{}, false, false
		}
		return subject, context, true, true
	default:
		return packtransfer.MountedInput{}, packtransfer.MountedInput{}, false, false
	}
}

func (a *Algebra) resolvePublicationInput(owner vocabulary.Operation, callback vocabulary.CallbackID, effect int, target vocabulary.Operation, source vocabulary.InputSource, module, occurrence identity.ContentID) (packtransfer.MountedInput, bool) {
	if target == 0 || source.Kind != vocabulary.InputSourceValueFormal && source.Kind != vocabulary.InputSourceValuesVar {
		return packtransfer.MountedInput{}, false
	}
	var mountedSource vocabulary.InputSource
	switch source.Kind {
	case vocabulary.InputSourceValueFormal:
		var formal vocabulary.ValueFormal
		var sourceOK bool
		if callback == 0 {
			formal, sourceOK = a.contract.Operations.EffectValueArgumentAt(owner, effect, int(source.Ordinal))
		} else {
			formal, sourceOK = a.contract.Operations.CallbackEffectValueArgumentAt(callback, effect, int(source.Ordinal))
		}
		if !sourceOK {
			return packtransfer.MountedInput{}, false
		}
		mountedSource = vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(formal)}
	case vocabulary.InputSourceValuesVar:
		input, inputOK := a.contract.Operations.Input(target)
		tail, variable, tailOK := a.contract.Operations.ValuesTail(input)
		if !inputOK || !tailOK || tail != vocabulary.ValuesVariable || variable != vocabulary.ValuesVar(source.Ordinal) {
			return packtransfer.MountedInput{}, false
		}
		var valuesVar vocabulary.ValuesVar
		var sourceOK bool
		if callback == 0 {
			valuesVar, sourceOK = a.contract.Operations.EffectValuesArgumentAt(owner, effect, int(source.Ordinal))
		} else {
			valuesVar, sourceOK = a.contract.Operations.CallbackEffectValuesArgumentAt(callback, effect, int(source.Ordinal))
		}
		if !sourceOK {
			return packtransfer.MountedInput{}, false
		}
		mountedSource = vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: uint32(valuesVar)}
	}
	return packtransfer.NewMountedInput(a.packs, module, occurrence, owner, mountedSource)
}
