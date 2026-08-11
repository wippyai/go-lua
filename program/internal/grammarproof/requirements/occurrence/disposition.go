package occurrence

import (
	"fmt"

	"github.com/wippyai/go-lua/program/internal/grammarproof"
	"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/grammar"
)

// ParserLaw is the finite reason a schema-derived field state cannot be
// produced by a successful parse. These laws describe parser actions, not Go
// zero values and not lowerer implementation structure.
type ParserLaw uint8

const (
	ParserLawInvalid ParserLaw = iota
	ParserLawRequiredChild
	ParserLawNonEmptyToken
	ParserLawNonEmptyList
	ParserLawConstructorNonEmpty
	ParserLawTokenPosition
	ParserLawSourceDiscriminant
)

// IngressLaw names a parser-reachable AST state which the public lowering
// ingress deliberately rejects. It is not parser impossibility and therefore
// must never be placed in parserImpossibleStates.
type IngressLaw uint8

const (
	IngressLawInvalid IngressLaw = iota
	IngressLawMalformedNumericLiteral
)

// SemanticLaw names the exact valid source permutation which was absent from
// the reduction-time observation trace. Its public source-to-Program witness
// remains the authority; this inventory merely prevents it being mislabeled
// parser-impossible.
type SemanticLaw uint8

const (
	SemanticLawInvalid SemanticLaw = iota
	SemanticLawScalarVararg
	SemanticLawGenericFunctionType
	SemanticLawEmptyInterface
	SemanticLawOptionalInterfaceField
)

type DispositionKind uint8

const (
	DispositionInvalid DispositionKind = iota
	DispositionParserImpossible
	DispositionSourceReachable
	DispositionPublicIngressRejected
)

// Disposition accounts for exactly one row which the reduction-time parser
// trace did not observe. It is deliberately separate from Report.Observed:
// neither observation nor impossibility is a Program semantic proof.
type Disposition struct {
	Requirement Requirement
	Kind        DispositionKind
	Parser      ParserLaw
	Semantic    SemanticLaw
	Ingress     IngressLaw
}

// SemanticWitnessSource returns the canonical corpus witness for one valid
// parser state which the reduction-time hook cannot observe reliably. The
// grammarproof matrix verifies the exact AST field state and public ingress;
// this mapping is therefore a reasoned exception, not a detached test case.
func SemanticWitnessSource(law SemanticLaw) (string, bool) {
	switch law {
	case SemanticLawScalarVararg:
		return "grammar:residue-scalar-vararg", true
	case SemanticLawGenericFunctionType:
		return "grammar:residue-generic-function-type", true
	case SemanticLawEmptyInterface:
		return "grammar:residue-empty-interface", true
	case SemanticLawOptionalInterfaceField:
		return "grammar:residue-optional-interface-field", true
	default:
		return "", false
	}
}

type namedState struct {
	constructor string
	field       string
	state       grammarproof.FieldState
}

const (
	dispositionGrammarDigest = "751315983a27d36121ace906bbc2216abbd0c082221bca99c7ace479874aa1d5"
	dispositionSchemaDigest  = "50039764153aa2f7cb14ce40bdd5424f8b309afb024e9f331381946ccafb5688"
)

// ClassifyResidue closes the parser-state observation residue against the
// exact frozen parser actions. It fails if either authority changes or a new
// residue row appears. A source-reachable row may become directly observed
// when its exact witness joins the canonical corpus; it then leaves this
// residue ledger and is carried by ordinary public-ingress evidence.
func ClassifyResidue(root string, report Report, schema grammar.Schema) ([]Disposition, error) {
	digest, err := grammarproof.GrammarActionDigest(root)
	if err != nil {
		return nil, fmt.Errorf("occurrence dispositions: parser action digest: %w", err)
	}
	if digest != dispositionGrammarDigest || schema.Digest() != dispositionSchemaDigest {
		return nil, fmt.Errorf("occurrence dispositions: frozen parser authority changed (grammar=%s schema=%s)", digest, schema.Digest())
	}
	fields, err := dispositionFields(schema)
	if err != nil {
		return nil, err
	}

	impossible := parserImpossibleStates()
	reachable := sourceReachableStates()
	ingressRejected := parserReachableIngressRejectedStates()
	classified := make([]Disposition, 0, len(report.Residue))
	seen := make(map[namedState]bool, len(report.Residue))
	for _, requirement := range report.Residue {
		fieldNames, ok := fields[requirement.Constructor]
		if !ok || requirement.Field < 0 || requirement.Field >= len(fieldNames) {
			return nil, fmt.Errorf("occurrence dispositions: invalid residue row %#v", requirement)
		}
		key := namedState{constructor: requirement.Constructor, field: fieldNames[requirement.Field], state: requirement.State}
		if seen[key] {
			return nil, fmt.Errorf("occurrence dispositions: duplicate residue row %#v", key)
		}
		seen[key] = true
		if law, ok := impossible[key]; ok {
			classified = append(classified, Disposition{Requirement: requirement, Kind: DispositionParserImpossible, Parser: law})
			continue
		}
		if law, ok := reachable[key]; ok {
			classified = append(classified, Disposition{Requirement: requirement, Kind: DispositionSourceReachable, Semantic: law})
			continue
		}
		if law, ok := ingressRejected[key]; ok {
			classified = append(classified, Disposition{Requirement: requirement, Kind: DispositionPublicIngressRejected, Ingress: law})
			continue
		}
		return nil, fmt.Errorf("occurrence dispositions: unclassified parser-state residue %#v", key)
	}
	for key := range impossible {
		if !seen[key] {
			return nil, fmt.Errorf("occurrence dispositions: stale parser-impossibility row %#v", key)
		}
	}
	for key := range ingressRejected {
		fieldNames, exists := fields[key.constructor]
		found := false
		if exists {
			for _, field := range fieldNames {
				if field == key.field {
					found = true
					break
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("occurrence dispositions: stale public-ingress-rejected row %#v", key)
		}
	}
	return classified, nil
}

// parserReachableIngressRejectedStates records AST states intentionally
// constructed by parser actions but rejected by public Lower. The malformed
// numeric literal branch is a parser fact with an ingress error, never an
// impossible parser state.
func parserReachableIngressRejectedStates() map[namedState]IngressLaw {
	return map[namedState]IngressLaw{
		{"LiteralTypeExpr", "Value", grammarproof.FieldStateAbsent}: IngressLawMalformedNumericLiteral,
	}
}

func dispositionFields(schema grammar.Schema) (map[string][]string, error) {
	result := make(map[string][]string, len(schema.Constructors))
	for _, constructor := range schema.Constructors {
		fields := make([]string, len(constructor.Fields))
		for _, field := range constructor.Fields {
			if field.Ordinal < 0 || field.Ordinal >= len(fields) || fields[field.Ordinal] != "" {
				return nil, fmt.Errorf("occurrence dispositions: malformed schema field for %s", constructor.Name)
			}
			fields[field.Ordinal] = field.Name
		}
		result[constructor.Name] = fields
	}
	return result, nil
}

func sourceReachableStates() map[namedState]SemanticLaw {
	return map[namedState]SemanticLaw{
		{"Comma3Expr", "AdjustRet", grammarproof.FieldStateTrue}:            SemanticLawScalarVararg,
		{"FunctionTypeExpr", "TypeParams", grammarproof.FieldStateNonEmpty}: SemanticLawGenericFunctionType,
		{"InterfaceDefStmt", "Members", grammarproof.FieldStateEmpty}:       SemanticLawEmptyInterface,
		{"InterfaceMember", "Optional", grammarproof.FieldStateTrue}:        SemanticLawOptionalInterfaceField,
	}
}

func parserImpossibleStates() map[namedState]ParserLaw {
	return map[namedState]ParserLaw{
		{"AnnotationExpr", "Name", grammarproof.FieldStateEmpty}:           ParserLawNonEmptyToken,
		{"AnnotatedTypeExpr", "Inner", grammarproof.FieldStateAbsent}:      ParserLawRequiredChild,
		{"AnnotatedTypeExpr", "Annotations", grammarproof.FieldStateEmpty}: ParserLawConstructorNonEmpty,
		{"ArithmeticOpExpr", "Operator", grammarproof.FieldStateEmpty}:     ParserLawNonEmptyToken,
		{"ArithmeticOpExpr", "Lhs", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"ArithmeticOpExpr", "Rhs", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"ArrayTypeExpr", "Element", grammarproof.FieldStateAbsent}:        ParserLawRequiredChild,
		{"AssertsTypeExpr", "ParamName", grammarproof.FieldStateEmpty}:     ParserLawNonEmptyToken,
		{"AssertsTypeExpr", "ParamPosition", grammarproof.FieldStateZero}:  ParserLawTokenPosition,
		{"AssignStmt", "Lhs", grammarproof.FieldStateEmpty}:                ParserLawNonEmptyList,
		{"AssignStmt", "Rhs", grammarproof.FieldStateEmpty}:                ParserLawNonEmptyList,
		{"AttrGetExpr", "Object", grammarproof.FieldStateAbsent}:           ParserLawRequiredChild,
		{"AttrGetExpr", "Key", grammarproof.FieldStateAbsent}:              ParserLawRequiredChild,
		{"AttrGetExpr", "KeySyntax", grammarproof.FieldStateZero}:          ParserLawSourceDiscriminant,
		{"CastExpr", "Expr", grammarproof.FieldStateAbsent}:                ParserLawRequiredChild,
		{"CastExpr", "Type", grammarproof.FieldStateAbsent}:                ParserLawRequiredChild,
		{"CastExpr", "Syntax", grammarproof.FieldStateZero}:                ParserLawSourceDiscriminant,
		{"ConditionalTypeExpr", "Check", grammarproof.FieldStateAbsent}:    ParserLawRequiredChild,
		{"ConditionalTypeExpr", "Extends", grammarproof.FieldStateAbsent}:  ParserLawRequiredChild,
		{"ConditionalTypeExpr", "Then", grammarproof.FieldStateAbsent}:     ParserLawRequiredChild,
		{"ConditionalTypeExpr", "Else", grammarproof.FieldStateAbsent}:     ParserLawRequiredChild,
		{"Field", "Value", grammarproof.FieldStateAbsent}:                  ParserLawRequiredChild,
		{"FuncCallStmt", "Expr", grammarproof.FieldStateAbsent}:            ParserLawRequiredChild,
		{"FuncDefStmt", "Name", grammarproof.FieldStateAbsent}:             ParserLawRequiredChild,
		{"FuncDefStmt", "Func", grammarproof.FieldStateAbsent}:             ParserLawRequiredChild,
		{"FunctionParamExpr", "Type", grammarproof.FieldStateAbsent}:       ParserLawRequiredChild,
		{"FunctionExpr", "ParList", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"GenericForStmt", "Names", grammarproof.FieldStateEmpty}:          ParserLawNonEmptyList,
		{"GenericForStmt", "NamePositions", grammarproof.FieldStateEmpty}:  ParserLawNonEmptyList,
		{"GenericForStmt", "Exprs", grammarproof.FieldStateEmpty}:          ParserLawNonEmptyList,
		{"GenericTypeExpr", "Base", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"GenericTypeExpr", "Args", grammarproof.FieldStateEmpty}:          ParserLawNonEmptyList,
		{"GotoStmt", "Label", grammarproof.FieldStateEmpty}:                ParserLawNonEmptyToken,
		{"IdentExpr", "Value", grammarproof.FieldStateEmpty}:               ParserLawNonEmptyToken,
		{"IfStmt", "Condition", grammarproof.FieldStateAbsent}:             ParserLawRequiredChild,
		{"IndexAccessExpr", "Object", grammarproof.FieldStateAbsent}:       ParserLawRequiredChild,
		{"IndexAccessExpr", "Index", grammarproof.FieldStateAbsent}:        ParserLawRequiredChild,
		{"InterfaceDefStmt", "Name", grammarproof.FieldStateEmpty}:         ParserLawNonEmptyToken,
		{"InterfaceDefStmt", "NamePosition", grammarproof.FieldStateZero}:  ParserLawTokenPosition,
		{"InterfaceMember", "Kind", grammarproof.FieldStateZero}:           ParserLawSourceDiscriminant,
		{"InterfaceMember", "Name", grammarproof.FieldStateEmpty}:          ParserLawNonEmptyToken,
		{"InterfaceMember", "NamePosition", grammarproof.FieldStateZero}:   ParserLawTokenPosition,
		{"InterfaceMember", "Type", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"IntersectionTypeExpr", "Types", grammarproof.FieldStateEmpty}:    ParserLawConstructorNonEmpty,
		{"KeyOfExpr", "Inner", grammarproof.FieldStateAbsent}:              ParserLawRequiredChild,
		{"LabelStmt", "Name", grammarproof.FieldStateEmpty}:                ParserLawNonEmptyToken,
		{"LocalAssignStmt", "Names", grammarproof.FieldStateEmpty}:         ParserLawNonEmptyList,
		{"LocalAssignStmt", "NamePositions", grammarproof.FieldStateEmpty}: ParserLawNonEmptyList,
		{"LogicalOpExpr", "Operator", grammarproof.FieldStateEmpty}:        ParserLawNonEmptyToken,
		{"LogicalOpExpr", "Lhs", grammarproof.FieldStateAbsent}:            ParserLawRequiredChild,
		{"LogicalOpExpr", "Rhs", grammarproof.FieldStateAbsent}:            ParserLawRequiredChild,
		{"MapTypeExpr", "Key", grammarproof.FieldStateAbsent}:              ParserLawRequiredChild,
		{"MapTypeExpr", "Value", grammarproof.FieldStateAbsent}:            ParserLawRequiredChild,
		{"NonNilAssertExpr", "Expr", grammarproof.FieldStateAbsent}:        ParserLawRequiredChild,
		{"NumberExpr", "Value", grammarproof.FieldStateEmpty}:              ParserLawNonEmptyToken,
		{"NumberForStmt", "Name", grammarproof.FieldStateEmpty}:            ParserLawNonEmptyToken,
		{"NumberForStmt", "NamePosition", grammarproof.FieldStateZero}:     ParserLawTokenPosition,
		{"NumberForStmt", "Init", grammarproof.FieldStateAbsent}:           ParserLawRequiredChild,
		{"NumberForStmt", "Limit", grammarproof.FieldStateAbsent}:          ParserLawRequiredChild,
		{"OptionalTypeExpr", "Inner", grammarproof.FieldStateAbsent}:       ParserLawRequiredChild,
		{"PrimitiveTypeExpr", "Name", grammarproof.FieldStateEmpty}:        ParserLawNonEmptyToken,
		{"RelationalOpExpr", "Operator", grammarproof.FieldStateEmpty}:     ParserLawNonEmptyToken,
		{"RelationalOpExpr", "Lhs", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"RelationalOpExpr", "Rhs", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"RecordFieldExpr", "Name", grammarproof.FieldStateEmpty}:          ParserLawNonEmptyToken,
		{"RecordFieldExpr", "NamePosition", grammarproof.FieldStateZero}:   ParserLawTokenPosition,
		{"RecordFieldExpr", "Type", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"RepeatStmt", "Condition", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"StringConcatOpExpr", "Lhs", grammarproof.FieldStateAbsent}:       ParserLawRequiredChild,
		{"StringConcatOpExpr", "Rhs", grammarproof.FieldStateAbsent}:       ParserLawRequiredChild,
		{"TypeDefStmt", "Name", grammarproof.FieldStateEmpty}:              ParserLawNonEmptyToken,
		{"TypeDefStmt", "NamePosition", grammarproof.FieldStateZero}:       ParserLawTokenPosition,
		{"TypeDefStmt", "Type", grammarproof.FieldStateAbsent}:             ParserLawRequiredChild,
		{"TypeOfExpr", "Expr", grammarproof.FieldStateAbsent}:              ParserLawRequiredChild,
		{"TypeParamExpr", "Name", grammarproof.FieldStateEmpty}:            ParserLawNonEmptyToken,
		{"TypeParamExpr", "NamePosition", grammarproof.FieldStateZero}:     ParserLawTokenPosition,
		{"TypeRefExpr", "Path", grammarproof.FieldStateEmpty}:              ParserLawConstructorNonEmpty,
		{"TypeRefExpr", "RootPosition", grammarproof.FieldStateZero}:       ParserLawTokenPosition,
		{"UnaryBNotOpExpr", "Expr", grammarproof.FieldStateAbsent}:         ParserLawRequiredChild,
		{"UnaryLenOpExpr", "Expr", grammarproof.FieldStateAbsent}:          ParserLawRequiredChild,
		{"UnaryMinusOpExpr", "Expr", grammarproof.FieldStateAbsent}:        ParserLawRequiredChild,
		{"UnaryNotOpExpr", "Expr", grammarproof.FieldStateAbsent}:          ParserLawRequiredChild,
		{"UnionTypeExpr", "Types", grammarproof.FieldStateEmpty}:           ParserLawConstructorNonEmpty,
		{"WhileStmt", "Condition", grammarproof.FieldStateAbsent}:          ParserLawRequiredChild,
	}
}
