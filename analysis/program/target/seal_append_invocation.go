package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"
)

func (c *Contract) appendCallbacks(owner vocabulary.Operation, input []callbackDraft, values map[string]vocabulary.Values, issued []vocabulary.CallbackID) ([]vocabulary.CallbackID, indexRange, error) {
	rangeOut, err := checkedStoredRange("callback table", len(c.callbacks), len(input))
	if err != nil {
		return nil, indexRange{}, err
	}
	if len(issued) != len(input) {
		return nil, indexRange{}, errors.New("target: operation callback geometry mismatch")
	}
	ids := append([]vocabulary.CallbackID(nil), issued...)
	for index := range input {
		callback := &input[index]
		effects, effectErr := c.appendEffects(effectOwnerCallback, callback.effects.effects)
		if effectErr != nil {
			return nil, indexRange{}, effectErr
		}
		id := issued[index]
		if id == 0 {
			return nil, indexRange{}, errors.New("target: operation callback geometry has zero handle")
		}
		ids[callback.source] = id
		callback.sealed = id
		arguments, valuesErr := lookupDraftValues(values, callback.arguments)
		if valuesErr != nil {
			return nil, indexRange{}, valuesErr
		}
		var outcomes [5]vocabulary.Values
		for terminal := range callback.outcomes {
			value, terminalErr := lookupDraftValues(values, callback.outcomes[terminal])
			if terminalErr != nil {
				return nil, indexRange{}, terminalErr
			}
			outcomes[terminal] = value
		}
		c.callbacks = append(c.callbacks, callbackRow{
			function: callback.function, admission: callback.admission,
			arguments: arguments, outcomes: outcomes,
			effects: effects, effectTail: callback.effects.tail, effectVar: callback.effects.variable,
		})
	}
	return ids, rangeOut, nil
}

func (c *Contract) appendEffects(owner effectOwner, input []effectDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("effect table", len(c.effects), len(input))
	if err != nil {
		return indexRange{}, err
	}
	valueArgs, err := totalEffectArguments(input, func(effect effectDraft) int { return len(effect.values) }, "effect value argument pool")
	if err != nil {
		return indexRange{}, err
	}
	typeArgs, err := totalEffectArguments(input, func(effect effectDraft) int { return len(effect.types) }, "effect type argument pool")
	if err != nil {
		return indexRange{}, err
	}
	valuesArgs, err := totalEffectArguments(input, func(effect effectDraft) int { return len(effect.valuesVar) }, "effect Values argument pool")
	if err != nil {
		return indexRange{}, err
	}
	rowArgs, err := totalEffectArguments(input, func(effect effectDraft) int { return len(effect.rows) }, "effect row argument pool")
	if err != nil {
		return indexRange{}, err
	}
	if _, err := checkedStoredRange("effect value argument pool", len(c.effectVals), valueArgs); err != nil {
		return indexRange{}, err
	}
	if _, err := checkedStoredRange("effect type argument pool", len(c.effectType), typeArgs); err != nil {
		return indexRange{}, err
	}
	if _, err := checkedStoredRange("effect Values argument pool", len(c.effectVars), valuesArgs); err != nil {
		return indexRange{}, err
	}
	if _, err := checkedStoredRange("effect row argument pool", len(c.effectRows), rowArgs); err != nil {
		return indexRange{}, err
	}
	for _, effect := range input {
		row := effectRow{owner: owner, target: effect.target}
		if effect.hasPublication {
			descriptor, publicationErr := freezePublicationEffect(effect.publication)
			if publicationErr != nil {
				return indexRange{}, publicationErr
			}
			row.publication, row.hasPublication = descriptor, true
		}
		if row.values, err = appendStoredRange(&c.effectVals, effect.values, "effect value argument pool"); err != nil {
			return indexRange{}, err
		}
		if row.types, err = appendStoredRange(&c.effectType, effect.types, "effect type argument pool"); err != nil {
			return indexRange{}, err
		}
		if row.valuesVar, err = appendStoredRange(&c.effectVars, effect.valuesVar, "effect Values argument pool"); err != nil {
			return indexRange{}, err
		}
		if row.rows, err = appendStoredRange(&c.effectRows, effect.rows, "effect row argument pool"); err != nil {
			return indexRange{}, err
		}
		c.effects = append(c.effects, row)
	}
	return rangeOut, nil
}

func (c *Contract) appendCallbackReleases(drafts []operationDraft) error {
	pending := make([][]callbackReleaseRow, len(c.operations))
	for draftIndex := range drafts {
		for callbackIndex := range drafts[draftIndex].callbacks {
			callback := drafts[draftIndex].callbacks[callbackIndex]
			if callback.release == nil {
				continue
			}
			if callback.sealed == 0 || callback.release.operation == 0 ||
				uint64(callback.release.operation) > uint64(len(pending)) {
				return errors.New("target: unresolved callback release")
			}
			pending[uint32(callback.release.operation)-1] = append(
				pending[uint32(callback.release.operation)-1],
				callbackReleaseRow{
					callback: callback.sealed, operation: callback.release.operation,
					input: callback.release.input, outcome: callback.release.outcome,
					mode: callback.release.mode, zeroBehavior: callback.release.zeroBehavior,
					zeroOutcome: callback.release.zeroOutcome,
				},
			)
		}
	}
	for operation := range pending {
		releases := pending[operation]
		if len(releases) == 0 {
			continue
		}
		sort.Slice(releases, func(left, right int) bool {
			return compareCallbackRelease(releases[left], releases[right]) < 0
		})
		for index := 1; index < len(releases); index++ {
			if releases[index-1].callback == releases[index].callback {
				return errors.New("target: callback has duplicate release")
			}
		}
		rangeOut, err := checkedStoredRange("callback release table", len(c.callbackReleases), len(releases))
		if err != nil {
			return err
		}
		c.operations[operation].releases = rangeOut
		for _, release := range releases {
			handle, handleErr := checkedStoredHandle("callback release table", len(c.callbackReleases))
			if handleErr != nil {
				return handleErr
			}
			c.callbackReleases = append(c.callbackReleases, release)
			c.callbacks[uint32(release.callback)-1].release = handle
		}
	}
	return nil
}

func (c *Contract) appendSubedges(owner vocabulary.Operation, input []subedgeDraft, callbacks []vocabulary.CallbackID, values map[string]vocabulary.Values, keys exactkey.Table) (indexRange, error) {
	rangeOut, err := checkedStoredRange("subedge table", len(c.subedges), len(input))
	if err != nil {
		return indexRange{}, err
	}
	ids := make([]vocabulary.SubedgeID, len(input))
	for index := range input {
		if input[index].source < 0 || input[index].source >= len(ids) {
			return indexRange{}, errors.New("target: malformed subedge source")
		}
		handle, handleErr := checkedStoredHandle("subedge table", len(c.subedges)+index)
		if handleErr != nil {
			return indexRange{}, handleErr
		}
		ids[input[index].source] = vocabulary.SubedgeID(handle)
		input[index].sealed = vocabulary.SubedgeID(handle)
	}
	for index := range input {
		edge := input[index]
		arguments, valuesErr := lookupDraftValues(values, edge.arguments)
		if valuesErr != nil {
			return indexRange{}, valuesErr
		}
		row := subedgeRow{
			owner: owner, role: edge.role, family: edge.family, callee: edge.callee,
			admission: edge.admission, arguments: arguments, ruleEntry: edge.ruleEntry,
		}
		origins, originsErr := c.appendSubedgeArgumentOrigins(edge.argumentOrigins)
		if originsErr != nil {
			return indexRange{}, originsErr
		}
		row.argumentOrigins = origins
		if edge.callee == vocabulary.SubedgeCalleeCallback {
			if edge.callback == 0 || int(edge.callback) > len(callbacks) || callbacks[edge.callback-1] == 0 {
				return indexRange{}, errors.New("target: unresolved callback subedge callee")
			}
			row.callback = callbacks[edge.callback-1]
			callbackIndex := uint32(row.callback) - 1
			if c.callbacks[callbackIndex].subedge != 0 {
				return indexRange{}, errors.New("target: callback has multiple direct subedges")
			}
			c.callbacks[callbackIndex].subedge = ids[edge.source]
		}
		if edge.callee == vocabulary.SubedgeCalleeCapturedInitialRead {
			if edge.readRootID == 0 {
				return indexRange{}, errors.New("target: unresolved captured initial read root")
			}
			key, keyErr := exactKeyHandle(keys, edge.readKey)
			if keyErr != nil {
				return indexRange{}, keyErr
			}
			row.readRoot, row.readKey = edge.readRootID, key
		}
		if edge.callee == vocabulary.SubedgeCalleeMetaKey {
			key, keyErr := exactKeyHandle(keys, edge.metaKey)
			if keyErr != nil {
				return indexRange{}, keyErr
			}
			row.metaKey = key
		}
		for terminal := range edge.outcomes {
			value, terminalErr := lookupDraftValues(values, edge.outcomes[terminal])
			if terminalErr != nil {
				return indexRange{}, terminalErr
			}
			row.outcomes[terminal] = value
		}
		failure, failureErr := lookupDraftValues(values, edge.admissionFailure)
		if failureErr != nil {
			return indexRange{}, failureErr
		}
		row.admissionFailure = failure
		admissionRoute, admissionRouteErr := c.appendSubedgeRoute(edge.admissionRoute, ids, values)
		if admissionRouteErr != nil {
			return indexRange{}, admissionRouteErr
		}
		row.admissionRoute = admissionRoute
		for terminal, route := range edge.routes {
			item, itemErr := c.appendSubedgeRoute(route, ids, values)
			if itemErr != nil {
				return indexRange{}, itemErr
			}
			row.routes[terminal] = item
		}
		c.subedges = append(c.subedges, row)
	}
	return rangeOut, nil
}

func (c *Contract) appendSubedgeArgumentOrigins(input []subedgeArgumentOriginDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("subedge argument origin table", len(c.subedgeOrigins), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, origin := range input {
		c.subedgeOrigins = append(c.subedgeOrigins, subedgeArgumentOriginRow(origin))
	}
	return rangeOut, nil
}

func compareCallbackRelease(left, right callbackReleaseRow) int {
	if left.callback < right.callback {
		return -1
	}
	if left.callback > right.callback {
		return 1
	}
	if left.input < right.input {
		return -1
	}
	if left.input > right.input {
		return 1
	}
	if left.outcome < right.outcome {
		return -1
	}
	if left.outcome > right.outcome {
		return 1
	}
	if left.mode < right.mode {
		return -1
	}
	if left.mode > right.mode {
		return 1
	}
	if left.zeroBehavior < right.zeroBehavior {
		return -1
	}
	if left.zeroBehavior > right.zeroBehavior {
		return 1
	}
	if left.zeroOutcome < right.zeroOutcome {
		return -1
	}
	if left.zeroOutcome > right.zeroOutcome {
		return 1
	}
	return 0
}
