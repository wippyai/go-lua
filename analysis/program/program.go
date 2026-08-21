// Package program is the immutable root of one canonical Program.
package program

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/internal/framing"
)

// Program is the immutable root of one published canonical Source/Flow/Static
// owner set plus scalar authored ModuleID. Owner-specific relations remain
// behind their typed Views; the root exposes only generated schema queries
// over those immutable columns.
type Program struct {
	source   *source.Component
	flow     *flow.Component
	static   *static.Component
	moduleID identity.ContentID
	id       identity.ContentID
	counts   denominator.CountRows
}

// Available reports whether the three immutable Program owners, scalar
// ModuleID, and their provenance fence are sealed. Program itself is the
// construction input;
// there is no second transport or proof object around it. Publish validates
// the scalar four-ID provenance fence before program.id is assigned, so a sealed
// id is proof the fence already holds.
func (program *Program) Available() bool {
	return program != nil && program.source != nil && program.flow != nil && program.static != nil && program.moduleID.Available() &&
		program.id.Available()
}

var (
	errInvalidAssembly = errors.New("program: invalid Assembly")
	errUnavailable     = errors.New("program: unavailable owner identity")
	errProvenance      = errors.New("program: Flow provenance does not match scalar owner fence")
)

// Publish consumes assembly and publishes the one immutable Program root.
//
// Assembly is a one-shot transfer capability. Take is performed before any
// validation, so malformed or mismatched input is terminal and cannot be
// retried through a copied token. The Flow provenance fence is checked against
// the three exact child identities plus scalar ModuleID before the root identity is derived.
func Publish(assembly *flow.Assembly) (*Program, error) {
	if assembly == nil {
		return nil, errInvalidAssembly
	}

	sourceComponent, flowComponent, staticComponent, moduleID, err := assembly.Take()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidAssembly, err)
	}
	if sourceComponent == nil || flowComponent == nil || staticComponent == nil || !moduleID.Available() {
		return nil, errInvalidAssembly
	}

	sourceID := sourceComponent.ContentID()
	flowID := flowComponent.ContentID()
	staticID := staticComponent.ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return nil, errUnavailable
	}

	provenance := flowComponent.View().Provenance()
	if provenance.Source != sourceID || provenance.Flow != flowID ||
		provenance.Static != staticID || provenance.Module != moduleID {
		return nil, errProvenance
	}

	id, err := rootContentID(sourceID, flowID, staticID, moduleID)
	if err != nil {
		return nil, fmt.Errorf("program: derive root identity: %w", err)
	}
	sourceCounts, err := source.CountRows(sourceComponent.View())
	if err != nil {
		return nil, err
	}
	flowCounts, err := flow.CountRows(flowComponent.View())
	if err != nil {
		return nil, err
	}
	staticCounts, err := static.CountRows(staticComponent.View())
	if err != nil {
		return nil, err
	}
	// Module authored rows now live behind Flow and are authenticated by the
	// scalar moduleID. Their derived denominator family is sealed by the later
	// artifact boundary; the root retains no Module component or lifecycle.
	counts, err := combineProgramCountRows(sourceCounts, flowCounts, staticCounts)
	if err != nil {
		return nil, err
	}
	program := &Program{
		source:   sourceComponent,
		flow:     flowComponent,
		static:   staticComponent,
		moduleID: moduleID,
		id:       id,
		counts:   counts,
	}
	return program, nil
}

// Source returns the immutable Source owner view.
func (program *Program) Source() source.View {
	if program == nil {
		return source.View{}
	}
	return program.source.View()
}

// Flow returns the immutable Flow owner view.
func (program *Program) Flow() flow.View {
	if program == nil {
		return flow.View{}
	}
	return program.flow.View()
}

// Static returns the immutable Static owner view.
func (program *Program) Static() staticquery.View {
	if program == nil {
		return staticquery.View{}
	}
	return program.static.View()
}

// ContentID returns the composite identity of the exact four-ID owner codec.
func (program *Program) ContentID() identity.ContentID {
	if program == nil {
		return identity.ContentID{}
	}
	return program.id
}

// rootContentID is the sole root identity codec. Child identities are emitted
// as four independently framed raw 32-byte values in owner order. The root
// version is deliberately independent from each child codec version.
func rootContentID(sourceID, flowID, staticID, moduleID identity.ContentID) (identity.ContentID, error) {
	hash := sha256.New()
	var writer framing.Writer
	if err := writer.Reset(hash, "program/root", 1); err != nil {
		return identity.ContentID{}, err
	}
	for _, childID := range [...]identity.ContentID{sourceID, flowID, staticID, moduleID} {
		if err := writer.Bytes(childID[:]); err != nil {
			return identity.ContentID{}, err
		}
	}
	if err := writer.Finish(); err != nil {
		return identity.ContentID{}, err
	}
	var id identity.ContentID
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}, errors.New("program: invalid root identity length")
	}
	return id, nil
}
