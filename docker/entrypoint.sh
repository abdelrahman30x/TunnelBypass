#!/bin/sh
# TunnelBypass Docker entrypoint
#
# No arguments:
#   Starts the normal interactive CLI (wizard when no subcommand), same as a local terminal.
#   Use: docker run -it --rm -v tbdata:/data ...  (needs -it for menus)
#
# With arguments:
#   - "run ..." → ensures portable data root /data unless you pass --data-dir or --portable
#   - Other subcommands (wizard, status, hosts, ...) → passed through unchanged
#   - A bare transport name (e.g. "ssh", "hysteria") → run --data-dir /data <args>
#
# Data root: set TUNNELBYPASS_DATA_DIR=/data in the image (or use --data-dir on run).
# Non-interactive: tunnelbypass run --data-dir /data --spec /data/spec.yaml <transport>
# Ports: configure via CLI flags; nothing is fixed in the Dockerfile.

set -e
bin=/usr/local/bin/tunnelbypass

if [ "$#" -eq 0 ]; then
	exec "$bin"
fi

case "$1" in
run)
	shift
	for a in "$@"; do
		if [ "$a" = "--data-dir" ] || [ "$a" = "--portable" ]; then
			exec "$bin" run "$@"
		fi
	done
	exec "$bin" run --data-dir /data "$@"
	;;
wizard|hosts|deps-tree|health|status|xray-svc|hysteria-svc|udpgw-svc)
	exec "$bin" "$@"
	;;
-*)
	exec "$bin" "$@"
	;;
*)
	exec "$bin" run --data-dir /data "$@"
	;;
esac
