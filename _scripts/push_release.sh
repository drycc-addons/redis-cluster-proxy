#!/usr/bin/env bash

GITHUB_BASE_URL="https://api.github.com/repos/${CI_REPO_OWNER}/${CI_REPO_NAME}"
GITHUB_AUTH_HEAD="Authorization: Bearer ${GITHUB_TOKEN}"

GITHUB_RELEASE_ID=$(curl -L -H "Authorization: Bearer ${GITHUB_TOKEN}" "${GITHUB_BASE_URL}/releases" | jq -r .[0].id)
GITHUB_ASSET_UPLOAD_URL="https://uploads.github.com/repos/${CI_REPO_OWNER}/${CI_REPO_NAME}/releases/${GITHUB_RELEASE_ID}/assets"
curl -s --data-binary @bin/redis-cluster-proxy -H "Content-Type: application/octet-stream" -H "${GITHUB_AUTH_HEAD}" -X POST "${GITHUB_ASSET_UPLOAD_URL}?name=redis-cluster-proxy-${CI_SYSTEM_PLATFORM}"