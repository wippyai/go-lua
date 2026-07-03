package judgment

import "sort"

// CodeSpec is the single registration record for a semantic judgment code.
// Renderers and policy layers may reference these specs, but producers should
// not invent code metadata locally.
type CodeSpec struct {
	Code             Code
	SubjectKind      SubjectKind
	RequiredEvidence []EvidenceKind
	DefaultVerdict   Verdict
}

// Registry is an immutable table of known judgment codes.
type Registry struct {
	specs map[Code]CodeSpec
}

var defaultRegistry = NewRegistry([]CodeSpec{
	{
		Code:        CodeCallArgType,
		SubjectKind: SubjectCallArgument,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceUserAssertion,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictUnknown,
	},
	{
		Code:        CodeCallArity,
		SubjectKind: SubjectCallExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceUserAssertion,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeCallCallee,
		SubjectKind: SubjectCallExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceUserAssertion,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictUnknown,
	},
	{
		Code:        CodeAssignment,
		SubjectKind: SubjectPath,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceUserAssertion,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictUnknown,
	},
	{
		Code:        CodeAssignmentTarget,
		SubjectKind: SubjectPath,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeReturn,
		SubjectKind: SubjectReturnValue,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceUserAssertion,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictUnknown,
	},
	{
		Code:        CodeNonNilAssertion,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeNumericForOperand,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeFrozenTable,
		SubjectKind: SubjectPath,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeLifecycle,
		SubjectKind: SubjectPath,
		RequiredEvidence: []EvidenceKind{
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeUnusedLocal,
		SubjectKind: SubjectPath,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeDeadAssignment,
		SubjectKind: SubjectPath,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeChannelSelect,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeDiscriminatedUnion,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeOptional,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeResultShape,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeRegistration,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeTableDispatch,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeUnresolvedValue,
		SubjectKind: SubjectPath,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeUnresolvedType,
		SubjectKind: SubjectPath,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeRedundantCondition,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeMemberRead,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
			EvidenceMissingProof,
		},
		DefaultVerdict: VerdictRefuted,
	},
	{
		Code:        CodeConcatOperand,
		SubjectKind: SubjectExpression,
		RequiredEvidence: []EvidenceKind{
			EvidenceAbstractFact,
		},
		DefaultVerdict: VerdictRefuted,
	},
})

// DefaultRegistry returns the standard judgment-code registry.
func DefaultRegistry() Registry {
	return defaultRegistry
}

// NewRegistry builds a registry from specs. Code ownership is explicit and
// one-spec-per-code; empty or duplicate codes are construction errors.
func NewRegistry(specs []CodeSpec) Registry {
	out := make(map[Code]CodeSpec, len(specs))
	for _, spec := range specs {
		if spec.Code == "" {
			panic("judgment: empty code spec")
		}
		if _, exists := out[spec.Code]; exists {
			panic("judgment: duplicate code spec for " + string(spec.Code))
		}
		out[spec.Code] = cloneCodeSpec(spec)
	}
	return Registry{specs: out}
}

// Lookup returns the registered spec for code.
func (r Registry) Lookup(code Code) (CodeSpec, bool) {
	spec, ok := r.specs[code]
	if !ok {
		return CodeSpec{}, false
	}
	return cloneCodeSpec(spec), true
}

// Codes returns every registered code in deterministic order.
func (r Registry) Codes() []Code {
	out := make([]Code, 0, len(r.specs))
	for code := range r.specs {
		out = append(out, code)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

// Validate reports whether a judgment matches its code registration.
func (r Registry) Validate(j Judgment) bool {
	spec, ok := r.Lookup(j.Code)
	if !ok {
		return false
	}
	if spec.SubjectKind != SubjectUnknown && j.Subject.Kind != spec.SubjectKind {
		return false
	}
	for _, required := range spec.RequiredEvidence {
		if !j.Evidence.Has(required) {
			return false
		}
	}
	return true
}

func cloneCodeSpec(spec CodeSpec) CodeSpec {
	spec.RequiredEvidence = append([]EvidenceKind(nil), spec.RequiredEvidence...)
	return spec
}
