package protocol

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	sealedrows "github.com/wippyai/go-lua/internal/rows"
)

// Table is the sealed protocol value composed into target.Contract. Its row
// storage remains private to this owner and is sealed through the shared rows
// substrate.
type Table struct {
	protocols          sealedrows.Rows[protocolRow]
	states             sealedrows.Pool[stateRow]
	acquisitions       sealedrows.Pool[acquisitionRow]
	transitions        sealedrows.Pool[transitionRow]
	transitionOutcomes sealedrows.Pool[transitionOutcomeRow]
	escapes            sealedrows.Pool[escapeRow]
	callbackHolders    sealedrows.Pool[protocolCallbackHolderRow]
	opaque             vocabulary.Operation
}

type protocolRow struct {
	acquisitions    sealedrows.Span
	states          sealedrows.Span
	transitions     sealedrows.Span
	escapes         sealedrows.Span
	callbackHolders sealedrows.Span
}

type stateRow struct {
	name  string
	final bool
}

type acquisitionRow struct {
	operation vocabulary.Operation
	outcome   uint32
	result    uint32
	state     vocabulary.State
}

type transitionRow struct {
	operation vocabulary.Operation
	input     vocabulary.InputSource
	from      vocabulary.State
	outcomes  sealedrows.Span
}

type transitionOutcomeRow struct {
	outcome uint32
	to      vocabulary.State
}

type escapeRow struct {
	operation vocabulary.Operation
	input     vocabulary.InputSource
}

type protocolCallbackHolderRow struct {
	operation vocabulary.Operation
	input     vocabulary.InputSource
	callback  vocabulary.CallbackID
}

func (c *Table) appendProtocols(input []protocolDraft) error {
	if _, err := vocabulary.CheckedStoredLength("protocol table", len(input)); err != nil {
		return err
	}
	var (
		statePool             sealedrows.PoolBuilder[stateRow]
		acquisitionPool       sealedrows.PoolBuilder[acquisitionRow]
		transitionPool        sealedrows.PoolBuilder[transitionRow]
		transitionOutcomePool sealedrows.PoolBuilder[transitionOutcomeRow]
		escapePool            sealedrows.PoolBuilder[escapeRow]
		callbackHolderPool    sealedrows.PoolBuilder[protocolCallbackHolderRow]
	)
	protocols := make([]protocolRow, 0, len(input))
	for index := range input {
		draft := &input[index]
		row := protocolRow{}
		var err error
		row.states, err = appendPool(&statePool, draft.states, "protocol state table")
		if err != nil {
			return err
		}
		acquisitions := make([]acquisitionRow, len(draft.acquisitions))
		for i, item := range draft.acquisitions {
			acquisitions[i] = acquisitionRow{operation: item.operation, outcome: item.outcome, result: item.result, state: item.state}
		}
		row.acquisitions, err = appendPool(&acquisitionPool, acquisitions, "protocol acquisition table")
		if err != nil {
			return err
		}
		row.transitions, err = appendProtocolTransitions(&transitionPool, &transitionOutcomePool, draft.transitions)
		if err != nil {
			return err
		}
		escapes := make([]escapeRow, len(draft.escapes))
		for itemIndex, item := range draft.escapes {
			escapes[itemIndex] = escapeRow{operation: item.operation, input: item.input}
		}
		row.escapes, err = appendPool(&escapePool, escapes, "protocol escape table")
		if err != nil {
			return err
		}
		holders := make([]protocolCallbackHolderRow, len(draft.callbackHolders))
		for itemIndex, item := range draft.callbackHolders {
			if item.operation == 0 || item.callback == 0 {
				return errors.New("target: unresolved protocol callback holder")
			}
			holders[itemIndex] = protocolCallbackHolderRow{operation: item.operation, input: item.input, callback: item.callback}
		}
		row.callbackHolders, err = appendPool(&callbackHolderPool, holders, "protocol callback-holder table")
		if err != nil {
			return err
		}
		protocols = append(protocols, row)
	}
	c.protocols = sealedrows.NewRows(protocols)
	c.states = statePool.Seal()
	c.acquisitions = acquisitionPool.Seal()
	c.transitions = transitionPool.Seal()
	c.transitionOutcomes = transitionOutcomePool.Seal()
	c.escapes = escapePool.Seal()
	c.callbackHolders = callbackHolderPool.Seal()
	return nil
}

func appendProtocolTransitions(transitions *sealedrows.PoolBuilder[transitionRow], outcomePool *sealedrows.PoolBuilder[transitionOutcomeRow], input []transitionDraft) (sealedrows.Span, error) {
	transitionRows := make([]transitionRow, len(input))
	for index, item := range input {
		outcomes := make([]transitionOutcomeRow, len(item.outcomes))
		for i, outcome := range item.outcomes {
			outcomes[i] = transitionOutcomeRow{outcome: outcome.outcome, to: outcome.to}
		}
		rangeItems, appendErr := appendPool(outcomePool, outcomes, "protocol transition outcome table")
		if appendErr != nil {
			return sealedrows.Span{}, appendErr
		}
		transitionRows[index] = transitionRow{operation: item.operation, input: item.input, from: item.from, outcomes: rangeItems}
	}
	return appendPool(transitions, transitionRows, "protocol transition table")
}

// appendPool appends one contiguous relation segment to a shared sealed pool.
// The builder owns the resulting storage and the returned Span is the only
// coordinate a reader can use to select that segment.
func appendPool[Element any](builder *sealedrows.PoolBuilder[Element], input []Element, what string) (sealedrows.Span, error) {
	start := builder.Len()
	if _, err := vocabulary.CheckedStoredLength(what+" start", start); err != nil {
		return sealedrows.Span{}, err
	}
	if _, err := vocabulary.CheckedStoredLength(what+" length", len(input)); err != nil {
		return sealedrows.Span{}, err
	}
	if _, err := vocabulary.CheckedStoredTotal(what, start, len(input)); err != nil {
		return sealedrows.Span{}, err
	}
	span, ok := builder.Append(input)
	if !ok {
		return sealedrows.Span{}, fmt.Errorf("target/protocol: %s representation overflow", what)
	}
	return span, nil
}
