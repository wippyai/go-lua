// Package parseruses owns exact parser-product consumption contexts.
package parseruses

import (
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/occurrence"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/parserproducts"
)

// UseRole is a compact label on an exact parent slot. The slot is always the
// authority; the label makes the high-impact evaluation distinctions visible
// without introducing an overlapping context language. Module has no parser
// AST role in this dialect and is therefore deliberately absent here: it is a
// Program Module relation, pending that owner's output vector.
type UseRole uint8

const (
	UseRoleInvalid UseRole = iota
	UseRoleChild
	UseRoleLValue
	UseRoleControl
	UseRoleStatic
	UseRoleAdjustment
)

// ChildClass is the semantic carrier accepted by an exact parent field.
// It avoids collapsing expressions, statement blocks, static forms, and
// parser-structural children into one generic AST child edge.
type ChildClass uint8

const (
	ChildClassInvalid ChildClass = iota
	ChildClassValue
	ChildClassStatement
	ChildClassStatic
	ChildClassStructural
	ChildClassAdjustment
)

// ProgramUseClass is the fixed parser-use projection of the Program-use
// denominator. Its values classify parser AST-use coordinates only: they are
// neither Target API handles nor relation-owner rows.
type ProgramUseClass uint8

const (
	ProgramUseInvalid ProgramUseClass = iota
	ProgramUseExpression
	ProgramUseValues
	ProgramUseControl
	ProgramUseStatic
	ProgramUseTableKey
	ProgramUseTableBracketKey
	ProgramUseTableValue
	ProgramUseLValue
	ProgramUseAdjustment
	ProgramUseStatements
	ProgramUseFunctionBody
	ProgramUseFunctionName
	ProgramUseFunctionParameters
	ProgramUseTableFields
	ProgramUseFunctionDefinition
	ProgramUseAnnotations
	ProgramUseRecordFields
	ProgramUseTypeParameters
)

// OpenAxis records whether a direct use is empty, closed, or has a final-open
// Values member. It is derived only from an exact parser list route; callers
// must not collapse an absent list into an open position.
type OpenAxis uint8

const (
	OpenAxisInvalid OpenAxis = iota
	OpenAxisNotApplicable
	OpenAxisEmpty
	OpenAxisClosed
	OpenAxisFinalOpen
)

// TableAxis preserves the parser distinction among ordinary uses, array
// fields, named fields, bracket/dynamic fields, and table values.
type TableAxis uint8

const (
	TableAxisInvalid TableAxis = iota
	TableAxisNone
	TableAxisArray
	TableAxisNamed
	TableAxisBracket
	TableAxisDynamic
	TableAxisValue
)

// LValueAxis classifies this direct child edge only. Write eligibility of a
// constructed AttrGet/identifier root is represented separately by LValuePath,
// so its Object and Key children remain ordinary reads.
type LValueAxis uint8

const (
	LValueAxisInvalid LValueAxis = iota
	LValueAxisNo
	LValueAxisYes
)

// ValuesPosition distinguishes ordinary scalar evaluation from the lexical
// non-final and final-open positions of a Lua expression list.
type ValuesPosition uint8

const (
	ValuesPositionInvalid ValuesPosition = iota
	ValuesPositionNotApplicable
	ValuesPositionScalar
	ValuesPositionNonFinal
	ValuesPositionFinalOpen
)

// SequenceCoordinate names one parserproducts.SequenceLaw operand without
// copying its construction, segment tag, or action formula. ProductLaw joins
// make Production the identity boundary; Destination disambiguates a
// parser-private field within that production.
type SequenceCoordinate struct {
	Production string
	Tag        string
	Field      string
	Segment    int
}

// ValuesTail records one exact ProgramUseValues projection of a sealed
// sequence operand. Successor is another segment coordinate in the same
// sequence, present only when this operand is made non-final by that later
// operand. Its term and segment kind remain parserproducts authority.
type ValuesTail struct {
	Sequence  SequenceCoordinate
	Position  ValuesPosition
	Successor int
}

// LValuePath is a generated grammar projection from one assignment target
// through one sealed sequence element coordinate to a terminal constructor. It
// makes AttrGet/identifier target context explicit without giving their
// Object or Key children lvalue status. Sequence carries only a source coordinate;
// parserproducts remains the sole owner of sequence semantics.
type LValuePath struct {
	SeedProduction     string
	SeedOrdinal        int
	Sequence           SequenceCoordinate
	TerminalProduction string
	TerminalOrdinal    int
	TerminalForm       string
}

// UseSlot is one direct, factorized AST child-use context. ParentForm and
// ParentField are the exact semantic role; ChildType and Cardinality describe
// its declared child carrier. This preserves condition, lvalue, control,
// static, table, and nested-child distinctions without inventing a second
// generic context language.
type UseSlot struct {
	ParentForm    string
	ParentField   string
	ParentContext occurrence.Context
	Role          UseRole
	ChildType     string
	Cardinality   astcodec.FieldState
	// Target is the sole semantic owner of this schema-declared carrier. It
	// remains present even when the yacc action constructs the enclosing AST
	// form through local assembly rather than a root Result literal.
	Target      ProgramUseClass
	Disposition parserproducts.Disposition
	Source      string
	ParserLaw   occurrence.ParserLaw
}

// UsePath is one direct parser-action child edge. Term is an ID in the one
// parserproducts ActionTerms arena; this package never parses an expression.
type UsePath struct {
	ParentProduction string
	ParentOrdinal    int
	ParentForm       string
	ParentField      string
	Term             parserproducts.ActionTermID
	Role             UseRole
	Child            ChildClass
	Target           ProgramUseClass
	Open             OpenAxis
	Table            TableAxis
	LValue           LValueAxis
	Values           ValuesPosition
}

// HelperUsePath is one exact helper-product use. Applications is the
// one-based path through sealed HelperApplication vectors: the first
// coordinate belongs to the ProductLaw and later coordinates belong to the
// preceding HelperLaw. Instance is the one non-materializing substitution
// authority; parser uses never renders or reparses a substituted expression.
type HelperUsePath struct {
	Production   string
	Applications []uint16
	Helper       parserproducts.ActionSymbolID
	Ordinal      int
	ParentForm   string
	ParentField  string
	Instance     parserproducts.TermInstance
	Role         UseRole
	Child        ChildClass
	Target       ProgramUseClass
	Open         OpenAxis
	Table        TableAxis
	LValue       LValueAxis
	Values       ValuesPosition
}

// MutationUsePath carries one exact sealed edit into a semantic use. Edit
// retains typed Place and ActionTerm evidence; an EditAppend already names
// its new element in Value, so parser uses never inspects an append-shaped
// text expression.
type MutationUsePath struct {
	Production string
	Ordinal    int
	Edit       parserproducts.Edit
	Role       UseRole
	Child      ChildClass
	Target     ProgramUseClass
	Open       OpenAxis
	Table      TableAxis
	LValue     LValueAxis
}

// Evidence is the parser-use proof. It depends on exactly one parser-product
// evidence identity and retains no source-grammar or foreign-owner copy.
type Evidence struct {
	ProductsDigest   string
	Digest           string
	UseSlots         []UseSlot
	UsePaths         []UsePath
	HelperUsePaths   []HelperUsePath
	MutationUsePaths []MutationUsePath
	ValuesTails      []ValuesTail
	LValuePaths      []LValuePath
}

// Generated is assigned by checked-in generated evidence.
var Generated Evidence
