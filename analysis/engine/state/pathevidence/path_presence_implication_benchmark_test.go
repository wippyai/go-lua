package pathevidence

import (
	"strconv"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

var benchmarkPathPresenceLane Lane

func BenchmarkAddPathPresenceImplications(b *testing.B) {
	keys := keyspace.New()
	trigger := benchmarkPathPresenceKey(b, keys, "trigger@1")
	rows := make([]PathPresenceImplication, 32)
	for index := range rows {
		target := benchmarkPathPresenceKey(b, keys, pathdom.PathKey("target"+strconv.Itoa(index)+"@1"))
		rows[index] = NewPathPresenceImplication(trigger, presence.Present(), target, presence.Present())
	}
	b.Run("repeated", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			var lane Lane
			for _, row := range rows {
				lane, _ = lane.AddPathPresenceImplication(row)
			}
			benchmarkPathPresenceLane = lane
		}
	})
	b.Run("batched", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			benchmarkPathPresenceLane, _ = (Lane{}).AddPathPresenceImplications(rows)
		}
	})
}

func benchmarkPathPresenceKey(b *testing.B, keys *keyspace.KeySpace, raw pathdom.PathKey) keyspace.Key {
	b.Helper()
	key, ok := keys.FromStateKey(raw)
	if !ok {
		b.Fatalf("FromStateKey(%q) failed", raw)
	}
	return key
}
