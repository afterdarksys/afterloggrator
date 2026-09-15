def server_error(status):
    return status in ["500", "502", "503", "504"]
