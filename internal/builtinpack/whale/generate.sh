#!/bin/sh
set -eu
cd "$(dirname "$0")"

img2webp -lossless -exact -loop 0 \
  -d 3800 source/idle-open.png \
  -d 60 source/idle-half-closed.png \
  -d 90 source/idle-closed.png \
  -d 50 source/idle-half-closed.png \
  -o clips/idle_loop.webp
