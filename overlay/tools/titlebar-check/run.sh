#!/bin/sh
# Title-bar overflow check: serves overlay/ui and measures it in headless Chrome at 280/340/480 px,
# click-through on and off, both modes. Usage: overlay/tools/titlebar-check/run.sh  (needs Chrome and python3)
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
chrome=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
[ -x "$chrome" ] || chrome=$(command -v google-chrome || command -v chromium || true)
[ -n "$chrome" ] || { echo "no headless Chrome found (set CHROME=)"; exit 2; }
site=$(mktemp -d); trap 'kill $pid 2>/dev/null; rm -rf "$site"' EXIT
ln -s "$root/ui" "$site/ui"; cp "$here/check.html" "$site/check.html"
port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$site" >/dev/null 2>&1 & pid=$!
sleep 1
out=$("$chrome" --headless=new --disable-gpu --virtual-time-budget=8000 --dump-dom "http://127.0.0.1:$port/check.html" 2>/dev/null)
echo "$out" | sed -n '/<pre id="out">/,/<\/pre>/p' | sed 's/<[^>]*>//g'
echo "$out" | grep -q "<pre id=\"out\">PASS"
