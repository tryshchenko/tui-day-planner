#!/bin/sh
# Regenerates the README screenshots: the Go test draws each screen as SVG,
# then headless Chrome rasterizes it to a 2x PNG.
set -e
cd "$(dirname "$0")/../.."
UPDATE_SCREENSHOTS=1 go test -run TestScreenshots -count=1 .

CHROME=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
for svg in docs/screenshots/*.svg; do
	size=$(sed -n '1s/.*width="\([0-9]*\).*height="\([0-9]*\).*/\1,\2/p' "$svg")
	"$CHROME" --headless --disable-gpu --hide-scrollbars --force-device-scale-factor=2 \
		--default-background-color=00000000 --window-size="$size" \
		--screenshot="$PWD/${svg%.svg}.png" "file://$PWD/$svg" 2>/dev/null
	rm "$svg"
done
