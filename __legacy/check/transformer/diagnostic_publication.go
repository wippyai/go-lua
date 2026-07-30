package transformer

import (
	"fmt"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// ApplicationResidual is the portable, provider-owned result for a caller
// diagnostic obligation.  It deliberately has no caller artifact, call
// anchor, bound State, source span, or run-local handle.  Those are supplied
// only when a particular caller binds it through a BoundaryLens.
type ApplicationResidual struct {
	descriptor     ContentID
	declared       DeclaredCheckContext
	formalBoundary formal.Root
	guard          ContentID
	predicate      ContentID
	evidence       ContentID
	reportable     bool
}

// NewApplicationResidual turns a declared Application descriptor into a
// portable obligation.  Predicate and evidence are sealed content identities,
// not caller-rendered text.  A caller must still prove its bound guard before
// the reportable result may be published.
func NewApplicationResidual(
	descriptor DiagnosticDescriptor,
	declared DeclaredCheckContext,
	formalBoundary formal.Root,
	guard, predicate, evidence ContentID,
	reportable bool,
) (ApplicationResidual, error) {
	if descriptor.Owner != DiagnosticOwnerApplication || !descriptor.DiagnosticDescriptorID().Valid() || !declared.valid() ||
		!formalBoundary.Valid() || !guard.Valid() || !predicate.Valid() || !evidence.Valid() {
		return ApplicationResidual{}, fmt.Errorf("transformer: malformed application diagnostic residual")
	}
	return ApplicationResidual{
		descriptor: descriptor.DiagnosticDescriptorID(), declared: declared, formalBoundary: formalBoundary,
		guard: guard, predicate: predicate, evidence: evidence, reportable: reportable,
	}, nil
}

func (r ApplicationResidual) Descriptor() ContentID          { return r.descriptor }
func (r ApplicationResidual) Declared() DeclaredCheckContext { return r.declared }
func (r ApplicationResidual) FormalBoundary() formal.Root    { return r.formalBoundary }
func (r ApplicationResidual) Guard() ContentID               { return r.guard }
func (r ApplicationResidual) Predicate() ContentID           { return r.predicate }
func (r ApplicationResidual) Evidence() ContentID            { return r.evidence }
func (r ApplicationResidual) Reportable() bool               { return r.reportable }

func (r ApplicationResidual) CanonicalBytes() []byte {
	declared := r.declared.CanonicalBytes()
	if !r.descriptor.Valid() || declared == nil || !r.formalBoundary.Valid() || !r.guard.Valid() || !r.predicate.Valid() || !r.evidence.Valid() {
		return nil
	}
	encoded := make([]byte, 0, 256)
	encoded = appendCanonicalText(encoded, "application-diagnostic-residual/content-v1")
	encoded = append(encoded, r.descriptor[:]...)
	encoded = append(encoded, declared...)
	encoded = appendCanonicalRoot(encoded, r.formalBoundary)
	encoded = append(encoded, r.guard[:]...)
	encoded = append(encoded, r.predicate[:]...)
	encoded = append(encoded, r.evidence[:]...)
	if r.reportable {
		return append(encoded, 1)
	}
	return append(encoded, 0)
}

func (r ApplicationResidual) ContentID() ContentID {
	if encoded := r.CanonicalBytes(); encoded != nil {
		return contentID(encoded)
	}
	return ContentID{}
}

func (r ApplicationResidual) valid() bool { return r.CanonicalBytes() != nil }

type calleePublicationKey struct {
	artifact   ContentID
	body       lexicalidentity.StableLexicalBodyID
	descriptor ContentID
}

type applicationPublicationKey struct {
	caller     ContentID
	callAnchor ContentID
	descriptor ContentID
}

// DiagnosticPublisher is the ownership boundary for diagnostic publication.
// It stores only closed publications and deduplicates the two ownership forms
// by their respective frozen content identities.
type DiagnosticPublisher struct {
	mu          sync.Mutex
	callee      map[calleePublicationKey]DiagnosticPublication
	application map[applicationPublicationKey]DiagnosticPublication
}

func NewDiagnosticPublisher() *DiagnosticPublisher {
	return &DiagnosticPublisher{
		callee:      make(map[calleePublicationKey]DiagnosticPublication),
		application: make(map[applicationPublicationKey]DiagnosticPublication),
	}
}

// PublishCalleeCheck is provider-owned.  Importers cannot supply caller
// context here, and equal provider/body/descriptor decisions publish once.
func (p *DiagnosticPublisher) PublishCalleeCheck(descriptor DiagnosticDescriptor, declared DeclaredCheckContext, reportable bool) (DiagnosticPublication, bool, error) {
	if p == nil || descriptor.Owner != DiagnosticOwnerCalleeCheck || !descriptor.DiagnosticDescriptorID().Valid() || !declared.valid() {
		return DiagnosticPublication{}, false, fmt.Errorf("transformer: malformed callee diagnostic publication")
	}
	if !reportable {
		return DiagnosticPublication{}, false, nil
	}
	publication := DiagnosticPublication{Descriptor: descriptor.DiagnosticDescriptorID(), Owner: DiagnosticOwnerCalleeCheck, Declared: declared}
	if err := publication.Validate(); err != nil {
		return DiagnosticPublication{}, false, err
	}
	key := calleePublicationKey{artifact: declared.Artifact, body: declared.Body, descriptor: publication.Descriptor}
	p.mu.Lock()
	defer p.mu.Unlock()
	if prior, exists := p.callee[key]; exists {
		return prior, false, nil
	}
	p.callee[key] = publication
	return publication, true, nil
}

// PublishApplication binds one portable residual through the frozen caller
// lens.  An infeasible, widened-possibly-feasible, or non-reportable result
// emits nothing; only a positive certificate for this descriptor, formal
// guard, and exact binding can publish.
func (p *DiagnosticPublisher) PublishApplication(residual ApplicationResidual, bound BoundApplicationContext, feasibility FeasibilityCertificate) (DiagnosticPublication, bool, error) {
	if p == nil || !residual.valid() || bound.CanonicalBytes() == nil {
		return DiagnosticPublication{}, false, fmt.Errorf("transformer: malformed application diagnostic publication")
	}
	if !boundCoversFormalBoundary(bound, residual.formalBoundary) {
		return DiagnosticPublication{}, false, fmt.Errorf("transformer: application diagnostic residual has no matching boundary lens")
	}
	if !residual.reportable || !feasibility.Positive() {
		return DiagnosticPublication{}, false, nil
	}
	if feasibility.Descriptor != residual.descriptor || feasibility.Guard != residual.guard || feasibility.Binding != bound.Binding || feasibility.Application != bound.ContentID() {
		return DiagnosticPublication{}, false, fmt.Errorf("transformer: application feasibility proof does not match bound residual")
	}
	publication := DiagnosticPublication{
		Descriptor: residual.descriptor, Owner: DiagnosticOwnerApplication, Declared: residual.declared,
		Application: bound, Feasibility: feasibility,
	}
	if err := publication.Validate(); err != nil {
		return DiagnosticPublication{}, false, err
	}
	key := applicationPublicationKey{caller: bound.CallerArtifact, callAnchor: bound.CallAnchor, descriptor: residual.descriptor}
	p.mu.Lock()
	defer p.mu.Unlock()
	if prior, exists := p.application[key]; exists {
		return prior, false, nil
	}
	p.application[key] = publication
	return publication, true, nil
}

func boundCoversFormalBoundary(bound BoundApplicationContext, formalBoundary formal.Root) bool {
	for _, lens := range bound.Lenses {
		if lens.valid() && lens.Formal == formalBoundary {
			return true
		}
	}
	return false
}
