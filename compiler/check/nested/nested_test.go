package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestChild_Struct(t *testing.T) {
	c := Child{
		FuncName: "test",
		FuncSym:  cfg.SymbolID(1),
		IsLocal:  true,
	}
	if c.FuncName != "test" {
		t.Errorf("expected FuncName 'test', got %s", c.FuncName)
	}
	if c.FuncSym != 1 {
		t.Errorf("expected FuncSym 1, got %d", c.FuncSym)
	}
	if !c.IsLocal {
		t.Error("expected IsLocal true")
	}
}

func TestFuncInfo_Struct(t *testing.T) {
	fi := FuncInfo{
		Child: Child{
			FuncName: "test",
			FuncSym:  cfg.SymbolID(1),
			IsLocal:  true,
		},
	}
	if fi.FuncName != "test" {
		t.Errorf("expected FuncName 'test', got %s", fi.FuncName)
	}
}

func TestScopeGroup_Struct(t *testing.T) {
	sg := ScopeGroup{
		Hash:     12345,
		Funcs:    nil,
		MinPoint: cfg.Point(10),
	}
	if sg.Hash != 12345 {
		t.Errorf("expected Hash 12345, got %d", sg.Hash)
	}
	if sg.MinPoint != 10 {
		t.Errorf("expected MinPoint 10, got %d", sg.MinPoint)
	}
}
