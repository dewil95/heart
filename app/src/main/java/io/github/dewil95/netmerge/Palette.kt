package io.github.dewil95.netmerge

import android.content.Context
import android.content.res.Configuration

/**
 * Chart colors for light and dark mode. Wi-Fi and mobile data are the first two slots of the
 * data-viz reference palette (blue, orange), validated per mode for color-vision deficiencies
 * against these surfaces. Text always uses the ink colors, never a series color.
 */
internal class Palette(
    val wifi: Int,
    val mobile: Int,
    val surface: Int,
    val ink: Int,
    val ink2: Int,
    val muted: Int,
    val grid: Int,
    val baseline: Int,
    val wash: Int, // selected chip / card background
) {
    companion object {
        private val LIGHT = Palette(
            wifi = 0xFF2A78D6.toInt(), mobile = 0xFFEB6834.toInt(),
            surface = 0xFFFCFCFB.toInt(), ink = 0xFF0B0B0B.toInt(), ink2 = 0xFF52514E.toInt(),
            muted = 0xFF898781.toInt(), grid = 0xFFE1E0D9.toInt(), baseline = 0xFFC3C2B7.toInt(),
            wash = 0x140B0B0B,
        )
        private val DARK = Palette(
            wifi = 0xFF3987E5.toInt(), mobile = 0xFFD95926.toInt(),
            surface = 0xFF1A1A19.toInt(), ink = 0xFFF0EFEC.toInt(), ink2 = 0xFFC3C2B7.toInt(),
            muted = 0xFF898781.toInt(), grid = 0xFF2C2C2A.toInt(), baseline = 0xFF383835.toInt(),
            wash = 0x1AFFFFFF,
        )

        fun of(context: Context): Palette {
            val night = context.resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK
            return if (night == Configuration.UI_MODE_NIGHT_YES) DARK else LIGHT
        }
    }
}
