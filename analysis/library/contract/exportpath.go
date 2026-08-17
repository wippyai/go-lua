package contract

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/internal/framing"
)

// The export-path payload format. It is the one member payload encoding this
// package owns, and it owns it because the payload is nothing but an address:
// a metatable edge says that one metatable key resolves to one exported value,
// and the value is named by a path from the contract root exactly as every
// member address is. A form whose payload carries a TYPE - a callable
// signature, an effect row, a refinement - is owned by the layer that owns the
// type; this package holds such a payload as opaque bytes and never learns its
// shape. The callable envelope is the first of those formats to land, and it
// landed with its owner: an instance author writes it, and a member whose
// owning format has still not landed is published deferred.
const (
	pathDomain  = "analysis/library/contract/export-path"
	pathVersion = 1
)

// EncodePath writes one export path as a member payload body. The result is a
// complete framed stream: a payload is decodable on its own, so a reader that
// holds a member and its declared format never needs the enclosing instance to
// interpret it.
func EncodePath(path Path) ([]byte, error) {
	if !path.Available() {
		return nil, ErrMalformed
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, pathDomain, pathVersion); err != nil {
		return nil, err
	}
	if err := writer.Count(uint64(path.Len())); err != nil {
		return nil, err
	}
	for _, step := range path.steps {
		if err := writer.Record(recordStep); err != nil {
			return nil, err
		}
		if err := writer.Uint(uint64(step.Kind)); err != nil {
			return nil, err
		}
		if err := writer.String(step.Key); err != nil {
			return nil, err
		}
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DecodePath reads one export-path payload body.
func DecodePath(data []byte) (Path, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return Path{}, ErrMalformed
	}
	if err := reader.Header(pathDomain, pathVersion); err != nil {
		return Path{}, ErrMalformed
	}
	count, err := reader.Count()
	if err != nil || count > maxSteps {
		return Path{}, ErrMalformed
	}
	steps := make([]Step, 0, count)
	for index := uint64(0); index < count; index++ {
		if record, err := reader.Record(); err != nil || record != recordStep {
			return Path{}, ErrMalformed
		}
		kind, err := reader.Uint()
		if err != nil || kind > uint64(^uint8(0)) {
			return Path{}, ErrMalformed
		}
		key, err := reader.String(maxKey)
		if err != nil {
			return Path{}, ErrMalformed
		}
		steps = append(steps, Step{Kind: StepKind(kind), Key: key})
	}
	if err := reader.Finish(); err != nil {
		return Path{}, ErrMalformed
	}
	path := NewPath(steps...)
	if !path.Available() {
		return Path{}, ErrMalformed
	}
	return path, nil
}
