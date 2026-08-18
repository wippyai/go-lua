package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func (c *Contract) appendSuspensions(input []suspensionDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("suspension table", len(c.suspensions), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, suspension := range input {
		c.suspensions = append(c.suspensions, suspensionRow(suspension))
	}
	return rangeOut, nil
}

func (c *Contract) appendSpawns(owner vocabulary.Operation, input []spawnDraft, callbacks []vocabulary.CallbackID, outcomes []vocabulary.Values) (indexRange, error) {
	rangeOut, err := checkedStoredRange("spawn table", len(c.spawns), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, spawn := range input {
		if spawn.child == 0 || int(spawn.child) > len(callbacks) || int(spawn.childEntry) >= len(outcomes) || int(spawn.parentResume) >= len(outcomes) {
			return indexRange{}, errors.New("target: unresolved spawn")
		}
		c.spawns = append(c.spawns, spawnRow{
			owner: owner, function: spawn.function, child: callbacks[spawn.child-1],
			yield: spawn.yield, parentResume: spawn.parentResume,
			childEntry: outcomes[spawn.childEntry], resumeValues: outcomes[spawn.parentResume],
			alternatives: spawn.alternatives,
		})
	}
	return rangeOut, nil
}

func (c *Contract) appendResumes(owner vocabulary.Operation, input []resumeDraft, values map[string]vocabulary.Values) (indexRange, error) {
	rangeOut, err := checkedStoredRange("resume table", len(c.resumes), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, resume := range input {
		arguments, valuesErr := lookupDraftValues(values, resume.arguments)
		if valuesErr != nil {
			return indexRange{}, valuesErr
		}
		c.resumes = append(c.resumes, resumeRow{owner: owner, source: resume.source, carrier: resume.carrier, arguments: arguments, outcomes: resume.outcomes})
	}
	return rangeOut, nil
}

func (c *Contract) appendTransfers(owner vocabulary.Operation, input []transferDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("transfer table", len(c.transfers), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, transfer := range input {
		outcomes, outcomeErr := appendStoredRange(
			&c.transferOutcomes, transfer.outcomes, "transfer outcome table",
		)
		if outcomeErr != nil {
			return indexRange{}, outcomeErr
		}
		c.transfers = append(c.transfers, transferRow{
			owner: owner, endpoint: transfer.endpoint, payload: transfer.payload, alias: transfer.alias, identity: transfer.identity,
			capabilities: transfer.capabilities, outcomes: outcomes,
		})
	}
	return rangeOut, nil
}
