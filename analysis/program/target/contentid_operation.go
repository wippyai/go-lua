package target

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func encodeOperation(w *framing.Writer, c *Contract, op Operation) error {
	if err := w.Record(recordOperation); err != nil {
		return err
	}
	if err := w.Uint(uint64(op)); err != nil {
		return err
	}

	bindings := c.BindingCount(op)
	if err := w.Count(uint64(bindings)); err != nil {
		return err
	}
	for index := 0; index < bindings; index++ {
		if err := w.Record(recordBinding); err != nil {
			return err
		}
		namespace, ok := c.BindingNamespaceAt(op, index)
		if !ok {
			return errors.New("target: malformed binding")
		}
		if err := w.Uint(uint64(namespace)); err != nil {
			return err
		}
		if err := encodeBindingSegments(w, c, op, index, true); err != nil {
			return err
		}
		if err := encodeBindingSegments(w, c, op, index, false); err != nil {
			return err
		}
	}

	formals := c.TypeFormalCount(op)
	if err := w.Count(uint64(formals)); err != nil {
		return err
	}
	for index := 0; index < formals; index++ {
		constraint, found := c.TypeFormalConstraint(op, TypeFormal(index))
		if err := w.Bool(found); err != nil {
			return err
		}
		if found {
			if err := encodeType(w, c, constraint); err != nil {
				return err
			}
		}
	}
	if err := w.Uint(uint64(c.ValuesVarCount(op))); err != nil {
		return err
	}
	for variable := 0; variable < c.ValuesVarCount(op); variable++ {
		class, found := c.ValuesVarType(op, ValuesVar(variable))
		if !found {
			return errors.New("target: malformed Values variable type")
		}
		if err := encodeType(w, c, class); err != nil {
			return err
		}
	}
	if err := w.Uint(uint64(c.RowFormalCount(op))); err != nil {
		return err
	}
	input, ok := c.Input(op)
	if !ok {
		return errors.New("target: malformed input Values")
	}
	if err := encodeValues(w, c, input); err != nil {
		return err
	}

	callbacks := c.CallbackCount(op)
	if err := w.Count(uint64(callbacks)); err != nil {
		return err
	}
	for index := 0; index < callbacks; index++ {
		id, found := c.CallbackAt(op, index)
		if !found {
			return errors.New("target: malformed callback")
		}
		owner, found := c.CallbackOwner(id)
		if !found || owner != op {
			return errors.New("target: malformed callback owner")
		}
		if err := w.Record(recordCallback); err != nil {
			return err
		}
		if err := w.Uint(uint64(id)); err != nil {
			return err
		}
		source, found := c.CallbackFunction(id)
		if !found {
			return errors.New("target: malformed callback source")
		}
		if err := encodeCoordinate(w, uint64(source.Kind), uint64(source.Ordinal)); err != nil {
			return err
		}
		arguments, found := c.CallbackArguments(id)
		if !found {
			return errors.New("target: malformed callback arguments")
		}
		if err := encodeValues(w, c, arguments); err != nil {
			return err
		}
		admission, found := c.CallbackAdmission(id)
		if !found || !admission.Available() {
			return errors.New("target: malformed callback admission")
		}
		if err := w.Uint(uint64(admission)); err != nil {
			return err
		}
		for _, kind := range [...]flowkind.OutcomeKind{
			flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
			flowkind.OutcomeYield, flowkind.OutcomeCancel,
		} {
			values, found := c.CallbackOutcome(id, kind)
			if !found {
				return errors.New("target: malformed callback outcome")
			}
			if err := w.Uint(uint64(kind)); err != nil {
				return err
			}
			if err := encodeValues(w, c, values); err != nil {
				return err
			}
		}
		lifecycle, found := c.CallbackLifecycle(id)
		if !found {
			return errors.New("target: malformed callback lifecycle")
		}
		if err := w.Uint(uint64(lifecycle)); err != nil {
			return err
		}
		tail, variable, found := c.CallbackEffectTail(id)
		if !found {
			return errors.New("target: malformed callback effect tail")
		}
		if err := w.Uint(uint64(tail)); err != nil {
			return err
		}
		if err := w.Uint(uint64(variable)); err != nil {
			return err
		}
		effects := c.CallbackEffectCount(id)
		if err := w.Count(uint64(effects)); err != nil {
			return err
		}
		for effect := 0; effect < effects; effect++ {
			row, ok := c.callbackEffect(id, effect)
			if !ok {
				return errors.New("target: malformed callback effect")
			}
			if err := encodeEffectRow(w, c, row); err != nil {
				return err
			}
		}
		releaseOperation, releaseInput, releaseOutcome, releaseMode, hasRelease := c.CallbackRelease(id)
		if err := w.Bool(hasRelease); err != nil {
			return err
		}
		if hasRelease {
			if err := w.Record(recordCallbackRelease); err != nil {
				return err
			}
			if err := w.Uint(uint64(releaseOperation)); err != nil {
				return err
			}
			if err := w.Uint(uint64(releaseInput)); err != nil {
				return err
			}
			if err := w.Uint(uint64(releaseOutcome)); err != nil {
				return err
			}
			if err := w.Uint(uint64(releaseMode)); err != nil {
				return err
			}
			zeroBehavior, zeroOutcome, zeroOK := c.CallbackReleaseZero(id)
			if !zeroOK || !validCallbackReleaseZeroBehavior(zeroBehavior) {
				return errors.New("target: malformed callback release zero behavior")
			}
			if err := w.Uint(uint64(zeroBehavior)); err != nil {
				return err
			}
			switch zeroBehavior {
			case CallbackReleaseZeroThrow, CallbackReleaseZeroIdempotent:
				if err := w.Uint(uint64(zeroOutcome)); err != nil {
					return err
				}
			case CallbackReleaseZeroSuppress:
				if zeroOutcome != 0 {
					return errors.New("target: suppressed callback release retained an outcome")
				}
			default:
				return errors.New("target: malformed callback release zero behavior")
			}
		}
	}

	subedges := c.SubedgeCount(op)
	if err := w.Count(uint64(subedges)); err != nil {
		return err
	}
	for index := 0; index < subedges; index++ {
		edge, found := c.SubedgeAt(op, index)
		if !found {
			return errors.New("target: malformed subedge")
		}
		if err := encodeSubedge(w, c, op, edge); err != nil {
			return err
		}
	}

	outcomes := c.OutcomeCount(op)
	if err := w.Count(uint64(outcomes)); err != nil {
		return err
	}
	for index := 0; index < outcomes; index++ {
		if err := encodeOutcome(w, c, op, index); err != nil {
			return err
		}
	}

	suspensions := c.SuspensionCount(op)
	if err := w.Count(uint64(suspensions)); err != nil {
		return err
	}
	for index := 0; index < suspensions; index++ {
		yield, reentry, source, multiplicity, found := c.SuspensionAt(op, index)
		if !found {
			return errors.New("target: malformed suspension")
		}
		if err := w.Record(recordSuspension); err != nil {
			return err
		}
		if err := w.Uint(uint64(yield)); err != nil {
			return err
		}
		if err := w.Uint(uint64(reentry)); err != nil {
			return err
		}
		if err := w.Uint(uint64(source)); err != nil {
			return err
		}
		if err := w.Uint(uint64(multiplicity)); err != nil {
			return err
		}
	}
	spawns := c.SpawnCount(op)
	if err := w.Count(uint64(spawns)); err != nil {
		return err
	}
	for index := 0; index < spawns; index++ {
		spawn, found := c.SpawnIDAt(op, index)
		if !found {
			return errors.New("target: malformed spawn")
		}
		owner, function, child, yield, resume, entry, resumeValues, found := c.Spawn(spawn)
		if !found || owner != op {
			return errors.New("target: malformed spawn")
		}
		if err := w.Record(recordSpawn); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(function.Kind), uint64(function.Ordinal)); err != nil {
			return err
		}
		for _, value := range []uint64{uint64(child), uint64(yield), uint64(resume), uint64(entry), uint64(resumeValues)} {
			if err := w.Uint(value); err != nil {
				return err
			}
		}
		alternatives := c.SpawnSiblingCount(spawn)
		if err := w.Count(uint64(alternatives)); err != nil {
			return err
		}
		for sibling := 0; sibling < alternatives; sibling++ {
			alternative, found := c.SpawnSiblingAt(spawn, sibling)
			if !found {
				return errors.New("target: malformed spawn sibling")
			}
			if err := w.Uint(uint64(alternative)); err != nil {
				return err
			}
		}
	}
	resumes := c.ResumeCount(op)
	if err := w.Count(uint64(resumes)); err != nil {
		return err
	}
	for index := 0; index < resumes; index++ {
		resume, found := c.ResumeIDAt(op, index)
		if !found {
			return errors.New("target: malformed resume")
		}
		owner, source, carrier, arguments, found := c.Resume(resume)
		if !found || owner != op {
			return errors.New("target: malformed resume")
		}
		if err := w.Record(recordResume); err != nil {
			return err
		}
		if err := w.Uint(uint64(source)); err != nil {
			return err
		}
		if err := w.Uint(uint64(carrier)); err != nil {
			return err
		}
		if err := encodeValues(w, c, arguments); err != nil {
			return err
		}
		outcomes := c.ResumeOutcomeCount(resume)
		if err := w.Count(uint64(outcomes)); err != nil {
			return err
		}
		for outcome := 0; outcome < outcomes; outcome++ {
			kind, targetOutcome, found := c.ResumeOutcomeAt(resume, outcome)
			if !found {
				return errors.New("target: malformed resume outcome")
			}
			if err := w.Uint(uint64(kind)); err != nil {
				return err
			}
			if err := w.Uint(uint64(targetOutcome)); err != nil {
				return err
			}
		}
	}

	transfers := c.TransferCount(op)
	if err := w.Count(uint64(transfers)); err != nil {
		return err
	}
	for index := 0; index < transfers; index++ {
		endpoint, found := c.TransferEndpointAt(op, index)
		if !found {
			return errors.New("target: malformed transfer")
		}
		if err := w.Record(recordTransfer); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(endpoint.Kind), uint64(endpoint.Input)); err != nil {
			return err
		}
		payload, found := c.TransferPayloadAt(op, index)
		if !found {
			return errors.New("target: malformed transfer payload")
		}
		if err := encodeCoordinate(w, uint64(payload.Kind), uint64(payload.Ordinal)); err != nil {
			return err
		}
		alias, found := c.TransferAliasAt(op, index)
		if !found {
			return errors.New("target: malformed transfer alias")
		}
		if err := encodeCoordinate(w, uint64(alias.Kind), uint64(alias.Ordinal)); err != nil {
			return err
		}
		identity, found := c.TransferIdentityAt(op, index)
		if !found {
			return errors.New("target: malformed transfer identity")
		}
		if err := w.Uint(uint64(identity)); err != nil {
			return err
		}
		capabilities, found := c.TransferCapabilitiesAt(op, index)
		if !found {
			return errors.New("target: malformed transfer capabilities")
		}
		if err := w.Uint(uint64(capabilities)); err != nil {
			return err
		}
		count := c.TransferOutcomeCount(op, index)
		if err := w.Count(uint64(count)); err != nil {
			return err
		}
		for item := 0; item < count; item++ {
			outcome, possibility, found := c.TransferOutcomeAt(op, index, item)
			if !found {
				return errors.New("target: malformed transfer outcome")
			}
			if err := w.Uint(uint64(outcome)); err != nil {
				return err
			}
			if err := w.Uint(uint64(possibility)); err != nil {
				return err
			}
		}
	}

	tail, variable, found := c.EffectTail(op)
	if !found {
		return errors.New("target: malformed effect tail")
	}
	if err := w.Uint(uint64(tail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(variable)); err != nil {
		return err
	}
	effects := c.EffectCount(op)
	if err := w.Count(uint64(effects)); err != nil {
		return err
	}
	for index := 0; index < effects; index++ {
		if err := encodeEffect(w, c, op, index); err != nil {
			return err
		}
	}
	if replacement, key, access, resultOutcome, result, present := c.GsubTableReplacement(op); present {
		if err := w.Bool(true); err != nil {
			return err
		}
		if err := w.Record(recordGsubTableReplacement); err != nil {
			return err
		}
		for _, value := range []uint64{uint64(replacement), uint64(key), uint64(resultOutcome), uint64(result)} {
			if err := w.Uint(value); err != nil {
				return err
			}
		}
		role, found := c.SubedgeRole(access)
		if !found || role == 0 {
			return errors.New("target: malformed gsub table access")
		}
		if err := w.Uint(uint64(role)); err != nil {
			return err
		}
		aliases := c.GsubTableReplacementEffectAliasCount(op)
		if err := w.Count(uint64(aliases)); err != nil {
			return err
		}
		for index := 0; index < aliases; index++ {
			effect, found := c.GsubTableReplacementEffectAliasAt(op, index)
			if !found {
				return errors.New("target: malformed gsub table effect alias")
			}
			if err := w.Uint(uint64(effect)); err != nil {
				return err
			}
		}
	} else if err := w.Bool(false); err != nil {
		return err
	}
	return nil
}
