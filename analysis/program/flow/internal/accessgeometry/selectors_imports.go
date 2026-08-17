package accessgeometry

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	binding "github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func collectImportAliases(sourceView sourceColumns, flow authored.View, bindings binding.Result, view imports.View, aliases []bool, cellCount, bodyCount, callCount int) error {
	for index := 0; index < view.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1))
		row, ok := view.Import(term)
		if !ok || row.Term != term || keyspace.TermFamily(row.Call) != keyspace.FamilyCall || keyspace.TermOrdinal(row.Call) == 0 || int(keyspace.TermOrdinal(row.Call)) > callCount {
			return errors.New("program/flow/accessgeometry: malformed Module Import")
		}
		if row.Alias == 0 {
			continue
		}
		if keyspace.TermFamily(row.Alias) != keyspace.FamilyCell || keyspace.TermOrdinal(row.Alias) == 0 ||
			uint64(keyspace.TermOrdinal(row.Alias)) >= uint64(len(aliases)) || int(keyspace.TermOrdinal(row.Alias)) > cellCount {
			return errors.New("program/flow/accessgeometry: Import alias is outside Cell universe")
		}
		if err := validateImportAlias(sourceView, flow, bindings, row.Call, row.Alias, bodyCount); err != nil {
			return err
		}
		aliases[keyspace.TermOrdinal(row.Alias)] = true
	}
	return nil
}

func validateImportAlias(sourceView sourceColumns, flow authored.View, bindings binding.Result, call, alias keyspace.Term, bodyCount int) error {
	calls := flow.Calls()
	callOwner, callee, receiver, _, ok := calls.Get(call)
	if !ok || !validFlowBody(callOwner, bodyCount) || receiver != 0 || keyspace.TermFamily(callee) != keyspace.FamilyRead || keyspace.TermOrdinal(callee) == 0 {
		return errors.New("program/flow/accessgeometry: Import Alias Call is not a same-owner plain Call")
	}
	reads := flow.Storage().Reads()
	readOwner, sourceTerm, _, readOK := reads.Get(callee)
	if !readOK || readOwner != callOwner || keyspace.TermFamily(sourceTerm) != keyspace.FamilyCell || keyspace.TermOrdinal(sourceTerm) == 0 {
		return errors.New("program/flow/accessgeometry: Import Alias Call callee is not a same-owner global Read")
	}
	cells := flow.Storage().Cells()
	cellKind, cellBody, exact, cellOK := cells.Get(sourceTerm)
	if !cellOK || cellKind != authored.CellGlobal || cellBody != 0 || exact == 0 {
		return errors.New("program/flow/accessgeometry: Import Alias callee is not a canonical global Cell")
	}
	atom, exactOK := sourceView.Keys().Exact(exact)
	if !exactOK || atom.Kind != keyspace.LiteralString || atom.String != "require" {
		return errors.New("program/flow/accessgeometry: Import Alias global Cell is not require")
	}
	if role, roleOK := bindings.Role(alias); !roleOK || role != kind.CellLocal {
		return errors.New("program/flow/accessgeometry: Import Alias is not a Bindings-local Cell")
	}
	host, hostOK := bindings.Host(alias)
	if !hostOK || keyspace.TermFamily(host) != keyspace.FamilyBind || keyspace.TermOrdinal(host) == 0 {
		return errors.New("program/flow/accessgeometry: Import Alias has no unique Bind host")
	}
	binds := flow.Storage().Binds()
	bindOwner, valuesTerm, bindOK := binds.Get(host)
	if !bindOK || bindOwner != callOwner {
		return errors.New("program/flow/accessgeometry: Import Alias Bind owner disagrees")
	}
	aliasCellKind, aliasBody, aliasKey, aliasOK := cells.Get(alias)
	if !aliasOK || aliasCellKind != authored.CellLocal || aliasBody != callOwner || aliasKey != 0 {
		return errors.New("program/flow/accessgeometry: Import Alias is not a same-owner local Cell")
	}
	order := sourceView.Binds()
	if length, lengthOK := order.Len(host); !lengthOK || length != 1 {
		return errors.New("program/flow/accessgeometry: Import Alias Source Bind order is not exactly one Cell")
	}
	orderedAlias, orderedOK := order.At(host, 0)
	if !orderedOK || orderedAlias != alias {
		return errors.New("program/flow/accessgeometry: Import Alias Source Bind order disagrees")
	}
	values := flow.Values()
	valuesOwner, tail, valuesOK := values.Get(valuesTerm)
	if !valuesOK || valuesOwner != callOwner {
		return errors.New("program/flow/accessgeometry: Import Alias Bind Values is not closed and same-owner")
	}
	length, lengthOK := values.Len(valuesTerm)
	position, positionOK := values.Position(valuesTerm, 0)
	if !lengthOK || !positionOK {
		return errors.New("program/flow/accessgeometry: Import Alias Bind Values has no first position")
	}
	// A one-cell Lua Bind consumes the first result of its RHS.  A direct
	// require Call may therefore be represented either as the sole fixed
	// value or as the open tail at position zero.  Preserve the exact Call
	// identity in both forms; nil-fill, a later tail offset, extra fixed
	// values, and a fixed value plus an open tail are not this alias shape.
	if position.Fixed == call {
		if tail != 0 || length != 1 {
			return errors.New("program/flow/accessgeometry: Import Alias Bind Values is not exactly one fixed member")
		}
	} else if position.Tail == call && position.TailOffset == 0 {
		if length != 0 {
			return errors.New("program/flow/accessgeometry: Import Alias Bind Values has unexpected fixed members")
		}
	} else {
		return errors.New("program/flow/accessgeometry: Import Alias Bind Values does not equal the Import Call")
	}
	return nil
}
