package link

import (
	"github.com/wippyai/go-lua/program/keyspace"
)

func targetStringKey(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}
