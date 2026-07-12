package concrete

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/semantic/transaction"
)

func encodeCanonical(backend *Backend) ([]byte, error) {
	var out encoder
	out.string(Schema)
	out.bytes(backend.primitives.CanonicalBytes())
	out.uint32(len(backend.cellOrder))
	for _, key := range backend.cellOrder {
		binding := backend.cells[key]
		out.uint8(uint8(key.kind))
		out.string(key.capability)
		out.string(key.id)
		out.string(binding.implementationID)
	}
	opcodes := make([]string, 0, len(backend.opcodes))
	for opcode := range backend.opcodes {
		opcodes = append(opcodes, opcode)
	}
	sort.Strings(opcodes)
	out.uint32(len(opcodes))
	for _, opcode := range opcodes {
		binding := backend.opcodes[opcode]
		out.string(opcode)
		out.string(binding.implementationID)
		allowed := make([]transaction.Capability, 0, len(binding.allowed))
		for capability := range binding.allowed {
			allowed = append(allowed, capability)
		}
		sort.Slice(allowed, func(left, right int) bool {
			if allowed[left].ID != allowed[right].ID {
				return allowed[left].ID < allowed[right].ID
			}
			return allowed[left].Kind < allowed[right].Kind
		})
		out.uint32(len(allowed))
		for _, capability := range allowed {
			out.string(capability.ID)
			out.uint8(uint8(capability.Kind))
		}
	}
	if out.err != nil {
		return nil, out.err
	}
	return out.Bytes(), nil
}

type encoder struct {
	bytes.Buffer
	err error
}

func (e *encoder) string(value string) { e.bytes([]byte(value)) }
func (e *encoder) bytes(value []byte) {
	e.uint32(len(value))
	if e.err == nil {
		e.Write(value)
	}
}
func (e *encoder) uint8(value uint8) {
	if e.err == nil {
		e.WriteByte(value)
	}
}
func (e *encoder) uint32(value int) {
	if e.err != nil {
		return
	}
	if value < 0 || uint64(value) > uint64(^uint32(0)) {
		e.err = invalid("canonical encoding", errors.New("length exceeds uint32"))
		return
	}
	var buffer [4]byte
	binary.BigEndian.PutUint32(buffer[:], uint32(value))
	e.Write(buffer[:])
}
