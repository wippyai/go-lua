package program

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func encodeCanonical(p Program) ([]byte, error) {
	var e encoder
	e.str(Schema)
	e.u32(uint32(p.entry))
	e.members(p.members)
	e.u32(uint32(len(p.transactions)))
	for _, tx := range p.transactions {
		e.Write(tx.Digest[:])
	}
	e.u32(uint32(len(p.blocks)))
	for _, b := range p.blocks {
		e.u32(uint32(b.ID))
		e.str(string(b.Member))
		e.u32(uint32(len(b.Transactions)))
		for _, tx := range b.Transactions {
			e.Write(tx.Digest[:])
		}
	}
	e.u32(uint32(len(p.edges)))
	for _, edge := range p.edges {
		e.u32(uint32(edge.From))
		e.u32(uint32(edge.To))
		e.str(string(edge.Guard))
	}
	e.u32(uint32(len(p.observations)))
	for _, o := range p.observations {
		e.u32(uint32(o.ID))
		e.u32(uint32(o.At))
		e.WriteByte(byte(o.Kind))
		e.str(o.Schema)
	}
	e.u32(uint32(p.callSCC.ID))
	e.members(p.callSCC.Members)
	e.u32(uint32(len(p.loops)))
	for _, loop := range p.loops {
		e.u32(uint32(loop.ID))
		e.u32(uint32(loop.SCC))
		e.u32(uint32(loop.Parent))
		e.str(string(loop.Owner))
		e.u32(uint32(loop.Entry))
		e.blocks(loop.Blocks)
	}
	e.u32(uint32(len(p.routes)))
	for _, route := range p.routes {
		e.u32(uint32(route.At))
		e.u32(uint32(len(route.Known)))
		for _, target := range route.Known {
			e.str(string(target.Guard))
			e.str(string(target.Member))
		}
		if route.Residue.Unknown {
			e.WriteByte(1)
		} else {
			e.WriteByte(0)
		}
		if route.Residue.Native {
			e.WriteByte(1)
		} else {
			e.WriteByte(0)
		}
		e.WriteByte(byte(route.Completeness))
		e.Write(route.Proof[:])
	}
	if e.err != nil {
		return nil, e.err
	}
	return e.Bytes(), nil
}

type encoder struct {
	bytes.Buffer
	err error
}

func (e *encoder) u32(v uint32) { var b [4]byte; binary.BigEndian.PutUint32(b[:], v); e.Write(b[:]) }
func (e *encoder) str(v string) {
	if uint64(len(v)) > uint64(^uint32(0)) {
		e.err = fmt.Errorf("%w: codec: string too large", ErrInvalid)
		return
	}
	e.u32(uint32(len(v)))
	e.WriteString(v)
}
func (e *encoder) members(v []MemberID) {
	e.u32(uint32(len(v)))
	for _, x := range v {
		e.str(string(x))
	}
}
func (e *encoder) blocks(v []BlockID) {
	e.u32(uint32(len(v)))
	for _, x := range v {
		e.u32(uint32(x))
	}
}
