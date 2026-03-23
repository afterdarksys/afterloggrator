#!/bin/bash
mkdir -p /tmp/log-test
cat << 'EOF' > /tmp/log-test/auth.log
Jan 12 10:00:01 server sshd[1234]: Accepted publickey for ryan from 192.168.1.100 port 50432 ssh2
Jan 12 10:00:05 server su: pam_unix(su:session): session opened for user root by ryan(uid=1000)
EOF
cat << 'EOF' > /tmp/log-test/iptables.log
Jan 12 09:59:58 server kernel: [ 1234.5678] DROP IN=eth0 OUT= MAC=00:11:22:33:44:55 SRC=192.168.1.100 DST=10.0.0.1 LEN=60 TTL=64 PROTO=TCP SPT=50432 DPT=22
EOF
gzip -c /tmp/log-test/auth.log > /tmp/log-test/auth.log.gz
./afterloggrator -d -app sshd -correlate /tmp/log-test
