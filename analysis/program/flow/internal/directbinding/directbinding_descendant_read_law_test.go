package directbinding

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	flowbinding "github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	flowbody "github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestSealReadLocalCellUsesBodyAncestry(t *testing.T) {
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	sibling := keyspace.MakeTerm(keyspace.FamilyBody, 3)

	if err := sealReadCase(t, entry, child); err != nil {
		t.Fatalf("descendant Read rejected: %v", err)
	}
	if err := sealReadCase(t, child, sibling); err == nil ||
		!strings.Contains(err.Error(), "local Cell owner disagrees with Read") {
		t.Fatalf("sibling Read error = %v, want local-cell ancestry rejection", err)
	}
}

func sealReadCase(t *testing.T, cellBody, readOwner keyspace.Term) error {
	t.Helper()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	sibling := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	loopOne := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	loopTwo := keyspace.MakeTerm(keyspace.FamilyLoop, 2)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	nilOne := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nilTwo := keyspace.MakeTerm(keyspace.FamilyNil, 2)

	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{
		entry, child, sibling, cell, bind, values, loopOne, loopTwo, read, nilOne, nilTwo,
	} {
		counts[keyspace.TermFamily(term)]++
	}

	name := "directbinding-descendant-read.lua"
	input := source.Input{
		Name:  name,
		Nil:   []source.NilLiteral{{Owner: entry}, {Owner: entry}},
		Binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		Bodies: []source.BodySource{
			{Body: entry, Terms: []keyspace.Term{loopOne, loopTwo}},
			{Body: child},
			{Body: sibling},
		},
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	if cellBody != entry {
		input.Bodies[0].Terms = []keyspace.Term{loopOne, loopTwo}
		input.Bodies[1].Terms = []keyspace.Term{bind}
	} else {
		input.Bodies[0].Terms = []keyspace.Term{bind, loopOne, loopTwo}
	}

	sourceDraft, err := source.Build(input)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	defer sourceFinalizer.Abort()

	flowDraft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: cellBody}}},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: cellBody}},
			Reads: []authored.Read{{Owner: readOwner, Source: cell}},
			Binds: []authored.Bind{{Owner: cellBody, Values: values}},
		},
		Control: authored.ControlInput{
			Loops: []authored.Loop{
				{Owner: entry, Body: child, Kind: kind.LoopWhile, Control: nilOne},
				{Owner: entry, Body: sibling, Kind: kind.LoopWhile, Control: nilTwo},
			},
		},
	})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("flow.Finalizer: %v", err)
	}
	defer flowFinalizer.Abort()

	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer staticFinalizer.Abort()

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	defer moduleFinalizer.Abort()

	preimage, flowView, staticView := sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View()
	bodies, err := flowbody.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	bindings, err := flowbinding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	_, err = Seal(preimage, flowView, bodies, bindings, staticView, moduleFinalizer.View())
	return err
}
