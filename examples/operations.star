load("common.star", "server_error")

def filter(event):
    return event["cluster"] in ["prod-us", "prod-eu"] and server_error(event["fields"].get("status", ""))
