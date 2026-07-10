type Animal = { name: string }
type Dog = { name: string, breed: string }
local dogs: {Dog} = { { name = "rex", breed = "lab" } }
local animals: {Animal} = dogs
animals[1] = { name = "cat" }
local b: string = dogs[1].breed
return b
