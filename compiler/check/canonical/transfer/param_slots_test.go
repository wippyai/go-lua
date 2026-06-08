package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramboundary"
)

func setTransferParamSlotsForTest(t *Transfer, symbols ...cfg.SymbolID) {
	t.params = paramboundary.ParameterSlotsFromSymbols(symbols)
	t.symbolStorage.params = t.params
}
