package artifact

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// artifactIndexRead is the compile-time projection of one already-sealed
// Flow index-read row. It is deliberately private: the Artifact is the
// owner of the immutable row and no transformer-shaped access authority is
// retained after compilation.
type artifactIndexRead struct {
	term, base, lens, keyTerm keyspace.Term
	baseSpan, resultSpan      program.Span
	dynamicKeySpan            program.Span
	exactKey                  keyspace.Key
	exact                     bool
	id, baseID, lensID        identity.ContentID
	resultID                  identity.ContentID
}

// artifactIndexWrite is the compile-time projection of one already-sealed
// Flow index-write row. Assignment predecessor identity is borrowed directly
// from Flow; it is not reconstructed from storage order.
type artifactIndexWrite struct {
	term, base, lens, values keyspace.Term
	position                 int
	baseSpan                 program.Span
	dynamicKeySpan           program.Span
	exactKey                 keyspace.Key
	exact                    bool
	finish                   flow.Site
	predecessor              flow.Successor
	route                    identity.ContentID
	id, baseID, lensID       identity.ContentID
	valuesID, predecessorID  identity.ContentID
}

func (compiler *compiler) indexReadAt(index int) (artifactIndexRead, bool) {
	if compiler == nil || !compiler.input.Available() || index < 0 {
		return artifactIndexRead{}, false
	}
	geometry := compiler.input.Flow().AccessGeometry()
	reads := geometry.IndexAccesses().Reads()
	term, ok := reads.At(index)
	if !ok {
		return artifactIndexRead{}, false
	}
	base, keyTerm, lens, ok := reads.Get(term)
	if !ok {
		return artifactIndexRead{}, false
	}
	baseSpan, baseOK := compiler.input.Span(base)
	resultSpan, resultOK := compiler.input.Span(term)
	row := artifactIndexRead{term: term, base: base, lens: lens, keyTerm: keyTerm, baseSpan: baseSpan, resultSpan: resultSpan}
	if exactKey, exactOK := geometry.ExactLenses().Get(lens); exactOK {
		row.exact, row.exactKey = true, exactKey
	} else {
		if _, dynamicOK := geometry.DynamicLenses().Get(lens); !dynamicOK {
			return artifactIndexRead{}, false
		}
		row.dynamicKeySpan, _ = compiler.input.Span(keyTerm)
	}
	row.id = accessRoleID("program/transformer/index-read", compiler.input, term)
	row.baseID = accessSubroleID("program/transformer/index-base", compiler.input, term, base)
	lensDomain := "program/transformer/index-lens-dynamic"
	if row.exact {
		lensDomain = "program/transformer/index-lens-exact"
	}
	row.lensID = accessSubroleID(lensDomain, compiler.input, term, lens)
	row.resultID = accessSubroleID("program/transformer/index-result", compiler.input, term, term)
	return row, baseOK && resultOK && row.id.Available() && row.baseID.Available() && row.lensID.Available() && row.resultID.Available()
}

func (compiler *compiler) indexWriteAt(index int) (artifactIndexWrite, bool) {
	if compiler == nil || !compiler.input.Available() || index < 0 {
		return artifactIndexWrite{}, false
	}
	geometry := compiler.input.Flow().AccessGeometry()
	writes := geometry.IndexAccesses().Writes()
	term, ok := writes.At(index)
	if !ok {
		return artifactIndexWrite{}, false
	}
	base, keyTerm, values, position, lens, ok := writes.Get(term)
	if !ok || position < 0 {
		return artifactIndexWrite{}, false
	}
	baseSpan, baseOK := compiler.input.Span(base)
	finishTerm, finishTermOK := compiler.input.Flow().Ports().Finish(term)
	finish, finishOK := compiler.input.Flow().Causal().Sites().ForTerm(finishTerm)
	finishOK = finishTermOK && finishOK && compiler.input.OwnsSite(finish)
	predecessor, predecessorOK := compiler.input.Flow().Causal().Successors().AssignmentPredecessor(term)
	row := artifactIndexWrite{term: term, base: base, lens: lens, values: values, position: position, baseSpan: baseSpan, finish: finish, predecessor: predecessor}
	if exactKey, exactOK := geometry.ExactLenses().Get(lens); exactOK {
		row.exact, row.exactKey = true, exactKey
	} else {
		if _, dynamicOK := geometry.DynamicLenses().Get(lens); !dynamicOK {
			return artifactIndexWrite{}, false
		}
		row.dynamicKeySpan, _ = compiler.input.Span(keyTerm)
	}
	identityProof, identityOK := predecessor.Identity()
	predecessorID, route, predecessorIDOK := compiler.input.AssignmentPredecessorID(term)
	routeOK := route.Available()
	finishTerm, finishTermOK = finish.Term()
	portFinish, portFinishOK := compiler.input.Flow().Ports().Finish(term)
	provenance := compiler.input.Flow().Provenance()
	predecessorOK = predecessorOK && finishOK && identityOK && routeOK && predecessorIDOK && finishTermOK && portFinishOK &&
		portFinish == finishTerm && predecessor.To == finishTerm && predecessor.Arm == flow.BoundaryLocal &&
		identityProof.To() == finishTerm && identityProof.Arm() == flow.BoundaryLocal && identityProof.Provenance() == provenance
	row.route = route
	row.id = accessRoleID("program/transformer/index-write", compiler.input, term)
	row.baseID = accessSubroleID("program/transformer/index-base", compiler.input, term, base)
	lensDomain := "program/transformer/index-lens-dynamic"
	if row.exact {
		lensDomain = "program/transformer/index-lens-exact"
	}
	row.lensID = accessSubroleID(lensDomain, compiler.input, term, lens)
	row.valuesID = accessSubroleID("program/transformer/index-values", compiler.input, term, values, uint64(position))
	row.predecessorID = predecessorID
	return row, baseOK && predecessorOK && row.id.Available() && row.baseID.Available() && row.lensID.Available() && row.valuesID.Available() && row.predecessorID.Available()
}

func (compiler *compiler) valuesOccurrence(term keyspace.Term) (ValuesRow, bool) {
	return compiler.valueRowForTerm(term)
}

// roleID and accessRoleID intentionally use the old Program framing while
// the artifact schema is migrated. This preserves published row identity
// across the ownership move; no access row or transformer object is
// retained.
func roleID(domain string, input *program.Program, write func(*framing.Writer) bool) identity.ContentID {
	programID := input.ContentID()
	if !programID.Available() || write == nil {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || writer.Bytes(programID[:]) != nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var result identity.ContentID
	sum := hash.Sum(result[:0])
	if len(sum) != len(result) {
		return identity.ContentID{}
	}
	return result
}

func accessRoleID(domain string, input *program.Program, term keyspace.Term, values ...uint64) identity.ContentID {
	return roleID(domain, input, func(writer *framing.Writer) bool {
		if !writeAccessSemantic(writer, input, term) {
			return false
		}
		for _, value := range values {
			if writer.Uint(value) != nil {
				return false
			}
		}
		return true
	})
}

func accessSubroleID(domain string, input *program.Program, access, term keyspace.Term, values ...uint64) identity.ContentID {
	return roleID(domain, input, func(writer *framing.Writer) bool {
		if !writeAccessSemantic(writer, input, access) || !writeAccessSemantic(writer, input, term) {
			return false
		}
		for _, value := range values {
			if writer.Uint(value) != nil {
				return false
			}
		}
		return true
	})
}

func writeAccessSemantic(writer *framing.Writer, input *program.Program, term keyspace.Term) bool {
	if writer == nil {
		return false
	}
	path, ok := input.Flow().SemanticTermPath(term)
	return ok && path.Available() && writer.Bytes(path[:]) == nil
}
