#!/bin/sh
# start.sh starts a virtual X display with two monitors side by side at
# different refresh rates, for testing gunim's multi-monitor behaviour on
# a machine with one screen or none.
#
#   tools/multimon/start.sh [display] [rate1] [rate2]
#   DISPLAY=:30 go test ./driver/desktop/
#   DISPLAY=:30 go run ./example/twowindows
#
# It runs Xorg with the dummy video driver as the current user, which
# needs the xserver-xorg-video-dummy package, and calls the server
# binary directly, past the wrapper that allows only console users. The
# monitors report their rates to GLFW, but the dummy driver has no
# vertical blank, so frames are paced by gunim's fallback timer, and GL
# is Mesa's software renderer.
set -eu
display=${1:-:30}
rate1=${2:-60}
rate2=${3:-144}
dir=$(cd "$(dirname "$0")" && pwd)
log=${TMPDIR:-/tmp}/gunim-multimon-${display#:}.log

/usr/lib/xorg/Xorg "$display" -noreset -nolisten tcp \
	-config "$dir/xorg.conf" -configdir "$dir/none" -logfile "$log" >/dev/null 2>&1 &
for _ in 1 2 3 4 5 6 7 8 9 10; do
	DISPLAY=$display xrandr >/dev/null 2>&1 && break
	sleep 0.5
done

# modeline prints the name and timings of a 1280x720 mode at rate Hz.
modeline() {
	cvt 1280 720 "$1" | sed -n 's/^Modeline "\([^"]*\)"/\1/p' | sed "s/^[^ ]*/1280x720_$1/"
}
export DISPLAY="$display"
for rate in "$rate1" "$rate2"; do
	# shellcheck disable=SC2046
	xrandr --newmode $(modeline "$rate") 2>/dev/null || true
done
xrandr --addmode DUMMY0 "1280x720_$rate1"
xrandr --addmode DUMMY1 "1280x720_$rate2"
xrandr --output DUMMY0 --mode "1280x720_$rate1" --pos 0x0 --primary \
	--output DUMMY1 --mode "1280x720_$rate2" --pos 1280x0
xrandr --listactivemonitors
echo "display $display is up; stop it with: pkill -f 'Xorg $display'"
