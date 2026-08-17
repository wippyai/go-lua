package storage

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestTableRejectsForeignWriterOwnership(t *testing.T) {
	first, second := Writer{}, Writer{}
	mark := first.TableMark()
	if _, err := second.Table(source.Span{File: "storage.lua"}, mark, 1); err == nil {
		t.Fatal("Table accepted a mark owned by another Writer")
	}
}
