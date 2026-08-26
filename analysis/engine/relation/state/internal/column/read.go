package column

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// ReadPart is one nonempty key-local support partition with its semantic cell
// and lineage sidecar. Undefined sparse terminals are omitted; explicit
// ProvenAbsent/UnprovenMissing cells remain visible with an unavailable value
// token.
type ReadPart struct {
	key     geometry.Key
	region  support.Mask
	cell    Cell
	lineage model.LineageRef
}

// Key returns the physical scalar coordinate supplied to Read.
func (part ReadPart) Key() geometry.Key { return part.key }

// Region returns the exact support partition where this terminal pair holds.
func (part ReadPart) Region() support.Mask { return part.region }

// Cell returns the semantic terminal payload.
func (part ReadPart) Cell() Cell { return part.cell }

// Lineage returns the independent lineage terminal.
func (part ReadPart) Lineage() model.LineageRef { return part.lineage }

// ReadScratch is caller-owned partition storage and support work for one
// borrowed read stream. It carries no column state and is not safe for
// concurrent use.
type ReadScratch struct {
	manager  *guard.Manager
	support  *support.Work
	semantic []semanticPartition
	lineage  []lineagePartition
}

// NewReadScratch reserves a reusable read shell for manager. The manager is
// sealed and must match the Column generation passed to Read.
func NewReadScratch(manager *guard.Manager) *ReadScratch {
	if manager == nil {
		return nil
	}
	return &ReadScratch{manager: manager, support: support.New(manager)}
}

// Reset clears only borrowed partition vectors; it retains support work and
// its capacity for the next read.
func (scratch *ReadScratch) Reset() {
	if scratch == nil {
		return
	}
	clear(scratch.semantic)
	scratch.semantic = scratch.semantic[:0]
	clear(scratch.lineage)
	scratch.lineage = scratch.lineage[:0]
}

// Available reports whether scratch owns a support manager shell.
func (scratch *ReadScratch) Available() bool {
	return scratch != nil && scratch.manager != nil && scratch.support != nil && scratch.support.OwnsManager(scratch.manager)
}

// Borrowed is a generation-fenced immutable read handle. All partition
// storage is supplied by ReadScratch, so opening this handle allocates no
// state and does not copy roots.
type Borrowed struct {
	version Version
	fence   binding.Fence
}

// Available reports whether borrowed still names its exact immutable version.
func (borrowed Borrowed) Available() bool {
	return borrowed.version.Available() && borrowed.fence.Same(borrowed.version.Fence())
}

// Read streams every nonempty semantic/lineage terminal partition under key
// and within. It never chooses a sole terminal and never synthesizes a
// default. Returning false from visit stops the stream with (false,true).
func (borrowed Borrowed) Read(key geometry.Key, within support.Mask, scratch *ReadScratch, visit func(ReadPart) bool) (completed, valid bool) {
	if !borrowed.Available() || !borrowed.fence.Same(borrowed.version.Fence()) {
		return false, false
	}
	return borrowed.version.read(key, within, scratch, visit)
}

// Scan streams every committed sparse key in canonical ascending geometry
// order. Each key is expanded through the same semantic/lineage partition
// path as Read; undefined sparse keys never invoke visit. Caller-owned
// scratch is reused for every key and no key slice is exposed.
func (borrowed Borrowed) Scan(within support.Mask, scratch *ReadScratch, visit func(ReadPart) bool) (completed, valid bool) {
	if !borrowed.Available() || !borrowed.fence.Same(borrowed.version.Fence()) {
		return false, false
	}
	return borrowed.version.scan(within, scratch, visit)
}

// Read is the direct immutable-version read entry point.
func (version Version) Read(key geometry.Key, within support.Mask, scratch *ReadScratch, visit func(ReadPart) bool) (completed, valid bool) {
	return version.read(key, within, scratch, visit)
}

// Scan streams every committed sparse key in canonical ascending geometry.Key
// order. It retains explicit absence terminals, omits undefined columns, and
// stops with (false,true) when visit declines a partition.
func (version Version) Scan(within support.Mask, scratch *ReadScratch, visit func(ReadPart) bool) (completed, valid bool) {
	return version.scan(within, scratch, visit)
}

func (version Version) scan(within support.Mask, scratch *ReadScratch, visit func(ReadPart) bool) (completed, valid bool) {
	if !version.Available() || !within.Valid() || within.Manager() != version.column.guards || scratch == nil || !scratch.Available() || scratch.manager != version.column.guards || visit == nil {
		return false, false
	}
	scratch.Reset()
	for _, key := range version.state.keys {
		completed, valid := version.read(key, within, scratch, visit)
		if !valid {
			return false, false
		}
		if !completed {
			return false, true
		}
	}
	return true, true
}

func (version Version) read(key geometry.Key, within support.Mask, scratch *ReadScratch, visit func(ReadPart) bool) (completed, valid bool) {
	if !version.Available() || !within.Valid() || within.Manager() != version.column.guards || scratch == nil || !scratch.Available() || scratch.manager != version.column.guards || visit == nil {
		return false, false
	}
	scratch.Reset()
	semanticValue, semanticPresent, semanticValid := version.column.semanticGraph.Get(version.state.semantic, factor, key)
	lineageValue, lineagePresent, lineageValid := version.column.lineageGraph.Get(version.state.lineage, factor, key)
	if !semanticValid || !lineageValid || semanticPresent != lineagePresent {
		return false, false
	}
	if !semanticPresent {
		return true, true
	}
	if completed, valid := version.collectSemantic(semanticValue, within, scratch); !completed || !valid {
		return false, false
	}
	if completed, valid := version.collectLineage(lineageValue, within, scratch); !completed || !valid {
		return false, false
	}
	// A valid read window may simply contain no committed terminal.  That is
	// an ordinary empty selection, not a malformed authority: callers use the
	// completed/valid pair to distinguish it from a foreign fence, bad scratch,
	// or failed partition traversal.  Refusing here makes a disjoint guard
	// fiber impossible to populate after a sibling fiber has been published.
	if len(scratch.semantic) == 0 || len(scratch.lineage) == 0 {
		return true, true
	}
	for _, semantic := range scratch.semantic {
		for _, lineage := range scratch.lineage {
			overlap, ok := support.IntersectWithWork(scratch.support, nil, semantic.region, lineage.region)
			if !ok {
				return false, false
			}
			if support.Empty(overlap) {
				continue
			}
			cell, ok := version.semanticValue(semantic.id)
			if !ok || lineage.id == (terminal.ID[model.LineageRef]{}) {
				return false, false
			}
			lineageValue, ok := version.column.lineageArena.Value(lineage.id)
			if !ok || !lineageValue.Available() {
				return false, false
			}
			if !visit(ReadPart{key: key, region: overlap, cell: cell, lineage: lineageValue}) {
				return false, true
			}
		}
	}
	return true, true
}

func (version Version) collectSemantic(value diagram.Value[Cell], within support.Mask, scratch *ReadScratch) (bool, bool) {
	return version.column.semanticGraph.PartitionValueTerminals(value, within, scratch.support, func(id terminal.ID[Cell], region support.Mask) bool {
		if id == (terminal.ID[Cell]{}) || support.Empty(region) {
			return true
		}
		if !version.column.semanticArena.Valid(id) {
			return false
		}
		scratch.semantic = append(scratch.semantic, semanticPartition{id: id, region: region})
		return true
	})
}

func (version Version) collectLineage(value diagram.Value[model.LineageRef], within support.Mask, scratch *ReadScratch) (bool, bool) {
	return version.column.lineageGraph.PartitionValueTerminals(value, within, scratch.support, func(id terminal.ID[model.LineageRef], region support.Mask) bool {
		if id == (terminal.ID[model.LineageRef]{}) || support.Empty(region) {
			return true
		}
		if !version.column.lineageArena.Valid(id) {
			return false
		}
		scratch.lineage = append(scratch.lineage, lineagePartition{id: id, region: region})
		return true
	})
}

type lineagePartition struct {
	id     terminal.ID[model.LineageRef]
	region support.Mask
}
