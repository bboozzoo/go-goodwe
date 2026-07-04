#!/bin/sh
# SPDX-License-Identifier: BSD-3-Clause
#
# Entrypoint for the goodwe Docker image.
#
# If the first argument is "goodwe", exec it directly. For "goodwe-daemon" with extra args, exec directly.
# with the remaining arguments. This allows the container to be used as
# either the CLI tool or the daemon.
#
# Otherwise, build flags from environment variables and run goodwe-daemon.

set -e

case "${1}" in
  goodwe)
    exec "$@"
    ;;
  goodwe-daemon)
    # With extra arguments, run directly (e.g. -offline-analyze-voltage).
    # Without extra arguments (default CMD), fall through to env-based mode.
    if [ $# -gt 1 ]; then
      exec "$@"
    fi
    ;;
esac

# Build daemon flags from environment variables.
# Only set flags for variables that are explicitly configured.

FLAGS=""

if [ -n "${INVERTER_IP}" ]; then
  FLAGS="${FLAGS} -inverterip ${INVERTER_IP}"
fi

if [ -n "${POLL_INTERVAL}" ]; then
  FLAGS="${FLAGS} -poll ${POLL_INTERVAL}"
fi

if [ -n "${DB_STORE}" ]; then
  FLAGS="${FLAGS} -dbstore ${DB_STORE}"
else
  FLAGS="${FLAGS} -dbstore sqlite:///var/lib/goodwe/goodwe.db"
fi

if [ "${DASHBOARD}" = "true" ]; then
  FLAGS="${FLAGS} -dashboard"
fi

if [ "${DEBUG}" = "true" ]; then
  FLAGS="${FLAGS} -debug"
fi

if [ -n "${LISTEN}" ]; then
  FLAGS="${FLAGS} -listen ${LISTEN}"
fi

# shellcheck disable=SC2086
exec /usr/local/bin/goodwe-daemon ${FLAGS} "$@"