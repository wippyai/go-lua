package operation

import "github.com/wippyai/go-lua/analysis/program/target/vocabulary"

// CallbackCount and CallbackAt expose the dense callback table through an
// operation's owner-issued span.
func (core Core) CallbackCount(op vocabulary.Operation) int {
	row, ok := core.operation(op)
	if !ok {
		return 0
	}
	return row.callbacks.Len()
}

func (core Core) CallbackAt(op vocabulary.Operation, index int) (vocabulary.CallbackID, bool) {
	row, ok := core.operation(op)
	if !ok {
		return 0, false
	}
	callback, ok := core.geometry.callbacks.At(int(row.callbacks.start) + index)
	if !ok {
		return 0, false
	}
	return callback.id, true
}

func (core Core) CallbackOwner(id vocabulary.CallbackID) (vocabulary.Operation, bool) {
	row, ok := core.callback(id)
	if !ok {
		return 0, false
	}
	return row.owner, true
}

// CallbackIndex resolves an owner-issued callback ID to its local ordinal.
// The ordinal is stored when Core compiles the callback table, so identity
// sealing does not rediscover it by scanning operations.
func (core Core) CallbackIndex(id vocabulary.CallbackID) (vocabulary.Operation, int, bool) {
	row, ok := core.callback(id)
	if !ok {
		return 0, 0, false
	}
	return row.owner, int(row.ordinal), true
}

func (core Core) CallbackSource(id vocabulary.CallbackID) (vocabulary.InputSource, bool) {
	row, ok := core.callback(id)
	if !ok {
		return vocabulary.InputSource{}, false
	}
	if opaque, opaqueOK := core.Opaque(); opaqueOK && row.owner == opaque {
		return vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}, true
	}
	if row.function.Kind != 0 {
		return row.function, true
	}
	return vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(row.source)}, true
}

func (core Core) CallbackLifecycle(id vocabulary.CallbackID) (vocabulary.CallbackLifecycle, bool) {
	row, ok := core.callback(id)
	if !ok {
		return 0, false
	}
	return row.lifecycle, true
}

// CallbackOpaque reports whether the callback belongs to the synthesized
// opaque operation. The owner and opaque handles are both issued by Core.
func (core Core) CallbackOpaque(id vocabulary.CallbackID) bool {
	owner, ownerOK := core.CallbackOwner(id)
	opaque, opaqueOK := core.Opaque()
	return ownerOK && opaqueOK && owner == opaque
}

func (core Core) callback(id vocabulary.CallbackID) (callbackRow, bool) {
	if id == 0 {
		return callbackRow{}, false
	}
	callback, callbackOK := core.geometry.callbacks.At(int(id) - 1)
	if !callbackOK || callback.id != id || callback.owner == 0 {
		return callbackRow{}, false
	}
	if _, ownerOK := core.operation(callback.owner); !ownerOK {
		return callbackRow{}, false
	}
	return callback, true
}
