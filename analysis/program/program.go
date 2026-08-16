// Package program is the immutable root of one canonical Program.
package program

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// Program is the immutable root of one published canonical owner quartet.
// Owner-specific relations remain behind their typed Views; the root exposes
// only generated schema queries over those immutable columns.
type Program struct {
	source            *source.Component
	flow              *flow.Component
	static            *static.Component
	module            *imports.Component
	id                identity.ContentID
	allocationReceipt *allocationReceipt
	valuesCatalog     *valuesCatalog
	returnCatalog     *returnCatalog
}

var (
	errInvalidAssembly = errors.New("program: invalid Assembly")
	errUnavailable     = errors.New("program: unavailable owner identity")
	errProvenance      = errors.New("program: Flow provenance does not match owner quartet")
	errAllocation      = errors.New("program: allocation receipt unavailable")
)

// Publish consumes assembly and publishes the one immutable Program root.
//
// Assembly is a one-shot transfer capability. Take is performed before any
// validation, so malformed or mismatched input is terminal and cannot be
// retried through a copied token. The Flow provenance fence is checked against
// the four exact child identities before the root identity is derived.
func Publish(assembly *flow.Assembly) (*Program, error) {
	if assembly == nil {
		return nil, errInvalidAssembly
	}

	sourceComponent, flowComponent, staticComponent, moduleComponent, err := assembly.Take()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidAssembly, err)
	}
	if sourceComponent == nil || flowComponent == nil || staticComponent == nil || moduleComponent == nil {
		return nil, errInvalidAssembly
	}

	sourceID := sourceComponent.Cold().ContentID()
	flowID := flowComponent.ContentID()
	staticID := staticComponent.ContentID()
	moduleID := moduleComponent.View().ContentID()
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
	program := &Program{
		source: sourceComponent,
		flow:   flowComponent,
		static: staticComponent,
		module: moduleComponent,
		id:     id,
	}
	allocationReceipt, allocationErr := buildAllocationReceipt(program)
	if allocationErr != nil {
		return nil, fmt.Errorf("%w: %v", errAllocation, allocationErr)
	}
	program.allocationReceipt = allocationReceipt
	valuesCatalog, valuesOK := buildValuesCatalog(program)
	if !valuesOK {
		return nil, errors.New("program: Values catalog unavailable")
	}
	program.valuesCatalog = valuesCatalog
	returnCatalog, returnsOK := buildReturnCatalog(program)
	if !returnsOK {
		return nil, errors.New("program: Return catalog unavailable")
	}
	program.returnCatalog = returnCatalog
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
func (program *Program) Static() static.View {
	if program == nil {
		return static.View{}
	}
	return program.static.View()
}

// Module returns the immutable Module owner view.
func (program *Program) Module() imports.View {
	if program == nil {
		return imports.View{}
	}
	return program.module.View()
}

// ContentID returns the composite identity of the exact four-owner quartet.
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
