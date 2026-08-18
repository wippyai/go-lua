package target

import (
	"errors"
	"fmt"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"
)

func (d operationDraft) freezeSpawns(input []vocabulary.SpawnSpec) ([]spawnDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("spawn table", len(input)); err != nil {
		return nil, err
	}
	if len(input) > 1 {
		return nil, errors.New("target: operation has multiple spawn relations")
	}
	out := make([]spawnDraft, len(input))
	bySource := make([]outcomeDraft, len(d.outcomes))
	for _, outcome := range d.outcomes {
		bySource[outcome.source] = outcome
	}
	for index, item := range input {
		if item.Function.Kind != vocabulary.InputSourceValueFormal || uint64(item.Function.Ordinal) >= uint64(d.valueFormalCount()) {
			return nil, fmt.Errorf("target: spawn %d has invalid function source", index)
		}
		if item.Child == 0 || uint64(item.Child) > uint64(len(d.callbacks)) {
			return nil, fmt.Errorf("target: spawn %d child callback outside scope", index)
		}
		var child callbackDraft
		foundChild := false
		for _, candidate := range d.callbacks {
			if candidate.source == int(item.Child-1) {
				child, foundChild = candidate, true
				break
			}
		}
		if !foundChild || child.function != item.Function || child.lifecycle != vocabulary.CallbackRetainedRequiredOnce {
			return nil, fmt.Errorf("target: spawn %d child is not the exact detached callback", index)
		}
		if uint64(item.Yield) >= uint64(len(d.outcomes)) || uint64(item.ParentResume) >= uint64(len(d.outcomes)) || uint64(item.ChildEntry) >= uint64(len(d.outcomes)) {
			return nil, fmt.Errorf("target: spawn %d outcome outside scope", index)
		}
		if bySource[item.Yield].kind != flowkind.OutcomeYield || bySource[item.ParentResume].kind != flowkind.OutcomeNormal {
			return nil, fmt.Errorf("target: spawn %d has invalid parent yield/resume outcomes", index)
		}
		if !emptyClosedValues(bySource[item.ChildEntry].values) || !emptyClosedValues(bySource[item.ParentResume].values) || compareValues(bySource[item.ChildEntry].values, bySource[item.ParentResume].values) != 0 {
			return nil, fmt.Errorf("target: spawn %d child entry and parent resume must share the closed empty Pack", index)
		}
		if len(item.Alternatives) != 2 || item.Alternatives[0] == item.Alternatives[1] ||
			(item.Alternatives[0] != vocabulary.SpawnChildEntryThenParentResume && item.Alternatives[0] != vocabulary.SpawnParentResumeThenChildEntry) ||
			(item.Alternatives[1] != vocabulary.SpawnChildEntryThenParentResume && item.Alternatives[1] != vocabulary.SpawnParentResumeThenChildEntry) {
			return nil, fmt.Errorf("target: spawn %d has incomplete sibling alternatives", index)
		}
		alternatives := [2]vocabulary.SpawnSiblingAlternative{item.Alternatives[0], item.Alternatives[1]}
		if alternatives[1] < alternatives[0] {
			alternatives[0], alternatives[1] = alternatives[1], alternatives[0]
		}
		out[index] = spawnDraft{function: item.Function, child: item.Child, yield: item.Yield, parentResume: item.ParentResume, childEntry: item.ChildEntry, alternatives: alternatives}
	}
	return out, nil
}

func (d operationDraft) freezeSuspensions(input []vocabulary.SuspensionSpec) ([]suspensionDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("suspension table", len(input)); err != nil {
		return nil, err
	}
	if len(input) == 0 {
		return nil, nil
	}
	bySource := make([]outcomeDraft, len(d.outcomes))
	for _, outcome := range d.outcomes {
		bySource[outcome.source] = outcome
	}
	out := make([]suspensionDraft, len(input))
	for index, item := range input {
		if uint64(item.Yield) >= uint64(len(bySource)) || uint64(item.Reentry) >= uint64(len(bySource)) {
			return nil, fmt.Errorf("target: suspension %d has outcome outside scope", index)
		}
		if bySource[item.Yield].kind != flowkind.OutcomeYield {
			return nil, fmt.Errorf("target: suspension %d yield is not OutcomeYield", index)
		}
		switch bySource[item.Reentry].kind {
		case flowkind.OutcomeNormal, flowkind.OutcomeThrow, flowkind.OutcomeCancel:
		default:
			return nil, fmt.Errorf("target: suspension %d reentry is not restorable", index)
		}
		if item.Source != vocabulary.ReentryByCall && item.Source != vocabulary.ReentryByProvider {
			return nil, fmt.Errorf("target: suspension %d has invalid reentry source", index)
		}
		if item.Multiplicity != vocabulary.ReentryOnce && item.Multiplicity != vocabulary.ReentryMany {
			return nil, fmt.Errorf("target: suspension %d has invalid multiplicity", index)
		}
		out[index] = suspensionDraft{yield: item.Yield, reentry: item.Reentry, source: item.Source, multiplicity: item.Multiplicity}
	}
	return out, nil
}

func (d operationDraft) freezeResumes(input []vocabulary.ResumeSpec) ([]resumeDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("resume table", len(input)); err != nil {
		return nil, err
	}
	bySource := make([]outcomeDraft, len(d.outcomes))
	for _, outcome := range d.outcomes {
		bySource[outcome.source] = outcome
	}
	out := make([]resumeDraft, len(input))
	for index, item := range input {
		switch item.Source {
		case vocabulary.ResumeSourceValueFormal:
			if uint64(item.Carrier) >= uint64(d.valueFormalCount()) {
				return nil, fmt.Errorf("target: resume %d carrier outside scope", index)
			}
		case vocabulary.ResumeSourceProduced:
			if item.Carrier != 0 {
				return nil, fmt.Errorf("target: produced resume %d carries ValueFormal", index)
			}
		default:
			return nil, fmt.Errorf("target: resume %d has invalid source", index)
		}
		arguments, argumentsErr := d.freezeValues(item.Arguments, false)
		if argumentsErr != nil {
			return nil, fmt.Errorf("target: resume %d arguments: %w", index, argumentsErr)
		}
		// A resume payload is precisely the open tail of this invocation after
		// its fixed carrier operands. It cannot name an unrelated local Values
		// variable: closed and unknown inputs have no exact payload relation.
		if d.input.tail != vocabulary.ValuesVariable ||
			arguments.tail != vocabulary.ValuesVariable || arguments.varID != d.input.varID ||
			len(arguments.types) != 0 || len(arguments.suffix) != 0 {
			return nil, fmt.Errorf("target: resume %d arguments are not the input Values tail", index)
		}
		if len(item.Outcomes) != 5 {
			return nil, fmt.Errorf("target: resume %d has incomplete cross-activation outcomes", index)
		}
		var outcomes [5]uint32
		seen := [5]bool{}
		for outcomeIndex, outcome := range item.Outcomes {
			resumeKind, ok := vocabulary.CrossActivationOutcomeIndex(outcome.Kind)
			if !ok {
				return nil, fmt.Errorf("target: resume %d outcome %d has invalid cross-activation kind", index, outcomeIndex)
			}
			if seen[resumeKind] {
				return nil, fmt.Errorf("target: resume %d has duplicate cross-activation kind", index)
			}
			if uint64(outcome.Outcome) >= uint64(len(d.outcomes)) {
				return nil, fmt.Errorf("target: resume %d outcome %d outside scope", index, outcomeIndex)
			}
			// A restored outcome instantiates this existing tail, or a closed
			// outcome deliberately discards it. Unknown cannot express that exact
			// transport law and is therefore inadmissible at this boundary.
			if bySource[outcome.Outcome].values.tail == vocabulary.ValuesUnknown {
				return nil, fmt.Errorf("target: resume %d outcome %d has unknown Values tail", index, outcomeIndex)
			}
			seen[resumeKind] = true
			outcomes[resumeKind] = outcome.Outcome
		}
		for kind := range seen {
			if !seen[kind] {
				return nil, fmt.Errorf("target: resume %d has incomplete cross-activation outcomes", index)
			}
		}
		out[index] = resumeDraft{source: item.Source, carrier: item.Carrier, arguments: arguments, outcomes: outcomes}
	}
	return out, nil
}

func (d operationDraft) freezeTransfers(input []vocabulary.TransferSpec) ([]transferDraft, error) {
	if _, err := vocabulary.CheckedStoredLength("transfer table", len(input)); err != nil {
		return nil, err
	}
	out := make([]transferDraft, len(input))
	for index, item := range input {
		if !validTransferEndpoint(item.Endpoint, d.valueFormalCount()) {
			return nil, fmt.Errorf("target: transfer %d has invalid endpoint", index)
		}
		if !validTransferInputSource(item.Payload, d) {
			return nil, fmt.Errorf("target: transfer %d payload outside scope", index)
		}
		if !validTransferInputSource(item.Alias, d) {
			return nil, fmt.Errorf("target: transfer %d alias outside scope", index)
		}
		if !validTransferIdentity(item.Identity) {
			return nil, fmt.Errorf("target: transfer %d has invalid identity relation", index)
		}
		if !validTransferCapabilities(item.Capabilities) {
			return nil, fmt.Errorf("target: transfer %d has invalid capability relation", index)
		}
		if len(item.Outcomes) != len(d.outcomes) {
			return nil, fmt.Errorf("target: transfer %d has incomplete outcome authority", index)
		}
		if _, err := vocabulary.CheckedStoredLength("transfer outcome table", len(item.Outcomes)); err != nil {
			return nil, err
		}
		bySource := make([]vocabulary.TransferPossibility, len(d.outcomes))
		seen := make([]bool, len(d.outcomes))
		for outcomeIndex, outcome := range item.Outcomes {
			if uint64(outcome.Outcome) >= uint64(len(d.outcomes)) {
				return nil, fmt.Errorf("target: transfer %d outcome %d outside scope", index, outcomeIndex)
			}
			if seen[outcome.Outcome] {
				return nil, fmt.Errorf("target: transfer %d has duplicate outcome authority", index)
			}
			if !validTransferPossibility(outcome.Possibility) {
				return nil, fmt.Errorf("target: transfer %d outcome %d has invalid possibility", index, outcomeIndex)
			}
			seen[outcome.Outcome] = true
			bySource[outcome.Outcome] = outcome.Possibility
		}
		canonical := make([]vocabulary.TransferPossibility, len(d.outcomes))
		for canonicalOutcome, outcome := range d.outcomes {
			canonical[canonicalOutcome] = bySource[outcome.source]
		}
		out[index] = transferDraft{
			endpoint: item.Endpoint, payload: item.Payload, alias: item.Alias, identity: item.Identity,
			capabilities: item.Capabilities, outcomes: canonical,
		}
	}
	sort.Slice(out, func(left, right int) bool {
		return compareTransfer(out[left], out[right]) < 0
	})
	for index := 1; index < len(out); index++ {
		if compareTransferIdentity(out[index-1], out[index]) == 0 {
			return nil, errors.New("target: duplicate transfer endpoint/payload/alias")
		}
	}
	return out, nil
}
