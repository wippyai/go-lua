package corpus

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/internal/relparity"
)

// ErrEmptyCorpus refuses a walk that found nothing to compare.
//
// A driver that reports parity over zero fixtures reports nothing while
// reading as success, which is the one outcome a differential measurement
// must never produce. An empty enumeration is a broken corpus root, and the
// walk says so instead of returning an empty agreement.
var ErrEmptyCorpus = errors.New("corpus: enumeration found no fixture")

// Enumerate lists the frozen fixture corpus under one checkout, in canonical
// order, and refuses an empty result.
//
// The listing is the parity harness's own corpus enumeration, reused rather
// than restated, so the driver and the harness range over one definition of
// what a fixture is.
func Enumerate(checkout string) ([]string, error) {
	fixtures, err := relparity.ListFixtures(checkout)
	if err != nil {
		return nil, err
	}
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("%w: %s/%s", ErrEmptyCorpus, checkout, relparity.FixtureRelativeRoot)
	}
	return fixtures, nil
}

// Select applies one shard of the corpus and refuses an empty selection for
// the same reason Enumerate refuses an empty walk.
func Select(fixtures []string, shard, shards int) ([]string, error) {
	selected, err := relparity.Shard(fixtures, shard, shards)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("%w: shard %d of %d", ErrEmptyCorpus, shard, shards)
	}
	return selected, nil
}

// Digest is the identity of the compared corpus, so a report states which
// fixture list it ranged over.
func Digest(fixtures []string) string { return relparity.FixtureListDigest(fixtures) }
