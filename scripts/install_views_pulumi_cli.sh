#!/usr/bin/env bash

set -euo pipefail

PULUMI_CLI_VERSION="pr#19821"
DEST=".pulumi"

if ! [ -x "$DEST/bin/pulumi" ]; then
    echo "Installing pulumi ${PULUMI_CLI_VERSION} to ${DEST}"
    sh <(curl -fsSL https://get.pulumi.com) --version "$PULUMI_CLI_VERSION" --install-root "$DEST" --no-edit-path
fi
