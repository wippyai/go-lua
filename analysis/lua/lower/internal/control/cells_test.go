package control

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"testing"
)

func TestControlLoopCellsKeepDeclarationOrderAcrossInlineAndOverflowStorage(t *testing.T) {
	var writer Writer
	mark := writer.CellMark()
	for index := 0; index < len(writer.cellInline)+1; index++ {
		if err := writer.RememberCell(keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1))); err != nil {
			t.Fatal(err)
		}
	}
	if writer.CellMark() != len(writer.cellInline)+1 || writer.cellSlice()[0] != keyspace.MakeTerm(keyspace.FamilyCell, 1) {
		t.Fatalf("cell storage = %d/%v", writer.CellMark(), writer.cellSlice())
	}
	writer.truncateCells(mark)
	if writer.CellMark() != mark || len(writer.cellSlice()) != 0 {
		t.Fatal("truncateCells did not restore the mark")
	}
}
