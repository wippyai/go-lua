package primitive

import (
	"bytes"
	"encoding/binary"
	"errors"
)

func encodeCanonical(registry Registry) ([]byte, error) {
	var out encoder
	out.string(Schema)
	out.uint32(len(registry.intrinsics))
	for _, descriptor := range registry.intrinsics {
		out.string(descriptor.ID)
		out.uint16(descriptor.SchemaVersion)
		binding := registry.bindings[descriptor.ID]
		out.string(binding.implementationID)
	}
	out.uint32(len(registry.programs))
	for _, program := range registry.programs {
		out.string(program.ID)
		out.uint16(program.SchemaVersion)
		out.uint32(len(program.Steps))
		for _, step := range program.Steps {
			out.WriteByte(byte(step.kind))
			switch step.kind {
			case StepTransaction:
				out.bytes(step.transaction.CanonicalBytes())
			case StepIntrinsicCall:
				out.string(step.intrinsic.ID)
				out.uint16(step.intrinsic.SchemaVersion)
				out.bytes(step.intrinsic.Payload)
			}
		}
	}
	out.uint32(len(registry.coverage))
	for _, row := range registry.coverage {
		out.string(row.ProgramID)
		out.string(row.LeafID)
		out.WriteByte(byte(row.Role))
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

func (e *encoder) uint16(value uint16) {
	if e.err != nil {
		return
	}
	var buffer [2]byte
	binary.BigEndian.PutUint16(buffer[:], value)
	e.Write(buffer[:])
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
