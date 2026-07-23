package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestOutboundFrameBoundaryLensCoversEveryStructuralSourceRole(t *testing.T) {
	registry := standard.Registry()
	bodyID := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	keys := keyspace.New()
	arena := NewArena(registry)
	if !arena.bindLexicalOwner(bodyID) {
		t.Fatal("bind lexical owner")
	}

	const (
		param   symbol.ID = 701
		local   symbol.ID = 702
		ambient symbol.ID = 703
	)
	for _, id := range []symbol.ID{local, ambient} {
		if arena.bindEnvironmentSymbol(id) == 0 {
			t.Fatalf("bind environment symbol %d", id)
		}
	}
	const callPoint cfg.Point = 17
	if arena.bindCallResult(callPoint, 2) == 0 {
		t.Fatal("bind call result")
	}
	if err := arena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}

	localRoot, exact := arena.middleRoot(statekey.SymbolValue(local))
	if !exact {
		t.Fatal("resolve local Middle root")
	}
	resultSlot := statekey.CallResult(uint32(callPoint), 2)
	resultRoot, exact := arena.middleRoot(resultSlot)
	if !exact {
		t.Fatal("resolve result Middle root")
	}

	paramTerm := arena.Path(Root{Kind: RootParam})
	localTerm := arena.Path(localRoot, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	resultTerm := arena.Path(resultRoot)
	ambientTerm := arena.EnvironmentPath(ambient)
	if paramTerm == 0 || localTerm == 0 || resultTerm == 0 || ambientTerm == 0 {
		t.Fatal("freeze boundary source terms")
	}
	arena.Seal()

	paramPath := keys.FromPath(pathdom.NewPath(param, ""))
	body := relationProgramBody{
		body: bodyID,
		keys: keys,
		roots: relationRootCarrier{
			shape: Shape{Params: 1},
			roots: []relationStateRoot{{
				root: Root{Kind: RootParam}, slot: statekey.SymbolValue(param), path: paramPath,
			}},
		},
		relation: Relation{shape: Shape{Params: 1}, arena: arena},
	}
	tests := []struct {
		name       string
		term       PathTerm
		wantSlot   statekey.Value
		wantSyntax pathdom.Path
		private    bool
	}{
		{name: "parameter", term: paramTerm, wantSlot: statekey.SymbolValue(param), wantSyntax: pathdom.NewPath(param, "")},
		{name: "current-local", term: localTerm, wantSyntax: pathdom.Path{Symbol: local, Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}}},
		{name: "call-result", term: resultTerm, wantSlot: resultSlot, private: true},
		{name: "ambient", term: ambientTerm, wantSlot: statekey.SymbolValue(ambient), wantSyntax: pathdom.NewPath(ambient, "")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, slot, syntax, exact := outboundFramePathTerm(&body, test.term)
			if !exact || path.Kind == keyspace.KindInvalid || keys.FormatReadOnly(path) == "" {
				t.Fatalf("source has no exact structural boundary image: path=%#v exact=%v", path, exact)
			}
			if slot != test.wantSlot {
				t.Fatalf("slot = %d, want %d", slot, test.wantSlot)
			}
			if test.private {
				if !syntax.IsEmpty() {
					t.Fatalf("private carrier leaked source syntax: %#v", syntax)
				}
				return
			}
			if syntax.Key() != test.wantSyntax.Key() {
				t.Fatalf("syntax = %q, want %q", syntax.Key(), test.wantSyntax.Key())
			}
		})
	}

	resultPath, resultSyntax, exact := outboundFrameValueSlotPath(&body, resultSlot)
	if !exact || resultPath.Kind == keyspace.KindInvalid || keys.FormatReadOnly(resultPath) == "" || !resultSyntax.IsEmpty() {
		t.Fatalf("pathless call result has no private structural image: path=%#v syntax=%#v exact=%v", resultPath, resultSyntax, exact)
	}
}
