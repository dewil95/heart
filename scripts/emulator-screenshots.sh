#!/usr/bin/env bash
# CI smoke test: installs the APK on a running emulator, starts merging, opens every screen and
# saves a screenshot of each plus the app's log. Usage: emulator-screenshots.sh <apk> <out-dir>
set -euo pipefail

apk=$1
out=$2
pkg=io.github.dewil95.netmerge
mkdir -p "$out"

shot() { # name [seconds to wait first]
  sleep "${2:-3}"
  adb exec-out screencap -p >"$out/$1.png"
  echo "screenshot $1"
}

# Taps the first on-screen element whose text starts with $1 (case-insensitive). Only for screens
# that sit still: the dump waits for the UI to go idle, so it is retried a few times.
tap_text() {
  local b="" try
  for try in 1 2 3 4; do
    adb shell rm -f /sdcard/ui.xml
    adb shell uiautomator dump /sdcard/ui.xml >/dev/null || true
    b=$(adb shell cat /sdcard/ui.xml 2>/dev/null | tr '>' '\n' | grep -i "text=\"$1" | head -1 |
      sed -E 's/.*bounds="\[([0-9]+),([0-9]+)\]\[([0-9]+),([0-9]+)\]".*/\1 \2 \3 \4/' || true)
    [ -n "$b" ] && break
    sleep 2
  done
  if [ -z "$b" ]; then
    echo "no element with text \"$1\""
    return 1
  fi
  set -- $b
  adb shell input tap $((($1 + $3) / 2)) $((($2 + $4) / 2))
}

open_screen() { # apps | usage
  adb shell am start -W -n $pkg/.MainActivity --es open "$1" >/dev/null
}

# Parallel downloads from the emulator's shell (its traffic goes through the VPN like an app's),
# so both networks get connections. The file is this app's own 27 MB APK on GitHub: Cloudflare's
# speed test rate-limits repeated runs from one IP. Best effort: skipped if the image has no curl.
traffic() {
  adb shell 'command -v curl >/dev/null || { echo "no curl on this image"; exit 0; }
    for i in 1 2 3 4 5 6 7 8; do
      curl -sL -o /dev/null --max-time 25 -w "download $i: HTTP %{http_code}, %{size_download} bytes, %{speed_download} B/s\n" \
        "https://github.com/dewil95/heart/releases/download/android-latest/net-merge-debug.apk" &
    done
    wait' || true
}

save_log() {
  adb logcat -d | grep -E "netmerge|GoLog|net-merge|AndroidRuntime|FATAL" | grep -v glBindAttribLocation >"$out/logcat.txt" || true
}
trap save_log EXIT

adb install -r "$apk"
# What a user grants by hand: the VPN request, usage access, notifications.
adb shell appops set $pkg ACTIVATE_VPN allow
adb shell appops set $pkg GET_USAGE_STATS allow
adb shell pm grant $pkg android.permission.POST_NOTIFICATIONS || true
adb logcat -c

adb shell am start -W -n $pkg/.MainActivity
shot 1-main-off
tap_text "Start merging"
sleep 8 # internet probes
traffic &
shot 2-main-merging 10
wait
shot 3-main-after-downloads 2

open_screen apps
shot 4-apps 4
adb shell input keyevent KEYCODE_BACK

open_screen usage
shot 5-usage-7-days 6
tap_text "30 days"
shot 6-usage-30-days 6
tap_text "Today"
shot 7-usage-today 6
adb shell input keyevent KEYCODE_BACK

adb shell cmd uimode night yes
sleep 2
open_screen usage
shot 8-usage-dark 6
adb shell input keyevent KEYCODE_BACK
shot 9-main-dark 2
adb shell cmd uimode night no

# A crash shows up as the app's process missing.
if ! adb shell pidof $pkg >/dev/null; then
  echo "net-merge is not running: it crashed, see logcat.txt"
  exit 1
fi
