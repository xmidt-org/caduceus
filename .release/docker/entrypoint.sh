# SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
# SPDX-License-Identifier: Apache-2.0
#!/usr/bin/env sh
set -e

# check arguments for an option that would cause /caduceus to stop
# return true if there is one
_want_help() {
    local arg
    for arg; do
        case "$arg" in
            -'?'|--help|-v)
                return 0
                ;;
        esac
    done
    return 1
}

_main() {
    # if command starts with an option, prepend caduceus
    if [ "${1:0:1}" = '-' ]; then
        set -- /caduceus "$@"
    fi

    # skip setup if they aren't running /caduceus or want an option that stops /caduceus
    if [ "$1" = '/caduceus' ] && ! _want_help "$@"; then
        echo "Entrypoint script for caduceus Server ${VERSION} started."

        if [ ! -s /etc/caduceus/caduceus.yaml ]; then
            echo "Building out template for file"
            /bin/spruce merge /tmp/caduceus_spruce.yaml > /etc/caduceus/caduceus.yaml
        fi
    fi

    exec "$@"
}

_main "$@"
