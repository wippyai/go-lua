// Package occurrence owns parser-constructor field-state requirements and
// their temporary-parser witnesses. It deliberately stops before Program law
// discharge: an observed AST occurrence is not evidence of correct lowering.
package occurrence

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// Context is the parser-owned semantic class in which a constructor first
// crosses into lowering. More precise Program contexts (including final
// Values position and static typeof nesting) remain separate obligations.
type Context uint8

const (
	ContextInvalid Context = iota
	ContextStatement
	ContextExpression
	ContextStaticType
	ContextStructural
)

// Requirement is one independently schema-derived AST field state. The type
// and field symbols identify compiler syntax; Context and State are closed
// proof vocabulary rather than string-labelled laws.
type Requirement struct {
	Constructor string
	Field       int
	State       astcodec.FieldState
	Context     Context
}

// Witness ties one observed parser field state to one exact accepted source.
// It is evidence for a schema-derived Requirement, never a way to add or
// remove requirements from the denominator.
type Witness struct {
	Requirement Requirement
	Source      string
}

// Report is a parser-state observation inventory, not a Program-law report.
// Residue contains every required state without a temporary-parser witness.
// Neither Observed nor Residue implies that a state has a typed Program
// relation: that requires an exact semantic-family source law.
type Report struct {
	Required []Requirement
	Observed []Requirement
	Residue  []Requirement
	Witness  []Witness
}

func (r Report) RequiredCount() int { return len(r.Required) }
func (r Report) ObservedCount() int { return len(r.Observed) }
func (r Report) ResidueCount() int  { return len(r.Residue) }

// Derive constructs the finite constructor x field-representation-state x
// parser-context denominator directly from AST declarations. It over-approximates
// parser state until an action-level impossibility proof is present; that is
// intentional, because silently treating unobserved states as impossible was
// the former false-completion defect.
func Derive(schema parsersource.Schema) (Report, error) {
	report := Report{}
	for _, constructor := range schema.Constructors {
		if !constructor.Semantic {
			continue
		}
		context := contextFor(constructor.Class)
		for _, field := range constructor.Fields {
			for _, state := range statesFor(field.Form) {
				report.Required = append(report.Required, Requirement{
					Constructor: constructor.Name, Field: field.Ordinal, State: state, Context: context,
				})
			}
		}
	}
	sortRequirements(report.Required)
	return report, nil
}

// Observe records only exact temporary-parser AST field states. A trace for a
// parser production may observe multiple rows but never invents one; every
// trace field must be represented in the independently derived schema or the
// inventory fails closed. This function deliberately cannot discharge a
// Program-law obligation.
func Observe(report Report, schema parsersource.Schema, traces []grammarproof.SemanticTrace) (Report, error) {
	if len(report.Required) == 0 {
		return Report{}, fmt.Errorf("occurrence requirements: empty denominator")
	}
	fields, err := schemaFields(schema)
	if err != nil {
		return Report{}, err
	}
	required := make(map[Requirement]bool, len(report.Required))
	for _, requirement := range report.Required {
		required[requirement] = true
	}
	seen := make(map[Requirement]string)
	for _, trace := range traces {
		if trace.Production == "" || trace.Source == "" {
			return Report{}, fmt.Errorf("occurrence requirements: incomplete trace provenance")
		}
		for _, occurrence := range trace.Occurrences {
			constructor, exists := fields[occurrence.Type]
			if !exists {
				return Report{}, fmt.Errorf("occurrence requirements: parser emitted unknown ast.%s", occurrence.Type)
			}
			if !constructor.semantic {
				continue
			}
			context := contextFor(constructor.class)
			for _, field := range occurrence.Fields {
				ordinal, exists := constructor.byName[field.Name]
				if !exists {
					if parsersource.StructuralEmbedding(field.Name) {
						continue
					}
					return Report{}, fmt.Errorf("occurrence requirements: ast.%s emitted unknown exported field %s", occurrence.Type, field.Name)
				}
				requirement := Requirement{Constructor: occurrence.Type, Field: ordinal, State: field.State, Context: context}
				if !required[requirement] {
					return Report{}, fmt.Errorf("occurrence requirements: ast.%s field %s has unclassified state %d", occurrence.Type, field.Name, field.State)
				}
				if current, exists := seen[requirement]; !exists || trace.Source < current {
					seen[requirement] = trace.Source
				}
			}
		}
	}
	report.Observed = make([]Requirement, 0, len(seen))
	report.Residue = make([]Requirement, 0, len(report.Required)-len(seen))
	for _, requirement := range report.Required {
		if source, exists := seen[requirement]; exists {
			report.Observed = append(report.Observed, requirement)
			report.Witness = append(report.Witness, Witness{Requirement: requirement, Source: source})
		} else {
			report.Residue = append(report.Residue, requirement)
		}
	}
	sortRequirements(report.Observed)
	sortRequirements(report.Residue)
	sort.Slice(report.Witness, func(left, right int) bool {
		if requirementLess(report.Witness[left].Requirement, report.Witness[right].Requirement) {
			return true
		}
		if requirementLess(report.Witness[right].Requirement, report.Witness[left].Requirement) {
			return false
		}
		return report.Witness[left].Source < report.Witness[right].Source
	})
	return report, nil
}

type constructorFields struct {
	class    parsersource.ConstructorClass
	semantic bool
	byName   map[string]int
}

func schemaFields(schema parsersource.Schema) (map[string]constructorFields, error) {
	result := make(map[string]constructorFields, len(schema.Constructors))
	for _, constructor := range schema.Constructors {
		if constructor.Name == "" || constructor.Class == 0 {
			return nil, fmt.Errorf("occurrence requirements: invalid AST constructor schema")
		}
		entry := constructorFields{class: constructor.Class, semantic: constructor.Semantic, byName: make(map[string]int, len(constructor.Fields))}
		for _, field := range constructor.Fields {
			if field.Name == "" || field.Ordinal < 0 {
				return nil, fmt.Errorf("occurrence requirements: invalid schema field for ast.%s", constructor.Name)
			}
			if _, exists := entry.byName[field.Name]; exists {
				return nil, fmt.Errorf("occurrence requirements: duplicate schema field %s for ast.%s", field.Name, constructor.Name)
			}
			entry.byName[field.Name] = field.Ordinal
		}
		result[constructor.Name] = entry
	}
	return result, nil
}

func contextFor(class parsersource.ConstructorClass) Context {
	switch class {
	case parsersource.ConstructorStatement:
		return ContextStatement
	case parsersource.ConstructorExpression:
		return ContextExpression
	case parsersource.ConstructorTypeExpression:
		return ContextStaticType
	default:
		return ContextStructural
	}
}

func statesFor(form parsersource.FieldForm) []astcodec.FieldState {
	switch form {
	case parsersource.FieldFormOptional, parsersource.FieldFormMapping, parsersource.FieldFormInterface:
		return []astcodec.FieldState{astcodec.FieldStateAbsent, astcodec.FieldStatePresent}
	case parsersource.FieldFormSequence, parsersource.FieldFormString:
		return []astcodec.FieldState{astcodec.FieldStateEmpty, astcodec.FieldStateNonEmpty}
	case parsersource.FieldFormBool:
		return []astcodec.FieldState{astcodec.FieldStateFalse, astcodec.FieldStateTrue}
	case parsersource.FieldFormNamed, parsersource.FieldFormScalar:
		return []astcodec.FieldState{astcodec.FieldStateZero, astcodec.FieldStateNonZero}
	default:
		return nil
	}
}

func sortRequirements(requirements []Requirement) {
	sort.Slice(requirements, func(left, right int) bool {
		return requirementLess(requirements[left], requirements[right])
	})
}

func requirementLess(left, right Requirement) bool {
	if left.Constructor != right.Constructor {
		return left.Constructor < right.Constructor
	}
	if left.Field != right.Field {
		return left.Field < right.Field
	}
	if left.Context != right.Context {
		return left.Context < right.Context
	}
	return left.State < right.State
}
