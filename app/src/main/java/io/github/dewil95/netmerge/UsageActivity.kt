package io.github.dewil95.netmerge

import android.app.Activity
import android.content.ActivityNotFoundException
import android.content.Intent
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.net.Uri
import android.os.Bundle
import android.provider.Settings
import android.text.format.Formatter
import android.view.Gravity
import android.view.View
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import kotlin.math.roundToInt

/**
 * Everyday Wi-Fi and mobile data use: the total for the period, the split between the two, a bar per
 * day (or per 2 hours today), and the same split per app. One period switch at the top drives all of it.
 */
class UsageActivity : Activity() {

    private lateinit var p: Palette
    private lateinit var content: LinearLayout
    private lateinit var chips: Map<UsageData.Period, TextView>
    private var period = UsageData.Period.WEEK
    private var report: UsageData.Report? = null
    private var generation = 0
    private var showNumbers = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        p = Palette.of(this)
        window.decorView.setBackgroundColor(p.surface)

        val column = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(20), dp(24), dp(20), dp(24))
        }
        column.addView(text("Usage", 24f, p.ink, bold = true))
        column.addView(text("Wi-Fi and mobile data used on this phone, from Android's own counters.", 14f, p.ink2), below(4))

        // One period switch above everything it controls.
        val row = LinearLayout(this).apply { orientation = LinearLayout.HORIZONTAL }
        chips = UsageData.Period.entries.associateWith { per ->
            text(per.title, 14f, p.ink).apply {
                setPadding(dp(14), dp(8), dp(14), dp(8))
                setOnClickListener { choose(per) }
                row.addView(this, LinearLayout.LayoutParams(WRAP_CONTENT, WRAP_CONTENT).apply { marginEnd = dp(8) })
            }
        }
        column.addView(row, below(16))

        content = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        column.addView(content, below(8))

        setContentView(ScrollView(this).apply {
            clipToPadding = false
            addView(column)
            padForSystemBars()
        })
        renderChips()
    }

    override fun onResume() {
        super.onResume()
        load() // also picks up "Usage access" granted in Settings meanwhile
    }

    private fun choose(per: UsageData.Period) {
        if (per == period) return
        period = per
        renderChips()
        load()
    }

    private fun renderChips() {
        for ((per, chip) in chips) {
            val on = per == period
            chip.setTypeface(null, if (on) Typeface.BOLD else Typeface.NORMAL)
            chip.setTextColor(if (on) p.ink else p.ink2)
            chip.background = GradientDrawable().apply {
                cornerRadius = dp(16).toFloat()
                if (on) setColor(p.wash) else setStroke(dp(1), p.grid)
            }
        }
    }

    private fun load() {
        if (!UsageData.hasAccess(this)) {
            renderNoAccess()
            return
        }
        val gen = ++generation
        val per = period
        content.alpha = 0.5f // keep the previous numbers on screen while loading, no flash
        if (content.childCount == 0) content.addView(text("Loading…", 14f, p.ink2))
        Thread {
            val r = UsageData.load(this, per)
            runOnUiThread {
                if (gen == generation && !isDestroyed) {
                    report = r
                    render(r)
                }
            }
        }.start()
    }

    private fun renderNoAccess() {
        content.alpha = 1f
        content.removeAllViews()
        content.addView(
            text(
                "To show this, net-merge needs \"Usage access\". Android keeps these counters for every " +
                    "app; nothing leaves your phone.",
                15f, p.ink,
            ),
            below(8),
        )
        content.addView(
            android.widget.Button(this).apply {
                text = "Allow usage access"
                setOnClickListener { openUsageAccessSettings() }
            },
            below(12),
        )
    }

    private fun openUsageAccessSettings() {
        val forApp = Intent(Settings.ACTION_USAGE_ACCESS_SETTINGS, Uri.parse("package:$packageName"))
        try {
            startActivity(forApp)
        } catch (e: ActivityNotFoundException) {
            startActivity(Intent(Settings.ACTION_USAGE_ACCESS_SETTINGS))
        }
    }

    private fun render(r: UsageData.Report) {
        content.alpha = 1f
        content.removeAllViews()

        // The one big number, then the split.
        val caption = when (r.period) {
            UsageData.Period.TODAY -> "used today"
            UsageData.Period.WEEK -> "used in the last 7 days"
            UsageData.Period.MONTH -> "used in the last 30 days"
        }
        content.addView(text(size(r.total), 48f, p.ink, bold = true), below(8))
        content.addView(text(caption, 14f, p.ink2))

        val tiles = LinearLayout(this).apply { orientation = LinearLayout.HORIZONTAL }
        tiles.addView(tile("Wi-Fi", r.wifi, r.total, p.wifi), LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f))
        tiles.addView(View(this), LinearLayout.LayoutParams(dp(12), 1))
        tiles.addView(tile("Mobile data", r.mobile, r.total, p.mobile), LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f))
        content.addView(tiles, below(16))

        // Chart card: legend, the selected bar's numbers, the bars, and a numbers table on demand.
        val card = card()
        card.addView(legend())
        val readout = text("", 14f, p.ink2)
        card.addView(readout, below(8))
        val chart = UsageBarChart(this, p).apply {
            onSelect = { i ->
                val s = r.slots[i]
                readout.text = "${s.label}  ·  Wi-Fi ${size(s.wifi)}  ·  Mobile ${size(s.mobile)}"
            }
            slots = r.slots
        }
        card.addView(chart, below(8))
        val table = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        val toggle = text("", 14f, p.ink, bold = true).apply { setPadding(0, dp(12), 0, dp(4)) }
        fun renderTable() {
            toggle.text = if (showNumbers) "Hide numbers" else "Show numbers"
            table.removeAllViews()
            if (!showNumbers) return
            table.addView(tableRow("", -1, -1))
            for (s in r.slots.asReversed()) {
                table.addView(tableRow(s.label, s.wifi, s.mobile))
            }
        }
        toggle.setOnClickListener {
            showNumbers = !showNumbers
            renderTable()
        }
        renderTable()
        card.addView(toggle)
        card.addView(table)
        content.addView(card, below(16))

        // Per app.
        content.addView(text("By app", 18f, p.ink, bold = true), below(24))
        if (r.apps.isEmpty()) {
            content.addView(text("No data use recorded in this period.", 14f, p.ink2), below(8))
        }
        val max = r.apps.maxOfOrNull { it.total } ?: 1L
        for (a in r.apps) content.addView(appRow(a, max), below(4))

        content.addView(
            text(
                "Counts all traffic on this phone, with or without net-merge. While merging, Android " +
                    "splits each app's traffic over Wi-Fi and mobile data. Times follow Android's 2-hour " +
                    "buckets. On Android 8 and 9, mobile data may show as zero.",
                12f, p.muted,
            ),
            below(24),
        )
    }

    private fun tile(name: String, bytes: Long, total: Long, color: Int): View {
        val t = card()
        t.addView(swatchLabel(name, color))
        t.addView(text(size(bytes), 22f, p.ink, bold = true), below(4))
        val share = if (total > 0) (bytes * 100.0 / total).roundToInt() else 0
        t.addView(text("$share% of total", 13f, p.ink2))
        return t
    }

    private fun legend(): View = LinearLayout(this).apply {
        orientation = LinearLayout.HORIZONTAL
        addView(swatchLabel("Wi-Fi", p.wifi))
        addView(View(context), LinearLayout.LayoutParams(dp(16), 1))
        addView(swatchLabel("Mobile data", p.mobile))
    }

    /** A colored square beside ink text: identity from the mark, never from colored text. */
    private fun swatchLabel(name: String, color: Int): View = LinearLayout(this).apply {
        orientation = LinearLayout.HORIZONTAL
        gravity = Gravity.CENTER_VERTICAL
        addView(
            View(context).apply {
                background = GradientDrawable().apply {
                    cornerRadius = dp(2).toFloat()
                    setColor(color)
                }
            },
            LinearLayout.LayoutParams(dp(10), dp(10)).apply { marginEnd = dp(6) },
        )
        addView(text(name, 13f, p.ink2))
    }

    /** A row of the numbers table; negative values make the header. */
    private fun tableRow(label: String, wifi: Long, mobile: Long): View = LinearLayout(this).apply {
        orientation = LinearLayout.HORIZONTAL
        setPadding(0, dp(4), 0, dp(4))
        val header = wifi < 0
        val ink = if (header) p.muted else p.ink
        addView(text(label, 13f, p.ink2), LinearLayout.LayoutParams(0, WRAP_CONTENT, 1.4f))
        addView(
            text(if (header) "Wi-Fi" else size(wifi), 13f, ink).apply { gravity = Gravity.END },
            LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f),
        )
        addView(
            text(if (header) "Mobile" else size(mobile), 13f, ink).apply { gravity = Gravity.END },
            LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f),
        )
    }

    private fun appRow(a: UsageData.AppUsage, biggest: Long): View = LinearLayout(this).apply {
        orientation = LinearLayout.HORIZONTAL
        gravity = Gravity.CENTER_VERTICAL
        setPadding(0, dp(8), 0, dp(8))
        addView(
            ImageView(context).apply { a.icon?.let { setImageDrawable(it) } },
            LinearLayout.LayoutParams(dp(32), dp(32)).apply { marginEnd = dp(12) },
        )
        val middle = LinearLayout(context).apply {
            orientation = LinearLayout.VERTICAL
            val head = LinearLayout(context).apply {
                orientation = LinearLayout.HORIZONTAL
                addView(text(a.label, 15f, p.ink).apply { maxLines = 1 }, LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f))
                addView(text(size(a.total), 15f, p.ink, bold = true))
            }
            addView(head)
            addView(SplitBar(context, p).apply { wifi = a.wifi; mobile = a.mobile; max = biggest }, below(6))
            addView(text("Wi-Fi ${size(a.wifi)}  ·  Mobile ${size(a.mobile)}", 12f, p.ink2), below(4))
        }
        addView(middle, LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f))
    }

    private fun card(): LinearLayout = LinearLayout(this).apply {
        orientation = LinearLayout.VERTICAL
        setPadding(dp(14), dp(12), dp(14), dp(12))
        background = GradientDrawable().apply {
            cornerRadius = dp(12).toFloat()
            setColor(p.surface)
            setStroke(dp(1), p.grid)
        }
    }

    private fun size(bytes: Long): String = Formatter.formatShortFileSize(this, bytes)

    private fun text(s: String, sizeSp: Float, color: Int, bold: Boolean = false) = TextView(this).apply {
        text = s
        textSize = sizeSp
        setTextColor(color)
        if (bold) setTypeface(typeface, Typeface.BOLD)
    }

    private fun below(topDp: Int) =
        LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT).apply { topMargin = dp(topDp) }
}
