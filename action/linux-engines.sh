#!/usr/bin/env bash
# Linux engine setup for the composite action (tasks/backlog.md, B1): the
# parity fonts for every engine, plus Weston and the WebKitGTK driver when
# WebKitGTK is wanted.
#
# Two things make this script more than an `apt-get install`:
#
# - The download is the slow part, and the Ubuntu mirror can be slow
#   depending on demand (2026-11-20: about 40-50 KB/s, and a consumer's job
#   hit GitHub's 30-minute limit in this step). Downloads run under a time budget, and
#   past it the step warns and moves on: Chrome and Firefox ship with the
#   image, so a slow mirror loses WebKitGTK, not the whole Linux host.
# - The .debs are kept in a directory the action caches. On an exact cache
#   hit (same runner image) they are installed with dpkg and nothing is
#   downloaded at all.
#
# Usage:
#   linux-engines.sh plan      writes want-webkit and key to $GITHUB_OUTPUT
#   linux-engines.sh install   installs; writes complete=true|false
#
# Environment: ENGINES (the action's input), OMNISNAP_APT_ARCHIVES (the cached
# directory), OMNISNAP_SETUP_BUDGET (seconds for the downloads, default 300),
# OMNISNAP_CACHE_HIT (true on an exact cache hit).
#
# Report-only: this script always exits 0.

set -uo pipefail

out=${GITHUB_OUTPUT:-/dev/null}
archives=${OMNISNAP_APT_ARCHIVES:-${RUNNER_TEMP:-/tmp}/omnisnap-apt}
budget=${OMNISNAP_SETUP_BUDGET:-300}
cache_hit=${OMNISNAP_CACHE_HIT:-false}

fonts=(fonts-liberation fonts-noto-mono)
webkit=(weston webkitgtk-webdriver)
# Ubuntu 26.04 renamed the driver package; older images still use this one.
webkit_old=(weston webkit2gtk-driver)

want_webkit() {
	local e list
	[ -z "${ENGINES//[[:space:],]/}" ] && return 0
	IFS=, read -ra list <<<"$ENGINES"
	for e in "${list[@]}"; do
		e=${e//[[:space:]]/}
		[ "$e" = webkitgtk ] && return 0
	done
	return 1
}

# The image fixes the installed base, so the same image always needs the same
# .debs: the key ends with its version. The prefix (same package set, arch and
# OS) lets a newer image start from the previous image's .debs, which apt
# reuses where the versions still match. Off the hosted images ImageVersion is
# unset, and the key says so.
plan() {
	local set=fonts prefix
	want_webkit && set=webkit
	prefix="omnisnap-apt-v1-$set-${RUNNER_ARCH:-unknown}-${ImageOS:-unknown}-"
	{
		echo "want-webkit=$([ "$set" = webkit ] && echo true || echo false)"
		echo "prefix=$prefix"
		echo "key=$prefix${ImageVersion:-unknown}"
	} >>"$out"
}

# A timeout can cut apt's output mid-line, and GitHub only reads a workflow
# command at the start of a line.
warn() { printf '\n::warning::%s\n' "$*"; }

# has_candidate PKG: the image's archive can install PKG. `apt-cache show`
# succeeds for a name that is only referenced, with nothing to install.
has_candidate() {
	local c
	c=$(apt-cache policy "$1" 2>/dev/null | awk '/Candidate:/ {print $2}')
	[ -n "$c" ] && [ "$c" != "(none)" ]
}

start=$SECONDS
left() { echo $((budget - (SECONDS - start))); }

apt_opts=(-o "Dir::Cache::Archives=$archives" -o APT::Keep-Downloaded-Packages=true)

# download PKG... : fetch into the archives directory, within what is left of
# the budget. Only the download is bounded: killing it is safe, while killing
# dpkg mid-install would leave the package database half configured.
download() {
	local t
	t=$(left)
	if [ "$t" -le 0 ]; then
		warn "engine setup: the ${budget}s download budget is spent; skipped $*"
		return 1
	fi
	if ! sudo timeout --kill-after=10 "$t" apt-get "${apt_opts[@]}" install -y --download-only "$@"; then
		if [ "$(left)" -le 0 ]; then
			warn "engine setup: downloading $* did not finish in the ${budget}s budget (slow package mirror?); skipped"
		else
			warn "engine setup: downloading $* failed; skipped"
		fi
		return 1
	fi
}

# install PKG... : download, then install from the local archives only.
install_set() {
	download "$@" || return 1
	sudo apt-get "${apt_opts[@]}" install -y --no-download "$@"
}

# On an exact hit, install every cached .deb without touching the network.
# Only for a known image: on an unknown one the cached versions may not match.
from_cache() {
	[ "$cache_hit" = true ] || return 1
	[ "${ImageVersion:-}" != "" ] || return 1
	compgen -G "$archives/*.deb" >/dev/null || return 1
	sudo dpkg -i --skip-same-version "$archives"/*.deb
}

# Keep only the .debs whose version is the one now installed, so a partial
# restore from an older image never travels on under the new image's key.
prune() {
	local deb pkg ver have
	for deb in "$archives"/*.deb; do
		[ -e "$deb" ] || continue
		pkg=$(dpkg-deb -f "$deb" Package) ver=$(dpkg-deb -f "$deb" Version)
		have=$(dpkg-query -W -f='${Version}' "$pkg" 2>/dev/null || true)
		[ "$have" = "$ver" ] || sudo rm -f "$deb"
	done
	sudo chmod -R a+rX "$archives"
}

install() {
	local ok=true
	sudo mkdir -p "$archives/partial"
	sudo chown -R _apt:root "$archives/partial" 2>/dev/null || true

	local cache_failed=false
	if from_cache; then
		echo "engine setup: installed from the cache in $((SECONDS - start))s, nothing downloaded"
		echo "complete=true" >>"$out"
		return
	fi
	if [ "$cache_hit" = true ]; then
		warn "engine setup: the cached packages did not install; downloading them"
		cache_failed=true
	fi

	local t
	t=$(left)
	if [ "$t" -le 0 ] || ! sudo timeout --kill-after=10 "$t" apt-get update; then
		warn "engine setup: apt-get update did not finish in the ${budget}s budget; no engine packages installed"
		echo "complete=false" >>"$out"
		return
	fi
	# A failed dpkg -i leaves packages unpacked but not configured; apt refuses
	# to install anything else until they are fixed.
	if [ "$cache_failed" = true ]; then
		t=$(left)
		[ "$t" -gt 0 ] && sudo timeout --kill-after=10 "$t" apt-get "${apt_opts[@]}" -f install -y
	fi

	install_set "${fonts[@]}" || ok=false
	if want_webkit; then
		# Pick the driver package this image's archive has, so the older name
		# is not tried (and warned about) after a failed download.
		if ! has_candidate webkitgtk-webdriver; then
			webkit=("${webkit_old[@]}")
		fi
		install_set "${webkit[@]}" || ok=false
	fi
	prune
	echo "engine setup: done in $((SECONDS - start))s (complete: $ok)"
	echo "complete=$ok" >>"$out"
}

case "${1:-}" in
plan) plan ;;
install) install ;;
*)
	echo "usage: $0 plan|install" >&2
	;;
esac
exit 0
