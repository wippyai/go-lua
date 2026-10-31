type Row = {id: string}
type Executor = {query: (self: Executor) -> ({Row}?, string?)}
type DB = {release: (self: DB) -> (), query: (self: DB, string) -> ({Row}?, string?)}
type Builder = {
    from: (self: Builder, string) -> Builder,
    where: (self: Builder, string, any) -> Builder,
    run_with: (self: Builder, DB) -> Executor,
}
type SQL = {builder: {select: (string) -> Builder}}
return {} :: SQL
