package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/internal/framing"
)

func encodeProtocol(w *framing.Writer, c *Contract, protocol vocabulary.Protocol) error {
	if err := w.Record(recordProtocol); err != nil {
		return err
	}
	if err := w.Uint(uint64(protocol)); err != nil {
		return err
	}
	states := c.stateCount(protocol)
	if err := w.Count(uint64(states)); err != nil {
		return err
	}
	for index := 0; index < states; index++ {
		state, ok := c.stateAt(protocol, index)
		if !ok {
			return errors.New("target: malformed protocol state")
		}
		final, found := c.stateFinal(protocol, state)
		if !found {
			return errors.New("target: malformed state finality")
		}
		if err := w.Record(recordState); err != nil {
			return err
		}
		if err := w.Bool(final); err != nil {
			return err
		}
	}
	acquisitions := c.protocolAcquisitionCount(protocol)
	if err := w.Count(uint64(acquisitions)); err != nil {
		return err
	}
	for index := 0; index < acquisitions; index++ {
		op, outcome, result, state, ok := c.protocolAcquisitionAt(protocol, index)
		if !ok {
			return errors.New("target: malformed acquisition")
		}
		if err := w.Record(recordAcquisition); err != nil {
			return err
		}
		if err := w.Uint(uint64(op)); err != nil {
			return err
		}
		if err := w.Uint(uint64(outcome)); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Uint(uint64(state)); err != nil {
			return err
		}
	}
	transitions := c.transitionCount(protocol)
	if err := w.Count(uint64(transitions)); err != nil {
		return err
	}
	for index := 0; index < transitions; index++ {
		op, kind, ordinal, from, ok := c.transitionAt(protocol, index)
		if !ok {
			return errors.New("target: malformed transition")
		}
		if err := w.Record(recordTransition); err != nil {
			return err
		}
		if err := w.Uint(uint64(op)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(kind), uint64(ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(from)); err != nil {
			return err
		}
		outcomes := c.transitionOutcomeCount(protocol, index)
		if err := w.Count(uint64(outcomes)); err != nil {
			return err
		}
		for outcomeIndex := 0; outcomeIndex < outcomes; outcomeIndex++ {
			outcome, to, found := c.transitionOutcomeAt(protocol, index, outcomeIndex)
			if !found {
				return errors.New("target: malformed transition outcome")
			}
			if err := w.Record(recordTransitionOutcome); err != nil {
				return err
			}
			if err := w.Uint(uint64(outcome)); err != nil {
				return err
			}
			if err := w.Uint(uint64(to)); err != nil {
				return err
			}
		}
	}
	escapes := c.escapeCount(protocol)
	if err := w.Count(uint64(escapes)); err != nil {
		return err
	}
	for index := 0; index < escapes; index++ {
		op, kind, ordinal, ok := c.escapeAt(protocol, index)
		if !ok {
			return errors.New("target: malformed escape")
		}
		if err := w.Record(recordEscape); err != nil {
			return err
		}
		if err := w.Uint(uint64(op)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(kind), uint64(ordinal)); err != nil {
			return err
		}
	}
	holders := c.protocolCallbackHolderCount(protocol)
	if err := w.Count(uint64(holders)); err != nil {
		return err
	}
	for index := 0; index < holders; index++ {
		op, input, callback, ok := c.protocolCallbackHolderAt(protocol, index)
		if !ok {
			return errors.New("target: malformed protocol callback holder")
		}
		if err := w.Record(recordProtocolCallbackHolder); err != nil {
			return err
		}
		if err := w.Uint(uint64(op)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(input.Kind), uint64(input.Ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(callback)); err != nil {
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
