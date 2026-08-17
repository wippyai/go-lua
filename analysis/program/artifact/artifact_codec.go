// Package artifact owns Program's one portable persistence boundary.
//
// The artifact stream stores only the immutable target/envelope identity and
// the four authored owner sections.  Flow, Source positions, Static indexes,
// Module resolution, and every other derived projection are rebuilt through
// the ordinary owner assembly on decode.
package artifact

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/analysis/program/target"
)

// Encode binds one published Program to one exact sealed target Contract.
// There is no unbound, legacy, or compatibility representation.
func Encode(p *program.Program, contract *target.Contract, metadata Metadata) ([]byte, error) {
	targetID, ok := targetIdentity(contract)
	if !ok {
		return nil, ErrUnavailableTarget
	}
	if p == nil || !p.ContentID().Available() || !ownerViewsAvailable(p) {
		return nil, ErrUnavailableProgram
	}
	dependencies, err := canonicalDependencies(metadata)
	if err != nil {
		return nil, err
	}
	entry, ok := p.Source().Index().Entry()
	if !ok {
		return nil, ErrUnavailableProgram
	}

	destination := newArtifactBuffer(artifactMaxBytes)
	var writer framing.Writer
	if err := writer.Reset(destination, artifactDomain, artifactVersion); err != nil {
		return nil, encodeError(err)
	}
	if err := writeEnvelope(&writer, targetID, p.ContentID(), entry, metadata.Provenance, dependencies); err != nil {
		return nil, encodeError(err)
	}
	// These are the only four payload authorities and their order is part of
	// the v20 stream grammar.
	if err := source.WriteArtifactSection(&writer, p.Source()); err != nil {
		return nil, encodeError(err)
	}
	if err := flow.WriteArtifactSection(&writer, p.Flow()); err != nil {
		return nil, encodeError(err)
	}
	if err := static.WriteArtifactSection(&writer, p.Static()); err != nil {
		return nil, encodeError(err)
	}
	if err := imports.WriteArtifactSection(&writer, p.Module()); err != nil {
		return nil, encodeError(err)
	}
	if err := writer.Finish(); err != nil {
		return nil, encodeError(err)
	}

	data := destination.Bytes()
	measure, err := framing.Scan(data, artifactMaxBytes)
	if err != nil {
		return nil, encodeError(err)
	}
	if !artifactMeasureAllowed(measure) {
		return nil, ErrLimit
	}
	return data, nil
}

// Decode accepts only the v20 stream bound to contract and reconstructs a
// fresh owner quartet through the ordinary Build/Finalizer/Assemble/Publish
// path. No derived section is read or retained.
func Decode(data []byte, contract *target.Contract, expectedDependencies []Dependency) (*program.Program, Metadata, error) {
	targetID, ok := targetIdentity(contract)
	if !ok {
		return nil, Metadata{}, ErrUnavailableTarget
	}
	if len(data) > artifactMaxBytes {
		return nil, Metadata{}, ErrLimit
	}
	expected, err := canonicalDependencies(Metadata{Dependencies: expectedDependencies})
	if err != nil {
		if errors.Is(err, ErrLimit) {
			return nil, Metadata{}, ErrLimit
		}
		return nil, Metadata{}, ErrDependencyMismatch
	}
	measure, err := framing.Scan(data, artifactMaxBytes)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	if !artifactMeasureAllowed(measure) {
		return nil, Metadata{}, ErrLimit
	}
	reader, err := framing.NewReader(data, artifactMaxBytes)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	if err := reader.Header(artifactDomain, artifactVersion); err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	envelope, err := readEnvelope(reader, targetID, measure.StringBytes, expected)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}

	// Child sections are decoded in the same fixed order as Encode. Each child
	// parser performs its own value-copy preflight before allocating rows.
	sourceInput, err := source.ReadArtifactSection(reader)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	flowInput, err := flow.ReadArtifactSection(reader)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	staticInput, err := static.ReadArtifactSection(reader)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	moduleInput, err := imports.ReadArtifactSection(reader)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	if err := reader.Finish(); err != nil {
		return nil, Metadata{}, decodeError(err)
	}

	p, err := rebuild(sourceInput, flowInput, staticInput, moduleInput, envelope.Entry)
	if err != nil {
		return nil, Metadata{}, decodeError(fmt.Errorf("rebuild: %w", err))
	}
	if p.ContentID() != envelope.Program {
		return nil, Metadata{}, ErrNoncanonical
	}
	metadata := Metadata{
		Provenance:   envelope.Provenance,
		Dependencies: append([]Dependency(nil), envelope.Dependencies...),
	}
	canonicalBytes, err := Encode(p, contract, metadata)
	if err != nil {
		return nil, Metadata{}, decodeError(err)
	}
	if !bytes.Equal(data, canonicalBytes) {
		return nil, Metadata{}, ErrNoncanonical
	}
	return p, metadata, nil
}
