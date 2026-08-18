package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/canonical"
)

type field struct {
	bytes []byte
	uint  uint64
	kind  uint8
}

const (
	fieldBytes uint8 = iota + 1
	fieldUint
	fieldBool
)

func bytesField(value identity.ContentID) field { return field{bytes: value[:], kind: fieldBytes} }
func keyField(value schema.Key) field           { return field{bytes: []byte(value), kind: fieldBytes} }
func uintField(value uint64) field              { return field{uint: value, kind: fieldUint} }
func boolField(value bool) field {
	if value {
		return field{uint: 1, kind: fieldBool}
	}
	return field{kind: fieldBool}
}

func digest(domain string, version uint64, fields ...field) identity.ContentID {
	var writer canonical.DigestWriter
	if writer.Reset(domain, version) != nil {
		return identity.ContentID{}
	}
	for _, value := range fields {
		var err error
		switch value.kind {
		case fieldBytes:
			err = writer.Bytes(value.bytes)
		case fieldUint, fieldBool:
			err = writer.Uint(value.uint)
		default:
			return identity.ContentID{}
		}
		if err != nil {
			return identity.ContentID{}
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	return identity.ContentID(writer.Sum())
}
