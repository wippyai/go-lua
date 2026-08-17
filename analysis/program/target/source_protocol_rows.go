package target

import "github.com/wippyai/go-lua/analysis/program/semanticsource"

func (c *Contract) buildProtocolSourceRows(views *SourceViews) bool {
	if c == nil || views == nil {
		return false
	}
	var ok bool
	views.protocol, ok = sourceRows(c, tokenTarget(semanticsource.OriginTargetProtocol, 0), func(emitter *targetRowEmitter) {
		for index := 0; index < c.ProtocolCount(); index++ {
			protocol, found := c.ProtocolAt(index)
			emitter.row(func(writer *sourceRowWriter) {
				writer.u64(uint64(protocol))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return false
	}
	views.protocolState, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.StateCount(protocol); index++ {
			state, found := c.StateAt(protocol, index)
			name, nameOK := c.StateName(protocol, state)
			final, finalOK := c.StateFinal(protocol, state)
			emitter.row(func(writer *sourceRowWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(state))
				writer.text(name)
				writer.u64(boolWord(final))
				if !found || !nameOK || !finalOK {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return false
	}
	views.protocolAcquisition, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolAcquisition), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.ProtocolAcquisitionCount(protocol); index++ {
			op, outcome, result, state, found := c.ProtocolAcquisitionAt(protocol, index)
			emitter.row(func(writer *sourceRowWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(op))
				writer.u64(uint64(outcome))
				writer.u64(uint64(result))
				writer.u64(uint64(state))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return false
	}
	views.protocolTransition, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransition), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.TransitionCount(protocol); index++ {
			op, input, ordinal, from, found := c.TransitionAt(protocol, index)
			emitter.row(func(writer *sourceRowWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(op))
				writer.u64(uint64(input))
				writer.u64(uint64(ordinal))
				writer.u64(uint64(from))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return false
	}
	views.protocolTransitionOutcome, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransitionOutcome), func(emitter *targetRowEmitter, protocol Protocol) {
		for transition := 0; transition < c.TransitionCount(protocol); transition++ {
			for index := 0; index < c.TransitionOutcomeCount(protocol, transition); index++ {
				emitter.row(func(writer *sourceRowWriter) {
					outcome, state, found := c.TransitionOutcomeAt(protocol, transition, index)
					writer.u64(uint64(protocol))
					writer.u64(uint64(transition))
					writer.u64(uint64(index))
					writer.u64(uint64(outcome))
					writer.u64(uint64(state))
					if !found {
						writer.valid = false
					}
				})
			}
		}
	})
	if !ok {
		return false
	}
	views.protocolEscape, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolEscape), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.EscapeCount(protocol); index++ {
			op, input, ordinal, found := c.EscapeAt(protocol, index)
			emitter.row(func(writer *sourceRowWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(op))
				writer.u64(uint64(input))
				writer.u64(uint64(ordinal))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return false
	}
	views.protocolCallbackHolder, ok = c.protocolRows(tokenTarget(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolCallbackHolder), func(emitter *targetRowEmitter, protocol Protocol) {
		for index := 0; index < c.ProtocolCallbackHolderCount(protocol); index++ {
			op, input, callback, found := c.ProtocolCallbackHolderAt(protocol, index)
			emitter.row(func(writer *sourceRowWriter) {
				writer.u64(uint64(protocol))
				writer.u64(uint64(op))
				writer.input(input)
				writer.u64(uint64(callback))
				if !found {
					writer.valid = false
				}
			})
		}
	})
	if !ok {
		return false
	}

	return true
}
