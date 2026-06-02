package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSymbolStoragePolicyClassifiesEnvOwnerAndCapturedCells(t *testing.T) {
	envTransfer := New(input.Inputs{}, Config{})
	if got := envTransfer.symbolStorage.class(cfg.SymbolID(77)); got != symbolStorageEnv {
		t.Fatalf("env storage class = %v, want %v", got, symbolStorageEnv)
	}

	ownerTransfer, _, ownerSym := ownerCellParamTestTransfer(t)
	if got := ownerTransfer.symbolStorage.class(ownerSym); got != symbolStorageOwnerCell {
		t.Fatalf("owner storage class = %v, want %v", got, symbolStorageOwnerCell)
	}

	capturedTransfer, _, capturedSym := captureCellTestTransfer(t)
	if got := capturedTransfer.symbolStorage.class(capturedSym); got != symbolStorageCapturedCell {
		t.Fatalf("captured storage class = %v, want %v", got, symbolStorageCapturedCell)
	}
}

func TestSymbolStoragePolicyEnvClassReadsEnvNotSameIDCell(t *testing.T) {
	const sym = cfg.SymbolID(81)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.String),
		},
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(typ.Number)}}),
	}

	got, ok := tr.symbolValue(&out, sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("env-class symbolValue = %v/%v, want string/true", got.ProjectValue(), ok)
	}
}

func TestClosureCaptureCellsPreservesExistingCellStoreValue(t *testing.T) {
	const sym = cfg.SymbolID(82)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.String),
		},
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(typ.Number)}}),
	}

	got := tr.closureCaptureCells(&out, []cfg.SymbolID{sym})
	av, ok := got.Value(sym)
	if !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("closure capture cell = %v/%v, want existing cell value", av.ProjectValue(), ok)
	}
}

func TestSymbolStoragePolicyCellBackedReadFallsBackToEnv(t *testing.T) {
	tr, _, sym := captureCellTestTransfer(t)
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.String),
		},
	}

	got, ok := tr.symbolValue(&out, sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("cell-backed fallback symbolValue = %v/%v, want string/true", got.ProjectValue(), ok)
	}
}

func TestSymbolStoragePolicyOwnerCellDoesNotEmitCallerEffect(t *testing.T) {
	tr, _, sym := ownerCellParamTestTransfer(t)
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.String),
		},
	}

	tr.writeSymbolValue(&out, sym, product.FromType(typ.Number), false, true)

	if _, ok := out.Env[flow.SymbolValueKey(sym)]; ok {
		t.Fatalf("owner cell write left Env[%s]", flow.SymbolValueKey(sym))
	}
	got, ok := out.Cells.Value(sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("owner cell value = %v/%v, want number/true", got.ProjectValue(), ok)
	}
	if effects := out.CellEffects.Entries(); len(effects) != 0 {
		t.Fatalf("owner cell write emitted caller effects: %s", out.CellEffects.Format())
	}
}

func TestSymbolStoragePolicyCapturedCellEmitsCallerEffect(t *testing.T) {
	tr, _, sym := captureCellTestTransfer(t)
	out := flow.PointState{}

	tr.writeSymbolValue(&out, sym, product.FromType(typ.Boolean), false, true)

	got, ok := out.Cells.Value(sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Boolean) {
		t.Fatalf("captured cell value = %v/%v, want boolean/true", got.ProjectValue(), ok)
	}
	effects := out.CellEffects.Entries()
	if len(effects) != 1 || effects[0].Symbol != sym || !effects[0].MustWrite ||
		!typ.TypeEquals(effects[0].Value.ProjectValue(), typ.Boolean) {
		t.Fatalf("captured cell effects = %s, want one boolean must-write", out.CellEffects.Format())
	}
}
