package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPointEffectRecordsCaptureAndReceiverEffects(t *testing.T) {
	sym := cfg.SymbolID(21)
	cellValue := product.FromType(typ.String)
	receiverValue := product.FromType(typ.Number)
	out := PointState{
		Cells:           CaptureCellsDomain.Bottom(),
		CellEffects:     CaptureEffectsIdentity(),
		ReceiverEffects: ReceiverEffectsIdentity(),
	}
	effects := CaptureMustWrite(sym, cellValue)

	if !ApplyCaptureEffectsToCellStore(&out, effects) {
		t.Fatal("ApplyCaptureEffectsToCellStore reported no change")
	}
	if got, ok := out.Cells.Value(sym); !ok || !product.Domain.Equal(got, cellValue) {
		t.Fatalf("cell store value = %v/%v, want %v", got.ProjectValue(), ok, cellValue.ProjectValue())
	}
	if !RecordCaptureEffects(&out, effects) {
		t.Fatal("RecordCaptureEffects reported no change")
	}
	if !RecordReceiverWrite(&out, 0, receiverValue) {
		t.Fatal("RecordReceiverWrite reported no change")
	}
	if entries := out.ReceiverEffects.Entries(); len(entries) != 1 || entries[0].Slot != 0 || !entries[0].MustWrite {
		t.Fatalf("receiver effects = %#v, want one must-write slot 0", entries)
	}
}

func TestPointEffectRecordsPrototypeFacts(t *testing.T) {
	proto := cfg.SymbolID(31)
	instance := cfg.SymbolID(32)
	self := product.FromType(typ.NewRecord().Build())
	out := PointState{}

	if !RecordPrototypeSelf(&out, proto, self) {
		t.Fatal("RecordPrototypeSelf reported no change")
	}
	if got, ok := out.PrototypeSelf.Value(proto); !ok || !product.Domain.Equal(got, self) {
		t.Fatalf("prototype self = %v/%v, want %v", got.ProjectValue(), ok, self.ProjectValue())
	}
	if !BindPrototypeInstance(&out, instance, proto) {
		t.Fatal("BindPrototypeInstance reported no change")
	}
	if protos, ok := out.PrototypeInstances.Prototypes(instance); !ok || len(protos) != 1 || protos[0] != proto {
		t.Fatalf("prototype instances = %v/%v, want [%d]", protos, ok, proto)
	}
	if !ClearPrototypeInstance(&out, instance) {
		t.Fatal("ClearPrototypeInstance reported no change")
	}
	if _, ok := out.PrototypeInstances.Prototypes(instance); ok {
		t.Fatalf("prototype instance survived clear: %s", out.PrototypeInstances.Format())
	}
}
