def process_log(entry):
    tokens = entry["tokens"]
    
    for ip in tokens:
        # Check against Spamhaus Zen EBL
        is_bad = check_rbl(ip, "zen.spamhaus.org")
        
        if is_bad:
            # Perform a reverse lookup to see who it is
            hostname = resolve_ip(ip)
            if hostname == "":
                hostname = "Unknown Host"
                
            print("🛑 BLOCKED IP DETECTED in Logs! IP: " + ip + " (" + hostname + ")")
