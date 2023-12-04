#!/bin/bash

if [ $# -eq 0 ]; then
    echo "Error: urls must be set."
    exit 1
fi

BASE_DIR="easyp_volume"
mkdir -p "$BASE_DIR"

clone_repo() {
    REPO_URL=$1
    IFS='/'
    read -ra ADDR <<< "$REPO_URL"
    USER=${ADDR[3]}
    REPO=${ADDR[4]%.git}

    DIR_PATH="$BASE_DIR/$USER/$REPO"

    git clone "$REPO_URL" "$DIR_PATH"
}

for REPO_URL in "$@"; do
    clone_repo "$REPO_URL"
done

echo "Cloning is complete."
