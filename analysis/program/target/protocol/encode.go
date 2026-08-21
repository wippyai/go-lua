package protocol

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/internal/framing"
)

const (
	recordProtocol               uint64 = 18
	recordState                  uint64 = 19
	recordAcquisition            uint64 = 20
	recordTransition             uint64 = 21
	recordTransitionOutcome      uint64 = 22
	recordEscape                 uint64 = 23
	recordProtocolCallbackHolder uint64 = 24
)

// Encode writes the protocol owner's canonical identity contribution. Record
// values are part of target-contract/v1 and remain stable while the storage
// owner moves behind this sealed value boundary.
func (c *Table) Encode(w *framing.Writer) error {
	if c == nil {
		return errors.New("target/protocol: unavailable table")
	}
	for index := 0; index < c.ProtocolCount(); index++ {
		protocol, ok := c.ProtocolAt(index)
		if !ok {
			return errors.New("target/protocol: malformed protocol table")
		}
		if err := encodeProtocol(w, c, protocol); err != nil {
			return err
		}
	}
	return nil
}

func encodeProtocol(w *framing.Writer, c *Table, protocol vocabulary.Protocol) error {
	protocolRow, protocolOK := c.protocol(protocol)
	if !protocolOK {
		return errors.New("target/protocol: malformed protocol table")
	}
	if err := w.Record(recordProtocol); err != nil {
		return err
	}
	if err := w.Uint(uint64(protocol)); err != nil {
		return err
	}
	states := c.states.Count(protocolRow.states)
	if err := w.Count(uint64(states)); err != nil {
		return err
	}
	for index := 0; index < states; index++ {
		state, found := c.states.At(protocolRow.states, index)
		if !found {
			return errors.New("target: malformed protocol state")
		}
		if err := w.Record(recordState); err != nil {
			return err
		}
		if err := w.Bool(state.final); err != nil {
			return err
		}
	}
	acquisitions := c.acquisitions.Count(protocolRow.acquisitions)
	if err := w.Count(uint64(acquisitions)); err != nil {
		return err
	}
	for index := 0; index < acquisitions; index++ {
		acquisition, found := c.acquisitions.At(protocolRow.acquisitions, index)
		if !found {
			return errors.New("target: malformed acquisition")
		}
		if err := w.Record(recordAcquisition); err != nil {
			return err
		}
		if err := w.Uint(uint64(acquisition.operation)); err != nil {
			return err
		}
		if err := w.Uint(uint64(acquisition.outcome)); err != nil {
			return err
		}
		if err := w.Uint(uint64(acquisition.result)); err != nil {
			return err
		}
		if err := w.Uint(uint64(acquisition.state)); err != nil {
			return err
		}
	}
	transitions := c.transitions.Count(protocolRow.transitions)
	if err := w.Count(uint64(transitions)); err != nil {
		return err
	}
	for index := 0; index < transitions; index++ {
		transition, found := c.transitions.At(protocolRow.transitions, index)
		if !found {
			return errors.New("target: malformed transition")
		}
		if err := w.Record(recordTransition); err != nil {
			return err
		}
		if err := w.Uint(uint64(transition.operation)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(transition.input.Kind), uint64(transition.input.Ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(transition.from)); err != nil {
			return err
		}
		outcomes := c.transitionOutcomes.Count(transition.outcomes)
		if err := w.Count(uint64(outcomes)); err != nil {
			return err
		}
		for outcomeIndex := 0; outcomeIndex < outcomes; outcomeIndex++ {
			outcome, outcomeFound := c.transitionOutcomes.At(transition.outcomes, outcomeIndex)
			if !outcomeFound {
				return errors.New("target: malformed transition outcome")
			}
			if err := w.Record(recordTransitionOutcome); err != nil {
				return err
			}
			if err := w.Uint(uint64(outcome.outcome)); err != nil {
				return err
			}
			if err := w.Uint(uint64(outcome.to)); err != nil {
				return err
			}
		}
	}
	authoredEscapes := c.escapes.Count(protocolRow.escapes)
	escapes := authoredEscapes + derivedProtocolEscapes
	if err := w.Count(uint64(escapes)); err != nil {
		return err
	}
	for index := 0; index < escapes; index++ {
		var op vocabulary.Operation
		var input vocabulary.InputSource
		if index == authoredEscapes {
			op, input = c.opaque, vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}
		} else {
			escape, found := c.escapes.At(protocolRow.escapes, index)
			if !found {
				return errors.New("target: malformed escape")
			}
			op, input = escape.operation, escape.input
		}
		if err := w.Record(recordEscape); err != nil {
			return err
		}
		if err := w.Uint(uint64(op)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(input.Kind), uint64(input.Ordinal)); err != nil {
			return err
		}
	}
	holders := c.callbackHolders.Count(protocolRow.callbackHolders)
	if err := w.Count(uint64(holders)); err != nil {
		return err
	}
	for index := 0; index < holders; index++ {
		holder, found := c.callbackHolders.At(protocolRow.callbackHolders, index)
		if !found || holder.operation == 0 || holder.callback == 0 {
			return errors.New("target: malformed protocol callback holder")
		}
		if err := w.Record(recordProtocolCallbackHolder); err != nil {
			return err
		}
		if err := w.Uint(uint64(holder.operation)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(holder.input.Kind), uint64(holder.input.Ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(holder.callback)); err != nil {
			return err
		}
	}
	return nil
}

func encodeCoordinate(w *framing.Writer, kind, ordinal uint64) error {
	if err := w.Uint(kind); err != nil {
		return err
	}
	return w.Uint(ordinal)
}
