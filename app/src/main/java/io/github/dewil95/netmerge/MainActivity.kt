package io.github.dewil95.netmerge

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.net.Uri
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.view.View
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import mobile.Mobile

class MainActivity : Activity() {

    private companion object {
        const val REQUEST_VPN = 1
        const val REQUEST_NOTIFICATIONS = 2
        const val NOTE =
            "Mobile data stays on while merging, so part of your traffic uses your data plan.\n\n" +
                "Apps that open many connections at once get faster: downloads, browsers, app " +
                "updates, cloud uploads. A single connection (one video stream, one download " +
                "stream) always uses one network.\n\n" +
                "Some sites tie your login to one IP address and may ask you to log in again. " +
                "If an app doesn't work or keeps logging you out, add it to the apps that don't merge."
    }

    private val handler = Handler(Looper.getMainLooper())
    private lateinit var toggle: Button
    private lateinit var appsButton: Button
    private lateinit var overlayButton: Button
    private lateinit var headline: TextView
    private lateinit var error: TextView
    private lateinit var linksBox: LinearLayout
    private var running = false
    private var askedOverlayPermission = false

    private val poll = object : Runnable {
        override fun run() {
            render(MergeStatus.parse(Mobile.status()))
            handler.postDelayed(this, 1000)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(buildUi())
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQUEST_NOTIFICATIONS)
        }
    }

    override fun onResume() {
        super.onResume()
        val n = ExcludedApps.get(this).size
        appsButton.text = if (n == 0) "Apps that don't merge: none" else "Apps that don't merge: $n"
        if (askedOverlayPermission) { // back from Settings
            askedOverlayPermission = false
            if (Settings.canDrawOverlays(this)) SpeedOverlay.setEnabled(this, true)
        }
        renderOverlayButton()
        handler.post(poll)
    }

    private fun onOverlayToggle() {
        when {
            SpeedOverlay.isEnabled(this) -> SpeedOverlay.setEnabled(this, false)
            Settings.canDrawOverlays(this) -> SpeedOverlay.setEnabled(this, true)
            else -> {
                // Granted on a system screen; picked up in onResume.
                askedOverlayPermission = true
                startActivity(
                    Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION, Uri.parse("package:$packageName")),
                )
            }
        }
        renderOverlayButton()
    }

    private fun renderOverlayButton() {
        val on = SpeedOverlay.isEnabled(this) && Settings.canDrawOverlays(this)
        overlayButton.text = "Speed on screen over ${SpeedOverlay.SHOW_ABOVE_MBPS} Mbps: " + if (on) "on" else "off"
    }

    override fun onPause() {
        handler.removeCallbacks(poll)
        super.onPause()
    }

    private fun onToggle() {
        if (running) {
            startService(MergeVpnService.intent(this, MergeVpnService.ACTION_STOP))
            return
        }
        // Asks the user once to allow the VPN; null means it is already allowed.
        val consent = VpnService.prepare(this)
        if (consent != null) {
            startActivityForResult(consent, REQUEST_VPN)
        } else {
            start()
        }
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQUEST_VPN) {
            if (resultCode == RESULT_OK) {
                start()
            } else {
                MergeVpnService.lastError = "net-merge needs the VPN permission to see your apps' connections."
            }
        }
    }

    private fun start() {
        MergeVpnService.lastError = null
        startService(MergeVpnService.intent(this, MergeVpnService.ACTION_START))
    }

    private fun render(st: MergeStatus) {
        running = st.running
        toggle.text = if (running) "Stop merging" else "Start merging"
        headline.text = when {
            running && st.rate > 0 -> "${st.headline} · ${MergeStatus.mbit(st.rate)}"
            running -> st.headline
            else -> "Off. Turn on Wi-Fi and mobile data, then tap Start."
        }
        val err = MergeVpnService.lastError
        error.text = err ?: ""
        error.visibility = if (err != null && !running) View.VISIBLE else View.GONE

        linksBox.removeAllViews()
        for (l in st.links) {
            val card = LinearLayout(this).apply {
                orientation = LinearLayout.VERTICAL
                setPadding(dp(16), dp(12), dp(16), dp(12))
                background = GradientDrawable().apply {
                    cornerRadius = dp(12).toFloat()
                    setColor(0x1A888888)
                }
                alpha = if (l.present) 1f else 0.5f
            }
            card.addView(text("${l.label} · ${l.name}", 17f, bold = true))
            val flows = if (l.active == 1L) "1 connection" else "${l.active} connections"
            card.addView(text("${l.health} · $flows · ${MergeStatus.mbit(l.rateIn + l.rateOut)}", 15f))
            val addrs = listOf(l.ip4, l.ip6).filter { it.isNotEmpty() }.joinToString("  ")
            if (addrs.isNotEmpty()) card.addView(text(addrs, 12f).apply { alpha = 0.6f })
            linksBox.addView(card, LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT).apply { topMargin = dp(10) })
        }
    }

    private fun buildUi(): View {
        val column = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(20), dp(24), dp(20), dp(24))
        }
        column.addView(text("net-merge", 30f, bold = true))
        column.addView(text("Use Wi-Fi and mobile data at the same time.", 16f).apply { alpha = 0.7f })

        toggle = Button(this).apply {
            textSize = 18f
            setOnClickListener { onToggle() }
        }
        column.addView(toggle, below(24))
        headline = text("", 16f)
        column.addView(headline, below(16))
        error = text("", 14f).apply {
            setTextColor(0xFFD32F2F.toInt())
            visibility = View.GONE
        }
        column.addView(error, below(8))
        linksBox = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        column.addView(linksBox, below(8))
        appsButton = Button(this).apply {
            setOnClickListener { startActivity(Intent(this@MainActivity, AppsActivity::class.java)) }
        }
        column.addView(appsButton, below(20))
        overlayButton = Button(this).apply { setOnClickListener { onOverlayToggle() } }
        column.addView(overlayButton, below(8))
        column.addView(text(NOTE, 13f).apply { alpha = 0.6f }, below(28))
        column.addView(text("Version ${Mobile.Version}", 12f).apply { alpha = 0.4f }, below(16))

        return ScrollView(this).apply {
            clipToPadding = false
            addView(column)
            padForSystemBars()
        }
    }

    private fun text(s: String, sizeSp: Float, bold: Boolean = false) = TextView(this).apply {
        text = s
        textSize = sizeSp
        if (bold) setTypeface(typeface, Typeface.BOLD)
    }

    private fun below(topDp: Int) =
        LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT).apply { topMargin = dp(topDp) }
}
