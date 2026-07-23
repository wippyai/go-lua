package transformer

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
)

// Fingerprints only select a collision bucket. The corresponding structural
// equality routine remains the authority for every interned term.
const termFingerprintSeed uint64 = 0x7472616e73666f72 // "transfor"

func hashRoot(h uint64, root Root) uint64 {
	h = internalhash.MixHash(h, uint64(root.Kind))
	return internalhash.MixHash(h, uint64(root.Index))
}

func hashValueTerms(h uint64, terms []ValueTerm) uint64 {
	h = internalhash.MixHash(h, uint64(len(terms)))
	for _, term := range terms {
		h = internalhash.MixHash(h, uint64(term))
	}
	return h
}

func hashSegment(h uint64, value segment.Segment) uint64 {
	h = internalhash.MixHash(h, uint64(value.Kind))
	h = internalhash.MixHash(h, internalhash.FnvString(value.Name))
	return internalhash.MixHash(h, uint64(int64(value.Index)))
}

func allocationTemplateFingerprint(siteOwner uint64, siteTemplate string, siteOrdinal uint32) uint64 {
	h := internalhash.MixHash(termFingerprintSeed, 0x616c6c6f63)
	h = internalhash.MixHash(h, siteOwner)
	h = internalhash.MixHash(h, internalhash.FnvString(siteTemplate))
	return internalhash.MixHash(h, uint64(siteOrdinal))
}
