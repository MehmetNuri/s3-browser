#!/bin/sh
# Flathub allows either the Wayland or the X11 socket, not both. The legacy
# tray helper is an X11 client, so the sandbox gets the X11 socket and the
# window runs through XWayland; otherwise Electron picks Wayland and no tray
# can be shown on desktops without a StatusNotifier host.
exec zypak-wrapper /app/main/s3browser --ozone-platform=x11 "$@"
