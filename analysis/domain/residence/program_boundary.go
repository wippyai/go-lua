package residence

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// programBoundary is Residence's exact executable Program relation. Link
// supplies project topology (Shard -> Program); Program remains the sole
// authority for the relation itself.  It is deliberately private: callers
// obtain Residence Keys, never a second cross-domain source handle.
type programBoundary struct {
	shard linkproject.Shard
	term  keyspace.Term
	index uint32 // capture index; zero for Write and Return.
}

func (owner *schema) captureBoundaries() bool {
	for shardIndex := 0; shardIndex < owner.source.Project().Mounts().Count(); shardIndex++ {
		shard, shardOK := owner.source.Project().Mounts().At(shardIndex)
		p, programOK := owner.source.Project().Mounts().Program(shard)
		if !shardOK || !programOK || p == nil {
			return false
		}
		class, classOK := owner.classForShard(shard)
		if !classOK {
			return false
		}
		flow := p.Flow()
		authored := flow.Authored()
		functions := authored.Functions()
		executable := flow.Executable()
		for functionIndex := 0; functionIndex < functions.Count(); functionIndex++ {
			function, functionOK := functions.At(functionIndex)
			if !functionOK || function == 0 {
				return false
			}
			if !executable.Contains(function) {
				continue
			}
			count, countOK := functions.CaptureCount(function)
			if !countOK || count < 0 {
				return false
			}
			for index := 0; index < count; index++ {
				if uint64(index) > uint64(^uint32(0)) {
					return false
				}
				coordinate := programBoundary{shard: shard, term: function, index: uint32(index)}
				if _, _, captureOK := functions.CaptureAt(function, index); !captureOK ||
					!owner.addBoundaryClass(boundaryRow{kind: BoundaryCapture, id: owner.programBoundaryID(BoundaryCapture, coordinate), program: coordinate}, class) {
					return false
				}
			}
		}
	}
	return true
}

func (owner *schema) storeBoundaries() bool {
	for shardIndex := 0; shardIndex < owner.source.Project().Mounts().Count(); shardIndex++ {
		shard, shardOK := owner.source.Project().Mounts().At(shardIndex)
		p, programOK := owner.source.Project().Mounts().Program(shard)
		if !shardOK || !programOK || p == nil {
			return false
		}
		class, classOK := owner.classForShard(shard)
		if !classOK {
			return false
		}
		flow := p.Flow()
		authored := flow.Authored()
		writes := authored.Storage().Writes()
		executable := flow.Executable()
		exactLenses := authored.Access().Exact()
		dynamicLenses := authored.Access().Dynamic()
		for writeIndex := 0; writeIndex < writes.Count(); writeIndex++ {
			write, writeOK := writes.At(writeIndex)
			if !writeOK || write == 0 {
				return false
			}
			if !executable.Contains(write) {
				continue
			}
			_, target, writeRelationOK := writes.Get(write)
			if !writeRelationOK || target == 0 {
				return false
			}
			_, _, _, _, exactLens := exactLenses.Get(target)
			_, _, _, dynamicLens := dynamicLenses.Get(target)
			if !exactLens && !dynamicLens {
				continue
			}
			coordinate := programBoundary{shard: shard, term: write}
			if !owner.addBoundaryClass(boundaryRow{kind: BoundaryStore, id: owner.programBoundaryID(BoundaryStore, coordinate), program: coordinate}, class) {
				return false
			}
		}
	}
	return true
}

func (owner *schema) returnBoundaries() bool {
	for shardIndex := 0; shardIndex < owner.source.Project().Mounts().Count(); shardIndex++ {
		shard, shardOK := owner.source.Project().Mounts().At(shardIndex)
		p, programOK := owner.source.Project().Mounts().Program(shard)
		if !shardOK || !programOK || p == nil {
			return false
		}
		class, classOK := owner.classForShard(shard)
		if !classOK {
			return false
		}
		flow := p.Flow()
		authored := flow.Authored()
		returns := authored.Control().Returns()
		executable := flow.Executable()
		for returnIndex := 0; returnIndex < returns.Count(); returnIndex++ {
			term, returnOK := returns.At(returnIndex)
			if !returnOK || term == 0 {
				return false
			}
			if !executable.Contains(term) {
				continue
			}
			if _, _, relationOK := returns.Get(term); !relationOK {
				return false
			}
			coordinate := programBoundary{shard: shard, term: term}
			if !owner.addBoundaryClass(boundaryRow{kind: BoundaryReturn, id: owner.programBoundaryID(BoundaryReturn, coordinate), program: coordinate}, class) {
				return false
			}
		}
	}
	return true
}

func (owner *schema) programBoundaryID(kind BoundaryKind, coordinate programBoundary) keyspace.ContentID {
	if owner == nil || owner.source == nil {
		return keyspace.ContentID{}
	}
	shard, ok := owner.source.Project().Mounts().Index(coordinate.shard)
	if !ok {
		return keyspace.ContentID{}
	}
	var image [32 + 5*8]byte
	copy(image[:32], owner.linkID[:])
	binary.BigEndian.PutUint64(image[32:], 0x7265732d70726f67) // "res-prog"
	binary.BigEndian.PutUint64(image[40:], uint64(kind))
	binary.BigEndian.PutUint64(image[48:], uint64(shard+1))
	binary.BigEndian.PutUint64(image[56:], uint64(coordinate.term))
	binary.BigEndian.PutUint64(image[64:], uint64(coordinate.index))
	return keyspace.ContentID(sha256.Sum256(image[:]))
}

func (owner *schema) validProgramBoundary(kind BoundaryKind, coordinate programBoundary) bool {
	if owner == nil || owner.source == nil || coordinate.term == 0 {
		return false
	}
	p, ok := owner.source.Project().Mounts().Program(coordinate.shard)
	if !ok || p == nil || !p.Flow().Executable().Contains(coordinate.term) {
		return false
	}
	switch kind {
	case BoundaryCapture:
		functions := p.Flow().Authored().Functions()
		count, ok := functions.CaptureCount(coordinate.term)
		if !ok || count < 0 || uint64(count) > uint64(^uint32(0))+1 || uint64(coordinate.index) >= uint64(count) {
			return false
		}
		_, _, ok = functions.CaptureAt(coordinate.term, int(coordinate.index))
		return ok
	case BoundaryStore:
		flow := p.Flow()
		writes := flow.Authored().Storage().Writes()
		_, target, ok := writes.Get(coordinate.term)
		if !ok || target == 0 {
			return false
		}
		_, _, _, _, exactLens := flow.Authored().Access().Exact().Get(target)
		_, _, _, dynamicLens := flow.Authored().Access().Dynamic().Get(target)
		return exactLens || dynamicLens
	case BoundaryReturn:
		_, _, ok := p.Flow().Authored().Control().Returns().Get(coordinate.term)
		return ok
	default:
		return false
	}
}
