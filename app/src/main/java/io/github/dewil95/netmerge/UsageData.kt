package io.github.dewil95.netmerge

import android.Manifest
import android.app.AppOpsManager
import android.app.usage.NetworkStats
import android.app.usage.NetworkStatsManager
import android.content.Context
import android.content.pm.PackageManager
import android.graphics.drawable.Drawable
import android.net.ConnectivityManager
import android.os.Build
import android.os.Process
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Locale

/**
 * Wi-Fi and mobile data use from Android's own counters (NetworkStatsManager). It covers the whole
 * phone, with or without net-merge running, including days before net-merge was installed. Needs
 * "Usage access". While merging, Android splits each app's traffic over the networks the VPN uses.
 */
object UsageData {

    enum class Period(val title: String) { TODAY("Today"), WEEK("7 days"), MONTH("30 days") }

    /** One bar: a 2-hour slot (today) or a day. */
    class Slot(val label: String, val axisLabel: String?, val wifi: Long, val mobile: Long)

    class AppUsage(val label: String, val icon: Drawable?, val wifi: Long, val mobile: Long) {
        val total: Long get() = wifi + mobile
    }

    class Report(val period: Period, val slots: List<Slot>, val apps: List<AppUsage>) {
        val wifi: Long = slots.sumOf { it.wifi }
        val mobile: Long = slots.sumOf { it.mobile }
        val total: Long get() = wifi + mobile
    }

    private const val MAX_APPS = 40

    fun hasAccess(context: Context): Boolean {
        val ops = context.getSystemService(AppOpsManager::class.java)
        @Suppress("DEPRECATION") // each level deprecates the previous check; both still work
        val mode = if (Build.VERSION.SDK_INT >= 29) {
            ops.unsafeCheckOpNoThrow(AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), context.packageName)
        } else {
            ops.checkOpNoThrow(AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), context.packageName)
        }
        if (mode == AppOpsManager.MODE_DEFAULT) {
            return context.checkSelfPermission(Manifest.permission.PACKAGE_USAGE_STATS) ==
                PackageManager.PERMISSION_GRANTED
        }
        return mode == AppOpsManager.MODE_ALLOWED
    }

    /** Slow (dozens of system queries, app icons): call off the main thread. */
    fun load(context: Context, period: Period): Report {
        val nsm = context.getSystemService(NetworkStatsManager::class.java)
        val now = System.currentTimeMillis()
        val ranges = slotRanges(period, now)
        val slots = ranges.map { r ->
            Slot(r.label, r.axisLabel, device(nsm, WIFI, r.start, r.end), device(nsm, MOBILE, r.start, r.end))
        }
        return Report(period, slots, apps(context, nsm, ranges.first().start, now))
    }

    @Suppress("DEPRECATION") // NetworkStatsManager still takes the ConnectivityManager types
    private const val WIFI = ConnectivityManager.TYPE_WIFI

    @Suppress("DEPRECATION")
    private const val MOBILE = ConnectivityManager.TYPE_MOBILE

    // subscriberId null = every SIM (Android 10+; older versions may report no mobile data).
    private fun device(nsm: NetworkStatsManager, type: Int, start: Long, end: Long): Long = try {
        nsm.querySummaryForDevice(type, null, start, end).let { it.rxBytes + it.txBytes }
    } catch (e: Exception) {
        0L
    }

    private fun perUid(nsm: NetworkStatsManager, type: Int, start: Long, end: Long): Map<Int, Long> {
        val out = HashMap<Int, Long>()
        val stats = try {
            nsm.querySummary(type, null, start, end)
        } catch (e: Exception) {
            return out
        }
        val b = NetworkStats.Bucket()
        while (stats.hasNextBucket()) {
            stats.getNextBucket(b)
            out[b.uid] = (out[b.uid] ?: 0L) + b.rxBytes + b.txBytes
        }
        stats.close()
        return out
    }

    private fun apps(context: Context, nsm: NetworkStatsManager, start: Long, end: Long): List<AppUsage> {
        val wifi = perUid(nsm, WIFI, start, end)
        val mobile = perUid(nsm, MOBILE, start, end)
        val pm = context.packageManager
        val all = (wifi.keys + mobile.keys).map { uid ->
            val (label, icon) = describe(context, pm, uid)
            AppUsage(label, icon, wifi[uid] ?: 0L, mobile[uid] ?: 0L)
        }.filter { it.total > 0 }.sortedByDescending { it.total }
        if (all.size <= MAX_APPS) return all
        val rest = all.drop(MAX_APPS - 1)
        return all.take(MAX_APPS - 1) +
            AppUsage("Other apps (${rest.size})", null, rest.sumOf { it.wifi }, rest.sumOf { it.mobile })
    }

    private fun describe(context: Context, pm: PackageManager, uid: Int): Pair<String, Drawable?> {
        when (uid) {
            NetworkStats.Bucket.UID_REMOVED -> return "Removed apps" to null
            NetworkStats.Bucket.UID_TETHERING -> return "Hotspot & tethering" to null
            Process.SYSTEM_UID, 0 -> return "Android system" to null
        }
        val pkg = pm.getPackagesForUid(uid)?.firstOrNull() ?: return "App $uid" to null
        if (pkg == context.packageName) return "net-merge" to context.applicationInfo.loadIcon(pm)
        return try {
            val ai = pm.getApplicationInfo(pkg, 0)
            ai.loadLabel(pm).toString() to ai.loadIcon(pm)
        } catch (e: PackageManager.NameNotFoundException) {
            pkg to null
        }
    }

    private class Range(val start: Long, val end: Long, val label: String, val axisLabel: String?)

    private fun slotRanges(period: Period, now: Long): List<Range> {
        val cal = Calendar.getInstance().apply {
            timeInMillis = now
            set(Calendar.HOUR_OF_DAY, 0)
            set(Calendar.MINUTE, 0)
            set(Calendar.SECOND, 0)
            set(Calendar.MILLISECOND, 0)
        }
        val out = ArrayList<Range>()
        if (period == Period.TODAY) {
            // 2-hour slots up to now: Android keeps its counters in 2-hour buckets.
            val hm = SimpleDateFormat("HH:mm", Locale.getDefault())
            var h = 0
            while (h < 24 && cal.timeInMillis <= now) {
                val s = cal.timeInMillis
                cal.add(Calendar.HOUR_OF_DAY, 2)
                val axis = if (h % 6 == 0) hm.format(s) else null
                out += Range(s, minOf(cal.timeInMillis, now), "${hm.format(s)}–${hm.format(cal.timeInMillis)}", axis)
                h += 2
            }
            return out
        }
        val days = if (period == Period.WEEK) 7 else 30
        val full = SimpleDateFormat("EEE d MMM", Locale.getDefault())
        val short = SimpleDateFormat(if (period == Period.WEEK) "EEE" else "d MMM", Locale.getDefault())
        cal.add(Calendar.DAY_OF_MONTH, -(days - 1))
        for (i in 0 until days) {
            val s = cal.timeInMillis
            cal.add(Calendar.DAY_OF_MONTH, 1)
            // A month shows every 7th day, counted back from today, so the labels don't collide.
            val axis = if (period == Period.WEEK || (days - 1 - i) % 7 == 0) short.format(s) else null
            out += Range(s, minOf(cal.timeInMillis, now), full.format(s), axis)
        }
        return out
    }
}
