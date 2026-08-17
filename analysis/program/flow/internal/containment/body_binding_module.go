package containment

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// emitBodyBindingModule owns the three structural relations which are neither
// expression semantics nor static syntax:
//
//	Body -> lexical Body parent
//	Cell -> exact binding Host (Bind, Function, Loop, or Entry Body;
//	        Program root for globals)
//	Import -> authored Call
//
// These rows are emitted directly into the one kernel input.  No body graph,
// cell graph, or module graph is constructed here.
func emitBodyBindingModule(
	view authored.View,
	bodies *body.Result,
	bindingResult binding.Result,
	moduleView imports.View,
	entry keyspace.Term,
	counts [keyspace.FamilyCount]uint32,
	result *emission,
) error {
	if err := emitBodyParents(result, bodies, entry, counts); err != nil {
		return err
	}
	if err := emitCellHosts(result, view, bindingResult, entry, counts); err != nil {
		return err
	}
	if err := emitImports(result, moduleView, counts); err != nil {
		return err
	}
	return nil
}

func emitBodyParents(result *emission, bodies *body.Result, entry keyspace.Term, counts [keyspace.FamilyCount]uint32) error {
	if result == nil || keyspace.TermFamily(entry) != keyspace.FamilyBody || !validTerm(entry, counts) {
		return errors.New("program/flow/containment: invalid Body root")
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBody]; ordinal++ {
		child := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		at, ok := bodies.BodyAt(int(ordinal - 1))
		if !ok || at != child {
			return errors.New("program/flow/containment: noncanonical Body ordinal")
		}
		parent, hasParent := bodies.Parent(child)
		if child == entry {
			if hasParent || parent != 0 {
				return errors.New("program/flow/containment: Entry Body has a parent")
			}
			result.roots = append(result.roots, child)
			continue
		}
		if !hasParent || !validTerm(parent, counts) || keyspace.TermFamily(parent) != keyspace.FamilyBody || parent == child {
			return errors.New("program/flow/containment: non-entry Body lacks lexical parent")
		}
		result.edges = append(result.edges, kernelEdge{child: child, parent: parent})
	}
	return nil
}

func emitCellHosts(
	result *emission,
	view authored.View,
	bindingResult binding.Result,
	entry keyspace.Term,
	counts [keyspace.FamilyCount]uint32,
) error {
	if result == nil {
		return errors.New("program/flow/containment: nil Cell emission")
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyCell]; ordinal++ {
		cell := keyspace.MakeTerm(keyspace.FamilyCell, ordinal)
		role, ok := bindingResult.Role(cell)
		if !ok {
			return errors.New("program/flow/containment: Cell role unavailable")
		}
		host, ok := bindingResult.Host(cell)
		if !ok {
			return errors.New("program/flow/containment: Cell host unavailable")
		}
		if role == kind.CellGlobal {
			if host != 0 {
				return errors.New("program/flow/containment: global Cell has a host")
			}
			result.roots = append(result.roots, cell)
			continue
		}
		var hostFamily keyspace.Family
		switch role {
		case kind.CellLocal:
			hostFamily = keyspace.FamilyBind
		case kind.CellFormal, kind.CellFunctionVararg, kind.CellCapture:
			hostFamily = keyspace.FamilyFunction
		case kind.CellLoop:
			hostFamily = keyspace.FamilyLoop
		case kind.CellChunkVararg:
			hostFamily = keyspace.FamilyBody
		default:
			return errors.New("program/flow/containment: invalid Cell role")
		}
		if !termInFamily(host, hostFamily, counts) {
			return errors.New("program/flow/containment: Cell host family disagrees with role")
		}
		if role == kind.CellChunkVararg && host != entry {
			return errors.New("program/flow/containment: chunk Vararg Cell is not entry-hosted")
		}
		result.edges = append(result.edges, kernelEdge{child: cell, parent: host})
	}
	return nil
}

func emitImports(result *emission, view imports.View, counts [keyspace.FamilyCount]uint32) error {
	if result == nil {
		return errors.New("program/flow/containment: nil Import emission")
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyImport]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyImport, ordinal)
		row, ok := view.Import(term)
		if !ok || row.Term != term || !validTerm(row.Call, counts) || keyspace.TermFamily(row.Call) != keyspace.FamilyCall {
			return errors.New("program/flow/containment: malformed Module Import relation")
		}
		result.edges = append(result.edges, kernelEdge{child: term, parent: row.Call})
	}
	return nil
}
