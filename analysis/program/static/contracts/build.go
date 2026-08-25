package contracts

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/internal/rows"
)

// Build validates and seals the dense authored static sidecars for Flow
// Function and Call terms. It neither reconstructs Flow geometry nor
// evaluates a call.
func Build(input Input, counts [keyspace.FamilyCount]uint32) (Table, error) {
	var terms rows.PoolBuilder[keyspace.Term]
	function := rows.NewTableBuilder[FunctionContractRow](keyspace.FamilyFunction)
	for _, row := range input.Function {
		if !row.ReturnsKnown && len(row.Returns) != 0 {
			return Table{}, errors.New("program/static/contracts: omitted function returns have children")
		}
		for _, result := range row.Returns {
			if !staticrole.Node(counts, result) {
				return Table{}, errors.New("program/static/contracts: invalid function return")
			}
		}
		params, ok := terms.Append(row.TypeParams)
		if !ok {
			return Table{}, errors.New("program/static/contracts: oversized function type parameters")
		}
		returns, ok := terms.Append(row.Returns)
		if !ok {
			return Table{}, errors.New("program/static/contracts: oversized function returns")
		}
		sealed := FunctionContractRow{TypeParams: params, ReturnsKnown: row.ReturnsKnown, Returns: returns}
		if _, ok := function.Append(sealed); !ok {
			return Table{}, errors.New("program/static/contracts: oversized function contract table")
		}
	}

	// Calls seal after every function, so the terms appended below form one
	// contiguous segment whose width is the sealed call type-argument total.
	callTypeArgumentStart := terms.Len()
	call := rows.NewTableBuilder[CallContractRow](keyspace.FamilyCall)
	argumentID := rows.NewTableBuilder[identity.ContentID](keyspace.FamilyCall)
	for _, row := range input.Call {
		for _, typeArgument := range row.TypeArguments {
			if !staticrole.Node(counts, typeArgument) {
				return Table{}, errors.New("program/static/contracts: invalid call type argument")
			}
		}
		typeArguments, ok := terms.Append(row.TypeArguments)
		if !ok {
			return Table{}, errors.New("program/static/contracts: oversized call type arguments")
		}
		if _, ok := call.Append(CallContractRow{TypeArguments: typeArguments}); !ok {
			return Table{}, errors.New("program/static/contracts: oversized call contract table")
		}
		id, idOK := callTypeArgumentID(row.TypeArguments)
		if !idOK {
			return Table{}, errors.New("program/static/contracts: unavailable call type-argument identity")
		}
		if _, ok := argumentID.Append(id); !ok {
			return Table{}, errors.New("program/static/contracts: oversized call identity table")
		}
	}
	width := terms.Len() - callTypeArgumentStart
	if width < 0 || uint64(width) > uint64(^uint32(0)) {
		return Table{}, errors.New("program/static/contracts: unrepresentable call type-argument width")
	}
	return Table{
		function:          function.Seal(),
		call:              call.Seal(),
		callArgumentID:    argumentID.Seal(),
		terms:             terms.Seal(),
		callTypeArguments: uint32(width),
	}, nil
}

// callTypeArgumentID is the stable content identity of one call's authored
// type-argument sequence. It is authored-derived and retained, never
// recomputed at read time.
func callTypeArgumentID(terms []keyspace.Term) (id identity.ContentID, ok bool) {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/static/call-type-arguments", 1) != nil ||
		writeTerms(&writer, terms) != nil || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}, false
	}
	return id, id.Available()
}

func writeTerms(writer *framing.Writer, terms []keyspace.Term) error {
	if err := writer.Count(uint64(len(terms))); err != nil {
		return err
	}
	for _, term := range terms {
		if err := writer.Uint(uint64(term)); err != nil {
			return err
		}
	}
	return nil
}
