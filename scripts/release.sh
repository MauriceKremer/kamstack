#!/usr/bin/env bash
# Create a kamstack release: tag the matched pair, wait for their releases,
# pin them in stack.yml, then tag kamstack itself.
#
# Run from a checkout where kam/, kammon/ and kamstack/ are siblings:
#   ./scripts/release.sh            # asks for the version
#   ./scripts/release.sh 0.7.2      # or pass it
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
kamstack_dir="$(cd "$script_dir/.." && pwd)"
workspace="$(cd "$kamstack_dir/.." && pwd)"
kam_dir="$workspace/kam"
kammon_dir="$workspace/kammon"

usage() {
	echo "usage: $0 [--help] [version]"
	echo
	echo "Tags kam, kammon and kamstack with the same version, updates stack.yml,"
	echo "and waits for the component releases before tagging kamstack."
	echo "The version may be given with or without a leading v (0.7.2 or v0.7.2)."
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	usage
	exit 0
fi

version="${1:-}"
if [[ -z "$version" ]]; then
	read -r -p "New version (for example 0.7.2): " version
fi
version="${version#v}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
	echo "invalid version: $version" >&2
	exit 1
fi
tag="v$version"

for tool in git gh; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "missing required command: $tool" >&2
		exit 1
	fi
done

repo_dir() {
	case "$1" in
	kam) echo "$kam_dir" ;;
	kammon) echo "$kammon_dir" ;;
	kamstack) echo "$kamstack_dir" ;;
	esac
}

preflight() {
	local name="$1" dir
	dir="$(repo_dir "$name")"
	if [[ ! -d "$dir/.git" ]]; then
		echo "$name: no git repository at $dir" >&2
		exit 1
	fi
	git -C "$dir" fetch --tags --quiet origin
	if [[ -n "$(git -C "$dir" status --porcelain)" ]]; then
		echo "$name: working tree is not clean at $dir; commit or stash first" >&2
		git -C "$dir" status --short
		exit 1
	fi
	git -C "$dir" checkout main --quiet
	git -C "$dir" pull --ff-only --quiet origin main
	if git -C "$dir" rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
		echo "$name: tag $tag already exists locally" >&2
		exit 1
	fi
	if git -C "$dir" ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
		echo "$name: tag $tag already exists on origin" >&2
		exit 1
	fi
}

tag_repo() {
	local name="$1" dir
	dir="$(repo_dir "$name")"
	echo "==> tagging $name $tag"
	git -C "$dir" tag -a "$tag" -m "kamstack $tag"
	git -C "$dir" push origin "$tag"
}

wait_for_release() {
	local name="$1" dir repo deadline
	dir="$(repo_dir "$name")"
	repo="$(cd "$dir" && gh repo view --json nameWithOwner --jq .nameWithOwner)"
	echo "==> waiting for $repo $tag release"
	deadline=$((SECONDS + 600))
	while ((SECONDS < deadline)); do
		if gh release view "$tag" --repo "$repo" --json assets --jq '.assets[].name' 2>/dev/null | grep -qx SHA256SUMS; then
			echo "    $repo $tag is published"
			return 0
		fi
		sleep 10
	done
	echo "$repo $tag did not publish within 10 minutes" >&2
	exit 1
}

echo "Release $tag will be created in:"
for name in kam kammon kamstack; do
	echo "  $(repo_dir "$name")"
done
echo
read -r -p "Continue? [y/N] " answer
if [[ ! "$answer" =~ ^[yY] ]]; then
	echo "aborted"
	exit 1
fi

for name in kam kammon kamstack; do
	preflight "$name"
done

tag_repo kam
tag_repo kammon
wait_for_release kam
wait_for_release kammon

echo "==> pinning $tag in $kamstack_dir/stack.yml"
cd "$kamstack_dir"
git pull --ff-only --quiet origin main
sed -i.bak -E "s/^kam: v.*/kam: v$version/; s/^kammon: v.*/kammon: v$version/" stack.yml
rm -f stack.yml.bak
grep -q "^kam: v$version$" stack.yml
grep -q "^kammon: v$version$" stack.yml
git add stack.yml
git commit -m "chore: pin v$version"
git push origin main

tag_repo kamstack
wait_for_release kamstack

echo
echo "Release $tag is published."
echo "Deploy with:"
echo "  sudo kamstack update --check"
echo "  sudo kamstack update"
