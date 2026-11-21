#!/bin/sh
# Install the latest kamstack for this machine and hand over to setup.
# Usage: curl -fsSL https://github.com/<owner>/kamstack/releases/latest/download/install.sh | sudo sh
set -eu

REPO="${KAMSTACK_REPO:-MauriceKremer/kamstack}"
BASE="https://github.com/${REPO}/releases/latest/download"
DEST="/opt/kamstack/bin"

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  armv6l|armv7l|armhf) arch=armv7 ;;
  *) echo "install.sh: unsupported machine $(uname -m)" >&2; exit 1 ;;
esac

if [ "$(id -u)" -ne 0 ]; then
  echo "install.sh: run as root, for example: curl -fsSL .../install.sh | sudo sh" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "${BASE}/kamstack_linux_${arch}" -o "${tmp}/kamstack"
curl -fsSL "${BASE}/SHA256SUMS" -o "${tmp}/SHA256SUMS"
sum="$(awk -v name="kamstack_linux_${arch}" '$2 == name { print $1 }' "${tmp}/SHA256SUMS")"
[ -n "$sum" ] || { echo "install.sh: no checksum for kamstack_linux_${arch}" >&2; exit 1; }
echo "${sum}  ${tmp}/kamstack" | sha256sum -c -

install -d -m 0755 "${DEST}"
install -m 0755 "${tmp}/kamstack" "${DEST}/kamstack"
echo "Installed ${DEST}/kamstack"

# setup writes the configs and units; update then installs the pinned pair.
"${DEST}/kamstack" setup "$@"
exec "${DEST}/kamstack" update
