package io.github.dewil95.netmerge

import android.content.Context
import android.graphics.PixelFormat
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.provider.Settings
import android.view.Gravity
import android.view.WindowManager
import android.widget.TextView
import java.util.Locale

/**
 * A small see-through speed badge just under the status bar, top right (under the battery). It only
 * appears while traffic through net-merge is above [SHOW_ABOVE_MBPS]; apps can't draw inside the
 * status bar itself. Needs "Display over other apps".
 */
class SpeedOverlay(private val context: Context) {

    companion object {
        const val SHOW_ABOVE_MBPS = 100

        private const val PREFS = "settings"
        private const val KEY = "speed_overlay"

        fun isEnabled(context: Context): Boolean =
            context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getBoolean(KEY, false)

        fun setEnabled(context: Context, on: Boolean) {
            context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putBoolean(KEY, on).apply()
        }

        fun format(mbps: Double): String = if (mbps >= 1000) {
            String.format(Locale.US, "%.1f Gbps", mbps / 1000)
        } else {
            String.format(Locale.US, "%.0f Mbps", mbps)
        }
    }

    private val wm = context.getSystemService(WindowManager::class.java)
    private var badge: TextView? = null

    /** Shows the faster direction (usually download) while it is above the threshold. Main thread. */
    fun update(st: MergeStatus) {
        val down = st.links.sumOf { it.rateIn } * 8 / 1_000_000
        val up = st.links.sumOf { it.rateOut } * 8 / 1_000_000
        val mbps = maxOf(down, up)
        if (!isEnabled(context) || mbps < SHOW_ABOVE_MBPS || !Settings.canDrawOverlays(context)) {
            hide()
            return
        }
        val b = badge ?: show() ?: return
        b.text = (if (up > down) "↑ " else "↓ ") + format(mbps)
    }

    fun hide() {
        badge?.let { runCatching { wm.removeViewImmediate(it) } }
        badge = null
    }

    private fun show(): TextView? {
        val b = TextView(context).apply {
            textSize = 11f
            setTypeface(typeface, Typeface.BOLD)
            setTextColor(0xFFFFFFFF.toInt())
            setPadding(context.dp(6), context.dp(1), context.dp(6), context.dp(1))
            background = GradientDrawable().apply {
                cornerRadius = context.dp(8).toFloat()
                setColor(0x99000000.toInt())
            }
        }
        val params = WindowManager.LayoutParams(
            WindowManager.LayoutParams.WRAP_CONTENT,
            WindowManager.LayoutParams.WRAP_CONTENT,
            WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
            // Never takes focus or touches: taps go to whatever is underneath.
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or WindowManager.LayoutParams.FLAG_NOT_TOUCHABLE,
            PixelFormat.TRANSLUCENT,
        ).apply {
            gravity = Gravity.TOP or Gravity.END
            x = context.dp(8)
            y = context.dp(2)
            // See-through, and at most 0.8: from Android 12 a more opaque overlay from another app
            // blocks touches to the app below it.
            alpha = 0.75f
        }
        return try {
            wm.addView(b, params)
            badge = b
            b
        } catch (e: RuntimeException) { // permission revoked meanwhile
            null
        }
    }
}
