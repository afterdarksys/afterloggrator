#!/bin/bash
# Mock script that reads JSON from stdin and echoes an ML anomaly warning for ssh.
while read line; do
  if echo "$line" | grep -qi "DROP"; then
    echo '{"plugin": "ML-Detector", "msg": "High Anomaly Score (0.95) on firewall drop!", "color": "red"}'
  elif echo "$line" | grep -qi "Accepted"; then
    echo '{"plugin": "ML-Detector", "msg": "Standard Login (Score 0.01)", "color": "green"}'
  fi
done
