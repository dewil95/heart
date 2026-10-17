package io.github.dewil95.netmerge

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BatterySaverTest {

    private val others = setOf("wlan0")

    private fun link(name: String, state: String, rate: Double = 0.0) = MergeStatus.LinkStatus(
        name = name, label = name, ip4 = "", ip6 = "", state = state, state6 = "-", present = true,
        active = 0, total = 0, rateIn = rate, rateOut = 0.0, errors = 0, lastError = "",
    )

    /** Wi-Fi carrying [mbit] Mbit/s, plus mobile data idle. */
    private fun status(wifiState: String = "up", mbit: Double = 0.0) = MergeStatus(
        running = true, ipv6 = "",
        links = listOf(link("wlan0", wifiState, mbit * 1_000_000 / 8), link("rmnet0", "up")),
    )

    @Test
    fun offMeansMobileDataAlwaysUp() {
        assertTrue(BatterySaver().update(status(), others, enabled = false))
    }

    @Test
    fun idleWifiKeepsMobileDataOnStandby() {
        val s = BatterySaver()
        repeat(10) { assertFalse(s.update(status(mbit = 0.5), others, enabled = true)) }
    }

    @Test
    fun sustainedTrafficBringsMobileDataThenAQuietMinuteRestsIt() {
        val s = BatterySaver()
        repeat(BatterySaver.BUSY_SECONDS - 1) { assertFalse(s.update(status(mbit = 20.0), others, true)) }
        assertTrue("busy for long enough", s.update(status(mbit = 20.0), others, true))
        repeat(BatterySaver.QUIET_SECONDS - 1) { assertTrue(s.update(status(mbit = 0.5), others, true)) }
        assertFalse("a quiet minute", s.update(status(mbit = 0.5), others, true))
    }

    @Test
    fun aShortBurstIsNotEnough() {
        val s = BatterySaver()
        assertFalse(s.update(status(mbit = 20.0), others, true))
        assertFalse(s.update(status(mbit = 0.1), others, true))
        assertFalse(s.update(status(mbit = 20.0), others, true))
    }

    @Test
    fun wifiWithoutInternetBringsMobileDataAtOnce() {
        assertTrue(BatterySaver().update(status(wifiState = "no internet"), others, true))
    }

    @Test
    fun wifiStillBeingCheckedCountsAsWorking() {
        assertFalse(BatterySaver().update(status(wifiState = "checking"), others, true))
    }

    @Test
    fun noWifiAtAllMeansMobileData() {
        val onlyMobile = MergeStatus(true, "", listOf(link("rmnet0", "up")))
        assertTrue(BatterySaver().update(onlyMobile, others, true))
    }
}
