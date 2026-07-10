package judgment

// JIRSchemaVersion pins the judgment IR surface consumed by external runtime
// projectors. It covers the default judgment code registry and the exported
// judgment record shape. Bump it when a code is added, removed, renamed, or
// reclassified, or when the exported judgment/evidence/subject record fields
// change meaning or shape.
const JIRSchemaVersion = 10
