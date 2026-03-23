# AfterLoggrator Scriptlets

This directory is intended for **Starlark** (`.star`) scripts. 
Scripts placed here will be loaded automatically on startup if the `plugins-dir` argument points here. 

### Starlark Example
```starlark
def process_log(entry):
    line = entry["line"]
    app_id = entry["app_id"]
    tokens = entry["tokens"] // Now contains pre-extracted IPs and MAC addresses!
    
    # Custom business logic here
```
