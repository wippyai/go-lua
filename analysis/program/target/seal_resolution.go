package target

import (
	"errors"
	"fmt"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"
)

func (d *operationDraft) resolveEffects(all []operationDraft, sourceOperation []vocabulary.Operation) error {
	if err := d.resolveEffectList(d.effects, all, sourceOperation, "effect"); err != nil {
		return err
	}
	for index := range d.callbacks {
		if err := d.resolveEffectList(d.callbacks[index].effects.effects, all, sourceOperation, "callback effect"); err != nil {
			return fmt.Errorf("target: callback %d: %w", index, err)
		}
	}
	return nil
}

func (d *operationDraft) resolveEffectList(effects []effectDraft, all []operationDraft, sourceOperation []vocabulary.Operation, label string) error {
	for index := range effects {
		targetRef := effects[index].targetSource
		if targetRef == 0 || uint64(targetRef) > uint64(len(sourceOperation)) {
			return fmt.Errorf("target: %s %d references unknown operation", label, index)
		}
		targetOp := sourceOperation[uint32(targetRef)-1]
		if targetOp == 0 {
			return fmt.Errorf("target: %s %d references unresolved operation", label, index)
		}
		target := &all[uint32(targetOp)-1]
		if len(effects[index].values) != target.valueFormalCount() ||
			len(effects[index].types) != len(target.formals) ||
			uint64(len(effects[index].valuesVar)) != uint64(target.valuesVars) ||
			uint64(len(effects[index].rows)) != uint64(target.rowFormals) {
			return fmt.Errorf("target: %s %d does not match target ABI", label, index)
		}
		effects[index].target = targetOp
	}
	sort.Slice(effects, func(left, right int) bool { return compareEffect(effects[left], effects[right]) < 0 })
	return nil
}

func (d *operationDraft) resolveCallbackReleases(all []operationDraft, sourceOperation []vocabulary.Operation) error {
	for index := range d.callbacks {
		release := d.callbacks[index].release
		if release == nil {
			continue
		}
		if release.operationSource == 0 || uint64(release.operationSource) > uint64(len(sourceOperation)) {
			return fmt.Errorf("target: callback %d release references unknown operation", index)
		}
		targetOp := sourceOperation[uint32(release.operationSource)-1]
		if targetOp == 0 {
			return fmt.Errorf("target: callback %d release references unresolved operation", index)
		}
		target := &all[uint32(targetOp)-1]
		if len(target.bindings) == 0 {
			return fmt.Errorf("target: callback %d release operation is not source-visible", index)
		}
		if uint64(release.input) >= uint64(target.valueFormalCount()) {
			return fmt.Errorf("target: callback %d release input outside operation scope", index)
		}
		canonical, found := canonicalOutcomeForSource(target.outcomes, release.outcome)
		if !found {
			return fmt.Errorf("target: callback %d release outcome outside operation scope", index)
		}
		release.operation, release.outcome = targetOp, canonical
		switch release.zeroBehavior {
		case vocabulary.CallbackReleaseZeroSuppress:
			// No outcome coordinate is retained for a suppressed zero-holder
			// release. The frozen form stores only the behavior tag.
			release.zeroOutcome = 0
		case vocabulary.CallbackReleaseZeroThrow, vocabulary.CallbackReleaseZeroIdempotent:
			zeroOutcome, zeroFound := canonicalOutcomeForSource(target.outcomes, release.zeroOutcome)
			if !zeroFound {
				return fmt.Errorf("target: callback %d zero release outcome outside operation scope", index)
			}
			kind := target.outcomes[zeroOutcome].kind
			if release.zeroBehavior == vocabulary.CallbackReleaseZeroThrow && kind != flowkind.OutcomeThrow {
				return fmt.Errorf("target: callback %d zero release throw outcome is not Throw", index)
			}
			if release.zeroBehavior == vocabulary.CallbackReleaseZeroIdempotent && kind != flowkind.OutcomeNormal {
				return fmt.Errorf("target: callback %d zero release idempotent outcome is not Normal", index)
			}
			release.zeroOutcome = zeroOutcome
		default:
			return fmt.Errorf("target: callback %d release has invalid zero behavior", index)
		}
	}
	return nil
}

func canonicalOutcomeForSource(outcomes []outcomeDraft, source uint32) (uint32, bool) {
	for index := range outcomes {
		if uint32(outcomes[index].source) == source {
			return uint32(index), true
		}
	}
	return 0, false
}

func (d *operationDraft) resolveProduced(sourceOperation []vocabulary.Operation) error {
	for outcomeIndex := range d.outcomes {
		for producedIndex := range d.outcomes[outcomeIndex].produced {
			produced := &d.outcomes[outcomeIndex].produced[producedIndex]
			if produced.targetSource == 0 || uint64(produced.targetSource) > uint64(len(sourceOperation)) {
				return fmt.Errorf("target: produced operation %d references unknown operation", producedIndex)
			}
			produced.target = sourceOperation[uint32(produced.targetSource)-1]
			if produced.target == 0 {
				return fmt.Errorf("target: produced operation %d references unresolved operation", producedIndex)
			}
		}
	}
	return nil
}

func (d *operationDraft) resolveSuspensions() error {
	canonical := make([]uint32, len(d.outcomes))
	for index, outcome := range d.outcomes {
		canonical[outcome.source] = uint32(index)
	}
	if len(d.suspensions) != 0 {
		for index := range d.suspensions {
			row := &d.suspensions[index]
			row.yield = canonical[row.yield]
			row.reentry = canonical[row.reentry]
		}
		sort.Slice(d.suspensions, func(left, right int) bool {
			return compareSuspension(d.suspensions[left], d.suspensions[right]) < 0
		})
		for index := 1; index < len(d.suspensions); index++ {
			if compareSuspensionKey(d.suspensions[index-1], d.suspensions[index]) == 0 {
				return errors.New("target: duplicate suspension")
			}
		}
	}
	if err := d.resolveSpawns(canonical); err != nil {
		return err
	}
	for index := range d.resumes {
		for outcome := range d.resumes[index].outcomes {
			d.resumes[index].outcomes[outcome] = canonical[d.resumes[index].outcomes[outcome]]
		}
	}
	sort.Slice(d.resumes, func(left, right int) bool {
		return compareResume(d.resumes[left], d.resumes[right]) < 0
	})
	for index := 1; index < len(d.resumes); index++ {
		if compareResume(d.resumes[index-1], d.resumes[index]) == 0 {
			return errors.New("target: duplicate resume")
		}
	}
	return nil
}

func (d *operationDraft) resolveSpawns(canonical []uint32) error {
	for index := range d.spawns {
		spawn := &d.spawns[index]
		spawn.yield = canonical[spawn.yield]
		spawn.parentResume = canonical[spawn.parentResume]
		spawn.childEntry = canonical[spawn.childEntry]
		matched := false
		for _, suspension := range d.suspensions {
			if suspension.yield == spawn.yield && suspension.reentry == spawn.parentResume && suspension.source == vocabulary.ReentryByProvider && suspension.multiplicity == vocabulary.ReentryOnce {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("target: spawn %d lacks exact one-shot provider suspension", index)
		}
	}
	return nil
}

func validateProducedResumes(drafts []operationDraft, sourceOperation []vocabulary.Operation) error {
	callbackCapture := make([]bool, len(drafts)+1)
	for producer := range drafts {
		for outcome := range drafts[producer].outcomes {
			for _, produced := range drafts[producer].outcomes[outcome].produced {
				if produced.target == 0 || uint64(produced.target) >= uint64(len(callbackCapture)) {
					return errors.New("target: produced resume has unresolved target")
				}
				for _, capture := range produced.captures {
					if capture.Kind == vocabulary.CaptureCallback {
						callbackCapture[produced.target] = true
						break
					}
				}
			}
		}
	}
	for index := range drafts {
		draft := &drafts[index]
		needsProduced := false
		for _, resume := range draft.resumes {
			needsProduced = needsProduced || resume.source == vocabulary.ResumeSourceProduced
		}
		if !needsProduced {
			continue
		}
		if len(draft.bindings) != 0 {
			return errors.New("target: produced resume has source binding")
		}
		op := sourceOperation[draft.source]
		if op == 0 {
			return errors.New("target: produced resume has unresolved operation")
		}
		if uint64(op) >= uint64(len(callbackCapture)) || !callbackCapture[op] {
			return errors.New("target: produced resume lacks callback capture")
		}
	}
	return nil
}

// validateSpawnAuthority keeps detached spawning an explicitly selected
// contract authority.  A profile may omit it, but a sealed contract can never
// admit two independently schedulable spawn rows.
func validateSpawnAuthority(drafts []operationDraft) error {
	count := 0
	for index := range drafts {
		count += len(drafts[index].spawns)
	}
	if count > 1 {
		return errors.New("target: duplicate spawn authority")
	}
	return nil
}
