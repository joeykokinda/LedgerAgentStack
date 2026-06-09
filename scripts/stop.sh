#!/usr/bin/env bash
# Tear down the demo: stop the Speculos container and any running airlock.
docker rm -f speculos-eth >/dev/null 2>&1 || true
pkill -f 'bin/airlock' >/dev/null 2>&1 || true
pkill -f 'cmd/airlock' >/dev/null 2>&1 || true
echo "stopped Speculos container and airlock"
