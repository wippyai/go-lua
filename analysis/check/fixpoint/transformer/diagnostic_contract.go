package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// BoundaryLens is the portable formal-to-caller relation needed to bind an
// application obligation. It intentionally carries no caller source span or
// run-local identity; rendering happens only after application publication.
type BoundaryLens struct {
	Formal formal.Root
	Caller formal.Root
}

func (l BoundaryLens) valid() bool {
	return l.Formal.Valid() && l.Caller.Valid() && l.Formal.Owner() != (lexicalidentity.StableLexicalBodyID{}) && l.Caller.Owner() != (lexicalidentity.StableLexicalBodyID{})
}

// DeclaredCheckContext is the closed provider context for a source-owned
// CalleeCheck. It contains content identity, never a mutable workspace handle.
type DeclaredCheckContext struct {
	Artifact ContentID
	Body     lexicalidentity.StableLexicalBodyID
	Registry ContentID
}

func (c DeclaredCheckContext) valid() bool {
	return c.Artifact.Valid() && c.Registry.Valid() && c.Body != (lexicalidentity.StableLexicalBodyID{})
}

func (c DeclaredCheckContext) CanonicalBytes() []byte {
	if !c.valid() {
		return nil
	}
	encoded := make([]byte, 0, 96)
	encoded = appendCanonicalText(encoded, "declared-check-context/content-v1")
	encoded = append(encoded, c.Artifact[:]...)
	encoded = appendCanonicalOwner(encoded, c.Body)
	return append(encoded, c.Registry[:]...)
}

func (c DeclaredCheckContext) ContentID() ContentID {
	if encoded := c.CanonicalBytes(); encoded != nil {
		return contentID(encoded)
	}
	return ContentID{}
}

// BoundApplicationContext identifies a caller application by content, not by
// mutable call-site state. CallAnchor is a content address of the frozen call
// occurrence and is not a source span.
type BoundApplicationContext struct {
	CallerArtifact ContentID
	CallAnchor     ContentID
	Binding        ContentID
	Lenses         []BoundaryLens
}

func (c BoundApplicationContext) CanonicalBytes() []byte {
	if !c.CallerArtifact.Valid() || !c.CallAnchor.Valid() || !c.Binding.Valid() {
		return nil
	}
	lenses := append([]BoundaryLens(nil), c.Lenses...)
	sort.Slice(lenses, func(i, j int) bool {
		if lenses[i].Formal != lenses[j].Formal {
			return lenses[i].Formal.Less(lenses[j].Formal)
		}
		return lenses[i].Caller.Less(lenses[j].Caller)
	})
	for index, lens := range lenses {
		if !lens.valid() || index != 0 && lenses[index-1].Formal == lens.Formal {
			return nil
		}
	}
	encoded := make([]byte, 0, 128)
	encoded = appendCanonicalText(encoded, "bound-application-context/content-v1")
	encoded = append(encoded, c.CallerArtifact[:]...)
	encoded = append(encoded, c.CallAnchor[:]...)
	encoded = append(encoded, c.Binding[:]...)
	encoded = appendCanonicalUint64(encoded, uint64(len(lenses)))
	for _, lens := range lenses {
		encoded = appendCanonicalRoot(encoded, lens.Formal)
		encoded = appendCanonicalRoot(encoded, lens.Caller)
	}
	return encoded
}

func (c BoundApplicationContext) ContentID() ContentID {
	if encoded := c.CanonicalBytes(); encoded != nil {
		return contentID(encoded)
	}
	return ContentID{}
}

// FeasibilityCertificate records a positive proof produced by the same bound
// guarded state that evaluated an Application predicate. Unknown is never a
// certificate and therefore cannot cause publication.
type FeasibilityCertificate struct {
	Descriptor ContentID
	BoundState ContentID
	Guard      ContentID
	Feasible   bool
}

func (c FeasibilityCertificate) CanonicalBytes() []byte {
	if !c.Descriptor.Valid() || !c.BoundState.Valid() || !c.Guard.Valid() || !c.Feasible {
		return nil
	}
	encoded := make([]byte, 0, 112)
	encoded = appendCanonicalText(encoded, "feasibility-certificate/content-v1")
	encoded = append(encoded, c.Descriptor[:]...)
	encoded = append(encoded, c.BoundState[:]...)
	return append(encoded, c.Guard[:]...)
}

func (c FeasibilityCertificate) ContentID() ContentID {
	if encoded := c.CanonicalBytes(); encoded != nil {
		return contentID(encoded)
	}
	return ContentID{}
}

// DiagnosticPublication is the closed result of deciding a descriptor. The
// caller-facing form is valid only for an application owner with a positive
// feasibility certificate; CalleeCheck is source-owned and deduped separately.
type DiagnosticPublication struct {
	Descriptor  ContentID
	Owner       DiagnosticOwner
	Declared    DeclaredCheckContext
	Application BoundApplicationContext
	Feasibility FeasibilityCertificate
}

func (p DiagnosticPublication) Validate() error {
	if !p.Descriptor.Valid() || !p.Declared.valid() {
		return fmt.Errorf("transformer: diagnostic publication has no declared context")
	}
	switch p.Owner {
	case DiagnosticOwnerCalleeCheck:
		if p.Application.ContentID().Valid() || p.Feasibility.ContentID().Valid() {
			return fmt.Errorf("transformer: callee diagnostic publication has caller state")
		}
	case DiagnosticOwnerApplication:
		if !p.Application.ContentID().Valid() || !p.Feasibility.ContentID().Valid() || p.Feasibility.Descriptor != p.Descriptor {
			return fmt.Errorf("transformer: application diagnostic publication lacks a positive feasibility proof")
		}
	default:
		return fmt.Errorf("transformer: diagnostic publication has an unknown owner")
	}
	return nil
}

// DiagnosticDescriptorID returns the content identity of a portable candidate.
func (d DiagnosticDescriptor) DiagnosticDescriptorID() ContentID {
	if !d.valid() {
		return ContentID{}
	}
	encoded := make([]byte, 0, 256)
	encoded = appendCanonicalText(encoded, "diagnostic-descriptor/content-v1")
	encoded = appendCanonicalText(encoded, d.Candidate)
	encoded = appendCanonicalText(encoded, string(d.Owner))
	encoded = append(encoded, d.SourceAnchor[:]...)
	encoded = appendCanonicalUint64(encoded, uint64(len(d.GuardAtoms)))
	for _, atom := range d.GuardAtoms {
		encoded = appendCanonicalText(encoded, atom)
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(d.ReadSet)))
	for _, selector := range d.ReadSet {
		encoded = appendCanonicalSelector(encoded, selector)
	}
	encoded = appendCanonicalText(encoded, d.Predicate)
	encoded = appendCanonicalText(encoded, d.EvidenceRecipe)
	encoded = appendCanonicalText(encoded, d.BoundaryLens)
	return contentID(encoded)
}
