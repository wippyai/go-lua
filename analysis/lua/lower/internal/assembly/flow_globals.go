package assembly

import (
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// reserveGlobalCells installs the complete binder-owned global Cell prefix
// before lowering starts. It seeds Source's exact-key denominator from the
// census and fixes each Cell span from the census origin; no lower visitation
// can mint, reorder, or re-span a global.
func (c *Collector) reserveGlobalCells(census bind.GlobalCensus) error {
	if c == nil {
		return errors.New("program/lower/collector: nil collector for global census")
	}
	count := census.Len()
	if uint64(count) > uint64(keyspace.MaxTermOrdinal) {
		return fmt.Errorf("program/lower/collector: global census overflow %d", count)
	}
	if count == 0 {
		return nil
	}

	c.counts[keyspace.FamilyCell] = uint32(count)
	c.source.Families[keyspace.FamilyCell-1].Spans = make([]source.Span, count)
	if !c.flow.InitGlobalCells(count) {
		return fmt.Errorf("program/lower/collector: global Cell storage overflow %d", count)
	}
	c.flow.SetGlobalCensus(census)
	for index := 0; index < count; index++ {
		cell, ok := census.At(index)
		if !ok || !cell.Identity().Valid() || cell.Name() == "" ||
			cell.Slot() != uint32(index) || cell.Ordinal() != uint32(index+1) {
			return fmt.Errorf("program/lower/collector: malformed global census slot %d", index)
		}
		span, err := globalOriginSpan(c.name, cell.Origin())
		if err != nil {
			return fmt.Errorf("program/lower/collector: global %q origin: %w", cell.Name(), err)
		}
		c.source.Families[keyspace.FamilyCell-1].Spans[index] = span
		if !c.flow.SetCell(index, authored.Cell{Kind: authored.CellGlobal}) {
			return fmt.Errorf("program/lower/collector: global Cell storage slot %d", index)
		}
		ordinal := int(cell.Ordinal())
		if ordinal <= 0 || ordinal > int(keyspace.MaxTermOrdinal) {
			return fmt.Errorf("program/lower/collector: global Cell spelling slot %d", index)
		}
		if len(c.source.CellSpellings) < ordinal {
			c.source.CellSpellings = append(c.source.CellSpellings, make([]source.CellSpelling, ordinal-len(c.source.CellSpellings))...)
		}
		c.source.CellSpellings[ordinal-1] = source.CellSpelling{Cell: keyspace.MakeTerm(keyspace.FamilyCell, cell.Ordinal()), Name: cell.Name()}
		if c.source.CellSpellings[ordinal-1].Cell == 0 {
			return fmt.Errorf("program/lower/collector: global Cell spelling slot %d", index)
		}
		if !c.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: cell.Name()}) {
			return fmt.Errorf("program/lower/collector: global %q exact seed rejected", cell.Name())
		}
	}
	return nil
}

// globalOriginSpan converts binder coordinates to the collector's source
// span without accepting a lowerer-supplied replacement. The logical source
// name is the sole Source filename authority; a non-empty binder filename
// must agree with it.
func globalOriginSpan(name string, origin ast.Position) (source.Span, error) {
	if name == "" {
		return source.Span{}, errors.New("empty collector source name")
	}
	if origin.File != "" && origin.File != name {
		return source.Span{}, fmt.Errorf("foreign source file %q", origin.File)
	}
	values := []int{origin.Line, origin.Column, origin.EndLine, origin.EndColumn}
	for _, value := range values {
		if value < 0 || uint64(value) > uint64(math.MaxUint32) {
			return source.Span{}, fmt.Errorf("invalid coordinate %d", value)
		}
	}
	return source.Span{
		File:      name,
		StartLine: uint32(origin.Line),
		StartCol:  uint32(origin.Column),
		EndLine:   uint32(origin.EndLine),
		EndCol:    uint32(origin.EndColumn),
	}, nil
}
