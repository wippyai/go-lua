-- Runtime type validators used by tests

type Point = { x: number, y: number }
type User = { id: string, name: string }
type OptionalAge = { age?: number }
type LocalConfig = { host: string, port: number }

return {
	Point = Point,
	User = User,
	OptionalAge = OptionalAge,
	LocalConfig = LocalConfig
}
