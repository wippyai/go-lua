package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
)

// Authored relation views.  Each name below is the single authored relation
// type published under its public name; the private package keeps every
// constructor and lifecycle operation unexported, so the capability fence is
// unchanged.
type (
	Values        = authored.Values
	Tables        = authored.Tables
	Fields        = authored.Fields
	Access        = authored.Access
	ExactLenses   = authored.ExactLenses
	DynamicLenses = authored.DynamicLenses
	Storage       = authored.Storage
	Cells         = authored.Cells
	Reads         = authored.Reads
	Varargs       = authored.Varargs
	Binds         = authored.Binds
	Assigns       = authored.Assigns
	Writes        = authored.Writes
	Functions     = authored.Functions
	Calls         = authored.Calls
	Operators     = authored.Operators
	Unaries       = authored.Unaries
	Binaries      = authored.Binaries
	Selects       = authored.Selects
	Returns       = authored.Returns
	Labels        = authored.Labels
	Gotos         = authored.Gotos
	Branches      = authored.Branches
	Loops         = authored.Loops
	Claims        = authored.Claims
	TypeValues    = authored.TypeValues
	Control       = authored.Control
	Breaks        = authored.Breaks
)

// Authored is the authored-relation query surface. It withholds the private
// Cold projection and publishes only its content identity.
type Authored struct{ view authored.View }

func (view Authored) ContentID() identity.ContentID { return view.view.Cold().ContentID() }
func (view Authored) Values() Values                { return view.view.Values() }
func (view Authored) Tables() Tables                { return view.view.Tables() }
func (view Authored) Fields() Fields                { return view.view.Fields() }
func (view Authored) Access() Access                { return view.view.Access() }
func (view Authored) Storage() Storage              { return view.view.Storage() }
func (view Authored) Functions() Functions          { return view.view.Functions() }
func (view Authored) Calls() Calls                  { return view.view.Calls() }
func (view Authored) Operators() Operators          { return view.view.Operators() }
func (view Authored) Control() Control              { return view.view.Control() }
func (view Authored) Claims() Claims                { return view.view.Claims() }
func (view Authored) TypeValues() TypeValues        { return view.view.TypeValues() }
