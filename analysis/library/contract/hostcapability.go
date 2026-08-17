package contract

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/internal/framing"
)

// The host-capability payload format. It is owned here for the reason the rule
// delegation is: the payload is nothing but IDENTITIES. A capability's meaning,
// its family and whether it is active belong to the vocabulary that audits it;
// this format names the ones a host grants the environment it boots, so a
// contract published into that environment may exercise them and no reader has to
// reconstruct the grant from the labels it happens to observe.
//
// The grant is the outermost authority a contract can reference, which is why
// only the environment class declares the form: an individual library that could
// grant itself a capability would be its own host. The grant is published as one
// member at the contract root, because it is a fact about the environment as a
// whole and there is no exported value a capability attaches to.
const (
	hostCapabilityDomain  = "analysis/library/contract/host-capability"
	hostCapabilityVersion = 1
	// maxCapabilities bounds the decoder's reservation. The audited vocabulary
	// is small; the bound only keeps a hostile count finite.
	maxCapabilities = 1 << 12
)

// EncodeHostCapabilities writes one host grant as a member payload body. The
// identities are written in ascending order and each appears once: a set written
// two ways would publish two identities for one environment, and a capability
// granted twice is granted once.
func EncodeHostCapabilities(granted []string) ([]byte, error) {
	if len(granted) == 0 {
		return nil, ErrMalformed
	}
	for index, capability := range granted {
		if capability == "" {
			return nil, ErrMalformed
		}
		if index != 0 && granted[index-1] >= capability {
			return nil, ErrMalformed
		}
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, hostCapabilityDomain, hostCapabilityVersion); err != nil {
		return nil, err
	}
	if err := writer.Count(uint64(len(granted))); err != nil {
		return nil, err
	}
	for _, capability := range granted {
		if err := writer.Record(recordCapability); err != nil {
			return nil, err
		}
		if err := writer.String(capability); err != nil {
			return nil, err
		}
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DecodeHostCapabilities reads one host-capability payload body.
func DecodeHostCapabilities(data []byte) ([]string, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return nil, ErrMalformed
	}
	if err := reader.Header(hostCapabilityDomain, hostCapabilityVersion); err != nil {
		return nil, ErrMalformed
	}
	count, err := reader.Count()
	if err != nil || count == 0 || count > maxCapabilities {
		return nil, ErrMalformed
	}
	granted := make([]string, 0, count)
	for index := uint64(0); index < count; index++ {
		if record, err := reader.Record(); err != nil || record != recordCapability {
			return nil, ErrMalformed
		}
		capability, err := reader.String(maxKey)
		if err != nil || capability == "" {
			return nil, ErrMalformed
		}
		if index != 0 && granted[index-1] >= capability {
			return nil, ErrMalformed
		}
		granted = append(granted, capability)
	}
	if err := reader.Finish(); err != nil {
		return nil, ErrMalformed
	}
	return granted, nil
}
