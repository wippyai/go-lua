package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// OperatorKind names a canonical bound transfer family. It names semantics,
// never a factor representation, callback, or symbolic carrier.
type OperatorKind string

const (
	OperatorApply                 OperatorKind = "apply"
	OperatorPathReplacement       OperatorKind = "path-replacement"
	OperatorPathInvalidation      OperatorKind = "path-invalidation"
	OperatorIndexMutation         OperatorKind = "index-mutation"
	OperatorAllocationTemplate    OperatorKind = "allocation-template"
	OperatorObjectMaterialization OperatorKind = "object-materialization"
	OperatorEnvironmentWrite      OperatorKind = "environment-write"
	OperatorChannelSelect         OperatorKind = "channel-select"
	OperatorBranchRelations       OperatorKind = "branch-relations"
	OperatorCallResults           OperatorKind = "call-results"
	OperatorPresenceImplications  OperatorKind = "presence-implications"
	OperatorLoopControl           OperatorKind = "loop-control"
	OperatorGenericFor            OperatorKind = "generic-for"
	OperatorRootAssignment        OperatorKind = "root-assignment"
	OperatorCovariantExposure     OperatorKind = "covariant-exposure"
	OperatorContribution          OperatorKind = "contribution"
	OperatorExternalCall          OperatorKind = "external-call"
	OperatorOutcome               OperatorKind = "outcome"
	OperatorNonreturning          OperatorKind = "nonreturning"
	OperatorDefinition            OperatorKind = "definition"
	OperatorResource              OperatorKind = "resource"
	OperatorEntry                 OperatorKind = "entry"
	OperatorPublication           OperatorKind = "publication"
)

// AccessRole is a semantic access category. A stage-2 lowering maps these
// roles to concrete entry selectors, equation cells, and coordinate roots;
// neither the catalog nor its verifier assumes a particular DAG/row/BDD form.
type AccessRole string

const (
	AccessFlow              AccessRole = "flow"
	AccessEntry             AccessRole = "entry"
	AccessNodeEntry         AccessRole = "node-entry"
	AccessPublished         AccessRole = "published"
	AccessCalleeOutcome     AccessRole = "callee-outcome"
	AccessClosureDefinition AccessRole = "closure-definition"
	AccessGuard             AccessRole = "guard"
	AccessState             AccessRole = "state"
	AccessOutcome           AccessRole = "outcome"
	AccessDiagnostic        AccessRole = "diagnostic"
	AccessAllocation        AccessRole = "allocation"
	AccessBoundary          AccessRole = "boundary"
)

// OutcomeKind is an occurrence-local outcome alphabet. It is deliberately not
// an equivalence relation and cannot grant another occurrence producer rights.
type OutcomeKind string

const (
	OutcomeNormal       OutcomeKind = "normal"
	OutcomeNonreturning OutcomeKind = "nonreturning"
	OutcomeProtected    OutcomeKind = "protected"
	OutcomeSuspension   OutcomeKind = "suspension"
)

// ContractSelector is an exact representation-neutral semantic selector.
// Identity is its role plus stable name; Root is optional only for selectors
// that name an entry/cell surface rather than a coordinate root.
type ContractSelector struct {
	Role AccessRole
	Name string
	Root formal.Root
}

func (s ContractSelector) valid() bool {
	return validAccessRole(s.Role) && s.Name != "" && (s.Root == (formal.Root{}) || s.Root.Valid())
}

func (s ContractSelector) less(other ContractSelector) bool {
	if s.Role != other.Role {
		return s.Role < other.Role
	}
	if s.Name != other.Name {
		return s.Name < other.Name
	}
	return s.Root.Less(other.Root)
}

func (s ContractSelector) equal(other ContractSelector) bool {
	return s.Role == other.Role && s.Name == other.Name && s.Root == other.Root
}

// ContractDependency is content-addressed. Versions, pointers, and artifact
// names are intentionally excluded from dependency identity.
type ContractDependency struct {
	Kind string
	ID   ContentID
}

func (d ContractDependency) valid() bool { return d.Kind != "" && d.ID.Valid() }

func (d ContractDependency) less(other ContractDependency) bool {
	if d.Kind != other.Kind {
		return d.Kind < other.Kind
	}
	return string(d.ID[:]) < string(other.ID[:])
}

// DiagnosticOwner partitions source-owned checks from caller-owned residual
// obligations. Diagnostics are observations, never coordinate writes.
type DiagnosticOwner string

const (
	DiagnosticOwnerCalleeCheck DiagnosticOwner = "callee-check"
	DiagnosticOwnerApplication DiagnosticOwner = "application"
)

// DiagnosticDescriptor is a portable candidate, not a rendered diagnostic.
// Its owner determines the publication boundary; a caller span is forbidden
// from crossing an artifact boundary.
type DiagnosticDescriptor struct {
	Candidate      string
	Owner          DiagnosticOwner
	SourceAnchor   ContentID
	GuardAtoms     []string
	ReadSet        []ContractSelector
	Predicate      string
	EvidenceRecipe string
	BoundaryLens   string
}

func (d DiagnosticDescriptor) valid() bool {
	if d.Candidate == "" || !d.SourceAnchor.Valid() || d.Predicate == "" || d.EvidenceRecipe == "" ||
		(d.Owner != DiagnosticOwnerCalleeCheck && d.Owner != DiagnosticOwnerApplication) ||
		(d.Owner == DiagnosticOwnerApplication && d.BoundaryLens == "") ||
		(d.Owner == DiagnosticOwnerCalleeCheck && d.BoundaryLens != "") {
		return false
	}
	return selectorsValid(d.ReadSet) && stringsSortedUnique(d.GuardAtoms)
}

// OperatorContract is the complete declared semantic footprint of exactly one
// operator occurrence. ExecuteBound is intentionally absent: stage 2 supplies
// the one canonical kernel binding while this immutable record prevents that
// kernel from acquiring a hidden read/write surface.
type OperatorContract struct {
	Kind              OperatorKind
	Occurrence        formal.OccurrenceID
	Operands          []AccessRole
	Reads             []ContractSelector
	Writes            []ContractSelector
	GuardAtoms        []string
	Advances          []formal.LexicalClassID
	AliasSupport      []formal.LexicalClassID
	WriteAlphabet     []formal.Root
	Outcomes          []OutcomeKind
	DiagnosticOutputs []DiagnosticDescriptor
	Dependencies      []ContractDependency
}

func (c OperatorContract) CanonicalBytes() []byte {
	canonical, err := canonicalOperatorContract(c)
	if err != nil {
		return nil
	}
	encoded := make([]byte, 0, 512)
	encoded = appendCanonicalText(encoded, "operator-contract/content-v1")
	encoded = appendCanonicalText(encoded, string(canonical.Kind))
	encoded = appendCanonicalOccurrence(encoded, canonical.Occurrence)
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.Operands)))
	for _, operand := range canonical.Operands {
		encoded = appendCanonicalText(encoded, string(operand))
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.Reads)))
	for _, selector := range canonical.Reads {
		encoded = appendCanonicalSelector(encoded, selector)
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.Writes)))
	for _, selector := range canonical.Writes {
		encoded = appendCanonicalSelector(encoded, selector)
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.GuardAtoms)))
	for _, atom := range canonical.GuardAtoms {
		encoded = appendCanonicalText(encoded, atom)
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.Advances)))
	for _, class := range canonical.Advances {
		encoded = appendCanonicalClass(encoded, class)
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.AliasSupport)))
	for _, class := range canonical.AliasSupport {
		encoded = appendCanonicalClass(encoded, class)
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.WriteAlphabet)))
	for _, root := range canonical.WriteAlphabet {
		encoded = appendCanonicalRoot(encoded, root)
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.Outcomes)))
	for _, outcome := range canonical.Outcomes {
		encoded = appendCanonicalText(encoded, string(outcome))
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.DiagnosticOutputs)))
	for _, diagnostic := range canonical.DiagnosticOutputs {
		encoded = appendCanonicalText(encoded, diagnostic.Candidate)
		encoded = appendCanonicalText(encoded, string(diagnostic.Owner))
		encoded = append(encoded, diagnostic.SourceAnchor[:]...)
		encoded = appendCanonicalUint64(encoded, uint64(len(diagnostic.GuardAtoms)))
		for _, atom := range diagnostic.GuardAtoms {
			encoded = appendCanonicalText(encoded, atom)
		}
		encoded = appendCanonicalUint64(encoded, uint64(len(diagnostic.ReadSet)))
		for _, selector := range diagnostic.ReadSet {
			encoded = appendCanonicalSelector(encoded, selector)
		}
		encoded = appendCanonicalText(encoded, diagnostic.Predicate)
		encoded = appendCanonicalText(encoded, diagnostic.EvidenceRecipe)
		encoded = appendCanonicalText(encoded, diagnostic.BoundaryLens)
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(canonical.Dependencies)))
	for _, dependency := range canonical.Dependencies {
		encoded = appendCanonicalText(encoded, dependency.Kind)
		encoded = append(encoded, dependency.ID[:]...)
	}
	return encoded
}

func (c OperatorContract) ContentID() ContentID {
	encoded := c.CanonicalBytes()
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}

// VerifyAccess rejects an operation that attempts to read, write, advance,
// publish an outcome, observe a diagnostic, or depend on content outside its
// frozen declaration. It is deliberately a subset verifier: declarations may
// be conservative across guards, but an observed access may never be hidden.
func (c OperatorContract) VerifyAccess(access OperatorAccess) error {
	canonical, err := canonicalOperatorContract(c)
	if err != nil {
		return err
	}
	if access.Kind != canonical.Kind || access.Occurrence != canonical.Occurrence {
		return fmt.Errorf("transformer: operator access has foreign contract identity")
	}
	if err := verifySelectorSubset("read", access.Reads, canonical.Reads); err != nil {
		return err
	}
	if err := verifySelectorSubset("write", access.Writes, canonical.Writes); err != nil {
		return err
	}
	if err := verifyClassSubset("advance", access.Advances, canonical.Advances); err != nil {
		return err
	}
	if err := verifyOutcomeSubset(access.Outcomes, canonical.Outcomes); err != nil {
		return err
	}
	if err := verifyDiagnosticSubset(access.Diagnostics, canonical.DiagnosticOutputs); err != nil {
		return err
	}
	return verifyDependencySubset(access.Dependencies, canonical.Dependencies)
}

// OperatorAccess is a dynamic audit record supplied by a canonical bound
// kernel. It is not semantic state and must never affect evaluation results.
type OperatorAccess struct {
	Kind         OperatorKind
	Occurrence   formal.OccurrenceID
	Reads        []ContractSelector
	Writes       []ContractSelector
	Advances     []formal.LexicalClassID
	Outcomes     []OutcomeKind
	Diagnostics  []string
	Dependencies []ContractDependency
}

type operatorContractProfile struct {
	kind     OperatorKind
	operands []AccessRole
}

// OperatorContractCatalog is the immutable, representation-neutral catalog
// that Stage-2 lowerings target. A change to the vocabulary or a required
// operand role changes its content identity and therefore dependent artifacts.
type OperatorContractCatalog struct{}

func (OperatorContractCatalog) CanonicalBytes() []byte {
	encoded := make([]byte, 0, 512)
	encoded = appendCanonicalText(encoded, "operator-contract-catalog/content-v1")
	encoded = appendCanonicalUint64(encoded, uint64(len(frozenOperatorContractProfiles)))
	for _, profile := range frozenOperatorContractProfiles {
		encoded = appendCanonicalText(encoded, string(profile.kind))
		encoded = appendCanonicalUint64(encoded, uint64(len(profile.operands)))
		for _, operand := range profile.operands {
			encoded = appendCanonicalText(encoded, string(operand))
		}
	}
	return encoded
}

func (catalog OperatorContractCatalog) ContentID() ContentID {
	return contentID(catalog.CanonicalBytes())
}

func FrozenOperatorContractCatalog() OperatorContractCatalog { return OperatorContractCatalog{} }

// frozenOperatorContractProfiles is the Stage-1 complete transfer vocabulary.
// Adding a relation-step capability requires adding a profile in the same
// commit; the catalog test makes an unowned semantic family a freeze failure.
var frozenOperatorContractProfiles = []operatorContractProfile{
	{OperatorApply, []AccessRole{AccessFlow, AccessCalleeOutcome, AccessBoundary}},
	{OperatorPathReplacement, []AccessRole{AccessFlow, AccessState, AccessGuard}},
	{OperatorPathInvalidation, []AccessRole{AccessFlow, AccessState, AccessGuard}},
	{OperatorIndexMutation, []AccessRole{AccessFlow, AccessState, AccessGuard}},
	{OperatorAllocationTemplate, []AccessRole{AccessFlow, AccessAllocation, AccessGuard}},
	{OperatorObjectMaterialization, []AccessRole{AccessFlow, AccessAllocation, AccessGuard}},
	{OperatorEnvironmentWrite, []AccessRole{AccessFlow, AccessState, AccessGuard}},
	{OperatorChannelSelect, []AccessRole{AccessFlow, AccessState, AccessGuard}},
	{OperatorBranchRelations, []AccessRole{AccessFlow, AccessState, AccessGuard}},
	{OperatorCallResults, []AccessRole{AccessFlow, AccessState, AccessGuard}},
	{OperatorPresenceImplications, []AccessRole{AccessFlow, AccessNodeEntry, AccessState, AccessGuard}},
	{OperatorLoopControl, []AccessRole{AccessFlow}},
	{OperatorGenericFor, []AccessRole{AccessFlow, AccessNodeEntry, AccessPublished, AccessState}},
	{OperatorRootAssignment, []AccessRole{AccessFlow, AccessNodeEntry, AccessState, AccessGuard}},
	{OperatorCovariantExposure, []AccessRole{AccessFlow, AccessNodeEntry, AccessState, AccessGuard}},
	{OperatorContribution, []AccessRole{AccessFlow, AccessState}},
	{OperatorExternalCall, []AccessRole{AccessFlow, AccessPublished, AccessBoundary, AccessDiagnostic}},
	{OperatorOutcome, []AccessRole{AccessFlow, AccessOutcome}},
	{OperatorNonreturning, []AccessRole{AccessFlow, AccessOutcome}},
	{OperatorDefinition, []AccessRole{AccessEntry, AccessClosureDefinition, AccessBoundary}},
	{OperatorResource, []AccessRole{AccessFlow, AccessClosureDefinition}},
	{OperatorEntry, []AccessRole{AccessEntry}},
	{OperatorPublication, []AccessRole{AccessState, AccessOutcome, AccessDiagnostic}},
}

// NewOperatorContract starts an occurrence contract with the exact mandatory
// operand roles for its semantic family. The caller must add the concrete
// selector/guard/alphabet footprint before binding the operator in stage 2.
func NewOperatorContract(kind OperatorKind, occurrence formal.OccurrenceID) (OperatorContract, error) {
	profile, ok := operatorProfile(kind)
	if !ok || !occurrence.Valid() {
		return OperatorContract{}, fmt.Errorf("transformer: unknown operator contract kind or occurrence")
	}
	return OperatorContract{Kind: kind, Occurrence: occurrence, Operands: append([]AccessRole(nil), profile.operands...)}, nil
}

// FrozenOperatorKinds returns the complete, deterministic operation-family
// vocabulary. It is a catalog interface rather than an evaluator registry.
func FrozenOperatorKinds() []OperatorKind {
	out := make([]OperatorKind, len(frozenOperatorContractProfiles))
	for index, profile := range frozenOperatorContractProfiles {
		out[index] = profile.kind
	}
	return out
}

func operatorProfile(kind OperatorKind) (operatorContractProfile, bool) {
	for _, profile := range frozenOperatorContractProfiles {
		if profile.kind == kind {
			return profile, true
		}
	}
	return operatorContractProfile{}, false
}

func canonicalOperatorContract(contract OperatorContract) (OperatorContract, error) {
	profile, known := operatorProfile(contract.Kind)
	if !known || !contract.Occurrence.Valid() {
		return OperatorContract{}, fmt.Errorf("transformer: operator contract has invalid kind or occurrence")
	}
	out := contract
	out.Operands = append([]AccessRole(nil), out.Operands...)
	out.Reads = canonicalSelectors(out.Reads)
	out.Writes = canonicalSelectors(out.Writes)
	out.GuardAtoms = canonicalStrings(out.GuardAtoms)
	out.Advances = canonicalClasses(out.Advances)
	out.AliasSupport = canonicalClasses(out.AliasSupport)
	out.WriteAlphabet = canonicalRoots(out.WriteAlphabet)
	out.Outcomes = canonicalOutcomes(out.Outcomes)
	out.Dependencies = canonicalDependencies(out.Dependencies)
	out.DiagnosticOutputs = canonicalDiagnostics(out.DiagnosticOutputs)
	if !sameAccessRoles(out.Operands, profile.operands) || !selectorsOwned(out.Reads, out.Occurrence.Owner()) || !selectorsOwned(out.Writes, out.Occurrence.Owner()) ||
		!classesValid(out.Advances, out.Occurrence.Owner()) || !classesValid(out.AliasSupport, out.Occurrence.Owner()) ||
		!rootsValid(out.WriteAlphabet, out.Occurrence.Owner()) || !outcomesValid(out.Outcomes) ||
		!dependenciesValid(out.Dependencies) || !diagnosticsValid(out.DiagnosticOutputs) {
		return OperatorContract{}, fmt.Errorf("transformer: operator contract is incomplete or malformed")
	}
	for _, write := range out.Writes {
		if write.Root.Valid() && !containsRoot(out.WriteAlphabet, write.Root) {
			return OperatorContract{}, fmt.Errorf("transformer: operator contract write lies outside its occurrence alphabet")
		}
	}
	for _, descriptor := range out.DiagnosticOutputs {
		if !selectorsOwned(descriptor.ReadSet, out.Occurrence.Owner()) {
			return OperatorContract{}, fmt.Errorf("transformer: diagnostic descriptor reads a foreign selector")
		}
	}
	return out, nil
}

func appendCanonicalSelector(out []byte, selector ContractSelector) []byte {
	out = appendCanonicalText(out, string(selector.Role))
	out = appendCanonicalText(out, selector.Name)
	if selector.Root.Valid() {
		return appendCanonicalRoot(out, selector.Root)
	}
	return append(out, 0)
}

func validAccessRole(role AccessRole) bool {
	switch role {
	case AccessFlow, AccessEntry, AccessNodeEntry, AccessPublished, AccessCalleeOutcome, AccessClosureDefinition,
		AccessGuard, AccessState, AccessOutcome, AccessDiagnostic, AccessAllocation, AccessBoundary:
		return true
	default:
		return false
	}
}

func canonicalSelectors(in []ContractSelector) []ContractSelector {
	out := append([]ContractSelector(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}

func canonicalStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func canonicalClasses(in []formal.LexicalClassID) []formal.LexicalClassID {
	out := append([]formal.LexicalClassID(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Ordinal() < out[j].Ordinal() })
	return out
}

func canonicalRoots(in []formal.Root) []formal.Root {
	out := append([]formal.Root(nil), in...)
	sortFormalRoots(out)
	return out
}

func canonicalOutcomes(in []OutcomeKind) []OutcomeKind {
	out := append([]OutcomeKind(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func canonicalDependencies(in []ContractDependency) []ContractDependency {
	out := append([]ContractDependency(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}

func canonicalDiagnostics(in []DiagnosticDescriptor) []DiagnosticDescriptor {
	out := append([]DiagnosticDescriptor(nil), in...)
	for index := range out {
		out[index].GuardAtoms = canonicalStrings(out[index].GuardAtoms)
		out[index].ReadSet = canonicalSelectors(out[index].ReadSet)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Candidate < out[j].Candidate })
	return out
}

func stringsSortedUnique(values []string) bool {
	for index, value := range values {
		if value == "" || index != 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func selectorsValid(values []ContractSelector) bool {
	for index, value := range values {
		if !value.valid() || index != 0 && values[index-1].equal(value) {
			return false
		}
	}
	return true
}

func selectorsOwned(values []ContractSelector, owner lexicalidentity.StableLexicalBodyID) bool {
	if !selectorsValid(values) {
		return false
	}
	for _, selector := range values {
		if selector.Root.Valid() && selector.Root.Owner() != owner {
			return false
		}
	}
	return true
}

func classesValid(values []formal.LexicalClassID, owner lexicalidentity.StableLexicalBodyID) bool {
	for index, value := range values {
		if !value.Valid() || value.Owner() != owner || index != 0 && values[index-1] == value {
			return false
		}
	}
	return true
}

func rootsValid(values []formal.Root, owner lexicalidentity.StableLexicalBodyID) bool {
	for index, value := range values {
		if !value.Valid() || value.Owner() != owner || index != 0 && values[index-1] == value {
			return false
		}
	}
	return true
}

func outcomesValid(values []OutcomeKind) bool {
	for index, value := range values {
		switch value {
		case OutcomeNormal, OutcomeNonreturning, OutcomeProtected, OutcomeSuspension:
		default:
			return false
		}
		if index != 0 && values[index-1] == value {
			return false
		}
	}
	return true
}

func dependenciesValid(values []ContractDependency) bool {
	for index, value := range values {
		if !value.valid() || index != 0 && values[index-1] == value {
			return false
		}
	}
	return true
}

func diagnosticsValid(values []DiagnosticDescriptor) bool {
	for index, value := range values {
		if !value.valid() || index != 0 && values[index-1].Candidate == value.Candidate {
			return false
		}
	}
	return true
}

func containsRoot(roots []formal.Root, target formal.Root) bool {
	index := sort.Search(len(roots), func(index int) bool { return !roots[index].Less(target) })
	return index < len(roots) && roots[index] == target
}

func sameAccessRoles(left, right []AccessRole) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func verifySelectorSubset(kind string, observed, declared []ContractSelector) error {
	declared = canonicalSelectors(declared)
	for _, selector := range canonicalSelectors(observed) {
		index := sort.Search(len(declared), func(index int) bool { return !declared[index].less(selector) })
		if index == len(declared) || !declared[index].equal(selector) {
			return fmt.Errorf("transformer: operator has undeclared %s %s/%s", kind, selector.Role, selector.Name)
		}
	}
	return nil
}

func verifyClassSubset(kind string, observed, declared []formal.LexicalClassID) error {
	declared = canonicalClasses(declared)
	for _, class := range canonicalClasses(observed) {
		index := sort.Search(len(declared), func(index int) bool { return declared[index].Ordinal() >= class.Ordinal() })
		if index == len(declared) || declared[index] != class {
			return fmt.Errorf("transformer: operator has undeclared %s class", kind)
		}
	}
	return nil
}

func verifyOutcomeSubset(observed, declared []OutcomeKind) error {
	declared = canonicalOutcomes(declared)
	for _, outcome := range canonicalOutcomes(observed) {
		index := sort.Search(len(declared), func(index int) bool { return declared[index] >= outcome })
		if index == len(declared) || declared[index] != outcome {
			return fmt.Errorf("transformer: operator publishes an undeclared outcome %q", outcome)
		}
	}
	return nil
}

func verifyDiagnosticSubset(observed []string, declared []DiagnosticDescriptor) error {
	allowed := make(map[string]struct{}, len(declared))
	for _, descriptor := range declared {
		allowed[descriptor.Candidate] = struct{}{}
	}
	for _, candidate := range observed {
		if _, ok := allowed[candidate]; !ok {
			return fmt.Errorf("transformer: operator emits undeclared diagnostic candidate %q", candidate)
		}
	}
	return nil
}

func verifyDependencySubset(observed, declared []ContractDependency) error {
	declared = canonicalDependencies(declared)
	for _, dependency := range canonicalDependencies(observed) {
		index := sort.Search(len(declared), func(index int) bool { return !declared[index].less(dependency) })
		if index == len(declared) || declared[index] != dependency {
			return fmt.Errorf("transformer: operator has an undeclared content dependency %q", dependency.Kind)
		}
	}
	return nil
}

func operatorKindForStepCapability(capability formalRelationStepCapability) (OperatorKind, bool) {
	switch capability {
	case formalRelationStepCapabilityApply:
		return OperatorApply, true
	case formalRelationStepCapabilityPathReplacement:
		return OperatorPathReplacement, true
	case formalRelationStepCapabilityPathInvalidation:
		return OperatorPathInvalidation, true
	case formalRelationStepCapabilityIndexMutation:
		return OperatorIndexMutation, true
	case formalRelationStepCapabilityAllocationTemplate:
		return OperatorAllocationTemplate, true
	case formalRelationStepCapabilityObjectMaterialization:
		return OperatorObjectMaterialization, true
	case formalRelationStepCapabilityEnvironmentWrite:
		return OperatorEnvironmentWrite, true
	case formalRelationStepCapabilityChannelSelect:
		return OperatorChannelSelect, true
	case formalRelationStepCapabilityBranchRelations:
		return OperatorBranchRelations, true
	case formalRelationStepCapabilityCallResults:
		return OperatorCallResults, true
	case formalRelationStepCapabilityPresenceImplications:
		return OperatorPresenceImplications, true
	case formalRelationStepCapabilityLoopControl:
		return OperatorLoopControl, true
	case formalRelationStepCapabilityGenericFor:
		return OperatorGenericFor, true
	case formalRelationStepCapabilityRootAssignment:
		return OperatorRootAssignment, true
	case formalRelationStepCapabilityCovariantExposure:
		return OperatorCovariantExposure, true
	case formalRelationStepCapabilityContribution:
		return OperatorContribution, true
	case formalRelationStepCapabilityExternalCall:
		return OperatorExternalCall, true
	default:
		return "", false
	}
}
