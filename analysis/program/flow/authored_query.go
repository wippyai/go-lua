package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type Authored struct{ view authored.View }

func (view Authored) ContentID() identity.ContentID { return view.view.Cold().ContentID() }
func (view Authored) Values() Values                { return Values{view: view.view.Values()} }
func (view Authored) Tables() Tables                { return Tables{view: view.view.Tables()} }
func (view Authored) Fields() Fields                { return Fields{view: view.view.Fields()} }
func (view Authored) Access() Access                { return Access{view: view.view.Access()} }
func (view Authored) Storage() Storage              { return Storage{view: view.view.Storage()} }
func (view Authored) Functions() Functions          { return Functions{view: view.view.Functions()} }
func (view Authored) Calls() Calls                  { return Calls{view: view.view.Calls()} }
func (view Authored) Operators() Operators          { return Operators{view: view.view.Operators()} }
func (view Authored) Control() Control              { return Control{view: view.view.Control()} }
func (view Authored) Claims() Claims                { return Claims{view: view.view.Claims()} }
func (view Authored) TypeValues() TypeValues        { return TypeValues{view: view.view.TypeValues()} }

type Values struct{ view authored.Values }

func (view Values) Count() int                         { return view.view.Count() }
func (view Values) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Values) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	return view.view.Get(term)
}
func (view Values) Len(term keyspace.Term) (int, bool) { return view.view.Len(term) }
func (view Values) Member(term keyspace.Term, index int) (keyspace.Term, bool) {
	return view.view.Member(term, index)
}
func (view Values) Position(term keyspace.Term, index int) (Position, bool) {
	return view.view.Position(term, index)
}

type Tables struct{ view authored.Tables }

func (view Tables) Count() int                                   { return view.view.Count() }
func (view Tables) At(index int) (keyspace.Term, bool)           { return view.view.At(index) }
func (view Tables) Get(term keyspace.Term) (keyspace.Term, bool) { return view.view.Get(term) }
func (view Tables) FieldCount(term keyspace.Term) (int, bool)    { return view.view.FieldCount(term) }
func (view Tables) FieldAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	return view.view.FieldAt(term, index)
}

type Fields struct{ view authored.Fields }

func (view Fields) Count() int                         { return view.view.Count() }
func (view Fields) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Fields) Get(term keyspace.Term) (table, key, values keyspace.Term, fieldKind kind.FieldKind, ok bool) {
	return view.view.Get(term)
}
func (view Fields) Values(term keyspace.Term) (keyspace.Term, bool, bool) {
	return view.view.Values(term)
}

type Access struct{ view authored.Access }

func (view Access) Exact() ExactLenses     { return ExactLenses{view: view.view.Exact()} }
func (view Access) Dynamic() DynamicLenses { return DynamicLenses{view: view.view.Dynamic()} }

type ExactLenses struct{ view authored.ExactLenses }

func (view ExactLenses) Count() int                         { return view.view.Count() }
func (view ExactLenses) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view ExactLenses) Get(term keyspace.Term) (owner, base, source keyspace.Term, fieldKind kind.FieldKind, ok bool) {
	return view.view.Get(term)
}

type DynamicLenses struct{ view authored.DynamicLenses }

func (view DynamicLenses) Count() int                         { return view.view.Count() }
func (view DynamicLenses) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view DynamicLenses) Get(term keyspace.Term) (owner, base, key keyspace.Term, ok bool) {
	return view.view.Get(term)
}

type Storage struct{ view authored.Storage }

func (view Storage) Cells() Cells     { return Cells{view: view.view.Cells()} }
func (view Storage) Reads() Reads     { return Reads{view: view.view.Reads()} }
func (view Storage) Varargs() Varargs { return Varargs{view: view.view.Varargs()} }
func (view Storage) Binds() Binds     { return Binds{view: view.view.Binds()} }
func (view Storage) Assigns() Assigns { return Assigns{view: view.view.Assigns()} }
func (view Storage) Writes() Writes   { return Writes{view: view.view.Writes()} }

type Cells struct{ view authored.Cells }

func (view Cells) Count() int                         { return view.view.Count() }
func (view Cells) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Cells) Get(term keyspace.Term) (CellKind, keyspace.Term, keyspace.Key, bool) {
	return view.view.Get(term)
}

type Reads struct{ view authored.Reads }

func (view Reads) Count() int                         { return view.view.Count() }
func (view Reads) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Reads) Get(term keyspace.Term) (owner, source keyspace.Term, implicit, ok bool) {
	return view.view.Get(term)
}
func (view Reads) ImplicitCount() int                         { return view.view.ImplicitCount() }
func (view Reads) ImplicitAt(index int) (keyspace.Term, bool) { return view.view.ImplicitAt(index) }

type Varargs struct{ view authored.Varargs }

func (view Varargs) Count() int                         { return view.view.Count() }
func (view Varargs) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Varargs) Get(term keyspace.Term) (owner, cell keyspace.Term, ok bool) {
	return view.view.Get(term)
}

type Binds struct{ view authored.Binds }

func (view Binds) Count() int                         { return view.view.Count() }
func (view Binds) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Binds) Get(term keyspace.Term) (owner, values keyspace.Term, ok bool) {
	return view.view.Get(term)
}

type Assigns struct{ view authored.Assigns }

func (view Assigns) Count() int                         { return view.view.Count() }
func (view Assigns) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Assigns) Get(term keyspace.Term) (owner, values keyspace.Term, ok bool) {
	return view.view.Get(term)
}
func (view Assigns) WriteCount(term keyspace.Term) (int, bool) { return view.view.WriteCount(term) }
func (view Assigns) WriteAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	return view.view.WriteAt(term, index)
}

type Writes struct{ view authored.Writes }

func (view Writes) Count() int                         { return view.view.Count() }
func (view Writes) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Writes) Get(term keyspace.Term) (assign, target keyspace.Term, ok bool) {
	return view.view.Get(term)
}

type Functions struct{ view authored.Functions }

func (view Functions) Count() int                         { return view.view.Count() }
func (view Functions) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Functions) Get(term keyspace.Term) (owner, body, vararg keyspace.Term, ok bool) {
	return view.view.Get(term)
}
func (view Functions) CaptureCount(term keyspace.Term) (int, bool) {
	return view.view.CaptureCount(term)
}
func (view Functions) CaptureAt(term keyspace.Term, index int) (keyspace.Term, keyspace.Term, bool) {
	return view.view.CaptureAt(term, index)
}

type Calls struct{ view authored.Calls }

func (view Calls) Count() int                         { return view.view.Count() }
func (view Calls) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Calls) Get(term keyspace.Term) (owner, callee, receiver, actuals keyspace.Term, ok bool) {
	return view.view.Get(term)
}

type Operators struct{ view authored.Operators }

func (view Operators) Unaries() Unaries   { return Unaries{view: view.view.Unaries()} }
func (view Operators) Binaries() Binaries { return Binaries{view: view.view.Binaries()} }
func (view Operators) Selects() Selects   { return Selects{view: view.view.Selects()} }

type Unaries struct{ view authored.Unaries }

func (view Unaries) Count() int                         { return view.view.Count() }
func (view Unaries) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Unaries) Get(term keyspace.Term) (owner keyspace.Term, op kind.UnaryOp, operand keyspace.Term, ok bool) {
	return view.view.Get(term)
}

type Binaries struct{ view authored.Binaries }

func (view Binaries) Count() int                         { return view.view.Count() }
func (view Binaries) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Binaries) Get(term keyspace.Term) (owner keyspace.Term, op kind.BinaryOp, left, right keyspace.Term, ok bool) {
	return view.view.Get(term)
}

type Selects struct{ view authored.Selects }

func (view Selects) Count() int                         { return view.view.Count() }
func (view Selects) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Selects) Get(term keyspace.Term) (owner keyspace.Term, op kind.SelectOp, left, right keyspace.Term, ok bool) {
	return view.view.Get(term)
}

type Control struct{ view authored.Control }

func (view Control) Returns() Returns   { return Returns{view: view.view.Returns()} }
func (view Control) Breaks() Breaks     { return Breaks{view: view.view.Breaks()} }
func (view Control) Labels() Labels     { return Labels{view: view.view.Labels()} }
func (view Control) Gotos() Gotos       { return Gotos{view: view.view.Gotos()} }
func (view Control) Branches() Branches { return Branches{view: view.view.Branches()} }
func (view Control) Loops() Loops       { return Loops{view: view.view.Loops()} }

type Returns struct{ view authored.Returns }

func (view Returns) Count() int                         { return view.view.Count() }
func (view Returns) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Returns) Get(term keyspace.Term) (owner, values keyspace.Term, ok bool) {
	return view.view.Get(term)
}

type Breaks struct{ view authored.Breaks }

func (view Breaks) Count() int                                   { return view.view.Count() }
func (view Breaks) At(index int) (keyspace.Term, bool)           { return view.view.At(index) }
func (view Breaks) Get(term keyspace.Term) (keyspace.Term, bool) { return view.view.Target(term) }
func (view Breaks) Owner(term keyspace.Term) (keyspace.Term, bool) {
	return view.view.Get(term)
}

type Labels struct{ view authored.Labels }

func (view Labels) Count() int                                   { return view.view.Count() }
func (view Labels) At(index int) (keyspace.Term, bool)           { return view.view.At(index) }
func (view Labels) Get(term keyspace.Term) (keyspace.Term, bool) { return view.view.Get(term) }

type Gotos struct{ view authored.Gotos }

func (view Gotos) Count() int                         { return view.view.Count() }
func (view Gotos) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Gotos) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	return view.view.Get(term)
}

type Branches struct{ view authored.Branches }

func (view Branches) Count() int                         { return view.view.Count() }
func (view Branches) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Branches) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, keyspace.Term, keyspace.Term, bool) {
	return view.view.Get(term)
}

type Loops struct{ view authored.Loops }

func (view Loops) Count() int                         { return view.view.Count() }
func (view Loops) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Loops) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, kind.LoopKind, keyspace.Term, bool) {
	return view.view.Get(term)
}
func (view Loops) CellCount(term keyspace.Term) (int, bool) { return view.view.CellCount(term) }
func (view Loops) CellAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	return view.view.CellAt(term, index)
}

type Claims struct{ view authored.Claims }

func (view Claims) Count() int                         { return view.view.Count() }
func (view Claims) At(index int) (keyspace.Term, bool) { return view.view.At(index) }
func (view Claims) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, kind.ValueClaimKind, bool) {
	return view.view.Get(term)
}

type TypeValues struct{ view authored.TypeValues }

func (view TypeValues) Count() int                                   { return view.view.Count() }
func (view TypeValues) At(index int) (keyspace.Term, bool)           { return view.view.At(index) }
func (view TypeValues) Get(term keyspace.Term) (keyspace.Term, bool) { return view.view.Get(term) }
