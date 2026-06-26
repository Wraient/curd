#!/usr/bin/env bash

set -euo pipefail

AUR_REPO="${AUR_REPO:-$HOME/Projects/aur/curd}"
UPSTREAM_REPO="${UPSTREAM_REPO:-Wraient/curd}"
ASSET_NAME="${ASSET_NAME:-curd-linux-x86_64}"
REMOTE="${REMOTE:-origin}"
DRY_RUN="false"

usage() {
  cat <<'EOF'
Usage: Build/update-aur.sh [--dry-run]

Environment overrides:
  AUR_REPO       AUR git clone path (default: ~/Projects/aur/curd)
  UPSTREAM_REPO  GitHub repo in owner/name format (default: Wraient/curd)
  ASSET_NAME     Release asset to checksum (default: curd-linux-x86_64)
  REMOTE         Git remote to push (default: origin)
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

for arg in "$@"; do
  case "$arg" in
    --dry-run)
      DRY_RUN="true"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      usage
      exit 1
      ;;
  esac
done

require_cmd curl
require_cmd jq
require_cmd git
require_cmd sed
require_cmd sha256sum

if [[ ! -d "$AUR_REPO/.git" ]]; then
  echo "AUR repo not found at $AUR_REPO" >&2
  exit 1
fi

if [[ "$DRY_RUN" == "true" ]]; then
  echo "Dry run: skipping AUR repo pull."
else
  echo "Syncing AUR repo with upstream..."
  git -C "$AUR_REPO" pull --ff-only "$REMOTE"
fi

PKGBUILD_PATH="$AUR_REPO/PKGBUILD"
if [[ ! -f "$PKGBUILD_PATH" ]]; then
  echo "PKGBUILD not found at $PKGBUILD_PATH" >&2
  exit 1
fi

release_api="https://api.github.com/repos/${UPSTREAM_REPO}/releases/latest"
release_json="$(curl -fsSL "$release_api")"

tag_name="$(printf '%s' "$release_json" | jq -r '.tag_name')"
if [[ -z "$tag_name" || "$tag_name" == "null" ]]; then
  echo "Could not determine latest release tag from $release_api" >&2
  exit 1
fi

pkgver="${tag_name#v}"
download_url="$(printf '%s' "$release_json" | jq -r --arg n "$ASSET_NAME" '.assets[] | select(.name == $n) | .browser_download_url' | head -n1)"
if [[ -z "$download_url" || "$download_url" == "null" ]]; then
  echo "Asset $ASSET_NAME not found in release $tag_name" >&2
  exit 1
fi

digest="$(printf '%s' "$release_json" | jq -r --arg n "$ASSET_NAME" '.assets[] | select(.name == $n) | (.digest // empty)' | head -n1)"
if [[ "$digest" == sha256:* ]]; then
  sha256="${digest#sha256:}"
else
  tmp_file="$(mktemp)"
  trap 'rm -f "$tmp_file"' EXIT
  curl -fL "$download_url" -o "$tmp_file"
  sha256="$(sha256sum "$tmp_file" | awk '{print $1}')"
  rm -f "$tmp_file"
  trap - EXIT
fi

source_line="source=(\"https://github.com/${UPSTREAM_REPO}/releases/download/v\${pkgver}/${ASSET_NAME}\")"
sha_line="sha256sums=('${sha256}')"

PKGBUILD_BACKUP="$(mktemp)"
SRCINFO_PATH="$AUR_REPO/.SRCINFO"
SRCINFO_BACKUP=""
cp "$PKGBUILD_PATH" "$PKGBUILD_BACKUP"
if [[ -f "$SRCINFO_PATH" ]]; then
  SRCINFO_BACKUP="$(mktemp)"
  cp "$SRCINFO_PATH" "$SRCINFO_BACKUP"
fi

restore_dry_run_files() {
  if [[ "$DRY_RUN" == "true" ]]; then
    cp "$PKGBUILD_BACKUP" "$PKGBUILD_PATH"
    if [[ -n "$SRCINFO_BACKUP" ]]; then
      cp "$SRCINFO_BACKUP" "$SRCINFO_PATH"
    else
      rm -f "$SRCINFO_PATH"
    fi
  fi
  rm -f "$PKGBUILD_BACKUP"
  if [[ -n "$SRCINFO_BACKUP" ]]; then
    rm -f "$SRCINFO_BACKUP"
  fi
}
trap restore_dry_run_files EXIT

sed -i -E "s/^pkgver=.*/pkgver=${pkgver}/" "$PKGBUILD_PATH"

if grep -q '^source=' "$PKGBUILD_PATH"; then
  sed -i -E "s|^source=.*$|${source_line}|" "$PKGBUILD_PATH"
else
  if grep -q '^conflicts=' "$PKGBUILD_PATH"; then
    sed -i "/^conflicts=.*/a ${source_line}" "$PKGBUILD_PATH"
  else
    printf '\n%s\n' "$source_line" >> "$PKGBUILD_PATH"
  fi
fi

if grep -q '^sha256sums=' "$PKGBUILD_PATH"; then
  sed -i -E "s|^sha256sums=.*$|${sha_line}|" "$PKGBUILD_PATH"
else
  if grep -q '^source=' "$PKGBUILD_PATH"; then
    sed -i "/^source=.*/a ${sha_line}" "$PKGBUILD_PATH"
  else
    printf '%s\n' "$sha_line" >> "$PKGBUILD_PATH"
  fi
fi

if command -v makepkg >/dev/null 2>&1; then
  (cd "$AUR_REPO" && makepkg --printsrcinfo > .SRCINFO)
else
  echo "Warning: makepkg not found; skipping .SRCINFO refresh" >&2
fi

if cmp -s "$PKGBUILD_PATH" "$PKGBUILD_BACKUP" && [[ ! -f "$AUR_REPO/.SRCINFO" || -z "$(git -C "$AUR_REPO" status --porcelain -- .SRCINFO)" ]]; then
  echo "No PKGBUILD/.SRCINFO changes detected. Already up to date."
  exit 0
fi

echo "Updated to ${tag_name}"
echo "  pkgver: ${pkgver}"
echo "  asset:  ${ASSET_NAME}"
echo "  sha256: ${sha256}"

if [[ "$DRY_RUN" == "true" ]]; then
  git -C "$AUR_REPO" --no-pager diff -- PKGBUILD .SRCINFO || true
  exit 0
fi

trap - EXIT
rm -f "$PKGBUILD_BACKUP"
if [[ -n "$SRCINFO_BACKUP" ]]; then
  rm -f "$SRCINFO_BACKUP"
fi

git -C "$AUR_REPO" add PKGBUILD
if [[ -f "$AUR_REPO/.SRCINFO" ]]; then
  git -C "$AUR_REPO" add .SRCINFO
fi

if git -C "$AUR_REPO" diff --cached --quiet; then
  echo "No staged changes to commit."
  exit 0
fi

git -C "$AUR_REPO" commit -m "Update to ${tag_name}"
git -C "$AUR_REPO" push "$REMOTE"

echo "Pushed AUR update for ${tag_name}"
