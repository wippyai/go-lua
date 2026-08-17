package source

import "github.com/wippyai/go-lua/domain/value"

func sourceSeedContent(seed value.SourceSeed) (value.SourceSeed, [32]byte, bool) {
	id, ok := seed.ID()
	return seed, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

func sourceResultForSchema(schema *value.Schema, seed value.SourceSeed) (value.Coordinate, value.Value, bool) {
	if schema == nil {
		return value.Coordinate{}, value.Value{}, false
	}
	coordinate, fact, ok := seed.Result()
	if !ok || !schema.AdmitsCoordinate(coordinate, fact) || schema.Equal(fact, schema.Default()) {
		return value.Coordinate{}, value.Value{}, false
	}
	return coordinate, fact, true
}
