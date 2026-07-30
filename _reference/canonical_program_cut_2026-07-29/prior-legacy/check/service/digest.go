package service

import (
	"github.com/wippyai/go-lua/analysis/embedding"
)

// Digest is the embedding surface's stable SHA-256 content digest.
type Digest = embedding.Digest

func digestBytes(data []byte) Digest { return embedding.DigestBytes(data) }
