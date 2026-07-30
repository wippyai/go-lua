package transformer

import (
	"math"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFormalPointValueRoleBindsOnlyToEvolvingMiddleSlot(t *testing.T) {
	program := formalRootInputTestProgram(t, standard.Registry())
	slots := program.formalSlots
	if slots == nil {
		t.Fatal("formal slot space")
	}
	body := &program.bodies[0]
	pointValue, ok := formalMiddleSlotForStateKey(program, body, statekey.SymbolValue(symbol.ID(101)))
	if !ok {
		t.Fatal("point-local parameter value has no Middle slot")
	}
	input, ok := slots.Slot(body.body, Root{Kind: RootParam, Index: 0})
	if !ok {
		t.Fatal("parameter input slot")
	}
	if pointValue == input || pointValue.Vocabulary() != formal.Middle || input.Vocabulary() != formal.Input {
		t.Fatalf("point/input slots were conflated: point=%#v input=%#v", pointValue, input)
	}
	fromConcrete, ok := formalLiveValueSlotForDependency(program, body, statekey.ConcreteDependency(statekey.SymbolValue(symbol.ID(101))))
	if !ok || fromConcrete != pointValue {
		t.Fatalf("concrete Values dependency = %#v/%v, want Middle %#v", fromConcrete, ok, pointValue)
	}
	if _, ok := formalMiddleSlotForStateKey(program, body, statekey.SymbolValue(symbol.ID(9999))); ok {
		t.Fatal("unsealed lexical symbol manufactured a formal Middle slot")
	}
}

func TestFormalSlotSpaceImportsByFullBodyIdentity(t *testing.T) {
	firstBody := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-slot-first")))
	secondBody := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-slot-second")))
	shape := Shape{Params: 1, Results: 1}
	first, err := newSlotSpace([]slotSpaceBody{{id: firstBody, shape: shape}, {id: secondBody, shape: shape}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := newSlotSpace([]slotSpaceBody{{id: secondBody, shape: shape}, {id: firstBody, shape: shape}})
	if err != nil {
		t.Fatal(err)
	}

	source, ok := first.Slot(firstBody, Root{Kind: RootResult, Index: 0})
	if !ok {
		t.Fatal("source formal slot")
	}
	descriptor, ok := source.Root()
	if !ok || descriptor != formal.NewRoot(firstBody, source.RootOrdinal(), formal.Output) {
		t.Fatalf("source neutral root = %#v/%v", descriptor, ok)
	}
	imported, ok := second.Import(source)
	if !ok || imported.Body() != firstBody || imported.RootOrdinal() != source.RootOrdinal() || imported.Vocabulary() != source.Vocabulary() {
		t.Fatalf("imported slot = %#v/%v", imported, ok)
	}
	if imported == source {
		t.Fatal("cross-space import retained foreign dense authority")
	}
	sourceBytes, sourceOK := source.CanonicalBytes()
	importedBytes, importedOK := imported.CanonicalBytes()
	if !sourceOK || !importedOK || sourceBytes != importedBytes {
		t.Fatal("cross-space canonical identity changed")
	}
	foreign, ok := first.Slot(secondBody, Root{Kind: RootResult, Index: 0})
	if !ok {
		t.Fatal("foreign formal slot")
	}
	foreignBytes, _ := foreign.CanonicalBytes()
	if foreignBytes == sourceBytes {
		t.Fatal("distinct full lexical body identities collided")
	}
	input, ok := first.Slot(firstBody, Root{Kind: RootParam, Index: 0})
	inputBytes, inputOK := input.CanonicalBytes()
	if !ok || !inputOK || input.Vocabulary() != formal.Input || inputBytes == sourceBytes {
		t.Fatal("semantic IN and OUT vocabularies collided")
	}
}

func TestFormalSlotRootOrdinalHasNoPackedWidthCap(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-slot-wide")))
	space, err := newSlotSpace([]slotSpaceBody{{id: body, shape: Shape{Params: math.MaxUint32, Captures: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	slot, ok := space.Slot(body, Root{Kind: RootCapture, Index: 1})
	if !ok || slot.RootOrdinal() != uint64(math.MaxUint32)+2 {
		t.Fatalf("wide formal slot = %#v/%v", slot, ok)
	}
}

func TestFormalSlotVocabularyIsInferredFromTypedRoot(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-slot-vocabularies")))
	space, err := newSlotSpace([]slotSpaceBody{{id: body, shape: Shape{Params: 1, Results: 1}, middle: 1}})
	if err != nil {
		t.Fatal(err)
	}
	param, paramOK := space.Slot(body, Root{Kind: RootParam, Index: 0})
	result, resultOK := space.Slot(body, Root{Kind: RootResult, Index: 0})
	middle, middleOK := space.Slot(body, Root{Kind: RootMiddle, Index: 0})
	if !paramOK || param.Vocabulary() != formal.Input || !resultOK || result.Vocabulary() != formal.Output || !middleOK || middle.Vocabulary() != formal.Middle {
		t.Fatalf("typed vocabularies = %v/%v, %v/%v, %v/%v", param.Vocabulary(), paramOK, result.Vocabulary(), resultOK, middle.Vocabulary(), middleOK)
	}
	if _, ok := space.Slot(body, Root{Kind: RootGlobal, Index: 0}); ok {
		t.Fatal("arbitrary boundary root manufactured a MID coordinate")
	}
}
