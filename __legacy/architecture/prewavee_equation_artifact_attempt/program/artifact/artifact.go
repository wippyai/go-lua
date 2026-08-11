// Package artifact owns the one portable persistence boundary for sealed
// canonical Programs. It persists authored Program relations and immutable
// environment identity; all execution projections are rebuilt by Program
// Seal during opening.
package artifact

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/internal/core"
	"github.com/wippyai/go-lua/program/target"
)

// Dependency names one exact externally supplied artifact prerequisite. It is
// the core persistence row, not a parallel public transport type: callers and
// Program storage share one vocabulary and one defensive canonical copy.
type Dependency = core.ArtifactDependency

// SemanticKey is an exact producer identity/version pair for one derived
// equation declaration.  It is binary ContentID-based; artifact persistence
// has no string-keyed analysis vocabulary.
type SemanticKey = core.ArtifactSemanticKey

// EquationCache is the optional canonical derived body-equation section.
// Its rows are structural Program references and declaration identities only;
// it never carries Factor values, Rule closures, State, or a second solver
// representation.
type EquationCache = core.ArtifactEquationCache
type EquationBody = core.ArtifactEquationBody
type EquationEdge = core.ArtifactEquationEdge
type EquationRead = core.ArtifactEquationRead
type EquationBoundary = core.ArtifactEquationBoundary

// CanonicalEquationBodies derives Program's complete cacheable body equation
// inventory. It is the same authority used by artifact decode validation, so
// engine emission cannot drift into a second topology reconstruction.
func CanonicalEquationBodies(p *program.Program) ([]EquationBody, bool) {
	return core.CanonicalArtifactEquationBodies(p)
}

// MatchesCanonicalEquationBodies compares stored rows against Program one
// lexical Body at a time, avoiding a second whole-artifact topology during
// decode or solver admission.
func MatchesCanonicalEquationBodies(p *program.Program, bodies []EquationBody) bool {
	return core.MatchesCanonicalArtifactEquationBodies(p, bodies)
}

// EquationCachesFit is the portable weighted preflight for a caller-owned
// cache collection. It performs no semantic validation; owners call it before
// defensive copying so aggregate cache input cannot amplify memory.
func EquationCachesFit(caches []EquationCache) bool {
	return core.ArtifactEquationCachesFit(caches)
}

// Metadata is envelope evidence, not Program syntax. Provenance is an exact
// caller-supplied source revision or comparable immutable token.
type Metadata struct {
	Dependencies []Dependency
	Provenance   string
	Equations    *EquationCache
}

var (
	ErrUnavailableTarget = errors.New("program artifact: unavailable target contract")
	// ErrUnavailableProgram rejects an unsealed or otherwise unavailable
	// Program before any artifact bytes are published.
	ErrUnavailableProgram = core.ErrArtifactUnavailable
	// ErrTargetMismatch means bytes were sealed against a different immutable
	// target Contract. It is a hard boundary, never a relink hint.
	ErrTargetMismatch = core.ErrArtifactTarget
	// ErrNoncanonical rejects malformed bytes and alternate encodings that do
	// not exactly reproduce the one canonical artifact stream.
	ErrNoncanonical = core.ErrArtifactCanonical
	// ErrLimit rejects artifact bytes or reconstruction work above the fixed
	// persistence boundary. It never returns a partial Program or byte stream.
	ErrLimit = core.ErrArtifactLimit
)

// Encode binds a sealed Program to this exact sealed target Contract. There
// is no unbound form and no compatibility codec.
func Encode(p *program.Program, contract *target.Contract, metadata Metadata) ([]byte, error) {
	id, ok := targetID(contract)
	if !ok {
		return nil, ErrUnavailableTarget
	}
	var names uint64
	for _, dependency := range metadata.Dependencies {
		if dependency.Name == "" || !dependency.ID.Available() {
			return nil, ErrUnavailableProgram
		}
		width := uint64(len(dependency.Name))
		if width > ^uint64(0)-names {
			return nil, ErrLimit
		}
		names += width
	}
	if !core.ArtifactEnvelopeFits(metadata.Provenance, len(metadata.Dependencies), names) {
		return nil, ErrLimit
	}
	// This is the sole defensive dependency copy. The public metadata remains
	// caller-owned; core consumes this canonical owned slice without a bridge
	// or a second compatibility copy.
	dependencies := append([]Dependency(nil), metadata.Dependencies...)
	sort.Slice(dependencies, func(left, right int) bool {
		return dependencies[left].Name < dependencies[right].Name
	})
	for index := 1; index < len(dependencies); index++ {
		if dependencies[index-1].Name == dependencies[index].Name {
			return nil, ErrUnavailableProgram
		}
	}
	if !core.ArtifactEquationCacheFits(metadata.Equations) {
		return nil, ErrLimit
	}
	cache, ok := cloneEquationCache(metadata.Equations)
	if !ok {
		return nil, ErrUnavailableProgram
	}
	envelope := core.ArtifactEnvelope{Target: id, Provenance: metadata.Provenance, Dependencies: dependencies, Equations: cache}
	return core.EncodeArtifact(p, envelope)
}

// Decode accepts only an artifact bound to contract. It reconstructs the
// Program by one ordinary Seal; the optional cache can name only verified
// existing body topology and never installs Program Mu/control rows, a Link
// boundary/candidate, State, or solver schedule.
func Decode(data []byte, contract *target.Contract) (*program.Program, Metadata, error) {
	id, ok := targetID(contract)
	if !ok {
		return nil, Metadata{}, ErrUnavailableTarget
	}
	p, envelope, err := core.DecodeArtifact(data, id)
	if err != nil {
		return nil, Metadata{}, err
	}
	// Core constructed and exclusively owns this cache tree during Decode. The
	// public aliases use the same types, so transferring it avoids a second
	// peak-sized deep copy after the decoder's weighted allocation gate.
	metadata := Metadata{Provenance: envelope.Provenance, Dependencies: envelope.Dependencies, Equations: envelope.Equations}
	return p, metadata, nil
}

// cloneEquationCache establishes ordinary ownership for caller-provided
// Encode metadata after the weighted resource preflight. Decode transfers the
// decoder-owned cache directly and deliberately does not duplicate it.
func cloneEquationCache(source *EquationCache) (*EquationCache, bool) {
	if source == nil {
		return nil, true
	}
	result := *source
	result.Factors = append([]SemanticKey(nil), source.Factors...)
	result.Rules = append([]SemanticKey(nil), source.Rules...)
	result.Bodies = make([]EquationBody, len(source.Bodies))
	for index, body := range source.Bodies {
		result.Bodies[index] = body
		result.Bodies[index].Terms = append([]program.Term(nil), body.Terms...)
		result.Bodies[index].Edges = make([]EquationEdge, len(body.Edges))
		for edgeIndex, edge := range body.Edges {
			result.Bodies[index].Edges[edgeIndex] = edge
			result.Bodies[index].Edges[edgeIndex].MuDecisions = append([]program.Term(nil), edge.MuDecisions...)
		}
	}
	result.Boundary = make([]EquationBoundary, len(source.Boundary))
	for index, boundary := range source.Boundary {
		result.Boundary[index] = boundary
		result.Boundary[index].Reads = append([]EquationRead(nil), boundary.Reads...)
		result.Boundary[index].Writes = append([]uint64(nil), boundary.Writes...)
	}
	return &result, true
}

func targetID(contract *target.Contract) (program.ContentID, bool) {
	if contract == nil {
		return program.ContentID{}, false
	}
	id := contract.ContentID()
	return id, id.Available()
}
