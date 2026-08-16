// Package program is the immutable root of one canonical Program.
package program

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

// Program is the immutable root of one published canonical owner quartet.
//
// The root stores the four owner components, their composite identity, and
// one detached cold semantic-source receipt. Owner-specific relations remain
// behind their typed Views; the receipt is cardinality-only and is not a
// second mutable relation representation or forwarding query surface.
type Program struct {
	source            *source.Component
	flow              *flow.Component
	static            *static.Component
	module            *module.Component
	id                keyspace.ContentID
	semanticReceipt   SemanticSourceReceipt
	allocationReceipt *allocationReceipt
	pointAttachments  *pointAttachmentReceipt
	valuesCatalog     *valuesCatalog
	returnCatalog     *returnCatalog
}

var (
	errInvalidAssembly = errors.New("program: invalid Assembly")
	errUnavailable     = errors.New("program: unavailable owner identity")
	errProvenance      = errors.New("program: Flow provenance does not match owner quartet")
	errSemanticSource  = errors.New("program: semantic-source receipt unavailable")
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
	pointAttachments, pointsOK := buildPointAttachmentReceipt(program)
	if !pointsOK {
		return nil, errors.New("program: WTO point attachment receipt unavailable")
	}
	program.pointAttachments = pointAttachments
	// Allocation templates consume exact point-backed Program proofs. Install
	// the point attachment sidecar first so their construction-time
	// TransformerInput is complete; neither sidecar is rebuilt after publish.
	allocationReceipt, allocationErr := buildAllocationReceipt(program)
	if allocationErr != nil {
		return nil, fmt.Errorf("%w: %v", errAllocation, allocationErr)
	}
	program.allocationReceipt = allocationReceipt
	// Semantic-source publication is a cold seal product.  Build and validate
	// the detached receipt while the exact owner quartet is still in hand so
	// later mounts can replay the fixed 57-row denominator without reopening
	// any child query or retaining a hot registry.
	receipt, ok := buildProgramSemanticSourceReceipt(program)
	if !ok {
		return nil, errSemanticSource
	}
	program.semanticReceipt = receipt
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
func (program *Program) Module() module.View {
	if program == nil {
		return module.View{}
	}
	return program.module.View()
}

// ContentID returns the composite identity of the exact four-owner quartet.
func (program *Program) ContentID() keyspace.ContentID {
	if program == nil {
		return keyspace.ContentID{}
	}
	return program.id
}

// rootContentID is the sole root identity codec. Child identities are emitted
// as four independently framed raw 32-byte values in owner order. The root
// version is deliberately independent from each child codec version.
func rootContentID(sourceID, flowID, staticID, moduleID keyspace.ContentID) (keyspace.ContentID, error) {
	hash := sha256.New()
	var writer canonical.Writer
	if err := writer.Reset(hash, "program/root", 1); err != nil {
		return keyspace.ContentID{}, err
	}
	for _, childID := range [...]keyspace.ContentID{sourceID, flowID, staticID, moduleID} {
		if err := writer.Bytes(childID[:]); err != nil {
			return keyspace.ContentID{}, err
		}
	}
	if err := writer.Finish(); err != nil {
		return keyspace.ContentID{}, err
	}
	var id keyspace.ContentID
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}, errors.New("program: invalid root identity length")
	}
	return id, nil
}
