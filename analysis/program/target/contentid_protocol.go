package target

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/internal/framing"
)

func encodeProtocol(w *framing.Writer, c *Contract, protocol Protocol) error {
	if err := w.Record(recordProtocol); err != nil {
		return err
	}
	if err := w.Uint(uint64(protocol)); err != nil {
		return err
	}
	states := c.StateCount(protocol)
	if err := w.Count(uint64(states)); err != nil {
		return err
	}
	for index := 0; index < states; index++ {
		state, ok := c.StateAt(protocol, index)
		if !ok {
			return errors.New("target: malformed protocol state")
		}
		final, found := c.StateFinal(protocol, state)
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
	acquisitions := c.ProtocolAcquisitionCount(protocol)
	if err := w.Count(uint64(acquisitions)); err != nil {
		return err
	}
	for index := 0; index < acquisitions; index++ {
		op, outcome, result, state, ok := c.ProtocolAcquisitionAt(protocol, index)
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
	transitions := c.TransitionCount(protocol)
	if err := w.Count(uint64(transitions)); err != nil {
		return err
	}
	for index := 0; index < transitions; index++ {
		op, kind, ordinal, from, ok := c.TransitionAt(protocol, index)
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
		outcomes := c.TransitionOutcomeCount(protocol, index)
		if err := w.Count(uint64(outcomes)); err != nil {
			return err
		}
		for outcomeIndex := 0; outcomeIndex < outcomes; outcomeIndex++ {
			outcome, to, found := c.TransitionOutcomeAt(protocol, index, outcomeIndex)
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
	escapes := c.EscapeCount(protocol)
	if err := w.Count(uint64(escapes)); err != nil {
		return err
	}
	for index := 0; index < escapes; index++ {
		op, kind, ordinal, ok := c.EscapeAt(protocol, index)
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
	holders := c.ProtocolCallbackHolderCount(protocol)
	if err := w.Count(uint64(holders)); err != nil {
		return err
	}
	for index := 0; index < holders; index++ {
		op, input, callback, ok := c.ProtocolCallbackHolderAt(protocol, index)
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
