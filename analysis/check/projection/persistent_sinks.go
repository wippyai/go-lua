package projection

import "github.com/wippyai/go-lua/analysis/symbol"

type symbolKindReader interface {
	SymbolKind(symbol.ID) (symbol.Kind, bool)
}

type capturedSymbolReader interface {
	CapturedSymbols() []symbol.ID
}

func capturedSinkSymbols(reader capturedSymbolReader) map[symbol.ID]struct{} {
	if reader == nil {
		return nil
	}
	captures := reader.CapturedSymbols()
	if len(captures) == 0 {
		return nil
	}
	out := make(map[symbol.ID]struct{}, len(captures))
	for _, captured := range captures {
		if captured != 0 {
			out[captured] = struct{}{}
		}
	}
	return out
}

// persistentSinkSymbol reports whether id names a sink whose lifetime outlives
// the call. Capture identity is detached body metadata; projection never reads
// binder or Lua syntax to recover it.
func persistentSinkSymbol(kindReader symbolKindReader, captured map[symbol.ID]struct{}, id symbol.ID) bool {
	if _, ok := captured[id]; ok {
		return true
	}
	if kindReader == nil {
		return false
	}
	kind, ok := kindReader.SymbolKind(id)
	return ok && (kind == symbol.Upvalue || kind == symbol.Global)
}
