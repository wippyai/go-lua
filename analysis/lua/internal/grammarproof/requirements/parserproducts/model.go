// Package parserproducts owns cold parser construction evidence.
//
//go:generate go run ./cmd/generate -root ../../../../../../ -out evidence_gen.go
package parserproducts

import (
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/occurrence"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/recursion"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

type Disposition uint8

const (
	DispositionInvalid Disposition = iota
	DispositionObserved
	DispositionSemanticWitness
	DispositionImpossible
	DispositionPublicIngressRejected
)

// FieldState is one factor of the dependent parser-schema relation.
type FieldState struct {
	Form        string
	Field       string
	State       astcodec.FieldState
	Context     occurrence.Context
	Disposition Disposition
	Source      string
	ParserLaw   occurrence.ParserLaw
	SemanticLaw occurrence.SemanticLaw
	IngressLaw  occurrence.IngressLaw
}

// Product is one observed legal whole-constructor field vector.
type Product struct {
	Form    string
	Context occurrence.Context
	States  []astcodec.FieldState
	Source  string
}

type ActionForm uint8

const (
	ActionFormInvalid ActionForm = iota
	ActionFormDirectConstruct
	ActionFormForward
	ActionFormAssembly
)

// ActionValueKind distinguishes an omitted constructor coordinate from a
// term in Evidence.ActionTerms.
type ActionValueKind uint8

const (
	ActionValueInvalid ActionValueKind = iota
	ActionValueZero
	ActionValueTerm
)

// ProductField is one complete constructor-vector coordinate.
type ProductField struct {
	Field string
	Kind  ActionValueKind
	Term  ActionTermID
}

type ConstructorProduct struct {
	Ordinal     int
	Guard       Guard
	Constructor string
	Fields      []ProductField
}

// HelperApplication is one exact call edge. Actuals and destinations belong
// to Scope; Helper is a canonical callable symbol owned by a helper scope.
type HelperApplication struct {
	Helper  ActionSymbolID
	Scope   ActionScopeID
	Guard   Guard
	Actuals []ActionTermID
	Results []Place
}

type GuardAtomKind uint8

const (
	GuardAtomInvalid GuardAtomKind = iota
	GuardAtomNil
	GuardAtomLenEq
	GuardAtomEqConst
	GuardAtomTypeIn
	GuardAtomNumberParseClass
)

// NumberParseClass is the closed outcome set of the parser's number literal
// recognizer. It is intentionally an enum rather than a diagnostic string.
type NumberParseClass uint8

const (
	NumberParseClassUnknown NumberParseClass = iota
	NumberParseClassInteger
	NumberParseClassFloat
	NumberParseClassInvalid
)

// GuardAtom is one finite predicate. Negated provides the closed complement;
// a Guard with no atoms is True, otherwise it is their conjunction.
type GuardAtom struct {
	Kind       GuardAtomKind
	Negated    bool
	Term       ActionTermID
	Constant   ActionSymbolID
	SetStart   uint32
	SetCount   uint16
	ParseClass NumberParseClass
}

type Guard struct {
	Atoms []GuardAtom
}

type GuardedReturn struct {
	Ordinal int
	Guard   Guard
	Values  []ActionTermID
}

type RejectCondition uint8

const (
	RejectConditionInvalid RejectCondition = iota
	RejectWhenAll
	RejectUnlessAll
)

type Reject struct {
	Ordinal    int
	Condition  RejectCondition
	Guard      Guard
	Diagnostic ActionSymbolID
}

// MapIndex is the finite summary of one same-index range. Input and Output
// name helper formal/result slots; Element is evaluated in ItemScope.
type MapIndex struct {
	Scope     ActionScopeID
	ItemScope ActionScopeID
	Input     uint16
	Output    uint16
	Element   ActionTermID
}

type PresencePredicateKind uint8

const (
	PresencePredicateInvalid PresencePredicateKind = iota
	PresencePredicateAnyNonNilField
)

type ConditionalPresence struct {
	Scope     ActionScopeID
	Output    uint16
	Predicate PresencePredicateKind
	Input     uint16
	ItemField ActionSymbolID
}

// HelperSummary retains the proven finite map/presence relation for its
// owning HelperLaw. It has no duplicate helper-name identity.
type HelperSummary struct {
	Maps     []MapIndex
	Presence []ConditionalPresence
}

// HelperLaw is a helper-scope relation. Scope identifies its callable and
// formals; parameter spellings are cold syntax only and never evidence.
type HelperLaw struct {
	Scope       ActionScopeID
	Disposition HelperDisposition
	Digest      string
	Returns     []GuardedReturn
	Rejects     []Reject
	Products    []ConstructorProduct
	Helpers     []HelperApplication
	Edits       []Edit
	Summary     HelperSummary
}

type HelperDisposition uint8

const (
	HelperDispositionInvalid HelperDisposition = iota
	HelperDispositionSemantic
	HelperDispositionMetadata
	HelperDispositionDiagnostic
)

// ProductLaw is one yacc action relation. Scope owns every action term and
// edit below it.
type ProductLaw struct {
	Production   string
	Nonterminal  string
	RHS          []string
	ActionDigest string
	Scope        ActionScopeID
	Form         ActionForm
	Forward      int
	Products     []ConstructorProduct
	Helpers      []HelperApplication
	Edits        []Edit
	Rejects      []Reject
	Chains       []ChainLaw
}

// ChainLaw records an ordered singleton link over an input sequence. Seed is
// the root node before the links; tails are fields written on the last node.
// TailStart/TailCount address ActionTerms.ChainTails.
type ChainLaw struct {
	Scope     ActionScopeID
	Guard     Guard
	Input     ActionTermID
	Seed      ActionTermID
	LinkField ActionSymbolID
	TailStart uint32
	TailCount uint16
}

type Carrier struct {
	Form        string
	Field       string
	Class       parsersource.ConstructorClass
	ChildType   string
	Cardinality astcodec.FieldState
}

type SequenceConstruction uint8

const (
	SequenceConstructionInvalid SequenceConstruction = iota
	SequenceConstructionNil
	SequenceConstructionLiteral
	SequenceConstructionForward
	SequenceConstructionAppend
)

type SequenceSegmentKind uint8

const (
	SequenceSegmentInvalid SequenceSegmentKind = iota
	SequenceElement
	SequenceSpread
)

type SequenceSegment struct {
	Kind SequenceSegmentKind
	Term ActionTermID
}

type SequenceDestination struct {
	Tag   string
	Field string
}

type SequenceLaw struct {
	Production   string
	Scope        ActionScopeID
	Destination  SequenceDestination
	Construction SequenceConstruction
	Segments     []SequenceSegment
}

// FieldMutation attributes a parser action edit to its owning production.
type FieldMutation struct {
	Production string
	Edit       Edit
}

// Evidence is the complete parser-construction proof. ActionTerms is the
// only semantic-expression authority in this package.
type Evidence struct {
	GrammarDigest      string
	ParserSourceDigest string
	SchemaDigest       string
	IngressDigest      string
	Digest             string
	Fields             []FieldState
	Products           []Product
	ProductLaws        []ProductLaw
	HelperLaws         []HelperLaw
	Sequences          []SequenceLaw
	Mutations          []FieldMutation
	ActionTerms        ActionTerms
	Carriers           []Carrier
	Recursion          []recursion.Obligation
}

func (e Evidence) FieldCount() int   { return len(e.Fields) }
func (e Evidence) ProductCount() int { return len(e.Products) }
