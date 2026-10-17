package io.github.dewil95.netmerge

import org.json.JSONObject
import java.util.Locale

/** The JSON from Mobile.status(), parsed for the screen and the notification. */
data class MergeStatus(val running: Boolean, val ipv6: String, val links: List<LinkStatus>) {

    data class LinkStatus(
        val name: String,
        val label: String,
        val ip4: String,
        val ip6: String,
        val state: String, // IPv4 probe: "checking", "up", "no internet", or "-" without IPv4
        val state6: String,
        val present: Boolean,
        val active: Long,
        val total: Long,
        val rateIn: Double, // bytes/s
        val rateOut: Double,
    ) {
        val up: Boolean get() = present && (state == "up" || state6 == "up")

        /** The best of the two families, in words. */
        val health: String
            get() = when {
                !present -> "disconnected"
                up -> "working"
                state == "checking" || state6 == "checking" -> "checking…"
                else -> "no internet"
            }
    }

    val working: Int get() = links.count { it.up }
    val rate: Double get() = links.sumOf { it.rateIn + it.rateOut }

    val headline: String
        get() = when {
            !running -> "Off"
            working == 0 -> "Waiting for a network…"
            working == 1 -> "Using 1 network (add another to merge)"
            else -> "Merging $working networks"
        }

    val detail: String
        get() = links.filter { it.present }.joinToString(" · ") { "${it.label} ${mbit(it.rateIn + it.rateOut)}" }

    companion object {
        fun parse(json: String): MergeStatus {
            val o = JSONObject(json)
            val arr = o.optJSONArray("links")
            val links = (0 until (arr?.length() ?: 0)).map { i ->
                val l = arr!!.getJSONObject(i)
                LinkStatus(
                    name = l.optString("name"),
                    label = l.optString("label").ifEmpty { l.optString("name") },
                    ip4 = l.optString("ip4"),
                    ip6 = l.optString("ip6"),
                    state = l.optString("state", "-"),
                    state6 = l.optString("state6", "-"),
                    present = l.optBoolean("present"),
                    active = l.optLong("active"),
                    total = l.optLong("total"),
                    rateIn = l.optDouble("rateIn", 0.0),
                    rateOut = l.optDouble("rateOut", 0.0),
                )
            }
            return MergeStatus(o.optBoolean("running"), o.optString("ipv6"), links)
        }

        fun mbit(bytesPerSec: Double): String =
            String.format(Locale.US, "%.1f Mbit/s", bytesPerSec * 8 / 1_000_000)
    }
}
