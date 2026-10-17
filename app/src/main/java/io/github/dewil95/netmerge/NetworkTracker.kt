package io.github.dewil95.netmerge

import android.content.Context
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.os.Build
import android.os.Handler
import android.os.Looper
import org.json.JSONArray
import org.json.JSONObject
import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress
import java.util.concurrent.ConcurrentHashMap

/**
 * Tracks every network that can reach the internet (Wi-Fi, mobile data, Ethernet, ...) and reports
 * the list on the main thread whenever it changes. While started it also keeps mobile data up:
 * Android normally takes it down as soon as Wi-Fi is connected.
 */
class NetworkTracker(context: Context, private val onChange: (List<Link>) -> Unit) {

    data class Link(
        val network: Network,
        val name: String, // interface, e.g. wlan0 or rmnet_data0
        val label: String, // e.g. "Wi-Fi"
        val ip4: String?,
        val ip6: String?,
        val v4: Boolean,
        val v6: Boolean,
    ) {
        val handle: Long get() = network.networkHandle
    }

    private val cm = context.getSystemService(ConnectivityManager::class.java)
    private val handler = Handler(Looper.getMainLooper())

    // Main thread only.
    private val caps = HashMap<Network, NetworkCapabilities>()
    private val props = HashMap<Network, LinkProperties>()
    private var last: List<Link> = emptyList()

    // Read from Go's threads while pinning sockets.
    private val byHandle = ConcurrentHashMap<Long, Network>()

    private val keepMobile = object : ConnectivityManager.NetworkCallback() {}

    private val watch = object : ConnectivityManager.NetworkCallback() {
        // Since Android 8, onAvailable is always followed by these two, so they are enough.
        override fun onCapabilitiesChanged(network: Network, nc: NetworkCapabilities) {
            caps[network] = nc
            publish()
        }

        override fun onLinkPropertiesChanged(network: Network, lp: LinkProperties) {
            props[network] = lp
            publish()
        }

        override fun onLost(network: Network) {
            caps.remove(network)
            props.remove(network)
            publish()
        }
    }

    fun start() {
        val mobile = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addTransportType(NetworkCapabilities.TRANSPORT_CELLULAR)
            .build()
        cm.requestNetwork(mobile, keepMobile, handler)
        val all = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
            .build()
        cm.registerNetworkCallback(all, watch, handler)
    }

    fun stop() {
        runCatching { cm.unregisterNetworkCallback(watch) }
        runCatching { cm.unregisterNetworkCallback(keepMobile) }
        caps.clear()
        props.clear()
        byHandle.clear()
        last = emptyList()
    }

    /** The network for a handle passed to Go, or null once it is gone. Any thread. */
    fun network(handle: Long): Network? = byHandle[handle]

    /** The list last reported to onChange. Main thread. */
    fun current(): List<Link> = last

    private fun publish() {
        val links = caps.mapNotNull { (n, nc) -> props[n]?.let { link(n, nc, it) } }
            .sortedWith(compareBy({ it.label }, { it.name }))
        // Add before removing, so a socket being pinned right now never misses its network.
        for (l in links) byHandle[l.handle] = l.network
        byHandle.keys.retainAll(links.map { it.handle }.toSet())
        if (links != last) {
            last = links
            onChange(links)
        }
    }

    private fun link(n: Network, nc: NetworkCapabilities, lp: LinkProperties): Link? {
        val name = lp.interfaceName ?: return null
        if (nc.hasTransport(NetworkCapabilities.TRANSPORT_VPN) ||
            !nc.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
        ) {
            return null
        }
        val defaults = lp.routes.filter { it.isDefaultRoute }
        val route4 = defaults.any { it.destination.address is Inet4Address }
        val route6 = defaults.any { it.destination.address is Inet6Address }
        val addrs = lp.linkAddresses.map { it.address }
        val ip4 = addrs.firstOrNull { it is Inet4Address }
        val ip6 = addrs.firstOrNull { it is Inet6Address && isGlobal(it) }
        val v6 = ip6 != null && route6
        // An IPv6-only mobile network usually still carries IPv4 through 464XLAT, which has no
        // address on the network itself. The internet probe settles whether IPv4 really works.
        val xlat = ip4 == null && v6 && if (Build.VERSION.SDK_INT >= 30) {
            lp.nat64Prefix != null
        } else {
            nc.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)
        }
        val v4 = (ip4 != null && route4) || xlat
        if (!v4 && !v6) return null
        return Link(n, name, label(nc), ip4?.hostAddress, ip6?.hostAddress, v4, v6)
    }

    companion object {
        /** The list in the JSON form Mobile.setNetworks takes. */
        fun toJson(links: List<Link>): String = JSONArray(
            links.map { l ->
                JSONObject()
                    .put("name", l.name)
                    .put("label", l.label)
                    .put("handle", l.handle)
                    .put("ip4", l.ip4 ?: "")
                    .put("ip6", l.ip6 ?: "")
                    .put("v4", l.v4)
                    .put("v6", l.v6)
            },
        ).toString()

        private fun isGlobal(a: InetAddress): Boolean =
            !a.isLinkLocalAddress && !a.isLoopbackAddress &&
                !a.isSiteLocalAddress && // fec0::/10, e.g. the emulator's Wi-Fi
                (a.address[0].toInt() and 0xfe) != 0xfc // not a ULA (fc00::/7)

        private fun label(nc: NetworkCapabilities): String = when {
            nc.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "Wi-Fi"
            nc.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "Mobile data"
            nc.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "Ethernet"
            Build.VERSION.SDK_INT >= 31 && nc.hasTransport(NetworkCapabilities.TRANSPORT_USB) -> "USB"
            nc.hasTransport(NetworkCapabilities.TRANSPORT_BLUETOOTH) -> "Bluetooth"
            else -> "Network"
        }
    }
}
