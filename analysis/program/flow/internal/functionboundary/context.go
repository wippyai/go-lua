package functionboundary

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

const (
	contextDomain      = "wippy/program/flow/function-boundary-context"
	bodyContextDomain  = "wippy/program/flow/body-boundary-context"
	contextVersion     = uint64(1)
	bodyContextVersion = uint64(1)
)

// hashContext is deliberately term-based. It includes the exact owner
// quartet and every ordered existing constituent, never a physical row index,
// so equivalent seal/artifact replay receives the same identity.
func hashContext(result *Result, row functionRow) identity.ContentID {
	hash := sha256.New()
	hash.Write([]byte(contextDomain))
	hash.Write([]byte{0})
	var scalar [8]byte
	binary.BigEndian.PutUint64(scalar[:], contextVersion)
	hash.Write(scalar[:])
	for _, owner := range [...]identity.ContentID{result.sourceID, result.flowID, result.staticID, result.moduleID} {
		hash.Write(owner[:])
	}
	writeTerm(hash, row.function)
	writeTerm(hash, row.owner)
	writeTerm(hash, row.body)
	writeTerm(hash, row.entry)
	writeTerm(hash, row.vararg)
	writeRangeCount(hash, row.formals.end-row.formals.start)
	for index := row.formals.start; index < row.formals.end; index++ {
		writeTerm(hash, result.formals[index])
	}
	writeRangeCount(hash, row.captures.end-row.captures.start)
	for index := row.captures.start; index < row.captures.end; index++ {
		writeTerm(hash, result.captures[index].inner)
		writeTerm(hash, result.captures[index].outer)
	}
	writeRangeCount(hash, row.outcomes.end-row.outcomes.start)
	for index := row.outcomes.start; index < row.outcomes.end; index++ {
		exit := result.outcomes[index]
		writeTerm(hash, exit.term)
		writeTerm(hash, exit.body)
		hash.Write([]byte{byte(exit.kind)})
		writeTerm(hash, exit.target)
	}
	return identity.ContentID(hash.Sum(nil))
}

// hashBodyContext identifies only the existing Body/Entry and its complete
// ordered Outcome range. Function identity is deliberately absent so the root
// Body is represented by the same BodyBoundary shape as every other Body.
func hashBodyContext(result *Result, row bodyRow) identity.ContentID {
	hash := sha256.New()
	hash.Write([]byte(bodyContextDomain))
	hash.Write([]byte{0})
	var scalar [8]byte
	binary.BigEndian.PutUint64(scalar[:], bodyContextVersion)
	hash.Write(scalar[:])
	for _, owner := range [...]identity.ContentID{result.sourceID, result.flowID, result.staticID, result.moduleID} {
		hash.Write(owner[:])
	}
	writeTerm(hash, row.body)
	writeTerm(hash, row.entry)
	writeRangeCount(hash, row.outcomes.end-row.outcomes.start)
	for index := row.outcomes.start; index < row.outcomes.end; index++ {
		exit := result.outcomes[index]
		writeTerm(hash, exit.term)
		writeTerm(hash, exit.body)
		hash.Write([]byte{byte(exit.kind)})
		writeTerm(hash, exit.target)
	}
	return identity.ContentID(hash.Sum(nil))
}

func writeTerm(hash interface{ Write([]byte) (int, error) }, term keyspace.Term) {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], uint32(term))
	hash.Write(encoded[:])
}

func writeRangeCount(hash interface{ Write([]byte) (int, error) }, count uint32) {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], count)
	hash.Write(encoded[:])
}
