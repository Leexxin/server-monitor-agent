#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "sma installation must be run as root" >&2
    exit 1
fi

exit 0
