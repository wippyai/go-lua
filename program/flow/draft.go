// Package flow is the sole public Flow semantic owner.
//
// The authored relation is deliberately kept in internal/authored.  This
// file exposes only the lowerer's construction vocabulary and a construction
// Draft; authored Views and finalizers never cross this package boundary.
package flow

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
)

// Construction vocabulary.  These aliases keep lowering independent of the
// private storage package while exposing no authored lifecycle or query
// authority.  Build is the only operation that turns this vocabulary into a
// Draft, and Assemble is the only operation that consumes that Draft.
type (
	Input          = authored.Input
	ValuesInput    = authored.ValuesInput
	Value          = authored.Value
	AccessInput    = authored.AccessInput
	ExactLens      = authored.ExactLens
	DynamicLens    = authored.DynamicLens
	StorageInput   = authored.StorageInput
	Cell           = authored.Cell
	CellKind       = authored.CellKind
	Read           = authored.Read
	Vararg         = authored.Vararg
	Bind           = authored.Bind
	Assign         = authored.Assign
	Write          = authored.Write
	TablesInput    = authored.TablesInput
	Table          = authored.Table
	Field          = authored.Field
	FunctionsInput = authored.FunctionsInput
	Function       = authored.Function
	Capture        = authored.Capture
	Call           = authored.Call
	Return         = authored.Return
	Break          = authored.Break
	Label          = authored.Label
	Goto           = authored.Goto
	Branch         = authored.Branch
	Loop           = authored.Loop
	ControlInput   = authored.ControlInput
	OperatorsInput = authored.OperatorsInput
	Unary          = authored.Unary
	Binary         = authored.Binary
	Select         = authored.Select
	ValueClaim     = authored.ValueClaim
	TypeValue      = authored.TypeValue
	Range          = authored.Range
	Position       = authored.Position
)

const (
	CellLocal  = authored.CellLocal
	CellGlobal = authored.CellGlobal
)

// Draft is a construction-only Flow capability.  It has no public query,
// commit, or finalizer operation.  Copies share the private authored
// lifecycle; only Assemble can claim and consume it.
type Draft struct{ authored *authored.Draft }

// Build validates and copies the complete authored Flow vocabulary into one
// construction Draft.  Derived Outcome identities, exact-key joins,
// activation, evaluation ports, and all analysis projections are absent from
// this input and are created only by Assemble.
func Build(input Input) (*Draft, error) {
	owner, err := authored.Build(input)
	if err != nil {
		return nil, err
	}
	return &Draft{authored: owner}, nil
}

// Abort terminally discards an unclaimed authored Flow Draft. The root
// artifact rebuild uses this only when a later sibling Build fails, before
// Assemble has had a chance to claim Flow. Keeping the operation here avoids
// leaving an earlier construction capability live across a failed rebuild.
func (draft *Draft) claim() (authored.Finalizer, error) {
	if draft == nil || draft.authored == nil {
		return authored.Finalizer{}, errors.New("program/flow: nil or consumed Draft")
	}
	return draft.authored.Finalizer()
}
