package io.github.dewil95.netmerge

import android.content.Context

/**
 * Battery saver: mobile data stays on standby while another network (Wi-Fi, Ethernet) works and
 * isn't busy. It joins when traffic stays high for a few seconds (a download, where merging pays
 * off) or when the other networks stop working, and goes back to standby after a quiet minute.
 * One instance per merging session; [update] runs once a second.
 */
class BatterySaver {

    companion object {
        const val BUSY_BYTES_PER_SEC = 5_000_000 / 8 // 5 Mbit/s...
        const val BUSY_SECONDS = 3 // ...for this long: mobile data joins
        const val QUIET_BYTES_PER_SEC = 2_000_000 / 8 // under 2 Mbit/s...
        const val QUIET_SECONDS = 60 // ...for this long: back to standby

        private const val PREFS = "settings"
        private const val KEY = "battery_saver"

        fun isEnabled(context: Context): Boolean =
            context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getBoolean(KEY, false)

        fun setEnabled(context: Context, on: Boolean) {
            context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putBoolean(KEY, on).apply()
        }
    }

    private var busyFor = 0
    private var quietFor = 0
    private var wanted = false

    /**
     * Whether mobile data should be up now. [others] names the networks that aren't mobile data.
     * With the saver off, mobile data is always wanted.
     */
    fun update(st: MergeStatus, others: Set<String>, enabled: Boolean): Boolean {
        val otherWorks = st.links.any { it.name in others && it.usable }
        if (!enabled || !otherWorks) {
            busyFor = 0
            quietFor = 0
            wanted = true
            return true
        }
        val rate = st.rate
        if (rate >= BUSY_BYTES_PER_SEC) {
            busyFor++
            quietFor = 0
        } else {
            busyFor = 0
            quietFor = if (rate < QUIET_BYTES_PER_SEC) quietFor + 1 else 0
        }
        if (!wanted && busyFor >= BUSY_SECONDS) wanted = true
        if (wanted && quietFor >= QUIET_SECONDS) wanted = false
        return wanted
    }
}
