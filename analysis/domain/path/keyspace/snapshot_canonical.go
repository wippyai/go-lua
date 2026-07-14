package keyspace

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const (
	canonicalSnapshotRecord uint64 = 1
	canonicalKeyRecord      uint64 = 2
	canonicalSegmentRecord  uint64 = 3
)

// EncodeCanonical writes the exact solve-independent structural identity of s.
// The caller owns the canonical.Writer session and gains authority only after a
// successful Finish; this method publishes no digest or artifact on its own.
func (s Snapshot) EncodeCanonical(writer *canonical.Writer) error {
	if !s.valid {
		return fmt.Errorf("keyspace: cannot encode unsealed snapshot")
	}
	if err := writer.Record(canonicalSnapshotRecord); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(s.keys))); err != nil {
		return err
	}
	for _, key := range s.keys {
		if err := writer.Record(canonicalKeyRecord); err != nil {
			return err
		}
		if err := writer.Uint(uint64(key.kind)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(key.sym)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(key.version)); err != nil {
			return err
		}
		if err := writer.Bool(key.canon); err != nil {
			return err
		}
		if err := writer.Uint(uint64(key.root)); err != nil {
			return err
		}
		if err := writer.String(key.namedRoot); err != nil {
			return err
		}
		if err := writer.Count(uint64(len(key.segments))); err != nil {
			return err
		}
		for _, item := range key.segments {
			if err := writer.Record(canonicalSegmentRecord); err != nil {
				return err
			}
			if err := writer.Uint(uint64(item.Kind)); err != nil {
				return err
			}
			if err := writer.String(item.Name); err != nil {
				return err
			}
			if err := writer.Int(int64(item.Index)); err != nil {
				return err
			}
		}
	}
	return nil
}
