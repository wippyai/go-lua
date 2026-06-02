package metatable

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/flow"
)

// Index records the static Lua class edge mt.__index = proto.
type Index struct {
	MetatableSym cfg.SymbolID
	PrototypeSym cfg.SymbolID
}

// MethodReceiver records that a method body should seed its self slot from the
// runtime self value of PrototypeSym.
type MethodReceiver struct {
	PrototypeSym cfg.SymbolID
	SelfSlot     int
}

// PrototypeMethod records one static method identity available through a
// prototype table. The callable value shape remains in the value/product domain;
// this carrier exists so transfer can publish method identity into FunctionRefs
// when a runtime instance is linked to PrototypeSym.
type PrototypeMethod struct {
	PrototypeSym cfg.SymbolID
	Field        fieldkey.Key
	FuncRef      flow.FunctionRef
}

// SetMetatableSite records a setmetatable call whose metatable argument
// statically resolves to a known prototype. Transfer owns evaluating the
// instance argument at Point.
type SetMetatableSite struct {
	Point        cfg.Point
	MetatableSym cfg.SymbolID
	PrototypeSym cfg.SymbolID
}
