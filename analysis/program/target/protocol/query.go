package protocol

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// Counts is the protocol owner's complete denominator contribution. The root
// target contract publishes these values under schema-owned IDs; no root walk
// of protocol rows is needed.
type Counts struct {
	Protocols          int
	States             int
	Acquisitions       int
	Transitions        int
	TransitionOutcomes int
	Escapes            int
	CallbackHolders    int
}

func (c *Table) Counts() Counts {
	if c == nil {
		return Counts{}
	}
	return Counts{
		Protocols:          c.protocols.Count(),
		States:             c.states.Len(),
		Acquisitions:       c.acquisitions.Len(),
		Transitions:        c.transitions.Len(),
		TransitionOutcomes: c.transitionOutcomes.Len(),
		Escapes:            c.escapes.Len() + c.protocols.Count(),
		CallbackHolders:    c.callbackHolders.Len(),
	}
}

func (c *Table) ProtocolCount() int {
	if c == nil {
		return 0
	}
	return c.protocols.Count()
}

func (c *Table) ProtocolAt(index int) (vocabulary.Protocol, bool) {
	if c == nil || index < 0 {
		return 0, false
	}
	if _, ok := c.protocols.At(index); !ok {
		return 0, false
	}
	return vocabulary.Protocol(index + 1), true
}

func (c *Table) protocol(value vocabulary.Protocol) (protocolRow, bool) {
	if c == nil || value == 0 || uint64(value) > uint64(c.protocols.Count()) {
		return protocolRow{}, false
	}
	return c.protocols.At(int(value) - 1)
}

func (c *Table) ProtocolAcquisitionCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return c.acquisitions.Count(row.acquisitions)
}

func (c *Table) ProtocolAcquisitionAt(protocol vocabulary.Protocol, index int) (vocabulary.Operation, uint32, uint32, vocabulary.State, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 {
		return 0, 0, 0, 0, false
	}
	value, found := c.acquisitions.At(row.acquisitions, index)
	if !found {
		return 0, 0, 0, 0, false
	}
	return value.operation, value.outcome, value.result, value.state, true
}

func (c *Table) StateCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return c.states.Count(row.states)
}

func (c *Table) StateAt(protocol vocabulary.Protocol, index int) (vocabulary.State, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 {
		return 0, false
	}
	if _, found := c.states.At(row.states, index); !found {
		return 0, false
	}
	return vocabulary.State(index + 1), true
}

func (c *Table) state(protocol vocabulary.Protocol, state vocabulary.State) (stateRow, bool) {
	row, ok := c.protocol(protocol)
	if !ok || state == 0 || uint64(state) > uint64(c.states.Count(row.states)) {
		return stateRow{}, false
	}
	return c.states.At(row.states, int(state)-1)
}

func (c *Table) StateName(protocol vocabulary.Protocol, state vocabulary.State) (string, bool) {
	row, ok := c.state(protocol, state)
	if !ok {
		return "", false
	}
	return row.name, true
}

func (c *Table) StateFinal(protocol vocabulary.Protocol, state vocabulary.State) (bool, bool) {
	row, ok := c.state(protocol, state)
	if !ok {
		return false, false
	}
	return row.final, true
}

func (c *Table) TransitionCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return c.transitions.Count(row.transitions)
}

func (c *Table) transition(protocol vocabulary.Protocol, index int) (transitionRow, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 {
		return transitionRow{}, false
	}
	return c.transitions.At(row.transitions, index)
}

func (c *Table) TransitionAt(protocol vocabulary.Protocol, index int) (vocabulary.Operation, vocabulary.InputSourceKind, uint32, vocabulary.State, bool) {
	row, ok := c.transition(protocol, index)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.operation, row.input.Kind, row.input.Ordinal, row.from, true
}

func (c *Table) TransitionOutcomeCount(protocol vocabulary.Protocol, transition int) int {
	row, ok := c.transition(protocol, transition)
	if !ok {
		return 0
	}
	return c.transitionOutcomes.Count(row.outcomes)
}

func (c *Table) TransitionOutcomeAt(protocol vocabulary.Protocol, transition, index int) (uint32, vocabulary.State, bool) {
	row, ok := c.transition(protocol, transition)
	if !ok || index < 0 {
		return 0, 0, false
	}
	value, found := c.transitionOutcomes.At(row.outcomes, index)
	if !found {
		return 0, 0, false
	}
	return value.outcome, value.to, true
}

// derivedProtocolEscapes is the number of escapes the reader derives for every
// protocol: the opaque operation escaping on all inputs. It is derived rather
// than stored, so no escape row backs it.
const derivedProtocolEscapes = 1

func (c *Table) EscapeCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return c.escapes.Count(row.escapes) + derivedProtocolEscapes
}

func (c *Table) EscapeAt(protocol vocabulary.Protocol, index int) (vocabulary.Operation, vocabulary.InputSourceKind, uint32, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 {
		return 0, 0, 0, false
	}
	escapes := c.escapes.Count(row.escapes)
	if index >= escapes+derivedProtocolEscapes {
		return 0, 0, 0, false
	}
	if index == escapes {
		return c.opaque, vocabulary.InputSourceAllInputs, 0, true
	}
	value, found := c.escapes.At(row.escapes, index)
	if !found {
		return 0, 0, 0, false
	}
	return value.operation, value.input.Kind, value.input.Ordinal, true
}

// protocolCallbackHolderCount reports the complete authored retained-callback
// holder relation for one protocol. Unlike Escape, it has no opaque fallback:
// opaque behavior cannot fabricate a resource/callback correspondence.
func (c *Table) ProtocolCallbackHolderCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return c.callbackHolders.Count(row.callbackHolders)
}

// protocolCallbackHolderAt returns one exact
// Protocol × Operation × InputSource × CallbackID declaration.
func (c *Table) ProtocolCallbackHolderAt(protocol vocabulary.Protocol, index int) (vocabulary.Operation, vocabulary.InputSource, vocabulary.CallbackID, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 {
		return 0, vocabulary.InputSource{}, 0, false
	}
	holder, found := c.callbackHolders.At(row.callbackHolders, index)
	if !found {
		return 0, vocabulary.InputSource{}, 0, false
	}
	if holder.operation == 0 || holder.callback == 0 {
		return 0, vocabulary.InputSource{}, 0, false
	}
	return holder.operation, holder.input, holder.callback, true
}
