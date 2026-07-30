package embedding

// EmbeddingSchemaVersion pins the identity/snapshot DTOs consumed by hosts.
// It is independent of Judgment IR because a host can materialize and query
// checker inputs without serializing JIR.
const EmbeddingSchemaVersion = 1
