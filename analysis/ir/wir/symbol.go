package wir

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// SymbolKind is WIR's closed vocabulary for lexical value symbols. It mirrors
// binder symbol kinds without exposing transfer consumers to bind.Result.
type SymbolKind uint8

const (
	SymbolUnknown SymbolKind = iota
	SymbolParam
	SymbolLocal
	SymbolGlobal
	SymbolUpvalue
	SymbolFunction
)

// SymbolInfo records stable identity metadata for a symbol referenced by a WIR
// body. String identities are interned through ConstRef so instructions and
// metadata remain scalar handles into Body pools.
type SymbolInfo struct {
	Kind           SymbolKind
	Name           ConstRef
	RequireModule  ConstRef
	HasWrite       bool
	ImplicitGlobal bool
}

// SymbolInfoConfig is the external form used by lowering before string fields
// are interned into the body.
type SymbolInfoConfig struct {
	Kind           SymbolKind
	Name           string
	RequireModule  string
	HasWrite       bool
	ImplicitGlobal bool
}

// SymbolID is the scalar symbol identity used by WIR metadata.
type SymbolID = pathdom.SymbolID
