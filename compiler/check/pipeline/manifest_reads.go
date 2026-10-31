package pipeline

import (
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/types/io"
)

// trackedManifests keeps manifest inputs in the same read set as snapshot
// facts. The database is immutable for the duration of one Check call.
type trackedManifests struct {
	io.ManifestQuerier
	store *store.SessionStore
}

func (m trackedManifests) Manifest(path string) *io.Manifest {
	m.store.RecordManifestRead(path)
	return m.ManifestQuerier.Manifest(path)
}

func (m trackedManifests) Imports() map[string]*io.Manifest {
	m.store.RecordManifestRead("*")
	return m.ManifestQuerier.Imports()
}
