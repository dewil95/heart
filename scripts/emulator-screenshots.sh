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

# Taps the first on-screen element whose text starts with $1 (case-insensitive).
tap_text() {
  adb shell uiautomator dump /sdcard/ui.xml >/dev/null
  local b
  b=$(adb shell cat /sdcard/ui.xml | tr '>' '\n' | grep -i "text=\"$1" | head -1 |
    sed -E 's/.*bounds="\[([0-9]+),([0-9]+)\]\[([0-9]+),([0-9]+)\]".*/\1 \2 \3 \4/')
  if [ -z "$b" ]; then
    echo "no element with text \"$1\""
    return 1
  fi
  set -- $b
  adb shell input tap $((($1 + $3) / 2)) $((($2 + $4) / 2))
}

save_log() {
  adb logcat -d | grep -E "netmerge|GoLog|net-merge|AndroidRuntime|FATAL" >"$out/logcat.txt" || true
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
shot 2-main-merging 10 # time for the internet probes

tap_text "Apps that don't merge"
shot 3-apps 4
adb shell input keyevent KEYCODE_BACK

tap_text "Usage"
shot 4-usage-7-days 6
tap_text "30 days"
shot 5-usage-30-days 6
tap_text "Today"
shot 6-usage-today 6
adb shell input keyevent KEYCODE_BACK

adb shell cmd uimode night yes
sleep 3
tap_text "Usage"
shot 7-usage-dark 6
adb shell input keyevent KEYCODE_BACK
shot 8-main-dark 2
adb shell cmd uimode night no

# A crash shows up as the app's process missing.
if ! adb shell pidof $pkg >/dev/null; then
  echo "net-merge is not running: it crashed, see logcat.txt"
  exit 1
fi
