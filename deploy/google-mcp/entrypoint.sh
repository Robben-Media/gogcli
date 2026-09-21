#!/bin/sh
set -eu
# Read at startup so the password is not embedded in the image or Compose environment.
if [ -z "${GOG_KEYRING_PASSWORD:-}" ]; then
    password_file=${GOG_KEYRING_PASSWORD_FILE:-/run/secrets/keyring-password}
    if [ ! -r "$password_file" ]; then
        echo 'A readable keyring password secret is required.' >&2
        exit 1
    fi
    GOG_KEYRING_PASSWORD=$(cat "$password_file")
    if [ -z "$GOG_KEYRING_PASSWORD" ]; then
        echo 'The keyring password secret must not be empty.' >&2
        exit 1
    fi
    export GOG_KEYRING_PASSWORD
fi
# The HTTP service and onboarding share one state owner.
# --no-fork keeps the selected binary as PID 1 while retaining the lock.
exec flock --exclusive --nonblock --no-fork "${XDG_CONFIG_HOME:-/state}/.gog-mcp.lock" "$@"
