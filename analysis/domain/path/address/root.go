package address

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type rootKind uint8

const (
	rootInvalid rootKind = iota
	rootSymbol
	rootName
)

// Root is the semantic root identity of a path address.
type Root struct {
	kind   rootKind
	symbol symbol.ID
	name   string
}

// SymbolRoot builds a symbol-rooted identity.
func SymbolRoot(sym symbol.ID) (Root, bool) {
	if sym == 0 {
		return Root{}, false
	}
	return Root{kind: rootSymbol, symbol: sym}, true
}

// NamedRoot builds a non-symbol root identity.
func NamedRoot(name string) (Root, bool) {
	if name == "" {
		return Root{}, false
	}
	return Root{kind: rootName, name: name}, true
}

// RootOfPath extracts the stable root identity from a path.
func RootOfPath(path pathdom.Path) (Root, bool) {
	if path.Symbol != 0 {
		return SymbolRoot(path.Symbol)
	}
	return NamedRoot(path.Root)
}

// Symbol returns the symbol for symbol-rooted identities.
func (r Root) Symbol() (symbol.ID, bool) {
	return r.symbol, r.kind == rootSymbol && r.symbol != 0
}

// Name returns the root name for non-symbol identities.
func (r Root) Name() (string, bool) {
	return r.name, r.kind == rootName && r.name != ""
}

// Equal reports semantic root equality.
func (r Root) Equal(other Root) bool {
	return r.kind == other.kind && r.symbol == other.symbol && r.name == other.name
}

func (r Root) isValid() bool {
	switch r.kind {
	case rootSymbol:
		return r.symbol != 0
	case rootName:
		return r.name != ""
	default:
		return false
	}
}
