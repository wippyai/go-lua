package semanticpath

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func digestOutcome(bodyPath identity.ContentID, outcomeKind uint32, targetPath identity.ContentID) identity.ContentID {
	var payload [32 + 32 + 4 + 32]byte
	copy(payload[:], []byte("wippy/program/flow/semantic-outcome-path-v2"))
	offset := 32
	copy(payload[offset:], bodyPath[:])
	offset += 32
	binary.BigEndian.PutUint32(payload[offset:], outcomeKind)
	offset += 4
	copy(payload[offset:], targetPath[:])
	return sha256.Sum256(payload[:])
}

func digestPath(label string, parent identity.ContentID, role, aux uint32, span source.Span) identity.ContentID {
	var payload [64 + 8 + 20]byte
	copy(payload[:], label)
	copy(payload[32:64], parent[:])
	binary.BigEndian.PutUint32(payload[64:68], role)
	binary.BigEndian.PutUint32(payload[68:72], aux)
	binary.BigEndian.PutUint32(payload[72:76], span.StartLine)
	binary.BigEndian.PutUint32(payload[76:80], span.StartCol)
	binary.BigEndian.PutUint32(payload[80:84], span.EndLine)
	binary.BigEndian.PutUint32(payload[84:88], span.EndCol)
	return sha256.Sum256(payload[:])
}

func digestPath3(label string, parent identity.ContentID, role, aux, extra uint32, span source.Span) identity.ContentID {
	var payload [64 + 12 + 20]byte
	copy(payload[:], label)
	copy(payload[32:64], parent[:])
	binary.BigEndian.PutUint32(payload[64:68], role)
	binary.BigEndian.PutUint32(payload[68:72], aux)
	binary.BigEndian.PutUint32(payload[72:76], extra)
	binary.BigEndian.PutUint32(payload[76:80], span.StartLine)
	binary.BigEndian.PutUint32(payload[80:84], span.StartCol)
	binary.BigEndian.PutUint32(payload[84:88], span.EndLine)
	binary.BigEndian.PutUint32(payload[88:92], span.EndCol)
	return sha256.Sum256(payload[:])
}

func digestBytes(label string, parent, value identity.ContentID) identity.ContentID {
	var payload [96]byte
	copy(payload[:], label)
	copy(payload[32:64], parent[:])
	copy(payload[64:96], value[:])
	return sha256.Sum256(payload[:])
}
