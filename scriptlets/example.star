def filter(event):
    return event["fields"].get("rbl_listed", "false") == "true"
