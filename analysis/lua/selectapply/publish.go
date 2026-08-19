package selectapply

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/type/channelselect"
)

var errIncompleteFacts = errors.New("selectapply: incomplete case facts")

// Content is the sparse Snapshot payload of accepted select-case facts.
// Lookalike ordinals are omitted. An empty application list is an empty
// column, not a proven universe.
func Content(apps []Application) (snapshot.Content[identity.ContentID, channelselect.CaseFact], bool) {
	rows := make(map[identity.ContentID]channelselect.CaseFact)
	for _, app := range apps {
		for _, fact := range app.Facts.All() {
			id, ok := channelselect.CaseFactID(fact)
			if !ok {
				return snapshot.Content[identity.ContentID, channelselect.CaseFact]{}, false
			}
			if _, duplicate := rows[id]; duplicate {
				return snapshot.Content[identity.ContentID, channelselect.CaseFact]{}, false
			}
			rows[id] = fact
		}
	}
	return snapshot.Content[identity.ContentID, channelselect.CaseFact]{Rows: rows}, true
}

// Publish writes accepted select-case facts through the minted column
// capability. A zero capability unlocks no column.
func Publish(write engine.ColumnWrite[identity.ContentID, channelselect.CaseFact], builder *snapshot.Builder, apps []Application) error {
	content, ok := Content(apps)
	if !ok {
		return errIncompleteFacts
	}
	return engine.PublishColumn(write, builder, content)
}
