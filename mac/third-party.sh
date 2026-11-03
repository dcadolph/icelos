#!/bin/sh
# Print THIRD_PARTY.md for Icelos.app from mac/ffmpeg-sources.txt.
#
# Usage: mac/third-party.sh [version]
set -eu
here="$(cd "$(dirname "$0")" && pwd)"
version="${1:-dev}"
case "$version" in
dev | "") ref=main ;;
v*) ref="$version" ;;
*) ref="v$version" ;;
esac

cat <<HEAD
# Third-party software

Icelos.app bundles FFmpeg and the libraries listed below. Each is distributed under its own
license, named in the table. FFmpeg is built with the GPL and version 3 options, so the bundled
\`ffmpeg\` and \`ffprobe\` binaries are distributed under GPL-3.0-or-later, the same license as
Icelos. The license text is in \`LICENSE\` in the source repository.

HEAD

awk -F '\t' '
	/^#/ || NF < 5 { next }
	{
		n++
		name[n] = $1; ver[n] = $2; lic[n] = $3; url[n] = $4
		if (length($1) > w1) w1 = length($1)
		if (length($2) > w2) w2 = length($2)
		if (length($3) > w3) w3 = length($3)
		if (length($4) > w4) w4 = length($4)
	}
	function row(a, b, c, d) {
		printf "| %-*s | %-*s | %-*s | %-*s |\n", w1, a, w2, b, w3, c, w4, d
	}
	function rule(w,   s) {
		s = ""
		while (length(s) < w) s = s "-"
		return s
	}
	END {
		if (w1 < 4) w1 = 4
		if (w2 < 7) w2 = 7
		if (w3 < 7) w3 = 7
		if (w4 < 6) w4 = 6
		row("Name", "Version", "License", "Source")
		row(rule(w1), rule(w2), rule(w3), rule(w4))
		for (i = 1; i <= n; i++) row(name[i], ver[i], lic[i], url[i])
	}
' "$here/ffmpeg-sources.txt"

cat <<TAIL

The corresponding source for the bundled binaries is attached to each Icelos release as
\`Icelos_<version>_ffmpeg_source.tar\` at https://github.com/dcadolph/icelos/releases. It holds
every tarball above with \`mac/build-ffmpeg.sh\` and \`mac/ffmpeg-sources.txt\`, which build the
binaries from them with no download. The same scripts are at
https://github.com/dcadolph/icelos/tree/$ref. VideoToolbox, AudioToolbox, zlib, bzip2, and iconv
come from macOS.
TAIL
