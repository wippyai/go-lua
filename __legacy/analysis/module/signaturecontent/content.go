// Package signaturecontent derives immutable semantic identities for function
// signatures from the canonical module-boundary codecs.
package signaturecontent

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"

	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signature/wire"
)

const identityDomain = "go-lua.signature.content-id/v1"
const allocationDomain = "go-lua.signature.allocation-content-id/v1"

// Derive returns the full semantic content identity of sig.
func Derive(ctx context.Context, sig signature.Function) (signature.ContentID, error) {
	canonical, err := wire.CanonicalFunctionSignatureBytesContext(ctx, sig)
	if err != nil {
		return signature.ContentID{}, err
	}
	return framedDigest(ctx, identityDomain, canonical)
}

// DeriveAllocationTemplates returns the separately composable identity of the
// allocation-template lane used by call materialization. Absence is represented
// by the zero identity; callers must not mistake it for a derived artifact.
func DeriveAllocationTemplates(ctx context.Context, sig signature.Function) (signature.ContentID, error) {
	if sig.OperationalEffects == nil || len(sig.OperationalEffects.ReturnAllocationTemplates) == 0 {
		return signature.ContentID{}, nil
	}
	canonical, err := manifest.CanonicalAllocationTemplatesBytesContext(ctx, sig.OperationalEffects.ReturnAllocationTemplates)
	if err != nil {
		return signature.ContentID{}, err
	}
	return framedDigest(ctx, allocationDomain, canonical)
}

func framedDigest(ctx context.Context, domain string, payload []byte) (signature.ContentID, error) {
	if err := ctx.Err(); err != nil {
		return signature.ContentID{}, err
	}
	h := sha256.New()
	if err := writeFrame(h, []byte(domain)); err != nil {
		return signature.ContentID{}, err
	}
	if err := writeFrame(h, payload); err != nil {
		return signature.ContentID{}, err
	}
	if err := ctx.Err(); err != nil {
		return signature.ContentID{}, err
	}
	var id signature.ContentID
	copy(id[:], h.Sum(nil))
	return id, nil
}

func writeFrame(dst hash.Hash, value []byte) error {
	if uint64(len(value)) > uint64(^uint32(0)) {
		return errors.New("signature content frame exceeds uint32")
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	_, _ = dst.Write(length[:])
	_, _ = dst.Write(value)
	return nil
}
