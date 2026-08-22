package contract

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// SealedColumn is one immutable value sealed with the Contract by the owner
// that constructed it. Contract never interprets the value: it carries it and
// frames its identity, exactly as it carries every other sealed column.
//
// The seam exists because some columns of the sealed target are semantic
// rather than structural, and their meaning is owned above this package. The
// type-contract Semantics adapter crosses the same boundary in the same
// direction, for the same reason: the target model stores the declaration and
// asks its owner to interpret it.
type SealedColumn struct {
	// ID is the portable identity of Value. It is framed into the whole
	// contract identity, so a column that changes changes the contract.
	ID identity.ContentID
	// Value is the sealed column itself. Its type is private to the owner that
	// produced it and to the consumer that was written against that owner.
	Value any
}

// Available reports whether this column carries an identified value.
func (column SealedColumn) Available() bool {
	return column.Value != nil && column.ID.Available()
}

// ColumnSealer derives one sealed column over the finished operation core.
// It runs once, inside the Contract constructor's caller, after every table
// the column reads is final and before the Contract pointer escapes.
//
// The seam is carried by the semantic adapter every target already names,
// because a semantic column is exactly what that adapter is the authority
// for. A target therefore never declares its column separately from the
// domain that gives its declarations meaning.
type ColumnSealer interface {
	SealColumn(operations operationvalue.Core) (SealedColumn, error)
}

// SealColumn derives the sealed column of one target from the semantic
// adapter it names. An adapter that seals no column yields none, and a
// contract without one carries its absence into its identity.
func SealColumn(semantics schematype.Semantics, operations operationvalue.Core) (SealedColumn, error) {
	sealer, sealing := semantics.(ColumnSealer)
	if !sealing {
		return SealedColumn{}, nil
	}
	column, err := sealer.SealColumn(operations)
	if err != nil {
		return SealedColumn{}, err
	}
	if !column.Available() {
		return SealedColumn{}, errors.New("target: column sealer produced an unidentified column")
	}
	return column, nil
}

// Column returns the sealed semantic column carried by this Contract.
func (c *Contract) Column() SealedColumn {
	if c == nil {
		return SealedColumn{}
	}
	return c.column
}
