package escape

import (
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// programBoundary is Escape's private executable Program coordinate. Link
// supplies only project topology (Shard -> Program); it does not mint or
// cache the source relations Escape classifies.
type programBoundary struct {
	shard linkproject.Shard
	term  keyspace.Term
	index uint32 // capture index; zero for Write and Return.
}

func collectProgramBoundaries(source *link.Link, kind BoundaryKind) ([]boundary, bool) {
	if source == nil {
		return nil, false
	}
	project := source.Project()
	if project == nil {
		return nil, false
	}
	mounts := project.Mounts()
	coordinates := make([]boundary, 0)
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		shard, shardOK := mounts.At(shardIndex)
		p, programOK := mounts.Program(shard)
		if !shardOK || !programOK || p == nil {
			return nil, false
		}
		flow := p.Flow()
		authored := flow.Authored()
		functions := authored.Functions()
		writes := authored.Storage().Writes()
		returns := authored.Control().Returns()
		executable := flow.Executable()
		exactLenses := authored.Access().Exact()
		dynamicLenses := authored.Access().Dynamic()
		switch kind {
		case BoundaryCapture:
			for functionIndex := 0; functionIndex < functions.Count(); functionIndex++ {
				function, functionOK := functions.At(functionIndex)
				if !functionOK || function == 0 {
					return nil, false
				}
				if !executable.Contains(function) {
					continue
				}
				count, countOK := functions.CaptureCount(function)
				if !countOK || count < 0 {
					return nil, false
				}
				for index := 0; index < count; index++ {
					if uint64(index) > uint64(^uint32(0)) || uint64(len(coordinates)) >= uint64(^uint32(0)) {
						return nil, false
					}
					if _, _, captureOK := functions.CaptureAt(function, index); !captureOK {
						return nil, false
					}
					coordinates = append(coordinates, boundary{kind: kind, program: programBoundary{shard: shard, term: function, index: uint32(index)}})
				}
			}
		case BoundaryStore:
			for writeIndex := 0; writeIndex < writes.Count(); writeIndex++ {
				write, writeOK := writes.At(writeIndex)
				if !writeOK || write == 0 {
					return nil, false
				}
				if !executable.Contains(write) {
					continue
				}
				_, target, writeRelationOK := writes.Get(write)
				if !writeRelationOK || target == 0 {
					return nil, false
				}
				_, _, _, _, exactLens := exactLenses.Get(target)
				_, _, _, dynamicLens := dynamicLenses.Get(target)
				if exactLens || dynamicLens {
					if uint64(len(coordinates)) >= uint64(^uint32(0)) {
						return nil, false
					}
					coordinates = append(coordinates, boundary{kind: kind, program: programBoundary{shard: shard, term: write}})
				}
			}
		case BoundaryReturn:
			for returnIndex := 0; returnIndex < returns.Count(); returnIndex++ {
				term, returnOK := returns.At(returnIndex)
				if !returnOK || term == 0 {
					return nil, false
				}
				if !executable.Contains(term) {
					continue
				}
				if _, _, relationOK := returns.Get(term); !relationOK {
					return nil, false
				}
				if uint64(len(coordinates)) >= uint64(^uint32(0)) {
					return nil, false
				}
				coordinates = append(coordinates, boundary{kind: kind, program: programBoundary{shard: shard, term: term}})
			}
		default:
			return nil, false
		}
	}
	return coordinates, true
}
