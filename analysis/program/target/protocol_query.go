package target

func (c *Contract) ProtocolCount() int {
	if c == nil {
		return 0
	}
	return len(c.protocols)
}

func (c *Contract) ProtocolAt(index int) (Protocol, bool) {
	if c == nil || index < 0 || index >= len(c.protocols) {
		return 0, false
	}
	return Protocol(index + 1), true
}

func (c *Contract) protocol(value Protocol) (protocolRow, bool) {
	if c == nil || value == 0 || uint64(value) > uint64(len(c.protocols)) {
		return protocolRow{}, false
	}
	return c.protocols[uint32(value)-1], true
}

func (c *Contract) ProtocolAcquisitionCount(protocol Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.acquisitions.len()
}

func (c *Contract) ProtocolAcquisitionAt(protocol Protocol, index int) (Operation, uint32, uint32, State, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.acquisitions.len() {
		return 0, 0, 0, 0, false
	}
	value := c.acquisitions[row.acquisitions.start+uint32(index)]
	return value.operation, value.outcome, value.result, value.state, true
}

func (c *Contract) StateCount(protocol Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.states.len()
}

func (c *Contract) StateAt(protocol Protocol, index int) (State, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.states.len() {
		return 0, false
	}
	return State(index + 1), true
}

func (c *Contract) state(protocol Protocol, state State) (stateRow, bool) {
	row, ok := c.protocol(protocol)
	if !ok || state == 0 || uint64(state) > uint64(row.states.len()) {
		return stateRow{}, false
	}
	return c.states[row.states.start+uint32(state)-1], true
}

func (c *Contract) StateName(protocol Protocol, state State) (string, bool) {
	row, ok := c.state(protocol, state)
	if !ok {
		return "", false
	}
	return row.name, true
}

func (c *Contract) StateFinal(protocol Protocol, state State) (bool, bool) {
	row, ok := c.state(protocol, state)
	if !ok {
		return false, false
	}
	return row.final, true
}

func (c *Contract) TransitionCount(protocol Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.transitions.len()
}

func (c *Contract) transition(protocol Protocol, index int) (transitionRow, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.transitions.len() {
		return transitionRow{}, false
	}
	return c.transitions[row.transitions.start+uint32(index)], true
}

func (c *Contract) TransitionAt(protocol Protocol, index int) (Operation, InputSourceKind, uint32, State, bool) {
	row, ok := c.transition(protocol, index)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return row.operation, row.input.Kind, row.input.Ordinal, row.from, true
}

func (c *Contract) TransitionOutcomeCount(protocol Protocol, transition int) int {
	row, ok := c.transition(protocol, transition)
	if !ok {
		return 0
	}
	return row.outcomes.len()
}

func (c *Contract) TransitionOutcomeAt(protocol Protocol, transition, index int) (uint32, State, bool) {
	row, ok := c.transition(protocol, transition)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, 0, false
	}
	value := c.transitionOutcomes[row.outcomes.start+uint32(index)]
	return value.outcome, value.to, true
}

func (c *Contract) EscapeCount(protocol Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.escapes.len() + 1
}

func (c *Contract) EscapeAt(protocol Protocol, index int) (Operation, InputSourceKind, uint32, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.escapes.len()+1 {
		return 0, 0, 0, false
	}
	if index == row.escapes.len() {
		return c.opaque, InputSourceAllInputs, 0, true
	}
	value := c.escapes[row.escapes.start+uint32(index)]
	return value.operation, value.input.Kind, value.input.Ordinal, true
}

// ProtocolCallbackHolderCount reports the complete authored retained-callback
// holder relation for one protocol. Unlike Escape, it has no opaque fallback:
// opaque behavior cannot fabricate a resource/callback correspondence.
func (c *Contract) ProtocolCallbackHolderCount(protocol Protocol) int {
	row, ok := c.protocol(protocol)
	if !ok {
		return 0
	}
	return row.callbackHolders.len()
}

// ProtocolCallbackHolderAt returns one exact
// Protocol × Operation × InputSource × CallbackID declaration.
func (c *Contract) ProtocolCallbackHolderAt(protocol Protocol, index int) (Operation, InputSource, CallbackID, bool) {
	row, ok := c.protocol(protocol)
	if !ok || index < 0 || index >= row.callbackHolders.len() {
		return 0, InputSource{}, 0, false
	}
	holder := c.callbackHolders[row.callbackHolders.start+uint32(index)]
	if holder.operation == 0 || holder.callback == 0 {
		return 0, InputSource{}, 0, false
	}
	return holder.operation, holder.input, holder.callback, true
}
