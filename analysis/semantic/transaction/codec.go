package transaction

import (
	"bytes"
	"encoding/binary"
	"errors"
)

func encodeCanonical(transaction FrozenTransaction) ([]byte, error) {
	var out canonicalEncoder
	out.writeString(Schema)
	out.writeUint32(len(transaction.capabilities))
	for _, capability := range transaction.capabilities {
		out.writeString(capability.ID)
		out.WriteByte(byte(capability.Kind))
	}
	out.writeUint32(len(transaction.slots))
	for _, slot := range transaction.slots {
		out.writeString(slot.Capability)
		out.writeString(slot.ID)
		out.WriteByte(byte(slot.Kind))
	}
	out.writeUint32(len(transaction.overlays))
	for _, overlay := range transaction.overlays {
		out.writeString(overlay.id)
		out.Write([]byte{
			byte(overlay.policy.normal),
			byte(overlay.policy.raised),
			byte(overlay.policy.suspended),
			byte(overlay.policy.nonreturning),
		})
		out.writeUint32(len(overlay.operations))
		for _, operation := range overlay.operations {
			out.writeUint32(int(operation.target))
			out.writeString(operation.opcode)
			out.writeBytes(operation.payload)
		}
	}
	if out.err != nil {
		return nil, out.err
	}
	return out.Bytes(), nil
}

type canonicalEncoder struct {
	bytes.Buffer
	err error
}

func (e *canonicalEncoder) writeString(value string) {
	e.writeBytes([]byte(value))
}

func (e *canonicalEncoder) writeBytes(value []byte) {
	e.writeUint32(len(value))
	if e.err == nil {
		e.Write(value)
	}
}

func (e *canonicalEncoder) writeUint32(value int) {
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
