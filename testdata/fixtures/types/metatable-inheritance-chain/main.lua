type Animal = { name: string, speak: (self: Animal) -> string }
type Dog = { name: string, speak: (self: Dog) -> string, fetch: (self: Dog) -> string }

local Animal = {}
Animal.__index = Animal
function Animal.speak(self: Animal): string return self.name end

local Dog = setmetatable({}, { __index = Animal })
Dog.__index = Dog
function Dog.fetch(self: Dog): string return self.name .. " fetches" end

local function new_dog(name: string): Dog
    local self: Dog = { name = name, speak = Animal.speak, fetch = Dog.fetch }
    return setmetatable(self, Dog)
end

local d = new_dog("rex")
return d:speak() .. d:fetch()
