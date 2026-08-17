package programsupply

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"

	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/binder"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/programlaw"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/staticlaw"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

const (
	digestDomain  = "program.grammarproof.requirements.programsupply"
	digestVersion = 1

	programRecord = 1
	staticRecord  = 2
	binderRecord  = 3
)

// Build derives exact terminal IDs from the one generated denominator
// catalog and the three independently owned closed law denominators.
func Build() (Evidence, error) {
	entries := denominator.GeneratedRelationEntries()
	evidence, err := expected(entries)
	if err != nil {
		return Evidence{}, err
	}
	if err := evidence.Validate(entries); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

// Current validates and returns a detached copy of the generated evidence.
func Current() (Evidence, error) {
	entries := denominator.GeneratedRelationEntries()
	if err := Generated.Validate(entries); err != nil {
		return Evidence{}, err
	}
	return clone(Generated), nil
}

// Validate rejects every stale, missing, extra, reordered, wrong-terminal,
// and wrong-polarity row against the live generated denominator catalog.
func (e Evidence) Validate(entries []*denominator.RelationEntry) error {
	want, err := expected(entries)
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
func (e Evidence) Canonical() []byte {
	encoded, err := canonicalEvidence(e)
	if err != nil {
		return nil
	}
	return append([]byte(nil), encoded...)
}

// Closure derives a cycle-safe transitive parent closure, including the exact
// terminal relations themselves. Owner and form are read from the generated
// denominator entries; no parallel relation projection is persisted.
func Closure(entries []*denominator.RelationEntry, terminals []schema.EntryID) ([]Output, error) {
	if len(entries) == 0 || len(terminals) == 0 {
		return nil, fmt.Errorf("Program supply: empty terminal closure")
	}
	byID := make(map[schema.EntryID]*denominator.RelationEntry, len(entries))
	for _, entry := range entries {
		if entry == nil || !entry.ID().Available() {
			return nil, fmt.Errorf("Program supply: malformed denominator entry")
		}
		byID[entry.ID()] = entry
	}
	stack := append([]schema.EntryID(nil), terminals...)
	seen := make(map[schema.EntryID]bool, len(stack))
	for len(stack) != 0 {
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]
		if seen[id] {
			continue
		}
		entry, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("Program supply: unknown terminal ID")
		}
		if owner := entry.Owner(); owner < denominator.RelationOwnerProgramSource || owner > denominator.RelationOwnerProgramModule {
			return nil, fmt.Errorf("Program supply: terminal closure escaped Program ownership")
		}
		seen[id] = true
		stack = append(stack, entry.Parents()...)
	}
	result := make([]Output, 0, len(seen))
	for id := range seen {
		entry := byID[id]
		result = append(result, Output{Relation: id, Owner: entry.Owner(), Form: entry.Form()})
	}
	sort.Slice(result, func(left, right int) bool { return less(result[left].Relation, result[right].Relation) })
	return result, nil
}

const (
	programFlowOperators          = schema.Key("ProgramFlowOperators@-")
	programFlowUnaryNumeric       = schema.Key("ProgramFlowOperators@ProgramFlowUnaryNumeric")
	programFlowLength             = schema.Key("ProgramFlowOperators@ProgramFlowLength")
	programFlowArithmetic         = schema.Key("ProgramFlowOperators@ProgramFlowArithmetic")
	programFlowConcat             = schema.Key("ProgramFlowOperators@ProgramFlowConcat")
	programFlowBitwise            = schema.Key("ProgramFlowOperators@ProgramFlowBitwise")
	programFlowEquality           = schema.Key("ProgramFlowOperators@ProgramFlowEquality")
	programFlowOrder              = schema.Key("ProgramFlowOperators@ProgramFlowOrder")
	programFlowCall               = schema.Key("ProgramFlowCall@-")
	programFlowValues             = schema.Key("ProgramFlowValues@-")
	programFlowOutcome            = schema.Key("ProgramFlowOutcome@-")
	programFlowTypeValue          = schema.Key("ProgramFlowTypeValue@-")
	programStatic                 = schema.Key("ProgramStatic@-")
	programStaticTypeRef          = schema.Key("ProgramStatic@ProgramStaticTypeRef")
	programStaticFunctionContract = schema.Key("ProgramStatic@ProgramStaticFunctionContract")
	programStaticClaimTarget      = schema.Key("ProgramStatic@ProgramStaticClaimTarget")
	programStaticTypeValueTarget  = schema.Key("ProgramStatic@ProgramStaticTypeValueTarget")
	programStaticTypeof           = schema.Key("ProgramStatic@ProgramStaticTypeof")
	programStaticAnnotation       = schema.Key("ProgramStatic@ProgramStaticAnnotation")
	programStaticPublication      = schema.Key("ProgramStatic@ProgramStaticPublication")
	programModuleImport           = schema.Key("ProgramModuleImport@-")
)

func expected(entries []*denominator.RelationEntry) (Evidence, error) {
	if len(entries) == 0 {
		return Evidence{}, fmt.Errorf("Program supply: empty generated denominator catalog")
	}
	resolve := func(key schema.Key) (schema.EntryID, error) {
		for _, entry := range entries {
			if entry != nil && entry.Key() == key {
				return entry.ID(), nil
			}
		}
		return schema.EntryID{}, fmt.Errorf("Program supply: canonical terminal %q is absent", key)
	}
	type selector struct{ key schema.Key }
	terminals := func(selectors ...selector) ([]schema.EntryID, error) {
		result := make([]schema.EntryID, 0, len(selectors))
		for _, selected := range selectors {
			id, err := resolve(selected.key)
			if err != nil {
				return nil, err
			}
			result = append(result, id)
		}
		sort.Slice(result, func(left, right int) bool { return less(result[left], result[right]) })
		return result, nil
	}

	programRows := make([]ProgramLawRow, 0, len(programlaw.Requirements()))
	for _, requirement := range programlaw.Requirements() {
		selected := selector{key: programFlowOperators}
		switch requirement.Site {
		case programlaw.SiteUnary:
			switch requirement.Unary {
			case flowkind.UnaryNeg, flowkind.UnaryBitNot:
				selected.key = programFlowUnaryNumeric
			case flowkind.UnaryLen:
				selected.key = programFlowLength
			case flowkind.UnaryNot:
			default:
				return Evidence{}, fmt.Errorf("Program supply: unknown unary operator")
			}
		case programlaw.SiteBinary:
			switch requirement.Binary {
			case flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul, flowkind.BinaryDiv,
				flowkind.BinaryIDiv, flowkind.BinaryMod, flowkind.BinaryPow:
				selected.key = programFlowArithmetic
			case flowkind.BinaryConcat:
				selected.key = programFlowConcat
			case flowkind.BinaryBitAnd, flowkind.BinaryBitOr, flowkind.BinaryBitXor,
				flowkind.BinaryShiftLeft, flowkind.BinaryShiftRight:
				selected.key = programFlowBitwise
			case flowkind.BinaryEqual, flowkind.BinaryNotEqual:
				selected.key = programFlowEquality
			case flowkind.BinaryLess, flowkind.BinaryLessEqual, flowkind.BinaryGreater, flowkind.BinaryGreaterEqual:
				selected.key = programFlowOrder
			default:
				return Evidence{}, fmt.Errorf("Program supply: unknown binary operator")
			}
		case programlaw.SiteSelect:
		case programlaw.SiteCall:
			selected.key = programFlowCall
		case programlaw.SiteValues:
			selected.key = programFlowValues
		case programlaw.SiteOutcome:
			selected.key = programFlowOutcome
		default:
			return Evidence{}, fmt.Errorf("Program supply: unknown Program law site")
		}
		values, err := terminals(selected)
		if err != nil {
			return Evidence{}, err
		}
		programRows = append(programRows, ProgramLawRow{Requirement: requirement, Terminals: values})
	}

	staticRows := make([]StaticLawRow, 0, len(staticlaw.Requirements()))
	for _, family := range staticlaw.Requirements() {
		selected := []selector{{key: programStatic}}
		switch family {
		case staticlaw.FamilyTypeRef:
			selected = []selector{{key: programStaticTypeRef}}
		case staticlaw.FamilySignature:
			selected = []selector{{key: programStatic}, {key: programStaticFunctionContract}}
		case staticlaw.FamilyAssertion:
			selected = []selector{{key: programStatic}, {key: programStaticClaimTarget}}
		case staticlaw.FamilyTypeOf:
			selected = []selector{{key: programStaticTypeof}}
		case staticlaw.FamilyAnnotated:
			selected = []selector{{key: programStaticAnnotation}}
		case staticlaw.FamilyPrimitive, staticlaw.FamilyLiteral, staticlaw.FamilyOptional,
			staticlaw.FamilyUnion, staticlaw.FamilyIntersection, staticlaw.FamilyGeneric,
			staticlaw.FamilyArray, staticlaw.FamilyMap, staticlaw.FamilyRecord,
			staticlaw.FamilyKeyOf, staticlaw.FamilyIndexAccess, staticlaw.FamilyConditional:
		default:
			return Evidence{}, fmt.Errorf("Program supply: unknown static law family")
		}
		values, err := terminals(selected...)
		if err != nil {
			return Evidence{}, err
		}
		staticRows = append(staticRows, StaticLawRow{Family: family, Terminals: values})
	}

	binderRows := make([]BinderRow, 0, len(binder.Required()))
	for _, requirement := range binder.Required() {
		selected := []selector{{key: programStatic}}
		forbidden := false
		switch requirement.Transition {
		case binder.TransitionTypeDeclaration:
		case binder.TransitionTypeParameter, binder.TransitionUnresolvedTypeReference, binder.TransitionQualifiedTypeRoot:
			selected = []selector{{key: programStaticTypeRef}}
		case binder.TransitionRuntimePrimitive:
			selected = []selector{{key: programStatic}, {key: programStaticTypeValueTarget}}
		case binder.TransitionRuntimeDeclaration:
			selected = []selector{{key: programStaticTypeValueTarget}, {key: programStaticTypeRef}}
		case binder.TransitionRuntimeShadowRejected:
			selected = []selector{{key: programFlowTypeValue}, {key: programStaticTypeValueTarget}}
			forbidden = true
		case binder.TransitionStaticPublicationPair:
			selected = []selector{{key: programStaticPublication}}
		case binder.TransitionDirectRequireGlobal:
			selected = []selector{{key: programModuleImport}}
		default:
			return Evidence{}, fmt.Errorf("Program supply: unknown binder transition")
		}
		values, err := terminals(selected...)
		if err != nil {
			return Evidence{}, err
		}
		row := BinderRow{Requirement: requirement}
		if forbidden {
			row.Forbidden = values
		} else {
			row.Positive = values
		}
		binderRows = append(binderRows, row)
	}

	evidence := Evidence{ProgramLaws: programRows, StaticLaws: staticRows, BinderLaws: binderRows}
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
	var out bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&out, digestDomain, digestVersion); err != nil {
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
		if err := writeIDs(&writer, row.Terminals); err != nil {
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
		if err := writeIDs(&writer, row.Terminals); err != nil {
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
		if err := writeIDs(&writer, row.Positive); err != nil {
			return nil, err
		}
		if err := writeIDs(&writer, row.Forbidden); err != nil {
			return nil, err
		}
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return append([]byte(nil), out.Bytes()...), nil
}

func writeIDs(writer *framing.Writer, ids []schema.EntryID) error {
	if err := writer.Count(uint64(len(ids))); err != nil {
		return err
	}
	for _, id := range ids {
		if !id.Available() {
			return fmt.Errorf("Program supply: malformed EntryID")
		}
		if err := writer.Bytes(id[:]); err != nil {
			return err
		}
	}
	return nil
}

func less(left, right schema.EntryID) bool {
	return bytes.Compare(left[:], right[:]) < 0
}

func clone(e Evidence) Evidence {
	e.ProgramLaws = append([]ProgramLawRow(nil), e.ProgramLaws...)
	for index := range e.ProgramLaws {
		e.ProgramLaws[index].Terminals = append([]schema.EntryID(nil), e.ProgramLaws[index].Terminals...)
	}
	e.StaticLaws = append([]StaticLawRow(nil), e.StaticLaws...)
	for index := range e.StaticLaws {
		e.StaticLaws[index].Terminals = append([]schema.EntryID(nil), e.StaticLaws[index].Terminals...)
	}
	e.BinderLaws = append([]BinderRow(nil), e.BinderLaws...)
	for index := range e.BinderLaws {
		e.BinderLaws[index].Positive = append([]schema.EntryID(nil), e.BinderLaws[index].Positive...)
		e.BinderLaws[index].Forbidden = append([]schema.EntryID(nil), e.BinderLaws[index].Forbidden...)
	}
	return e
}
