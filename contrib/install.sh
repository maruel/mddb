#!/bin/bash
# Install the latest checksummed mddb release and start its per-user service.

set -euo pipefail
umask 077

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}
# Resolve existing ancestors physically without creating first-install paths.
resolved_dir() {
  local directory="$1" suffix='' base
  while [[ ! -d "$directory" ]]; do
    suffix="/$(basename -- "$directory")$suffix"
    directory="$(dirname -- "$directory")"
  done
  base="$(CDPATH='' cd -P -- "$directory" && pwd -P)"
  printf '%s%s\n' "$base" "$suffix"
}
(($# == 0)) || fail 'This installer takes no arguments.'
case "$(uname -s)" in
Linux) platform=linux ;;
Darwin) platform=darwin ;;
*) fail 'This installer supports Linux and macOS.' ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail 'Supported architectures are amd64 and arm64.' ;;
esac
: "${HOME:?HOME must be set}"
config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
user_paths=("$HOME" "$config_home")
if [[ "$platform" == linux ]]; then
  cache_home="${XDG_CACHE_HOME:-$HOME/.cache}"
  user_paths+=("$cache_home")
fi
for path in "${user_paths[@]}"; do
  [[ "$path" == /* ]] || fail 'User paths must be absolute.'
  if printf '%s' "$path" | LC_ALL=C grep -q '[[:cntrl:]]'; then fail 'User paths cannot contain control characters.'; fi
done
install_dir="$HOME/.local/bin"
binary_path="$install_dir/mddb"
config_path="$config_home/mddb/config.toml"
settings_path="$config_home/mddb/settings.json"
working_dir="$HOME/mddb"
data_dir="$working_dir/data"
if [[ "$platform" == linux ]]; then
  service_path="$config_home/systemd/user/mddb.service"
  command -v systemctl >/dev/null || fail 'systemctl is required.'
  systemctl --user show-environment >/dev/null || fail 'A running systemd user manager is required.'
else
  service_path="$HOME/Library/LaunchAgents/com.maruel.mddb.plist"
  logs="$HOME/Library/Logs/mddb"
  command -v launchctl >/dev/null || fail 'launchctl is required.'
  command -v plutil >/dev/null || fail 'plutil is required.'
fi
for tool in curl tar install; do command -v "$tool" >/dev/null || fail "$tool is required."; done
if command -v sha256sum >/dev/null; then
  hash_command=(sha256sum)
elif command -v shasum >/dev/null; then
  hash_command=(shasum -a 256)
else
  fail 'sha256sum or shasum is required.'
fi
destinations=("$binary_path" "$service_path")
if [[ "$platform" == darwin ]]; then destinations+=("$logs/stdout.log" "$logs/stderr.log"); fi
for path in "$config_path" "$settings_path" "${destinations[@]}"; do
  [[ ! -e "$path" || -f "$path" ]] || fail "Expected a file: $path"
done
for path in "$working_dir" "$data_dir"; do
  [[ ! -e "$path" || -d "$path" ]] || fail "Expected a directory: $path"
done
for path in "${destinations[@]}"; do
  if [[ "$path" -ef "$config_path" || "$path" -ef "$settings_path" ]]; then fail 'An installation destination aliases configuration or settings.'; fi
done
# Managed files must not replace or append documents through directory aliases.
data_root="$(resolved_dir "$data_dir")"
managed_dirs=("$install_dir" "$(dirname -- "$config_path")" "$(dirname -- "$service_path")")
if [[ "$platform" == darwin ]]; then managed_dirs+=("$logs"); fi
for directory in "${managed_dirs[@]}"; do
  physical="$(resolved_dir "$directory")"
  [[ ! "$directory" -ef "$data_dir" ]] || fail 'An installation directory aliases mddb data.'
  case "$physical/" in
  "$data_root/"*) fail 'An installation directory is inside mddb data.' ;;
  esac
done
if [[ "$platform" == darwin ]]; then
  # launchd appends to these paths, unlike the atomic binary/service replacement.
  for path in "$logs/stdout.log" "$logs/stderr.log"; do
    [[ ! -L "$path" ]] || fail 'Log files must not be links.'
    if [[ -e "$path" ]]; then
      linked="$(find "$path" -links +1 -print)"
      [[ -z "$linked" ]] || fail 'Log files must not have other hard links.'
    fi
  done
fi
work_dir="$(mktemp -d)"
readonly work_dir
pending=''
completed=()
cleanup() {
  status=$?
  trap - EXIT
  rm -rf -- "$work_dir"
  if [[ -n "$pending" ]]; then rm -f -- "$pending"; fi
  if ((status != 0 && ${#completed[@]} != 0)); then
    printf 'Installation failed after installing:\n' >&2
    printf '  %s\n' "${completed[@]}" >&2
    printf 'Correct the error and rerun.\n' >&2
  fi
  exit "$status"
}
trap cleanup EXIT
resolved="$(curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --head --output /dev/null --write-out '%{url_effective}' https://github.com/maruel/mddb/releases/latest)"
tag="${resolved##*/}"
tag_pattern='^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'
[[ "$tag" =~ $tag_pattern ]] || fail 'The latest stable release could not be resolved.'
asset="mddb_${platform}_${arch}.tar.gz"
base="https://github.com/maruel/mddb/releases/download/$tag"
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$base/$asset" --output "$work_dir/archive.tar.gz"
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$base/checksums.txt" --output "$work_dir/checksums.txt"
expected=''
matches=0
while IFS= read -r line || [[ -n "$line" ]]; do
  hash="${line:0:64}"
  if [[ "$line" == "$hash  $asset" && "$hash" =~ ^[[:xdigit:]]{64}$ ]]; then
    expected="$hash"
    matches=$((matches + 1))
  fi
done <"$work_dir/checksums.txt"
((matches == 1)) || fail "Expected exactly one SHA-256 entry for $asset."
expected="$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')"
actual="$("${hash_command[@]}" <"$work_dir/archive.tar.gz")"
[[ "${actual%% *}" == "$expected" ]] || fail 'Archive SHA-256 checksum mismatch.'
printf '%s\n' LICENSE config.toml mddb "mddb.$([[ "$platform" == linux ]] && printf service || printf plist)" | LC_ALL=C sort >"$work_dir/expected"
tar -tzf "$work_dir/archive.tar.gz" | LC_ALL=C sort >"$work_dir/members"
cmp -s "$work_dir/expected" "$work_dir/members" || fail 'Archive must contain exactly the expected release files at its root.'
tar -tvzf "$work_dir/archive.tar.gz" >"$work_dir/types"
while IFS= read -r line; do [[ "$line" == -* ]] || fail 'Archive members must be regular files.'; done <"$work_dir/types"
mkdir "$work_dir/files"
tar -xzf "$work_dir/archive.tar.gz" -C "$work_dir/files"
[[ -s "$work_dir/files/mddb" && -x "$work_dir/files/mddb" ]] || fail 'Archive binary must be nonempty and executable.'
if [[ "$platform" == linux ]]; then
  config_environment="$(printf '%s' "$config_home" | sed 's/\\/\\\\/g; s/"/\\"/g; s/%/%%/g')"
  cache_environment="$(printf '%s' "$cache_home" | sed 's/\\/\\\\/g; s/"/\\"/g; s/%/%%/g')"
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in
    '[Service]')
      printf '%s\n' "$line"
      printf 'Environment="XDG_CONFIG_HOME=%s"\nEnvironment="XDG_CACHE_HOME=%s"\n' "$config_environment" "$cache_environment"
      ;;
    'ReadWritePaths=%h/mddb/data %h/.config/mddb %h/.cache/mddb %h/.local/bin')
      printf 'ReadWritePaths="%%h/mddb/data" "%s/mddb" "%s/mddb" "%%h/.local/bin"\n' "$config_environment" "$cache_environment"
      ;;
    *) printf '%s\n' "$line" ;;
    esac
  done <"$work_dir/files/mddb.service" >"$work_dir/service"
else
  cp -- "$work_dir/files/mddb.plist" "$work_dir/service"
  plutil -replace ProgramArguments.0 -string "$binary_path" -- "$work_dir/service"
  plutil -replace WorkingDirectory -string "$working_dir" -- "$work_dir/service"
  plutil -replace StandardOutPath -string "$logs/stdout.log" -- "$work_dir/service"
  plutil -replace StandardErrorPath -string "$logs/stderr.log" -- "$work_dir/service"
  plutil -replace EnvironmentVariables.XDG_CONFIG_HOME -string "$config_home" -- "$work_dir/service"
  plutil -replace EnvironmentVariables.PATH -string "$install_dir:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin" -- "$work_dir/service"
  plutil -lint -s -- "$work_dir/service"
fi
mkdir -p -- "$install_dir" "$(dirname -- "$config_path")" "$(dirname -- "$service_path")" "$data_dir"
if [[ "$platform" == linux ]]; then mkdir -p -- "$cache_home/mddb"; fi
pending="$(mktemp "$install_dir/.mddb.XXXXXX")"
install -m 755 "$work_dir/files/mddb" "$pending"
mv -f -- "$pending" "$binary_path"
pending=''
completed+=(mddb)
if [[ ! -e "$config_path" ]]; then
  (
    set -o noclobber
    cat "$work_dir/files/config.toml" >"$config_path"
  )
  completed+=(configuration)
fi
pending="$(mktemp "$(dirname -- "$service_path")/.mddb-service.XXXXXX")"
install -m 600 "$work_dir/service" "$pending"
mv -f -- "$pending" "$service_path"
pending=''
completed+=("user service")
if [[ "$platform" == linux ]]; then
  systemctl --user daemon-reload
  systemctl --user enable mddb.service
  systemctl --user restart mddb.service
else
  mkdir -p -- "$logs"
  domain="gui/$(id -u)"
  if launchctl print "$domain/com.maruel.mddb" >/dev/null 2>&1; then launchctl bootout "$domain/com.maruel.mddb"; fi
  launchctl bootstrap "$domain" "$service_path"
fi
printf 'Installed: %s\nConfiguration: %s\nData: %s\n' "$binary_path" "$config_path" "$data_dir"
case ":$PATH:" in
*":$install_dir:"*) ;;
*) printf 'Add %s to PATH to use mddb from your terminal.\n' "$install_dir" ;;
esac
