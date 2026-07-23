local consumer_send = require("consumer_send")
local consumer_local = require("consumer_local")
local consumer_store = require("consumer_store")

consumer_send.run()
local body: string = consumer_local.run()
consumer_store.run()
print(body)
