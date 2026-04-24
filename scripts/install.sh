#!/bin/sh
set -eu

repo="yazanabuashour/openbrief"
default_version="__OPENBRIEF_VERSION__"

fail() {
  printf 'openbrief install: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

detect_os() {
  case "$(uname -s)" in
    Darwin) printf 'darwin' ;;
    Linux) printf 'linux' ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) printf 'amd64' ;;
    arm64 | aarch64) printf 'arm64' ;;
    *) fail "unsupported CPU architecture: $(uname -m)" ;;
  esac
}

resolve_latest_version() {
  latest_json="$(curl -fsSL "https://api.github.com/repos/${repo}/releases/latest")" ||
    fail "could not resolve latest GitHub Release"
  latest_tag="$(printf '%s\n' "$latest_json" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  [ -n "$latest_tag" ] || fail "could not read latest release tag"
  printf '%s' "$latest_tag"
}

select_version() {
  requested="${OPENBRIEF_VERSION:-$default_version}"
  case "$requested" in
    "" | "__OPENBRIEF_VERSION__" | latest) resolve_latest_version ;;
    v*) printf '%s' "$requested" ;;
    *) printf 'v%s' "$requested" ;;
  esac
}

first_writable_path_dir() {
  old_ifs="$IFS"
  IFS=:
  for dir in ${PATH:-}; do
    IFS="$old_ifs"
    [ -n "$dir" ] || dir="."
    [ "$dir" = "." ] && continue
    if [ -d "$dir" ] && [ -w "$dir" ]; then
      printf '%s' "$dir"
      return 0
    fi
    IFS=:
  done
  IFS="$old_ifs"
  return 1
}

select_install_dir() {
  if [ -n "${OPENBRIEF_INSTALL_DIR:-}" ]; then
    printf '%s' "$OPENBRIEF_INSTALL_DIR"
    return
  fi
  if dir="$(first_writable_path_dir)"; then
    printf '%s' "$dir"
    return
  fi
  [ -n "${HOME:-}" ] || fail "HOME is not set and no writable PATH directory was found"
  printf '%s/.local/bin' "$HOME"
}

need_cmd curl
need_cmd tar

os="$(detect_os)"
arch="$(detect_arch)"
tag="$(select_version)"
asset_version="${tag#v}"
archive="openbrief_${asset_version}_${os}_${arch}.tar.gz"
checksum="openbrief_${asset_version}_checksums.txt"
release_url="https://github.com/${repo}/releases/download/${tag}"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/openbrief-install.XXXXXX")"
install_dir="$(select_install_dir)"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

cd "$tmp_dir"
curl -fsSL "${release_url}/${archive}" -o "$archive"
curl -fsSL "${release_url}/${checksum}" -o "$checksum"
awk -v file="$archive" '$2 == file { print; found = 1 } END { exit found ? 0 : 1 }' "$checksum" > "expected-${archive}.sha256" ||
  fail "checksum entry not found for ${archive}"
if command -v shasum >/dev/null 2>&1; then
  shasum -a 256 -c "expected-${archive}.sha256" >/dev/null
elif command -v sha256sum >/dev/null 2>&1; then
  sha256sum -c "expected-${archive}.sha256" >/dev/null
else
  fail "missing required command: shasum or sha256sum"
fi
tar -xzf "$archive"
mkdir -p "$install_dir"
cp "openbrief_${asset_version}_${os}_${arch}/openbrief" "${install_dir}/openbrief"
chmod 755 "${install_dir}/openbrief"
"${install_dir}/openbrief" --version

printf '\nRegister the OpenBrief skill from:\n'
printf '  https://github.com/%s/tree/%s/skills/openbrief\n' "$repo" "$tag"
printf 'or release asset:\n'
printf '  %s/openbrief_%s_skill.tar.gz\n' "$release_url" "$asset_version"
