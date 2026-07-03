package judgment

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

// NewRegistry builds a registry from specs. Later duplicate codes are ignored;
// code ownership should be explicit and one-spec-per-code.
func NewRegistry(specs []CodeSpec) Registry {
	out := make(map[Code]CodeSpec, len(specs))
	for _, spec := range specs {
		if spec.Code == "" {
			continue
		}
		if _, exists := out[spec.Code]; exists {
			continue
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
