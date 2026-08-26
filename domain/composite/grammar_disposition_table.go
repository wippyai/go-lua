package composite

import (
	"github.com/wippyai/go-lua/analysis/schema"
)

// The grammar disposition table is the seal-time account of the Lua surface the
// analyzer admits. The parser census (analysis/lua/census) is the denominator:
// it derives, from parser.go.y and the compiler/ast declarations alone, one row
// for every parser alternative, every AST form those alternatives construct,
// and every exported carrier of those forms. This table is the numerator: it
// states, for each of those rows, what accounts for it.
//
// The two halves are joined by JoinGrammarCensus. The join is total in both
// directions - a census row with no disposition and a disposition naming no
// census row are both failures - so a new parser production, a new AST form, or
// a new carrier cannot enter the language without an author stating what
// accounts for it. That totality is the whole guarantee: nothing here observes
// a fixture, and nothing here is regenerated, because a table a generator
// refreshes would absorb a new form silently and prove nothing.
//
// Rule attribution is read from the artifact's own occurrence-to-role switch
// (analysis/program/artifact/occurrences.go, deriveRuleOccurrencesFailure). A
// form whose Program occurrence has no case in that switch is owned by no rule
// and says so with a reason, rather than being attributed to a plausible one.

// grammarCensusAuthority pins the exact census this account was authored
// against. Row totality alone is not enough: a semantic action rewritten in
// place keeps its production key and changes only what it builds, so the join
// would still close over the same row set while accounting for an action that
// no longer exists. Pinning the census digest makes every parser or AST change
// reach an author, and the pin is raised by whoever re-reads the affected rows.
const grammarCensusAuthority = "2712e2c51728c517ae41cd2a5a9ac8c12bcd36da5019dc3055659a0ffdbed6ef"

// GrammarRow is one parser census row key. The three prefixes are the three
// grains the census publishes: "production:" for a parser.go.y alternative,
// "form:" for an AST declaration the parser constructs, and "carrier:" for one
// exported field of such a form.
type GrammarRow string

// GrammarDisposition is the closed account one census row can be given.
type GrammarDisposition uint8

const (
	grammarDispositionInvalid GrammarDisposition = iota
	// grammarRuleOwned: the row's semantics reach the sealed rule surface. The
	// entry names every rule role the row's Program occurrences are issued to.
	grammarRuleOwned
	// grammarStructural: no rule template owns the row. The reason states why,
	// and the reasons are distinct: parser plumbing, a source coordinate, and a
	// computation the Program carries under no rule are different facts.
	grammarStructural
	// grammarRejected: the row is parser-reachable and the public lowering
	// ingress refuses it.
	grammarRejected
	// grammarParserImpossible: the row cannot arise from a successful parse.
	grammarParserImpossible
)

// GrammarReason is the stable account a non-rule-owned row carries. It is an
// ordinal identity, not a rendered sentence: a row's reason is compared, and a
// caller that wants prose asks String.
type GrammarReason uint8

const (
	grammarReasonInvalid GrammarReason = iota
	// grammarReasonPassThrough: the reduction threads an already-built value.
	grammarReasonPassThrough
	// grammarReasonListAccumulation: the reduction extends its own sequence.
	grammarReasonListAccumulation
	// grammarReasonEmptyAlternative: the reduction matches no symbol.
	grammarReasonEmptyAlternative
	// grammarReasonTokenForward: the reduction yields a lexical token.
	grammarReasonTokenForward
	// grammarReasonParserPlumbing: the reduction rearranges symbols and builds
	// no AST value.
	grammarReasonParserPlumbing
	// grammarReasonLexicalToken: the row is the lexical token form or one of
	// its carriers.
	grammarReasonLexicalToken
	// grammarReasonParserComponent: the row is a structural AST component that
	// only ever reaches the analyzer inside the form declaring it.
	grammarReasonParserComponent
	// grammarReasonSourceCoordinate: the carrier is a source position.
	grammarReasonSourceCoordinate
	// grammarReasonStaticTypeSurface: the row reaches the static type surface,
	// which issues no rule occurrence.
	grammarReasonStaticTypeSurface
	// grammarReasonControlFlow: the row lowers into Program control flow, which
	// issues no rule occurrence.
	grammarReasonControlFlow
	// grammarReasonUnownedComputation: the row lowers into a Program
	// computation - a unary primitive, a short-circuit selection, a value
	// claim, or a binary operator outside the arithmetic, equality, and order
	// partitions - for which the artifact's occurrence-to-role switch has no
	// case.
	grammarReasonUnownedComputation
	// grammarReasonFunctionBody: the row declares a callable body. Rule
	// observation happens at the activating call site, not at the declaration.
	grammarReasonFunctionBody
)

func (reason GrammarReason) Available() bool {
	return reason > grammarReasonInvalid && reason <= grammarReasonFunctionBody
}

func (reason GrammarReason) String() string {
	switch reason {
	case grammarReasonPassThrough:
		return "pass-through"
	case grammarReasonListAccumulation:
		return "list-accumulation"
	case grammarReasonEmptyAlternative:
		return "empty-alternative"
	case grammarReasonTokenForward:
		return "token-forward"
	case grammarReasonParserPlumbing:
		return "parser-plumbing"
	case grammarReasonLexicalToken:
		return "lexical-token"
	case grammarReasonParserComponent:
		return "parser-component"
	case grammarReasonSourceCoordinate:
		return "source-coordinate"
	case grammarReasonStaticTypeSurface:
		return "static-type-surface"
	case grammarReasonControlFlow:
		return "control-flow"
	case grammarReasonUnownedComputation:
		return "unowned-computation"
	case grammarReasonFunctionBody:
		return "function-body"
	default:
		return "invalid"
	}
}

// grammarRoles is the authored set of rule families a grammar row reaches.
//
// These bits are deliberately stable tags, not rule-table positions.  The
// sealed rule table is free to insert a declaration (as it did for the
// runtime-kind call rule) without silently retargeting every later grammar
// disposition. The key/group tables below translate tags through canonical
// rule keys or issuance vocabulary; the sealed catalog remains the authority
// for whether those identities are actually declared.
type grammarRoles uint32

const (
	roleNone        grammarRoles = 0
	roleValueSource grammarRoles = 1 << iota
	rolePackSource
	roleHeapIngress
	roleValueAllocation
	roleHeapEmpty
	roleHeapClosed
	roleRawGet
	roleRawSet
	roleCallDispatch
	roleEffectSelected
	roleEffectOpaque
	roleEffectBody
	roleCallActivation
	roleCallOccurrence
	roleStorageTransfer
	roleArithmetic
	roleEquality
	roleOrder
	rolePresence
	roleAllocationOccurrence
	roleReturnBoundaryOccurrence
	roleStorageBindTransferOccurrence
	roleStorageWriteOccurrence
)

// Call-result projections consume the same parser/call issuance surface as
// dispatch. Keeping this family expression here makes that provenance
// explicit instead of adding a rule-specific grammar exception.
const roleCallSurface = roleCallDispatch | roleCallOccurrence

// grammarRoleKeys is the stable vocabulary for grammar roles.  It is keyed by
// the same schema identities the rule declarations publish; it does not
// repeat their mutable dense slot ordinals.
var grammarRoleKeys = [...]struct {
	mask grammarRoles
	key  schema.Key
}{
	{roleValueSource, "value-source"},
	{rolePackSource, "pack-source"},
	{roleHeapIngress, "heap-ingress"},
	{roleValueAllocation, "value-allocation"},
	{roleHeapEmpty, "heap-empty"},
	{roleHeapClosed, "heap-closed"},
	{roleRawGet, "raw-get"},
	{roleRawSet, "raw-set"},
	{roleCallDispatch, "call-dispatch"},
	{roleEffectSelected, "effect-selected"},
	{roleEffectOpaque, "effect-opaque"},
	{roleEffectBody, "effect-body"},
	{roleCallActivation, "call-activation"},
	{roleStorageTransfer, "value-transfer"},
	{roleStorageTransfer, "static-transfer"},
	{roleArithmetic, "value-binary-arithmetic"},
	{roleEquality, "value-binary-equality"},
	{roleOrder, "value-binary-order"},
	{rolePresence, "value-presence-refinement"},
}

// grammarRoleGroups are not rule declarations. They are the vocabulary of a
// grammar row's source surface, expanded against the sealed rule issuance
// table. Each group is deliberately expressed by its canonical occurrence
// identity, so a mounted consumer added to an existing occurrence family is
// reached without another Program-side rule-name restatement. Link consumers
// have no occurrence group because their denominators are assembled after the
// grammar boundary.
var grammarRoleGroups = [...]struct {
	mask       grammarRoles
	occurrence schema.Key
}{
	{roleCallOccurrence, "occurrence/call"},
	// A subject-liveness span is issued at the boundary of the call it is
	// anchored at, so the call form in the source is the grammar row that
	// reaches its consumers too. It is deliberately the same role bit: the
	// span has no source form of its own to attribute.
	{roleCallOccurrence, "occurrence/subject-liveness"},
	{roleAllocationOccurrence, "occurrence/allocation"},
	{roleReturnBoundaryOccurrence, "occurrence/return-boundary"},
	{roleStorageBindTransferOccurrence, "occurrence/storage-bind-transfer"},
	{roleStorageWriteOccurrence, "occurrence/storage-write"},
}

func grammarRoleMask(key schema.Key) (grammarRoles, bool) {
	for _, role := range grammarRoleKeys {
		if role.key == key {
			return role.mask, true
		}
	}
	return roleNone, false
}

func grammarRoleGroupMatches(state *catalog, mask grammarRoles, key schema.Key) bool {
	entry, ok := templateForKey(state, key)
	if !ok {
		return false
	}
	for _, group := range grammarRoleGroups {
		if mask&group.mask == 0 {
			continue
		}
		for index := 0; index < entry.IssuanceCount(); index++ {
			issuance, issuanceOK := entry.IssuanceAt(index)
			if issuanceOK && issuance.Occurrence == group.occurrence {
				return true
			}
		}
	}
	return false
}

func (roles grammarRoles) known() grammarRoles {
	known := roleNone
	for _, role := range grammarRoleKeys {
		known |= role.mask
	}
	for _, group := range grammarRoleGroups {
		known |= group.mask
	}
	return known
}

func (roles grammarRoles) has(state *catalog, key schema.Key) bool {
	mask, ok := grammarRoleMask(key)
	if ok && roles&mask != 0 {
		return true
	}
	return grammarRoleGroupMatches(state, roles, key)
}

// grammarEntry is one authored disposition. Roles is populated only for a
// rule-owned row and Reason only for every other kind, so an entry states one
// account and never two.
type grammarEntry struct {
	Row         GrammarRow
	Disposition GrammarDisposition
	Reason      GrammarReason
	Roles       grammarRoles
}

// grammarDispositions is the authored table. It is deliberately hand-owned: a
// census row added by a parser or AST change lands here through an author's
// judgment, which is the only thing that makes the join evidence.
var grammarDispositions = []grammarEntry{
	// production rows: one per parser.go.y alternative.
	{"production:afunctioncall#1", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:annotation#1", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:annotation#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:annotation#3", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:annotations#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:annotations#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:args#1", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:args#2", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:args#3", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:args#4", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:block#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:calltypeargs#1", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:chunk#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:chunk#2", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:chunk#3", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:chunk1#1", grammarStructural, grammarReasonEmptyAlternative, roleNone},
	{"production:chunk1#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:chunk1#3", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:closegt#1", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:closegt#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:elseifs#1", grammarStructural, grammarReasonEmptyAlternative, roleNone},
	{"production:elseifs#2", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:expr#1", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"production:expr#10", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#11", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#12", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"production:expr#13", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"production:expr#14", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"production:expr#15", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"production:expr#16", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"production:expr#17", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"production:expr#18", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#19", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#2", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"production:expr#20", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#21", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#22", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#23", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#24", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#25", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#26", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#27", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#28", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#29", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#3", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"production:expr#30", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"production:expr#31", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#32", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#33", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#34", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#35", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#36", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#37", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"production:expr#4", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"production:expr#5", grammarRuleOwned, grammarReasonInvalid, rolePackSource},
	{"production:expr#6", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:expr#7", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:expr#8", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:expr#9", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:exprlist#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:exprlist#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:field#1", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"production:field#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:field#3", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:fieldlist#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:fieldlist#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:fieldlist#3", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:fieldname#1", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:fieldname#2", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:fieldname#3", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:fieldname#4", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:fieldname#5", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:fieldname#6", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:fieldname#7", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:fieldname#8", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:fieldsep#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:fieldsep#2", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:funcbody#1", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"production:funcbody#2", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"production:funcbody#3", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"production:funcbody#4", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"production:funcname#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:funcname#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:funcname1#1", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer},
	{"production:funcname1#2", grammarRuleOwned, grammarReasonInvalid, roleRawGet | roleValueSource},
	{"production:funcparam#1", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:funcparam#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:funcparamlist#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:funcparamlist#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:funcparamlist#3", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:funcparamlist#4", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:function#1", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"production:functioncall#1", grammarRuleOwned, grammarReasonInvalid, roleCallActivation | roleCallSurface | roleEffectBody | roleEffectOpaque | roleEffectSelected | rolePackSource},
	{"production:functioncall#2", grammarRuleOwned, grammarReasonInvalid, roleCallActivation | roleCallSurface | roleEffectBody | roleEffectOpaque | roleEffectSelected | rolePackSource},
	{"production:functioncall#3", grammarRuleOwned, grammarReasonInvalid, roleCallActivation | roleCallSurface | roleEffectBody | roleEffectOpaque | roleEffectSelected | rolePackSource},
	{"production:functioncall#4", grammarRuleOwned, grammarReasonInvalid, roleCallActivation | roleCallSurface | roleEffectBody | roleEffectOpaque | roleEffectSelected | rolePackSource},
	{"production:interfacebody#1", grammarStructural, grammarReasonEmptyAlternative, roleNone},
	{"production:interfacebody#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:interfacebody#3", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:interfaceextends#1", grammarStructural, grammarReasonEmptyAlternative, roleNone},
	{"production:interfaceextends#2", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:interfaceextends#3", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:interfacemethod#1", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:interfacemethod#2", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:interfaceref#1", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:interfaceref#2", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:laststat#1", grammarRuleOwned, grammarReasonInvalid, roleReturnBoundaryOccurrence},
	{"production:laststat#2", grammarRuleOwned, grammarReasonInvalid, roleReturnBoundaryOccurrence},
	{"production:laststat#3", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:methodname#1", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:methodname#2", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:methodname#3", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:methodname#4", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:methodname#5", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:methodname#6", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:namelist#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:namelist#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:optionaltypeparams#1", grammarStructural, grammarReasonEmptyAlternative, roleNone},
	{"production:optionaltypeparams#2", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:parlist#1", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:parlist#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:parlist#3", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:parlist#4", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:parlist#5", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:prefixexp#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:prefixexp#2", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:prefixexp#3", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:prefixexp#4", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:primarytypeexpr#1", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#10", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#11", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#12", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#13", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#14", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#15", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#16", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#17", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#18", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#19", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#2", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#20", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#21", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#22", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#23", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#24", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#25", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#26", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#27", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#28", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#29", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#3", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#30", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#31", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#32", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#33", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#34", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#35", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#36", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#37", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#38", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#39", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#4", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#40", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#5", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#6", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#7", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:primarytypeexpr#8", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:primarytypeexpr#9", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:qualifiedtyperef#1", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:qualifiedtyperef#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:returntypeannot#1", grammarStructural, grammarReasonEmptyAlternative, roleNone},
	{"production:returntypeannot#2", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:returntypeannot#3", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:returntypeannot#4", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:simpletypeexpr#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:simpletypeexpr#2", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:simpletypeexpr#3", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:stat#1", grammarRuleOwned, grammarReasonInvalid, roleRawSet | roleStorageTransfer | roleStorageWriteOccurrence},
	{"production:stat#10", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"production:stat#11", grammarRuleOwned, grammarReasonInvalid, roleRawSet | roleStorageTransfer | roleStorageWriteOccurrence},
	{"production:stat#12", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"production:stat#13", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"production:stat#14", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"production:stat#15", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#16", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#17", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#18", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#19", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#2", grammarRuleOwned, grammarReasonInvalid, roleCallActivation | roleCallSurface | roleEffectBody | roleEffectOpaque | roleEffectSelected | rolePackSource},
	{"production:stat#3", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#4", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#5", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#6", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#7", grammarStructural, grammarReasonControlFlow, roleNone},
	{"production:stat#8", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"production:stat#9", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"production:staticfieldname#1", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:staticfieldname#2", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:staticfieldname#3", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:staticfieldname#4", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:staticfieldname#5", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:staticfieldname#6", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:staticfieldname#7", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:staticfieldname#8", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:staticfieldname#9", grammarStructural, grammarReasonTokenForward, roleNone},
	{"production:string#1", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"production:tableconstructor#1", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence | roleHeapClosed | roleHeapEmpty | roleHeapIngress | roleValueAllocation},
	{"production:tableconstructor#2", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence | roleHeapClosed | roleHeapEmpty | roleHeapIngress | roleValueAllocation},
	{"production:typedname#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:typedname#2", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:typednamelist#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:typednamelist#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:typeexpr#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:typeexpr#2", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:typeexpr#3", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:typeexpr#4", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:typeexprlist#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:typeexprlist#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:typeexprlist2#1", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:typeexprlist2#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:typefield#1", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:typefield#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:typefieldlist#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:typefieldlist#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:typefieldlist#3", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:typefieldtype#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:typefieldtype#2", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"production:typeparam#1", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:typeparam#2", grammarStructural, grammarReasonParserComponent, roleNone},
	{"production:typeparamlist#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:typeparamlist#2", grammarStructural, grammarReasonListAccumulation, roleNone},
	{"production:typeparams#1", grammarStructural, grammarReasonParserPlumbing, roleNone},
	{"production:var#1", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer},
	{"production:var#2", grammarRuleOwned, grammarReasonInvalid, roleRawGet},
	{"production:var#3", grammarRuleOwned, grammarReasonInvalid, roleRawGet | roleValueSource},
	{"production:var#4", grammarRuleOwned, grammarReasonInvalid, roleRawGet | roleValueSource},
	{"production:var#5", grammarRuleOwned, grammarReasonInvalid, roleRawGet | roleValueSource},
	{"production:var#6", grammarRuleOwned, grammarReasonInvalid, roleRawGet | roleValueSource},
	{"production:var#7", grammarRuleOwned, grammarReasonInvalid, roleRawGet | roleValueSource},
	{"production:var#8", grammarRuleOwned, grammarReasonInvalid, roleRawGet | roleValueSource},
	{"production:varlist#1", grammarStructural, grammarReasonPassThrough, roleNone},
	{"production:varlist#2", grammarStructural, grammarReasonListAccumulation, roleNone},

	// form rows: one per AST declaration the parser constructs.
	{"form:AnnotatedTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:AnnotationExpr", grammarStructural, grammarReasonParserComponent, roleNone},
	{"form:ArithmeticOpExpr", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"form:ArrayTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:AssertsTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:AssignStmt", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer | roleRawSet | roleStorageWriteOccurrence},
	{"form:AttrGetExpr", grammarRuleOwned, grammarReasonInvalid, roleRawGet},
	{"form:BreakStmt", grammarStructural, grammarReasonControlFlow, roleNone},
	{"form:CastExpr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"form:Comma3Expr", grammarRuleOwned, grammarReasonInvalid, rolePackSource},
	{"form:ConditionalTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:DoBlockStmt", grammarStructural, grammarReasonControlFlow, roleNone},
	{"form:FalseExpr", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"form:Field", grammarStructural, grammarReasonParserComponent, roleNone},
	{"form:FuncCallExpr", grammarRuleOwned, grammarReasonInvalid, roleCallSurface | rolePackSource | roleEffectSelected | roleEffectOpaque | roleEffectBody | roleCallActivation},
	{"form:FuncCallStmt", grammarRuleOwned, grammarReasonInvalid, roleCallSurface | rolePackSource | roleEffectSelected | roleEffectOpaque | roleEffectBody | roleCallActivation},
	{"form:FuncDefStmt", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer | roleRawSet | roleStorageWriteOccurrence},
	{"form:FuncName", grammarStructural, grammarReasonParserComponent, roleNone},
	{"form:FunctionExpr", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"form:FunctionParamExpr", grammarStructural, grammarReasonParserComponent, roleNone},
	{"form:FunctionTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:GenericForStmt", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"form:GenericTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:GotoStmt", grammarStructural, grammarReasonControlFlow, roleNone},
	{"form:IdentExpr", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer},
	{"form:IfStmt", grammarStructural, grammarReasonControlFlow, roleNone},
	{"form:IndexAccessExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:InterfaceDefStmt", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:InterfaceMember", grammarStructural, grammarReasonParserComponent, roleNone},
	{"form:IntersectionTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:KeyOfExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:LabelStmt", grammarStructural, grammarReasonControlFlow, roleNone},
	{"form:LiteralTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:LocalAssignStmt", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"form:LogicalOpExpr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"form:MapTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:NilExpr", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"form:NonNilAssertExpr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"form:NumberExpr", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"form:NumberForStmt", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"form:OptionalTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:ParList", grammarStructural, grammarReasonParserComponent, roleNone},
	{"form:PrimitiveTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:RecordFieldExpr", grammarStructural, grammarReasonParserComponent, roleNone},
	{"form:RecordTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:RelationalOpExpr", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"form:RepeatStmt", grammarStructural, grammarReasonControlFlow, roleNone},
	{"form:ReturnStmt", grammarRuleOwned, grammarReasonInvalid, roleReturnBoundaryOccurrence},
	{"form:StringConcatOpExpr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"form:StringExpr", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"form:TableExpr", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence | roleHeapIngress | roleValueAllocation | roleHeapEmpty | roleHeapClosed},
	{"form:Token", grammarStructural, grammarReasonLexicalToken, roleNone},
	{"form:TrueExpr", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"form:TypeDefStmt", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:TypeOfExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:TypeParamExpr", grammarStructural, grammarReasonParserComponent, roleNone},
	{"form:TypeRefExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:UnaryBNotOpExpr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"form:UnaryLenOpExpr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"form:UnaryMinusOpExpr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"form:UnaryNotOpExpr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"form:UnionTypeExpr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"form:WhileStmt", grammarStructural, grammarReasonControlFlow, roleNone},

	// carrier rows: one per exported field of a constructed form.
	{"carrier:AnnotatedTypeExpr.Inner", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:AnnotatedTypeExpr.Annotations", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:AnnotationExpr.Name", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:AnnotationExpr.Args", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:ArithmeticOpExpr.Operator", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"carrier:ArithmeticOpExpr.Lhs", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"carrier:ArithmeticOpExpr.Rhs", grammarRuleOwned, grammarReasonInvalid, roleArithmetic},
	{"carrier:ArrayTypeExpr.Element", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:ArrayTypeExpr.Readonly", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:AssertsTypeExpr.ParamName", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:AssertsTypeExpr.ParamPosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:AssertsTypeExpr.NarrowTo", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:AssignStmt.Lhs", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer | roleRawSet | roleStorageWriteOccurrence},
	{"carrier:AssignStmt.Rhs", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer | roleRawSet | roleStorageWriteOccurrence},
	{"carrier:AttrGetExpr.Object", grammarRuleOwned, grammarReasonInvalid, roleRawGet},
	{"carrier:AttrGetExpr.Key", grammarRuleOwned, grammarReasonInvalid, roleRawGet},
	{"carrier:AttrGetExpr.KeySyntax", grammarRuleOwned, grammarReasonInvalid, roleRawGet},
	{"carrier:CastExpr.Expr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:CastExpr.Type", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:CastExpr.Syntax", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:Comma3Expr.AdjustRet", grammarRuleOwned, grammarReasonInvalid, rolePackSource},
	{"carrier:ConditionalTypeExpr.Check", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:ConditionalTypeExpr.Extends", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:ConditionalTypeExpr.Then", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:ConditionalTypeExpr.Else", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:DoBlockStmt.Stmts", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:Field.Key", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:Field.KeySyntax", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:Field.Value", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:FuncCallExpr.Func", grammarRuleOwned, grammarReasonInvalid, roleCallSurface | rolePackSource | roleEffectSelected | roleEffectOpaque | roleEffectBody | roleCallActivation},
	{"carrier:FuncCallExpr.Receiver", grammarRuleOwned, grammarReasonInvalid, roleCallSurface | rolePackSource | roleEffectSelected | roleEffectOpaque | roleEffectBody | roleCallActivation},
	{"carrier:FuncCallExpr.Method", grammarRuleOwned, grammarReasonInvalid, roleCallSurface | rolePackSource | roleEffectSelected | roleEffectOpaque | roleEffectBody | roleCallActivation},
	{"carrier:FuncCallExpr.MethodPosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:FuncCallExpr.Args", grammarRuleOwned, grammarReasonInvalid, roleCallSurface | rolePackSource | roleEffectSelected | roleEffectOpaque | roleEffectBody | roleCallActivation},
	{"carrier:FuncCallExpr.TypeArgs", grammarRuleOwned, grammarReasonInvalid, roleCallSurface | rolePackSource | roleEffectSelected | roleEffectOpaque | roleEffectBody | roleCallActivation},
	{"carrier:FuncCallExpr.AdjustRet", grammarRuleOwned, grammarReasonInvalid, roleCallSurface | rolePackSource | roleEffectSelected | roleEffectOpaque | roleEffectBody | roleCallActivation},
	{"carrier:FuncCallStmt.Expr", grammarRuleOwned, grammarReasonInvalid, roleCallSurface | rolePackSource | roleEffectSelected | roleEffectOpaque | roleEffectBody | roleCallActivation},
	{"carrier:FuncDefStmt.Name", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer | roleRawSet | roleStorageWriteOccurrence},
	{"carrier:FuncDefStmt.Func", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer | roleRawSet | roleStorageWriteOccurrence},
	{"carrier:FuncName.Func", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:FuncName.Receiver", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:FuncName.Method", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:FuncName.MethodPosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:FunctionExpr.TypeParams", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"carrier:FunctionExpr.ParList", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"carrier:FunctionExpr.ReturnTypes", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"carrier:FunctionExpr.ReturnsKnown", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"carrier:FunctionExpr.Stmts", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence},
	{"carrier:FunctionParamExpr.Name", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:FunctionParamExpr.NamePosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:FunctionParamExpr.Type", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:FunctionTypeExpr.TypeParams", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:FunctionTypeExpr.Params", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:FunctionTypeExpr.Variadic", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:FunctionTypeExpr.VariadicPosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:FunctionTypeExpr.Returns", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:GenericForStmt.Names", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:GenericForStmt.NamePositions", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:GenericForStmt.Exprs", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:GenericForStmt.Stmts", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:GenericTypeExpr.Base", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:GenericTypeExpr.Args", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:GotoStmt.Label", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:IdentExpr.Value", grammarRuleOwned, grammarReasonInvalid, roleStorageTransfer},
	{"carrier:IfStmt.Condition", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:IfStmt.Then", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:IfStmt.Else", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:IfStmt.HasElse", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:IfStmt.EndPosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:IndexAccessExpr.Object", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:IndexAccessExpr.Index", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:InterfaceDefStmt.Name", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:InterfaceDefStmt.NamePosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:InterfaceDefStmt.Extends", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:InterfaceDefStmt.Members", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:InterfaceMember.Kind", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:InterfaceMember.Name", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:InterfaceMember.NamePosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:InterfaceMember.Type", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:InterfaceMember.Optional", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:IntersectionTypeExpr.Types", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:KeyOfExpr.Inner", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:LabelStmt.Name", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:LiteralTypeExpr.Value", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:LocalAssignStmt.Names", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:LocalAssignStmt.NamePositions", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:LocalAssignStmt.Types", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:LocalAssignStmt.Exprs", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:LocalAssignStmt.LocalFunction", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:LogicalOpExpr.Operator", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:LogicalOpExpr.Lhs", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:LogicalOpExpr.Rhs", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:MapTypeExpr.Key", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:MapTypeExpr.Value", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:MapTypeExpr.Readonly", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:NonNilAssertExpr.Expr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:NumberExpr.Value", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"carrier:NumberForStmt.Name", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:NumberForStmt.NamePosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:NumberForStmt.Init", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:NumberForStmt.Limit", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:NumberForStmt.Step", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:NumberForStmt.Stmts", grammarRuleOwned, grammarReasonInvalid, roleStorageBindTransferOccurrence | roleStorageTransfer},
	{"carrier:OptionalTypeExpr.Inner", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:ParList.HasVargs", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:ParList.VarargType", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:ParList.VarargPosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:ParList.Names", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:ParList.NamePositions", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:ParList.Types", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:PrimitiveTypeExpr.Name", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:RecordFieldExpr.Name", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:RecordFieldExpr.NamePosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:RecordFieldExpr.Type", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:RecordFieldExpr.Optional", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:RecordTypeExpr.Fields", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:RecordTypeExpr.Readonly", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:RelationalOpExpr.Operator", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"carrier:RelationalOpExpr.Lhs", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"carrier:RelationalOpExpr.Rhs", grammarRuleOwned, grammarReasonInvalid, roleEquality | roleOrder | rolePresence},
	{"carrier:RepeatStmt.Condition", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:RepeatStmt.Stmts", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:ReturnStmt.Exprs", grammarRuleOwned, grammarReasonInvalid, roleReturnBoundaryOccurrence},
	{"carrier:StringConcatOpExpr.Lhs", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:StringConcatOpExpr.Rhs", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:StringExpr.Value", grammarRuleOwned, grammarReasonInvalid, roleValueSource},
	{"carrier:TableExpr.Fields", grammarRuleOwned, grammarReasonInvalid, roleAllocationOccurrence | roleHeapIngress | roleValueAllocation | roleHeapEmpty | roleHeapClosed},
	{"carrier:Token.Type", grammarStructural, grammarReasonLexicalToken, roleNone},
	{"carrier:Token.Name", grammarStructural, grammarReasonLexicalToken, roleNone},
	{"carrier:Token.Str", grammarStructural, grammarReasonLexicalToken, roleNone},
	{"carrier:Token.Pos", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:TypeDefStmt.Name", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:TypeDefStmt.NamePosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:TypeDefStmt.TypeParams", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:TypeDefStmt.Type", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:TypeOfExpr.Expr", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:TypeParamExpr.Name", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:TypeParamExpr.NamePosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:TypeParamExpr.Constraint", grammarStructural, grammarReasonParserComponent, roleNone},
	{"carrier:TypeRefExpr.Path", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:TypeRefExpr.RootPosition", grammarStructural, grammarReasonSourceCoordinate, roleNone},
	{"carrier:UnaryBNotOpExpr.Expr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:UnaryLenOpExpr.Expr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:UnaryMinusOpExpr.Expr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:UnaryNotOpExpr.Expr", grammarStructural, grammarReasonUnownedComputation, roleNone},
	{"carrier:UnionTypeExpr.Types", grammarStructural, grammarReasonStaticTypeSurface, roleNone},
	{"carrier:WhileStmt.Condition", grammarStructural, grammarReasonControlFlow, roleNone},
	{"carrier:WhileStmt.Stmts", grammarStructural, grammarReasonControlFlow, roleNone},
}

// GrammarCensus is the neutral projection of the parser census the join
// consumes. Keeping it neutral is deliberate: the sealed catalog states
// dispositions and never links against the frontend package that derives the
// denominator, so the join stays a pure function of two values and a law can
// hand it a deliberately damaged census and watch it refuse.
type GrammarCensus struct {
	// Digest is the census's own identity, pinned by grammarCensusAuthority.
	Digest string
	// Rows is every census row key, at all three grains.
	Rows []GrammarRow
	// Builds maps a production row to the form rows its reduction constructs.
	Builds map[GrammarRow][]GrammarRow
	// Declares maps a carrier row to the form row that declares it.
	Declares map[GrammarRow]GrammarRow
	// Coordinates is the subset of carrier rows that carry a source position.
	Coordinates map[GrammarRow]bool
	// Components is the subset of form rows the AST itself marks structural.
	Components map[GrammarRow]bool
}

// GrammarJoinReason names the exact way the account failed to close.
type GrammarJoinReason uint8

const (
	// GrammarJoinSealed is the zero value: the account closed.
	GrammarJoinSealed GrammarJoinReason = iota
	GrammarJoinCensusAuthorityChanged
	GrammarJoinMissingDisposition
	GrammarJoinForeignDisposition
	GrammarJoinDuplicateDisposition
	GrammarJoinMalformedEntry
	GrammarJoinUndeclaredRole
	GrammarJoinIncoherentProduction
	GrammarJoinIncoherentCarrier
	GrammarJoinMisclaimedComponent
	GrammarJoinRoleUnreached
)

func (reason GrammarJoinReason) String() string {
	switch reason {
	case GrammarJoinSealed:
		return "sealed"
	case GrammarJoinCensusAuthorityChanged:
		return "census authority changed"
	case GrammarJoinMissingDisposition:
		return "missing disposition"
	case GrammarJoinForeignDisposition:
		return "foreign disposition"
	case GrammarJoinDuplicateDisposition:
		return "duplicate disposition"
	case GrammarJoinMalformedEntry:
		return "malformed entry"
	case GrammarJoinUndeclaredRole:
		return "undeclared role"
	case GrammarJoinIncoherentProduction:
		return "incoherent production"
	case GrammarJoinIncoherentCarrier:
		return "incoherent carrier"
	case GrammarJoinMisclaimedComponent:
		return "misclaimed component"
	case GrammarJoinRoleUnreached:
		return "role unreached"
	default:
		return "unknown"
	}
}

// GrammarJoinFailure is the one rejection the join reports. Row names the
// census row or disposition at fault; Key is set only when the fault is about
// a rule declaration rather than a row.
type GrammarJoinFailure struct {
	Row    GrammarRow
	Reason GrammarJoinReason
	Key    schema.Key
}

func (failure GrammarJoinFailure) Available() bool { return failure.Reason != GrammarJoinSealed }

// JoinGrammarCensus closes the authored dispositions against a parser census.
// It is total in both directions and returns the first fault it finds, so a
// caller reads one exact reason instead of a list it has to rank.
func JoinGrammarCensus(compilation Compilation, value GrammarCensus) GrammarJoinFailure {
	if value.Digest != grammarCensusAuthority {
		return GrammarJoinFailure{Reason: GrammarJoinCensusAuthorityChanged}
	}
	declared := make(map[GrammarRow]grammarEntry, len(grammarDispositions))
	for _, entry := range grammarDispositions {
		if _, duplicate := declared[entry.Row]; duplicate {
			return GrammarJoinFailure{Row: entry.Row, Reason: GrammarJoinDuplicateDisposition}
		}
		if failure := checkGrammarEntry(compilation.catalog, entry); failure.Available() {
			return failure
		}
		declared[entry.Row] = entry
	}
	present := make(map[GrammarRow]bool, len(value.Rows))
	for _, row := range value.Rows {
		if present[row] {
			return GrammarJoinFailure{Row: row, Reason: GrammarJoinDuplicateDisposition}
		}
		present[row] = true
		if _, accounted := declared[row]; !accounted {
			return GrammarJoinFailure{Row: row, Reason: GrammarJoinMissingDisposition}
		}
	}
	for _, entry := range grammarDispositions {
		if !present[entry.Row] {
			return GrammarJoinFailure{Row: entry.Row, Reason: GrammarJoinForeignDisposition}
		}
	}
	if failure := checkGrammarCoherence(value, declared); failure.Available() {
		return failure
	}
	return checkGrammarRoleReach(compilation.catalog, declared)
}

// checkGrammarEntry states the shape law of one authored row: exactly one
// account, and every named role declared by the sealed rule table.
func checkGrammarEntry(state *catalog, entry grammarEntry) GrammarJoinFailure {
	switch entry.Disposition {
	case grammarRuleOwned:
		if entry.Roles == roleNone || entry.Reason != grammarReasonInvalid {
			return GrammarJoinFailure{Row: entry.Row, Reason: GrammarJoinMalformedEntry}
		}
	case grammarStructural, grammarRejected, grammarParserImpossible:
		if entry.Roles != roleNone || !entry.Reason.Available() {
			return GrammarJoinFailure{Row: entry.Row, Reason: GrammarJoinMalformedEntry}
		}
	default:
		return GrammarJoinFailure{Row: entry.Row, Reason: GrammarJoinMalformedEntry}
	}
	if entry.Roles&^entry.Roles.known() != 0 {
		return GrammarJoinFailure{Row: entry.Row, Reason: GrammarJoinUndeclaredRole}
	}
	for _, role := range grammarRoleKeys {
		if entry.Roles&role.mask == 0 {
			continue
		}
		template, declared := templateForKey(state, role.key)
		if !declared {
			return GrammarJoinFailure{Row: entry.Row, Reason: GrammarJoinUndeclaredRole}
		}
		if !template.Key().Available() {
			return GrammarJoinFailure{Row: entry.Row, Reason: GrammarJoinUndeclaredRole}
		}
	}
	for _, group := range grammarRoleGroups {
		if entry.Roles&group.mask == 0 {
			continue
		}
		declared := false
		for position := 0; state != nil && position < len(state.templates) && !declared; position++ {
			key := state.templates[position].Key()
			keyOK := key.Available()
			if !keyOK {
				continue
			}
			declared = grammarRoleGroupMatches(state, entry.Roles, key)
		}
		if !declared {
			return GrammarJoinFailure{Row: entry.Row, Reason: GrammarJoinUndeclaredRole}
		}
	}
	return GrammarJoinFailure{}
}

// checkGrammarCoherence states the containment laws. A production accounts for
// exactly the roles of the forms it builds, a carrier accounts for its form
// unless it carries a source coordinate, and a form the AST marks structural is
// owned by no rule. Without them an authored row could drift away from the
// census evidence it claims to describe while still joining.
func checkGrammarCoherence(value GrammarCensus, declared map[GrammarRow]grammarEntry) GrammarJoinFailure {
	for row, forms := range value.Builds {
		entry, known := declared[row]
		if !known {
			return GrammarJoinFailure{Row: row, Reason: GrammarJoinMissingDisposition}
		}
		expected := roleNone
		for _, form := range forms {
			built, ok := declared[form]
			if !ok {
				return GrammarJoinFailure{Row: form, Reason: GrammarJoinMissingDisposition}
			}
			expected |= built.Roles
		}
		if entry.Roles != expected {
			return GrammarJoinFailure{Row: row, Reason: GrammarJoinIncoherentProduction}
		}
	}
	for row, form := range value.Declares {
		entry, known := declared[row]
		if !known {
			return GrammarJoinFailure{Row: row, Reason: GrammarJoinMissingDisposition}
		}
		if value.Coordinates[row] {
			if entry.Disposition != grammarStructural || entry.Reason != grammarReasonSourceCoordinate {
				return GrammarJoinFailure{Row: row, Reason: GrammarJoinIncoherentCarrier}
			}
			continue
		}
		owner, known := declared[form]
		if !known {
			return GrammarJoinFailure{Row: form, Reason: GrammarJoinMissingDisposition}
		}
		if entry.Disposition != owner.Disposition || entry.Reason != owner.Reason || entry.Roles != owner.Roles {
			return GrammarJoinFailure{Row: row, Reason: GrammarJoinIncoherentCarrier}
		}
	}
	for row := range value.Components {
		entry, known := declared[row]
		if !known {
			return GrammarJoinFailure{Row: row, Reason: GrammarJoinMissingDisposition}
		}
		if entry.Disposition != grammarStructural {
			return GrammarJoinFailure{Row: row, Reason: GrammarJoinMisclaimedComponent}
		}
	}
	return GrammarJoinFailure{}
}

// checkGrammarRoleReach states the reverse totality: every mounted rule role is
// reached by at least one grammar row. A rule the language cannot feed is a
// rule with no source, and the account is only an equivalence if it closes in
// that direction too. Link-owned roles are excluded by the artifact's own
// mounted vocabulary: they are admitted at the Link boundary and have no
// grammar provenance at all.
func checkGrammarRoleReach(state *catalog, declared map[GrammarRow]grammarEntry) GrammarJoinFailure {
	reached := roleNone
	for _, entry := range declared {
		reached |= entry.Roles
	}
	if state == nil {
		return GrammarJoinFailure{Reason: GrammarJoinSealed}
	}
	for _, template := range state.templates {
		if template == nil || !template.Lane().Mounted() {
			continue
		}
		key := template.Key()
		if !reached.has(state, key) {
			return GrammarJoinFailure{Reason: GrammarJoinRoleUnreached, Key: key}
		}
	}
	return GrammarJoinFailure{}
}
