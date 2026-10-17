package io.github.dewil95.netmerge

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.util.Log

/**
 * "Start when the phone starts": merging starts after a reboot, and again after net-merge itself
 * is updated (an update stops the running app). Only if the user turned it on and the VPN is still
 * allowed: without that permission the start would need the app open.
 */
class BootReceiver : BroadcastReceiver() {

    companion object {
        private const val PREFS = "settings"
        private const val KEY = "start_on_boot"

        fun isEnabled(context: Context): Boolean =
            context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getBoolean(KEY, false)

        fun setEnabled(context: Context, on: Boolean) {
            context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putBoolean(KEY, on).apply()
        }
    }

    override fun onReceive(context: Context, intent: Intent) {
        // Both are protected broadcasts: only the system sends them.
        if (intent.action != Intent.ACTION_BOOT_COMPLETED && intent.action != Intent.ACTION_MY_PACKAGE_REPLACED) return
        if (!isEnabled(context) || VpnService.prepare(context) != null) return
        try {
            context.startForegroundService(MergeVpnService.intent(context, MergeVpnService.ACTION_START))
        } catch (e: RuntimeException) {
            Log.w("net-merge", "start on boot: $e")
        }
    }
}
