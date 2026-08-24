package inspect

import "strings"

// formatTarget renders the sealed target the fixture was linked against:
// its class index, its operation members, and its protocols. Every line names
// the contract accessor that produced it.
func formatTarget(session *Session) string {
	var b strings.Builder
	writef(&b, "session.Fixture=%s", session.fixture)
	contract := session.contract
	if contract == nil || !contract.ContentID().Available() {
		writef(&b, "link.Boundary.Target=unavailable")
		return b.String()
	}
	writef(&b, "contract.ContentID=%s", contract.ContentID())

	types := contract.Types()
	writef(&b, "contract.Types.Count=%d", types.Count())
	for index := 0; index < types.Count(); index++ {
		name, typ, ok := types.At(index)
		if !ok {
			continue
		}
		writef(&b, "contract.Types.At(%d).Name=%s", index, name)
		declaration, declarationOK := types.Declaration(typ)
		if !declarationOK {
			writef(&b, "contract.Types.Declaration(%s)=unavailable", name)
			continue
		}
		writef(&b, "contract.Types.Declaration(%s).Digest=%s", name, declaration.Digest())
		writef(&b, "contract.Types.Declaration(%s).ExternalFormals=%d", name, declaration.ExternalFormals())
		if primitive, primitiveOK := declaration.Primitive(); primitiveOK {
			writef(&b, "contract.Types.Declaration(%s).Primitive=%d", name, uint8(primitive))
		}
	}

	operations := contract.Operations
	writef(&b, "contract.Operations.OperationCount=%d", operations.OperationCount())
	writef(&b, "contract.Operations.SourceCount=%d", operations.SourceCount())
	writef(&b, "contract.Operations.BoundCount=%d", operations.BoundCount())
	for index := 0; index < operations.OperationCount(); index++ {
		operation, ok := operations.OperationAt(index)
		if !ok {
			continue
		}
		writef(&b, "contract.Operations.OperationAt(%d)=%d", index, uint32(operation))
		if id, idOK := contract.OperationContentID(operation); idOK {
			writef(&b, "contract.OperationContentID(%d)=%s", uint32(operation), id)
		}
		if anchor, anchorOK := operations.Anchor(operation); anchorOK {
			writef(&b, "contract.Operations.Anchor(%d)=%s", uint32(operation), anchor)
		}
	}

	protocols := contract.Protocols()
	writef(&b, "contract.Protocols.ProtocolCount=%d", protocols.ProtocolCount())
	for index := 0; index < protocols.ProtocolCount(); index++ {
		protocol, ok := protocols.ProtocolAt(index)
		if !ok {
			continue
		}
		writef(&b, "contract.Protocols.ProtocolAt(%d)=%d", index, uint32(protocol))
		writef(&b, "contract.Protocols.StateCount(%d)=%d", uint32(protocol), protocols.StateCount(protocol))
		for stateIndex := 0; stateIndex < protocols.StateCount(protocol); stateIndex++ {
			state, stateOK := protocols.StateAt(protocol, stateIndex)
			if !stateOK {
				continue
			}
			name, nameOK := protocols.StateName(protocol, state)
			if nameOK {
				writef(&b, "contract.Protocols.StateName(%d,%d)=%s", uint32(protocol), uint32(state), name)
			}
			if final, finalOK := protocols.StateFinal(protocol, state); finalOK {
				writef(&b, "contract.Protocols.StateFinal(%d,%d)=%t", uint32(protocol), uint32(state), final)
			}
		}
		writef(&b, "contract.Protocols.TransitionCount(%d)=%d", uint32(protocol), protocols.TransitionCount(protocol))
		for transitionIndex := 0; transitionIndex < protocols.TransitionCount(protocol); transitionIndex++ {
			operation, sourceKind, ordinal, state, transitionOK := protocols.TransitionAt(protocol, transitionIndex)
			if !transitionOK {
				continue
			}
			writef(&b, "contract.Protocols.TransitionAt(%d,%d)=operation:%d source:%d ordinal:%d state:%d",
				uint32(protocol), transitionIndex, uint32(operation), uint8(sourceKind), ordinal, uint32(state))
			outcomes := protocols.TransitionOutcomeCount(protocol, transitionIndex)
			for outcomeIndex := 0; outcomeIndex < outcomes; outcomeIndex++ {
				outcome, target, outcomeOK := protocols.TransitionOutcomeAt(protocol, transitionIndex, outcomeIndex)
				if !outcomeOK {
					continue
				}
				writef(&b, "contract.Protocols.TransitionOutcomeAt(%d,%d,%d)=outcome:%d state:%d",
					uint32(protocol), transitionIndex, outcomeIndex, outcome, uint32(target))
			}
		}
		writef(&b, "contract.Protocols.ProtocolRequirementCount(%d)=%d", uint32(protocol), protocols.ProtocolRequirementCount(protocol))
		writef(&b, "contract.Protocols.EscapeCount(%d)=%d", uint32(protocol), protocols.EscapeCount(protocol))
	}
	writef(&b, "contract.ExactKeyCount=%d", contract.ExactKeyCount())
	return b.String()
}
