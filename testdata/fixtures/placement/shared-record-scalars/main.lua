local record = {
    id = "shared",
    count = 1,
    ok = true,
}

process.send("worker-1", "record.ready", record)
