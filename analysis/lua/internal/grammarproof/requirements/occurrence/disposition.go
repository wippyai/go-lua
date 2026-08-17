package occurrence

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
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
	IngressLawUntypedFunctionTypeParameter
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
	state       astcodec.FieldState
}

const (
	dispositionGrammarDigest = "751315983a27d36121ace906bbc2216abbd0c082221bca99c7ace479874aa1d5"
	dispositionSchemaDigest  = "7b7e4cf3bd735684904b527f81d5dd6e4decd6e21e1416d17ac5b4b63c59d9ea"
)

// ClassifyResidue closes the parser-state observation residue against the
// exact frozen parser actions. It fails if either authority changes or a new
// residue row appears. A source-reachable row may become directly observed
// when its exact witness joins the canonical corpus; it then leaves this
// residue ledger and is carried by ordinary public-ingress evidence.
func ClassifyResidue(root string, report Report, schema parsersource.Schema) ([]Disposition, error) {
	digest, err := parsersource.GrammarActionDigest(root)
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
//
// The absent function-type parameter type is the same shape. An interface
// method declaration carries its parameters as typed-name entries, and the
// conversion to function-type parameters copies an entry whose type is absent
// when the source omits the annotation, so the parser builds the state and
// public Lower rejects the resulting signature.
func parserReachableIngressRejectedStates() map[namedState]IngressLaw {
	return map[namedState]IngressLaw{
		{"LiteralTypeExpr", "Value", astcodec.FieldStateAbsent}:  IngressLawMalformedNumericLiteral,
		{"FunctionParamExpr", "Type", astcodec.FieldStateAbsent}: IngressLawUntypedFunctionTypeParameter,
	}
}

func dispositionFields(schema parsersource.Schema) (map[string][]string, error) {
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
		{"Comma3Expr", "AdjustRet", astcodec.FieldStateTrue}:            SemanticLawScalarVararg,
		{"FunctionTypeExpr", "TypeParams", astcodec.FieldStateNonEmpty}: SemanticLawGenericFunctionType,
		{"InterfaceDefStmt", "Members", astcodec.FieldStateEmpty}:       SemanticLawEmptyInterface,
		{"InterfaceMember", "Optional", astcodec.FieldStateTrue}:        SemanticLawOptionalInterfaceField,
	}
}

func parserImpossibleStates() map[namedState]ParserLaw {
	return map[namedState]ParserLaw{
		{"AnnotationExpr", "Name", astcodec.FieldStateEmpty}:           ParserLawNonEmptyToken,
		{"AnnotatedTypeExpr", "Inner", astcodec.FieldStateAbsent}:      ParserLawRequiredChild,
		{"AnnotatedTypeExpr", "Annotations", astcodec.FieldStateEmpty}: ParserLawConstructorNonEmpty,
		{"ArithmeticOpExpr", "Operator", astcodec.FieldStateEmpty}:     ParserLawNonEmptyToken,
		{"ArithmeticOpExpr", "Lhs", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"ArithmeticOpExpr", "Rhs", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"ArrayTypeExpr", "Element", astcodec.FieldStateAbsent}:        ParserLawRequiredChild,
		{"AssertsTypeExpr", "ParamName", astcodec.FieldStateEmpty}:     ParserLawNonEmptyToken,
		{"AssertsTypeExpr", "ParamPosition", astcodec.FieldStateZero}:  ParserLawTokenPosition,
		{"AssignStmt", "Lhs", astcodec.FieldStateEmpty}:                ParserLawNonEmptyList,
		{"AssignStmt", "Rhs", astcodec.FieldStateEmpty}:                ParserLawNonEmptyList,
		{"AttrGetExpr", "Object", astcodec.FieldStateAbsent}:           ParserLawRequiredChild,
		{"AttrGetExpr", "Key", astcodec.FieldStateAbsent}:              ParserLawRequiredChild,
		{"AttrGetExpr", "KeySyntax", astcodec.FieldStateZero}:          ParserLawSourceDiscriminant,
		{"CastExpr", "Expr", astcodec.FieldStateAbsent}:                ParserLawRequiredChild,
		{"CastExpr", "Type", astcodec.FieldStateAbsent}:                ParserLawRequiredChild,
		{"CastExpr", "Syntax", astcodec.FieldStateZero}:                ParserLawSourceDiscriminant,
		{"ConditionalTypeExpr", "Check", astcodec.FieldStateAbsent}:    ParserLawRequiredChild,
		{"ConditionalTypeExpr", "Extends", astcodec.FieldStateAbsent}:  ParserLawRequiredChild,
		{"ConditionalTypeExpr", "Then", astcodec.FieldStateAbsent}:     ParserLawRequiredChild,
		{"ConditionalTypeExpr", "Else", astcodec.FieldStateAbsent}:     ParserLawRequiredChild,
		{"Field", "Value", astcodec.FieldStateAbsent}:                  ParserLawRequiredChild,
		{"FuncCallStmt", "Expr", astcodec.FieldStateAbsent}:            ParserLawRequiredChild,
		{"FuncDefStmt", "Name", astcodec.FieldStateAbsent}:             ParserLawRequiredChild,
		{"FuncDefStmt", "Func", astcodec.FieldStateAbsent}:             ParserLawRequiredChild,
		{"FunctionExpr", "ParList", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"GenericForStmt", "Names", astcodec.FieldStateEmpty}:          ParserLawNonEmptyList,
		{"GenericForStmt", "NamePositions", astcodec.FieldStateEmpty}:  ParserLawNonEmptyList,
		{"GenericForStmt", "Exprs", astcodec.FieldStateEmpty}:          ParserLawNonEmptyList,
		{"GenericTypeExpr", "Base", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"GenericTypeExpr", "Args", astcodec.FieldStateEmpty}:          ParserLawNonEmptyList,
		{"GotoStmt", "Label", astcodec.FieldStateEmpty}:                ParserLawNonEmptyToken,
		{"IdentExpr", "Value", astcodec.FieldStateEmpty}:               ParserLawNonEmptyToken,
		{"IfStmt", "Condition", astcodec.FieldStateAbsent}:             ParserLawRequiredChild,
		{"IndexAccessExpr", "Object", astcodec.FieldStateAbsent}:       ParserLawRequiredChild,
		{"IndexAccessExpr", "Index", astcodec.FieldStateAbsent}:        ParserLawRequiredChild,
		{"InterfaceDefStmt", "Name", astcodec.FieldStateEmpty}:         ParserLawNonEmptyToken,
		{"InterfaceDefStmt", "NamePosition", astcodec.FieldStateZero}:  ParserLawTokenPosition,
		{"InterfaceMember", "Kind", astcodec.FieldStateZero}:           ParserLawSourceDiscriminant,
		{"InterfaceMember", "Name", astcodec.FieldStateEmpty}:          ParserLawNonEmptyToken,
		{"InterfaceMember", "NamePosition", astcodec.FieldStateZero}:   ParserLawTokenPosition,
		{"InterfaceMember", "Type", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"IntersectionTypeExpr", "Types", astcodec.FieldStateEmpty}:    ParserLawConstructorNonEmpty,
		{"KeyOfExpr", "Inner", astcodec.FieldStateAbsent}:              ParserLawRequiredChild,
		{"LabelStmt", "Name", astcodec.FieldStateEmpty}:                ParserLawNonEmptyToken,
		{"LocalAssignStmt", "Names", astcodec.FieldStateEmpty}:         ParserLawNonEmptyList,
		{"LocalAssignStmt", "NamePositions", astcodec.FieldStateEmpty}: ParserLawNonEmptyList,
		{"LogicalOpExpr", "Operator", astcodec.FieldStateEmpty}:        ParserLawNonEmptyToken,
		{"LogicalOpExpr", "Lhs", astcodec.FieldStateAbsent}:            ParserLawRequiredChild,
		{"LogicalOpExpr", "Rhs", astcodec.FieldStateAbsent}:            ParserLawRequiredChild,
		{"MapTypeExpr", "Key", astcodec.FieldStateAbsent}:              ParserLawRequiredChild,
		{"MapTypeExpr", "Value", astcodec.FieldStateAbsent}:            ParserLawRequiredChild,
		{"NonNilAssertExpr", "Expr", astcodec.FieldStateAbsent}:        ParserLawRequiredChild,
		{"NumberExpr", "Value", astcodec.FieldStateEmpty}:              ParserLawNonEmptyToken,
		{"NumberForStmt", "Name", astcodec.FieldStateEmpty}:            ParserLawNonEmptyToken,
		{"NumberForStmt", "NamePosition", astcodec.FieldStateZero}:     ParserLawTokenPosition,
		{"NumberForStmt", "Init", astcodec.FieldStateAbsent}:           ParserLawRequiredChild,
		{"NumberForStmt", "Limit", astcodec.FieldStateAbsent}:          ParserLawRequiredChild,
		{"OptionalTypeExpr", "Inner", astcodec.FieldStateAbsent}:       ParserLawRequiredChild,
		{"PrimitiveTypeExpr", "Name", astcodec.FieldStateEmpty}:        ParserLawNonEmptyToken,
		{"RelationalOpExpr", "Operator", astcodec.FieldStateEmpty}:     ParserLawNonEmptyToken,
		{"RelationalOpExpr", "Lhs", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"RelationalOpExpr", "Rhs", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"RecordFieldExpr", "Name", astcodec.FieldStateEmpty}:          ParserLawNonEmptyToken,
		{"RecordFieldExpr", "NamePosition", astcodec.FieldStateZero}:   ParserLawTokenPosition,
		{"RecordFieldExpr", "Type", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"RepeatStmt", "Condition", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"StringConcatOpExpr", "Lhs", astcodec.FieldStateAbsent}:       ParserLawRequiredChild,
		{"StringConcatOpExpr", "Rhs", astcodec.FieldStateAbsent}:       ParserLawRequiredChild,
		{"TypeDefStmt", "Name", astcodec.FieldStateEmpty}:              ParserLawNonEmptyToken,
		{"TypeDefStmt", "NamePosition", astcodec.FieldStateZero}:       ParserLawTokenPosition,
		{"TypeDefStmt", "Type", astcodec.FieldStateAbsent}:             ParserLawRequiredChild,
		{"TypeOfExpr", "Expr", astcodec.FieldStateAbsent}:              ParserLawRequiredChild,
		{"TypeParamExpr", "Name", astcodec.FieldStateEmpty}:            ParserLawNonEmptyToken,
		{"TypeParamExpr", "NamePosition", astcodec.FieldStateZero}:     ParserLawTokenPosition,
		{"TypeRefExpr", "Path", astcodec.FieldStateEmpty}:              ParserLawConstructorNonEmpty,
		{"TypeRefExpr", "RootPosition", astcodec.FieldStateZero}:       ParserLawTokenPosition,
		{"UnaryBNotOpExpr", "Expr", astcodec.FieldStateAbsent}:         ParserLawRequiredChild,
		{"UnaryLenOpExpr", "Expr", astcodec.FieldStateAbsent}:          ParserLawRequiredChild,
		{"UnaryMinusOpExpr", "Expr", astcodec.FieldStateAbsent}:        ParserLawRequiredChild,
		{"UnaryNotOpExpr", "Expr", astcodec.FieldStateAbsent}:          ParserLawRequiredChild,
		{"UnionTypeExpr", "Types", astcodec.FieldStateEmpty}:           ParserLawConstructorNonEmpty,
		{"WhileStmt", "Condition", astcodec.FieldStateAbsent}:          ParserLawRequiredChild,
	}
}
