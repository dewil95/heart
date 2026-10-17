package io.github.dewil95.netmerge

import android.content.Context
import android.os.Build
import android.view.View
import android.view.WindowInsets

internal fun Context.dp(v: Int): Int = (v * resources.displayMetrics.density).toInt()

/** From Android 15 apps draw under the status and navigation bars; keep this view's content clear. */
internal fun View.padForSystemBars() {
    setOnApplyWindowInsetsListener { v, insets ->
        if (Build.VERSION.SDK_INT >= 30) {
            val bars = insets.getInsets(WindowInsets.Type.systemBars() or WindowInsets.Type.displayCutout())
            v.setPadding(bars.left, bars.top, bars.right, bars.bottom)
        } else {
            @Suppress("DEPRECATION")
            v.setPadding(
                insets.systemWindowInsetLeft,
                insets.systemWindowInsetTop,
                insets.systemWindowInsetRight,
                insets.systemWindowInsetBottom,
            )
        }
        insets
    }
}
