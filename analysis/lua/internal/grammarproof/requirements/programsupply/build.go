package programsupply

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/binder"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/programlaw"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/staticlaw"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/relations"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

const (
	digestDomain  = "program.grammarproof.requirements.programsupply"
	digestVersion = 1

	programRecord = 1
	staticRecord  = 2
	binderRecord  = 3
)

// Build derives exact terminal tokens from the canonical relation schema and
// the three independently owned closed law denominators.
func Build() (Evidence, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		return Evidence{}, fmt.Errorf("Program supply: canonical schema: %w", err)
	}
	evidence, err := expected(schema)
	if err != nil {
		return Evidence{}, err
	}
	if err := evidence.Validate(schema); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

// Current validates and returns a detached copy of the generated evidence.
func Current() (Evidence, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		return Evidence{}, fmt.Errorf("Program supply: canonical schema: %w", err)
	}
	if err := Generated.Validate(schema); err != nil {
		return Evidence{}, err
	}
	return clone(Generated), nil
}

// Validate rejects every stale, missing, extra, reordered, mistagged,
// wrong-terminal, and wrong-polarity row against the live canonical schema.
func (e Evidence) Validate(schema *relations.Schema) error {
	if schema == nil || e.SchemaDigest != schema.Digest().String() {
		return fmt.Errorf("Program supply: stale relation schema")
	}
	want, err := expected(schema)
	if err != nil {
		return err
	}
	digest, err := evidenceDigest(e)
	if err != nil || digest != e.Digest {
		return fmt.Errorf("Program supply: invalid evidence digest")
	}
	for _, row := range e.BinderLaws {
		if (len(row.Positive) == 0) == (len(row.Forbidden) == 0) {
			return fmt.Errorf("Program supply: binder row must have exactly one polarity")
		}
	}
	if !reflect.DeepEqual(e.ProgramLaws, want.ProgramLaws) ||
		!reflect.DeepEqual(e.StaticLaws, want.StaticLaws) ||
		!reflect.DeepEqual(e.BinderLaws, want.BinderLaws) {
		return fmt.Errorf("Program supply: evidence differs from exact typed denominator")
	}
	return nil
}

// Canonical returns the detached package-owned bytes whose SHA-256 is Digest.
// It returns nil when fixed-width identity material is malformed.
func (e Evidence) Canonical() []byte {
	encoded, err := canonicalEvidence(e)
	if err != nil {
		return nil
	}
	return append([]byte(nil), encoded...)
}

// Closure derives a cycle-safe transitive parent closure, including the exact
// terminal relations themselves. No owner, form, or closure row is persisted.
func Closure(schema *relations.Schema, terminals []Reference) ([]Output, error) {
	if schema == nil || len(terminals) == 0 {
		return nil, fmt.Errorf("Program supply: empty terminal closure")
	}
	rows := schema.Rows()
	byReference := make(map[Reference]relations.Row, len(rows))
	byToken := make(map[semanticsource.Token]relations.Row, len(rows))
	for _, row := range rows {
		ref := reference(row.Definition.Token())
		byReference[ref] = row
		byToken[row.Definition.Token()] = row
	}
	stack := make([]semanticsource.Token, 0, len(terminals))
	for _, terminal := range terminals {
		row, ok := byReference[terminal]
		if !ok {
			return nil, fmt.Errorf("Program supply: unknown terminal reference")
		}
		stack = append(stack, row.Definition.Token())
	}
	seen := make(map[semanticsource.Token]bool, len(stack))
	for len(stack) != 0 {
		last := len(stack) - 1
		token := stack[last]
		stack = stack[:last]
		if seen[token] {
			continue
		}
		row, ok := byToken[token]
		if !ok || row.Owner < relations.OwnerProgramSource || row.Owner > relations.OwnerProgramModule {
			return nil, fmt.Errorf("Program supply: terminal closure escaped Program ownership")
		}
		seen[token] = true
		stack = append(stack, row.Parents...)
	}
	result := make([]Output, 0, len(seen))
	for token := range seen {
		row := byToken[token]
		result = append(result, Output{Relation: reference(token), Owner: row.Owner, Form: row.Form})
	}
	sort.Slice(result, func(left, right int) bool { return less(result[left].Relation, result[right].Relation) })
	return result, nil
}

func expected(schema *relations.Schema) (Evidence, error) {
	if schema == nil {
		return Evidence{}, fmt.Errorf("Program supply: nil canonical schema")
	}
	resolve := func(origin semanticsource.Origin, facet semanticsource.Facet) (Reference, error) {
		for _, row := range schema.Rows() {
			token := row.Definition.Token()
			if token.Origin() == origin && token.Facet() == facet {
				return reference(token), nil
			}
		}
		return Reference{}, fmt.Errorf("Program supply: canonical terminal is absent")
	}
	type selector struct {
		origin semanticsource.Origin
		facet  semanticsource.Facet
	}
	terminals := func(selectors ...selector) ([]Reference, error) {
		result := make([]Reference, 0, len(selectors))
		for _, selected := range selectors {
			item, err := resolve(selected.origin, selected.facet)
			if err != nil {
				return nil, err
			}
			result = append(result, item)
		}
		sort.Slice(result, func(left, right int) bool { return less(result[left], result[right]) })
		return result, nil
	}

	programRows := make([]ProgramLawRow, 0, len(programlaw.Requirements()))
	for _, requirement := range programlaw.Requirements() {
		selected := selector{origin: semanticsource.OriginProgramFlowOperators}
		switch requirement.Site {
		case programlaw.SiteUnary:
			switch requirement.Unary {
			case flowkind.UnaryNeg, flowkind.UnaryBitNot:
				selected.facet = semanticsource.FacetProgramFlowUnaryNumeric
			case flowkind.UnaryLen:
				selected.facet = semanticsource.FacetProgramFlowLength
			case flowkind.UnaryNot:
				// Logical negation is an exact member of the authored Operators
				// relation. It has no independent semantic source range.
			default:
				return Evidence{}, fmt.Errorf("Program supply: unknown unary operator")
			}
		case programlaw.SiteBinary:
			switch requirement.Binary {
			case flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul, flowkind.BinaryDiv,
				flowkind.BinaryIDiv, flowkind.BinaryMod, flowkind.BinaryPow:
				selected.facet = semanticsource.FacetProgramFlowArithmetic
			case flowkind.BinaryConcat:
				selected.facet = semanticsource.FacetProgramFlowConcat
			case flowkind.BinaryBitAnd, flowkind.BinaryBitOr, flowkind.BinaryBitXor,
				flowkind.BinaryShiftLeft, flowkind.BinaryShiftRight:
				selected.facet = semanticsource.FacetProgramFlowBitwise
			case flowkind.BinaryEqual, flowkind.BinaryNotEqual:
				selected.facet = semanticsource.FacetProgramFlowEquality
			case flowkind.BinaryLess, flowkind.BinaryLessEqual, flowkind.BinaryGreater, flowkind.BinaryGreaterEqual:
				selected.facet = semanticsource.FacetProgramFlowOrder
			default:
				return Evidence{}, fmt.Errorf("Program supply: unknown binary operator")
			}
		case programlaw.SiteSelect:
			// Short-circuit selection is not a parallel "arms" source. Its
			// control and value branches are one exact Operators relation.
		case programlaw.SiteCall:
			selected.origin = semanticsource.OriginProgramFlowCall
		case programlaw.SiteValues:
			selected.origin = semanticsource.OriginProgramFlowValues
		case programlaw.SiteOutcome:
			selected.origin = semanticsource.OriginProgramFlowOutcome
		default:
			return Evidence{}, fmt.Errorf("Program supply: unknown Program law site")
		}
		terminals, err := terminals(selected)
		if err != nil {
			return Evidence{}, err
		}
		programRows = append(programRows, ProgramLawRow{Requirement: requirement, Terminals: terminals})
	}

	staticRows := make([]StaticLawRow, 0, len(staticlaw.Requirements()))
	for _, family := range staticlaw.Requirements() {
		selected := []selector{{origin: semanticsource.OriginProgramStatic}}
		switch family {
		case staticlaw.FamilyTypeRef:
			selected = []selector{{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticTypeRef}}
		case staticlaw.FamilySignature:
			selected = []selector{
				{origin: semanticsource.OriginProgramStatic},
				{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticFunctionContract},
			}
		case staticlaw.FamilyAssertion:
			selected = []selector{
				{origin: semanticsource.OriginProgramStatic},
				{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticClaimTarget},
			}
		case staticlaw.FamilyTypeOf:
			selected = []selector{{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticTypeof}}
		case staticlaw.FamilyAnnotated:
			selected = []selector{{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticAnnotation}}
		case staticlaw.FamilyPrimitive, staticlaw.FamilyLiteral, staticlaw.FamilyOptional,
			staticlaw.FamilyUnion, staticlaw.FamilyIntersection, staticlaw.FamilyGeneric,
			staticlaw.FamilyArray, staticlaw.FamilyMap, staticlaw.FamilyRecord,
			staticlaw.FamilyKeyOf,
			staticlaw.FamilyIndexAccess, staticlaw.FamilyConditional:
		default:
			return Evidence{}, fmt.Errorf("Program supply: unknown static law family")
		}
		terminals, err := terminals(selected...)
		if err != nil {
			return Evidence{}, err
		}
		staticRows = append(staticRows, StaticLawRow{Family: family, Terminals: terminals})
	}

	binderRows := make([]BinderRow, 0, len(binder.Required()))
	for _, requirement := range binder.Required() {
		selected := []selector{{origin: semanticsource.OriginProgramStatic}}
		forbidden := false
		switch requirement.Transition {
		case binder.TransitionTypeDeclaration:
		case binder.TransitionTypeParameter, binder.TransitionUnresolvedTypeReference, binder.TransitionQualifiedTypeRoot:
			selected = []selector{{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticTypeRef}}
		case binder.TransitionRuntimePrimitive:
			selected = []selector{
				{origin: semanticsource.OriginProgramStatic},
				{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticTypeValueTarget},
			}
		case binder.TransitionRuntimeDeclaration:
			selected = []selector{
				{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticTypeValueTarget},
				{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticTypeRef},
			}
		case binder.TransitionRuntimeShadowRejected:
			selected = []selector{
				{origin: semanticsource.OriginProgramFlowTypeValue},
				{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticTypeValueTarget},
			}
			forbidden = true
		case binder.TransitionStaticPublicationPair:
			selected = []selector{{origin: semanticsource.OriginProgramStatic, facet: semanticsource.FacetProgramStaticPublication}}
		case binder.TransitionDirectRequireGlobal:
			selected = []selector{{origin: semanticsource.OriginProgramModuleImport}}
		default:
			return Evidence{}, fmt.Errorf("Program supply: unknown binder transition")
		}
		terminals, err := terminals(selected...)
		if err != nil {
			return Evidence{}, err
		}
		row := BinderRow{Requirement: requirement}
		if forbidden {
			row.Forbidden = terminals
		} else {
			row.Positive = terminals
		}
		binderRows = append(binderRows, row)
	}

	evidence := Evidence{
		SchemaDigest: schema.Digest().String(),
		ProgramLaws:  programRows,
		StaticLaws:   staticRows,
		BinderLaws:   binderRows,
	}
	digest, err := evidenceDigest(evidence)
	if err != nil {
		return Evidence{}, err
	}
	evidence.Digest = digest
	return evidence, nil
}

func evidenceDigest(e Evidence) (string, error) {
	encoded, err := canonicalEvidence(e)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func canonicalEvidence(e Evidence) ([]byte, error) {
	schemaDigest, err := hex.DecodeString(e.SchemaDigest)
	if err != nil || len(schemaDigest) != sha256.Size {
		return nil, fmt.Errorf("Program supply: malformed schema digest")
	}
	var out bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&out, digestDomain, digestVersion); err != nil {
		return nil, err
	}
	if err := writer.String(e.SchemaDigest); err != nil {
		return nil, err
	}
	if err := writer.Count(uint64(len(e.ProgramLaws))); err != nil {
		return nil, err
	}
	for _, row := range e.ProgramLaws {
		if err := writer.Record(programRecord); err != nil {
			return nil, err
		}
		for _, value := range [...]uint64{uint64(row.Requirement.Site), uint64(row.Requirement.Unary), uint64(row.Requirement.Binary), uint64(row.Requirement.Select), uint64(row.Requirement.Call), uint64(row.Requirement.Values), uint64(row.Requirement.Outcome)} {
			if err := writer.Uint(value); err != nil {
				return nil, err
			}
		}
		if err := writeReferences(&writer, row.Terminals); err != nil {
			return nil, err
		}
	}
	if err := writer.Count(uint64(len(e.StaticLaws))); err != nil {
		return nil, err
	}
	for _, row := range e.StaticLaws {
		if err := writer.Record(staticRecord); err != nil {
			return nil, err
		}
		if err := writer.Uint(uint64(row.Family)); err != nil {
			return nil, err
		}
		if err := writeReferences(&writer, row.Terminals); err != nil {
			return nil, err
		}
	}
	if err := writer.Count(uint64(len(e.BinderLaws))); err != nil {
		return nil, err
	}
	for _, row := range e.BinderLaws {
		if err := writer.Record(binderRecord); err != nil {
			return nil, err
		}
		if err := writer.Uint(uint64(row.Requirement.Transition)); err != nil {
			return nil, err
		}
		if err := writeReferences(&writer, row.Positive); err != nil {
			return nil, err
		}
		if err := writeReferences(&writer, row.Forbidden); err != nil {
			return nil, err
		}
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return append([]byte(nil), out.Bytes()...), nil
}

func writeReferences(writer *framing.Writer, references []Reference) error {
	if err := writer.Count(uint64(len(references))); err != nil {
		return err
	}
	for _, item := range references {
		for _, value := range [...]uint64{uint64(item.Origin), uint64(item.Facet), uint64(item.Revision)} {
			if err := writer.Uint(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func reference(token semanticsource.Token) Reference {
	return Reference{Origin: token.Origin(), Facet: token.Facet(), Revision: token.Revision()}
}

func less(left, right Reference) bool {
	if left.Origin != right.Origin {
		return left.Origin < right.Origin
	}
	if left.Facet != right.Facet {
		return left.Facet < right.Facet
	}
	return left.Revision < right.Revision
}

func clone(e Evidence) Evidence {
	e.ProgramLaws = append([]ProgramLawRow(nil), e.ProgramLaws...)
	for index := range e.ProgramLaws {
		e.ProgramLaws[index].Terminals = append([]Reference(nil), e.ProgramLaws[index].Terminals...)
	}
	e.StaticLaws = append([]StaticLawRow(nil), e.StaticLaws...)
	for index := range e.StaticLaws {
		e.StaticLaws[index].Terminals = append([]Reference(nil), e.StaticLaws[index].Terminals...)
	}
	e.BinderLaws = append([]BinderRow(nil), e.BinderLaws...)
	for index := range e.BinderLaws {
		e.BinderLaws[index].Positive = append([]Reference(nil), e.BinderLaws[index].Positive...)
		e.BinderLaws[index].Forbidden = append([]Reference(nil), e.BinderLaws[index].Forbidden...)
	}
	return e
}
