#!/usr/bin/env bash
# Builds a self-contained AppImage with GTK and WebKitGTK bundled, so it runs
# without installing anything. Usage: build/linux/appimage.sh <output-file>
#
# WebKitGTK launches helper processes (WebKitWebProcess, WebKitNetworkProcess)
# from absolute /usr/... paths compiled into libwebkit2gtk. We copy those
# helpers into the AppDir and rewrite "/usr" to "././" (same length) inside the
# library; AppRun then starts in $APPDIR/usr so the relative paths resolve.
# This is the approach Tauri's AppImage bundler uses.
set -euo pipefail

OUT="${1:-quarry-linux-amd64.AppImage}"
ROOT="$(pwd)"
WORK="$(mktemp -d)"
APPDIR="$WORK/AppDir"
TOOLS="$WORK/tools"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/256x256/apps" "$TOOLS"

cp build/bin/quarry "$APPDIR/usr/bin/quarry"
cp build/linux/quarry.desktop "$APPDIR/usr/share/applications/quarry.desktop"
cp build/linux/icons/quarry-256.png "$APPDIR/usr/share/icons/hicolor/256x256/apps/quarry.png"

# WebKitGTK helper processes and the injected bundle.
WKDIR="$(dirname "$(find /usr/lib -name WebKitWebProcess -path '*webkit2gtk-4.1*' | head -1)")"
echo "WebKit helpers: $WKDIR"
mkdir -p "$APPDIR$WKDIR"
cp -a "$WKDIR"/. "$APPDIR$WKDIR"/

# Tools.
curl -fsSL -o "$TOOLS/linuxdeploy" https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-x86_64.AppImage
curl -fsSL -o "$TOOLS/linuxdeploy-plugin-gtk.sh" https://raw.githubusercontent.com/linuxdeploy/linuxdeploy-plugin-gtk/master/linuxdeploy-plugin-gtk.sh
curl -fsSL -o "$TOOLS/appimagetool" https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage
chmod +x "$TOOLS"/*
export PATH="$TOOLS:$PATH"

# Bundle the app, the WebKit helpers and every library they need.
export DEPLOY_GTK_VERSION=3
helpers=()
for f in "$APPDIR$WKDIR"/WebKit*Process; do helpers+=(--executable "$f"); done
libs=()
while IFS= read -r f; do libs+=(--library "$f"); done < <(find "$APPDIR$WKDIR" -name '*.so')
linuxdeploy --appimage-extract-and-run \
  --appdir "$APPDIR" \
  --executable "$APPDIR/usr/bin/quarry" \
  "${helpers[@]}" "${libs[@]}" \
  --desktop-file "$APPDIR/usr/share/applications/quarry.desktop" \
  --icon-file "$APPDIR/usr/share/icons/hicolor/256x256/apps/quarry.png" \
  --plugin gtk

# Make WebKit's compiled-in helper paths relative to $APPDIR/usr.
find "$APPDIR" -name 'libwebkit2gtk-4.1.so*' -type f -exec sed -i -e 's|/usr|././|g' {} +
mkdir -p "$APPDIR/apprun-hooks"
cat > "$APPDIR/apprun-hooks/zz-quarry-webkit.sh" <<'EOF'
# Resolve WebKitGTK's relative helper paths (see build/linux/appimage.sh).
cd "$APPDIR/usr"
EOF

ARCH=x86_64 appimagetool --appimage-extract-and-run "$APPDIR" "$ROOT/$OUT"
ls -la "$ROOT/$OUT"
rm -rf "$WORK"
