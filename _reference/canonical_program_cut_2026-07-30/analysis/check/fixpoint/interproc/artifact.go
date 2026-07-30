package interproc

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// ParameterSchema identifies the normalized, seeded entry representation a
// demanded body accepts. Its selectors are an inventory, not an instruction
// to read all of them.
type ParameterSchema struct {
	name      string
	selectors []EntrySelector
}

func NewParameterSchema(name string, selectors []EntrySelector) (ParameterSchema, error) {
	out := ParameterSchema{name: name, selectors: append([]EntrySelector(nil), selectors...)}
	sort.Slice(out.selectors, func(i, j int) bool { return out.selectors[i] < out.selectors[j] })
	for index, selector := range out.selectors {
		if out.name == "" || !selector.valid() || index != 0 && out.selectors[index-1] >= selector {
			return ParameterSchema{}, fmt.Errorf("interproc: malformed parameter schema")
		}
	}
	if out.name == "" {
		return ParameterSchema{}, fmt.Errorf("interproc: malformed parameter schema")
	}
	return out, nil
}

func (s ParameterSchema) Name() string { return s.name }
func (s ParameterSchema) Selectors() []EntrySelector {
	return append([]EntrySelector(nil), s.selectors...)
}
func (s ParameterSchema) Contains(selector EntrySelector) bool {
	index := sort.Search(len(s.selectors), func(index int) bool { return s.selectors[index] >= selector })
	return index < len(s.selectors) && s.selectors[index] == selector
}
func (s ParameterSchema) CanonicalBytes() []byte {
	if s.name == "" {
		return nil
	}
	for index, selector := range s.selectors {
		if !selector.valid() || index != 0 && s.selectors[index-1] >= selector {
			return nil
		}
	}
	out := appendText(nil, "interproc-parameter-schema/content-v1")
	out = appendText(out, s.name)
	out = appendU64(out, uint64(len(s.selectors)))
	for _, selector := range s.selectors {
		out = appendText(out, string(selector))
	}
	return out
}
func (s ParameterSchema) ContentID() ContentID {
	encoded := s.CanonicalBytes()
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}

// DiagnosticReadSet ties a portable diagnostic descriptor identity to the
// exact entry selectors its predicate/evidence may inspect. Descriptor IDs
// are supplied by the frozen diagnostic contract; caller spans never appear.
type DiagnosticReadSet struct {
	Descriptor ContentID
	Reads      []EntrySelector
}

func (d DiagnosticReadSet) less(other DiagnosticReadSet) bool {
	return string(d.Descriptor[:]) < string(other.Descriptor[:])
}

func canonicalDiagnosticReadSets(in []DiagnosticReadSet) ([]DiagnosticReadSet, error) {
	out := append([]DiagnosticReadSet(nil), in...)
	for index := range out {
		out[index].Reads = append([]EntrySelector(nil), out[index].Reads...)
		sort.Slice(out[index].Reads, func(left, right int) bool { return out[index].Reads[left] < out[index].Reads[right] })
		for readIndex, selector := range out[index].Reads {
			if !out[index].Descriptor.Valid() || !selector.valid() || readIndex != 0 && out[index].Reads[readIndex-1] >= selector {
				return nil, fmt.Errorf("interproc: malformed diagnostic read set")
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].less(out[j]) })
	for index := range out {
		if !out[index].Descriptor.Valid() || index != 0 && !out[index-1].less(out[index]) {
			return nil, fmt.Errorf("interproc: duplicate diagnostic read set")
		}
	}
	return out, nil
}

// DemandedBodyArtifact is an immutable interprocedural callable envelope.
// It wraps the complete frozen cyclic equation artifact rather than a
// post-fixpoint summary and has no entry value, cache cell, generation, or
// caller-owned diagnostic context.
type DemandedBodyArtifact struct {
	body        equation.CyclicArtifact
	schema      ParameterSchema
	demand      DemandKey
	certificate ReadProjectionCertificate
	solver      ContentID
	manifest    DependencyManifest
	diagnostics []DiagnosticReadSet
}

// NewDemandedBodyArtifact seals the stage-5 envelope.  Diagnostic reads and
// every projected selector must be covered by the parameter schema and by the
// certificate; absence is a construction error rather than an implicit broad
// entry read.
func NewDemandedBodyArtifact(
	body equation.CyclicArtifact,
	schema ParameterSchema,
	demand DemandKey,
	certificate ReadProjectionCertificate,
	solverPolicy ContentID,
	manifest DependencyManifest,
	diagnostics []DiagnosticReadSet,
) (DemandedBodyArtifact, error) {
	var err error
	body, err = cloneCyclicArtifact(body)
	if err != nil {
		return DemandedBodyArtifact{}, err
	}
	if body.CanonicalBytes() == nil || schema.CanonicalBytes() == nil || !demand.valid() || certificate.CanonicalBytes() == nil ||
		certificate.DemandKey() != demand || !solverPolicy.Valid() || manifest.CanonicalBytes() == nil {
		return DemandedBodyArtifact{}, fmt.Errorf("interproc: incomplete demanded body artifact")
	}
	for _, selector := range certificate.Selectors() {
		if !schema.Contains(selector) {
			return DemandedBodyArtifact{}, fmt.Errorf("interproc: certificate selector %q lies outside parameter schema", selector)
		}
	}
	canonicalDiagnostics, err := canonicalDiagnosticReadSets(diagnostics)
	if err != nil {
		return DemandedBodyArtifact{}, err
	}
	for _, descriptor := range canonicalDiagnostics {
		for _, selector := range descriptor.Reads {
			if !schema.Contains(selector) || !certificate.Covers(ReadDiagnostic, selector) {
				return DemandedBodyArtifact{}, fmt.Errorf("interproc: diagnostic read %q is absent from certificate", selector)
			}
		}
	}
	return DemandedBodyArtifact{
		body: body, schema: schema, demand: demand, certificate: certificate, solver: solverPolicy,
		manifest: manifest, diagnostics: canonicalDiagnostics,
	}, nil
}

func (a DemandedBodyArtifact) Body() equation.CyclicArtifact {
	body, err := cloneCyclicArtifact(a.body)
	if err != nil {
		return equation.CyclicArtifact{}
	}
	return body
}
func (a DemandedBodyArtifact) ParameterSchema() ParameterSchema           { return a.schema }
func (a DemandedBodyArtifact) DemandKey() DemandKey                       { return a.demand }
func (a DemandedBodyArtifact) ReadCertificate() ReadProjectionCertificate { return a.certificate }
func (a DemandedBodyArtifact) SolverPolicyID() ContentID                  { return a.solver }
func (a DemandedBodyArtifact) Dependencies() DependencyManifest           { return a.manifest }
func (a DemandedBodyArtifact) DiagnosticReadSets() []DiagnosticReadSet {
	out := append([]DiagnosticReadSet(nil), a.diagnostics...)
	for index := range out {
		out[index].Reads = append([]EntrySelector(nil), out[index].Reads...)
	}
	return out
}

// CanonicalBytes commits to every input that can affect the future canonical
// VM/WTO result: equation graph and frozen schedule, entry schema, requested
// output, read certificate, solver policy, dependencies, and diagnostic
// descriptors. No version counter participates.
func (a DemandedBodyArtifact) CanonicalBytes() []byte {
	body := a.body.CanonicalBytes()
	schema := a.schema.CanonicalBytes()
	certificate := a.certificate.CanonicalBytes()
	manifest := a.manifest.CanonicalBytes()
	if body == nil || schema == nil || certificate == nil || manifest == nil || !a.demand.valid() || !a.solver.Valid() || a.certificate.DemandKey() != a.demand {
		return nil
	}
	diagnostics, err := canonicalDiagnosticReadSets(a.diagnostics)
	if err != nil {
		return nil
	}
	for _, descriptor := range diagnostics {
		for _, selector := range descriptor.Reads {
			if !a.schema.Contains(selector) || !a.certificate.Covers(ReadDiagnostic, selector) {
				return nil
			}
		}
	}
	out := appendText(nil, "demanded-body-artifact/content-v1")
	out = appendBytes(out, body)
	out = appendBytes(out, schema)
	out = appendText(out, string(a.demand))
	out = appendBytes(out, certificate)
	out = append(out, a.solver[:]...)
	out = appendBytes(out, manifest)
	out = appendU64(out, uint64(len(diagnostics)))
	for _, descriptor := range diagnostics {
		out = append(out, descriptor.Descriptor[:]...)
		out = appendU64(out, uint64(len(descriptor.Reads)))
		for _, selector := range descriptor.Reads {
			out = appendText(out, string(selector))
		}
	}
	return out
}

func (a DemandedBodyArtifact) ContentID() ContentID {
	encoded := a.CanonicalBytes()
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}

func cloneCyclicArtifact(in equation.CyclicArtifact) (equation.CyclicArtifact, error) {
	artifact := equation.Artifact{Equations: make([]equation.Equation, len(in.Artifact.Equations))}
	for index, item := range in.Artifact.Equations {
		artifact.Equations[index] = item
		artifact.Equations[index].Guards = append([]equation.Guard(nil), item.Guards...)
		artifact.Equations[index].Dependencies = append([]equation.Coordinate(nil), item.Dependencies...)
		artifact.Equations[index].Operands = append([]equation.Operand(nil), item.Operands...)
		for operandIndex := range artifact.Equations[index].Operands {
			artifact.Equations[index].Operands[operandIndex].Term.Encoding = append([]byte(nil), item.Operands[operandIndex].Term.Encoding...)
		}
	}
	cells := make(map[equation.Coordinate]equation.CellID, len(in.CellForTarget))
	for target, cell := range in.CellForTarget {
		cells[target] = cell
	}
	return equation.NewCyclicArtifact(artifact, cells, in.Plan, in.Dependencies, in.Selectors, in.ParameterCells)
}
