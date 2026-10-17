package io.github.dewil95.netmerge

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.drawable.Icon
import android.net.VpnService
import android.os.Handler
import android.os.Looper
import android.os.ParcelFileDescriptor
import android.util.Log
import mobile.Mobile
import mobile.SocketBinder
import java.io.IOException

/**
 * The VPN is only a way to see every app's connections: nothing leaves the phone through it.
 * Each connection is handed to the Go core, which re-opens it on one of the real networks
 * (pinned with [bindSocket]), spreading connections over all of them.
 */
class MergeVpnService : VpnService(), SocketBinder {

    companion object {
        const val ACTION_START = "io.github.dewil95.netmerge.START"
        const val ACTION_STOP = "io.github.dewil95.netmerge.STOP"

        /** The list of apps that don't merge changed: rebuild the VPN with it. */
        const val ACTION_RELOAD = "io.github.dewil95.netmerge.RELOAD"

        private const val TAG = "net-merge"
        private const val CHANNEL_ID = "status"
        private const val NOTIFICATION_ID = 1

        // Same addresses as net-merge on the desktop. The IPv6 one is a ULA on purpose: with only a
        // ULA source, address selection prefers IPv4 for dual-stack sites, and IPv4 is spread over
        // every network.
        private const val TUN_IP4 = "198.18.0.1"
        private const val TUN_IP6 = "fd6e:6d65:7267::1"

        /** Why the last start failed, shown in the app; null when it didn't. */
        @Volatile
        var lastError: String? = null

        fun intent(context: Context, action: String): Intent =
            Intent(context, MergeVpnService::class.java).setAction(action)
    }

    private val handler = Handler(Looper.getMainLooper())

    @Volatile // also read by bindSocket on Go's threads
    private var tracker: NetworkTracker? = null
    private var running = false
    private val overlay by lazy { SpeedOverlay(this) }
    private var ticks = 0

    // Every second for the speed badge; the notification every other second.
    private val refresh = object : Runnable {
        override fun run() {
            val st = MergeStatus.parse(Mobile.status())
            overlay.update(st)
            if (ticks++ % 2 == 0) notifyStatus(st)
            handler.postDelayed(this, 1000)
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                stopMerging()
                stopSelf()
                return START_NOT_STICKY
            }
            ACTION_RELOAD -> {
                if (running) {
                    reload()
                } else {
                    stopSelf() // nothing to apply now; the list is read at the next start
                }
                return START_STICKY
            }
        }
        // ACTION_START from the app, or android.net.VpnService from the system (always-on VPN).
        startMerging()
        return START_STICKY
    }

    private fun startMerging() {
        if (running) return
        lastError = null
        val t = NetworkTracker(this, ::onNetworksChanged)
        tracker = t
        t.start()
        if (startEngine()) {
            running = true
            handler.post(refresh)
        }
    }

    /**
     * Establishing again replaces the VPN interface; Go then swaps to the new one and closes the old.
     * Open connections restart, which is fine for a settings change.
     */
    private fun reload() {
        if (startEngine()) {
            tracker?.let { t -> setUnderlyingNetworks(t.current().map { it.network }.toTypedArray()) }
        }
    }

    /** Opens the VPN interface and hands it to Go. On failure the service stops with [lastError] set. */
    private fun startEngine(): Boolean {
        val mtu = Mobile.DefaultMTU.toInt()
        val tun = try {
            buildTun(mtu)
        } catch (e: Exception) {
            fail("Could not create the VPN: ${e.message}")
            return false
        }
        if (tun == null) {
            fail("VPN permission was taken away. Open net-merge and tap Start again.")
            return false
        }
        try {
            Mobile.start(tun.detachFd(), mtu, this)
        } catch (e: Exception) {
            fail("Could not start merging: ${e.message}")
            return false
        }
        return true
    }

    private fun buildTun(mtu: Int): ParcelFileDescriptor? {
        val b = Builder()
            .setSession(getString(R.string.app_name))
            .setMtu(mtu)
            .addAddress(TUN_IP4, 24)
            .addRoute("0.0.0.0", 0)
            .addAddress(TUN_IP6, 64)
            .addRoute("::", 0)
            // Apps' DNS goes through the TUN like everything else, so it needs a resolver that
            // every network can reach.
            .addDnsServer("1.1.1.1")
            .addDnsServer("8.8.8.8")
            // Our own sockets (the re-opened connections) must never come back in.
            .addDisallowedApplication(packageName)
            .setConfigureIntent(openAppIntent())
        // Apps the user chose not to merge skip the VPN and use Android's default network.
        for (pkg in ExcludedApps.get(this)) {
            try {
                b.addDisallowedApplication(pkg)
            } catch (e: PackageManager.NameNotFoundException) {
                // Uninstalled since it was chosen.
            }
        }
        return b.establish()
    }

    private fun fail(message: String) {
        Log.e(TAG, message)
        lastError = message
        stopMerging()
        stopSelf()
    }

    private fun stopMerging() {
        handler.removeCallbacks(refresh)
        overlay.hide()
        tracker?.stop()
        tracker = null
        if (running) {
            Mobile.stop() // closes the TUN, which ends the VPN
            running = false
        }
        getSystemService(NotificationManager::class.java).cancel(NOTIFICATION_ID)
    }

    /** The user turned the VPN off in Settings, or another VPN app took over. */
    override fun onRevoke() {
        stopMerging()
        stopSelf()
    }

    override fun onDestroy() {
        stopMerging()
        super.onDestroy()
    }

    private fun onNetworksChanged(links: List<NetworkTracker.Link>) {
        try {
            Mobile.setNetworks(NetworkTracker.toJson(links))
        } catch (e: Exception) {
            Log.e(TAG, "setNetworks", e)
        }
        // Lets Android show the right networks (and metered state) for the VPN.
        setUnderlyingNetworks(links.map { it.network }.toTypedArray())
    }

    /** Called by Go, on its own threads, for every socket it opens. */
    override fun bindSocket(fd: Int, networkHandle: Long): Boolean {
        val network = tracker?.network(networkHandle) ?: return false
        if (!protect(fd)) return false
        return try {
            // fromFd dups the fd; binding the dup binds the same socket.
            ParcelFileDescriptor.fromFd(fd).use { network.bindSocket(it.fileDescriptor) }
            true
        } catch (e: IOException) {
            Log.w(TAG, "bindSocket: $e")
            false
        }
    }

    private fun openAppIntent(): PendingIntent = PendingIntent.getActivity(
        this,
        0,
        Intent(this, MainActivity::class.java),
        PendingIntent.FLAG_IMMUTABLE,
    )

    private fun notifyStatus(st: MergeStatus) {
        val nm = getSystemService(NotificationManager::class.java)
        if (nm.getNotificationChannel(CHANNEL_ID) == null) {
            nm.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "Merging status", NotificationManager.IMPORTANCE_LOW),
            )
        }
        val stop = PendingIntent.getService(this, 1, intent(this, ACTION_STOP), PendingIntent.FLAG_IMMUTABLE)
        val icon = Icon.createWithResource(this, R.drawable.ic_stat_merge)
        val n = Notification.Builder(this, CHANNEL_ID)
            .setSmallIcon(icon)
            .setContentTitle(st.headline)
            .setContentText(st.detail)
            .setContentIntent(openAppIntent())
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setShowWhen(false)
            .addAction(Notification.Action.Builder(icon, "Stop", stop).build())
            .build()
        nm.notify(NOTIFICATION_ID, n)
    }
}
