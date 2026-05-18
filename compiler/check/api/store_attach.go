package api

import "github.com/wippyai/go-lua/types/db"

// StoreKey is the typed attachment key for StoreReader.
var StoreKey = db.NewAttachmentKey[StoreReader]("check.StoreReader")

// AttachStore attaches a store to the query context for lookup.
func AttachStore(ctx *db.QueryContext, store StoreReader) {
	if ctx == nil || store == nil {
		return
	}
	db.Attach(ctx, StoreKey, store)
}

// StoreFrom retrieves the StoreReader from a db.QueryContext.
func StoreFrom(ctx *db.QueryContext) StoreReader {
	store, _ := db.Attached(ctx, StoreKey)
	return store
}
