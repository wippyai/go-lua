package target

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func (c *Contract) protocolCount() int {
	if c == nil {
		return 0
	}
	return len(c.protocols)
}

func (c *Contract) protocolAt(index int) (vocabulary.Protocol, bool) {
	if c == nil || index < 0 || index >= len(c.protocols) {
		return 0, false
	}
	return vocabulary.Protocol(index + 1), true
}

func (c *Contract) protocol(value vocabulary.Protocol) (protocolRow, bool) {
	if c == nil || value == 0 || uint64(value) > uint64(len(c.protocols)) {
		return protocolRow{}, false
	}
	return c.protocols[uint32(value)-1], true
}

func (c *Contract) protocolAcquisitionCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.acquisitions.len()
}

func (c *Contract) protocolAcquisitionAt(protocol vocabulary.Protocol, index int) (vocabulary.Operation, uint32, uint32, vocabulary.State, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.acquisitions.len() {
		return 0, 0, 0, 0, false
	}
	value := c.acquisitions[row.acquisitions.start+uint32(index)]
	return value.operation, value.outcome, value.result, value.state, true
}

func (c *Contract) stateCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.states.len()
}

func (c *Contract) stateAt(protocol vocabulary.Protocol, index int) (vocabulary.State, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.states.len() {
		return 0, false
	}
	return vocabulary.State(index + 1), true
}

func (c *Contract) state(protocol vocabulary.Protocol, state vocabulary.State) (stateRow, bool) {
	row, ok := c.protocol(protocol)
	if !ok || state == 0 || uint64(state) > uint64(row.states.len()) {
		return stateRow{}, false
	}
	return c.states[row.states.start+uint32(state)-1], true
}

func (c *Contract) stateName(protocol vocabulary.Protocol, state vocabulary.State) (string, bool) {
	row, ok := c.state(protocol, state)
	if !ok {
		return "", false
	}
	return row.name, true
}

func (c *Contract) stateFinal(protocol vocabulary.Protocol, state vocabulary.State) (bool, bool) {
	row, ok := c.state(protocol, state)
	if !ok {
		return false, false
	}
	return row.final, true
}

func (c *Contract) transitionCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.transitions.len()
}

func (c *Contract) transition(protocol vocabulary.Protocol, index int) (transitionRow, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.transitions.len() {
		return transitionRow{}, false
	}
	return c.transitions[row.transitions.start+uint32(index)], true
}

func (c *Contract) transitionAt(protocol vocabulary.Protocol, index int) (vocabulary.Operation, vocabulary.InputSourceKind, uint32, vocabulary.State, bool) {
	row, ok := c.transition(protocol, index)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.operation, row.input.Kind, row.input.Ordinal, row.from, true
}

func (c *Contract) transitionOutcomeCount(protocol vocabulary.Protocol, transition int) int {
	row, ok := c.transition(protocol, transition)
	if !ok {
		return 0
	}
	return row.outcomes.len()
}

func (c *Contract) transitionOutcomeAt(protocol vocabulary.Protocol, transition, index int) (uint32, vocabulary.State, bool) {
	row, ok := c.transition(protocol, transition)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	value := c.transitionOutcomes[row.outcomes.start+uint32(index)]
	return value.outcome, value.to, true
}

// derivedProtocolEscapes is the number of escapes the reader derives for every
// protocol: the opaque operation escaping on all inputs. It is derived rather
// than stored, so no escape row backs it.
const derivedProtocolEscapes = 1

func (c *Contract) escapeCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.escapes.len() + derivedProtocolEscapes
}

func (c *Contract) escapeAt(protocol vocabulary.Protocol, index int) (vocabulary.Operation, vocabulary.InputSourceKind, uint32, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.escapes.len()+derivedProtocolEscapes {
		return 0, 0, 0, false
	}
	if index == row.escapes.len() {
		return c.opaque, vocabulary.InputSourceAllInputs, 0, true
	}
	value := c.escapes[row.escapes.start+uint32(index)]
	return value.operation, value.input.Kind, value.input.Ordinal, true
}

// protocolCallbackHolderCount reports the complete authored retained-callback
// holder relation for one protocol. Unlike Escape, it has no opaque fallback:
// opaque behavior cannot fabricate a resource/callback correspondence.
func (c *Contract) protocolCallbackHolderCount(protocol vocabulary.Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.callbackHolders.len()
}

// protocolCallbackHolderAt returns one exact
// Protocol × Operation × InputSource × CallbackID declaration.
func (c *Contract) protocolCallbackHolderAt(protocol vocabulary.Protocol, index int) (vocabulary.Operation, vocabulary.InputSource, vocabulary.CallbackID, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.callbackHolders.len() {
		return 0, vocabulary.InputSource{}, 0, false
	}
	holder := c.callbackHolders[row.callbackHolders.start+uint32(index)]
	if holder.operation == 0 || holder.callback == 0 {
		return 0, vocabulary.InputSource{}, 0, false
	}
	return holder.operation, holder.input, holder.callback, true
}
