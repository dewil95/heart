package io.github.dewil95.netmerge

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Path
import android.graphics.RectF
import android.text.format.Formatter
import android.util.TypedValue
import android.view.MotionEvent
import android.view.View
import kotlin.math.ceil
import kotlin.math.floor
import kotlin.math.log10
import kotlin.math.pow

/**
 * Stacked bars, Wi-Fi at the baseline and mobile data on top, one per slot. Tapping or dragging
 * selects a bar; the others fade so the selected one reads at a glance. Marks follow the data-viz
 * spec: bars at most 24dp wide, a 2dp surface gap between segments, 4dp rounded data-end and a
 * square baseline, hairline grid, labels in the muted ink.
 */
internal class UsageBarChart(context: Context, private val p: Palette) : View(context) {

    var slots: List<UsageData.Slot> = emptyList()
        set(v) {
            field = v
            selected = -1
            select(v.lastIndex) // the latest slot is shown by default, no tap needed
            invalidate()
        }

    var onSelect: ((Int) -> Unit)? = null
    private var selected = -1

    private val fill = Paint(Paint.ANTI_ALIAS_FLAG)
    private val line = Paint().apply { strokeWidth = 1f }
    private val label = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        color = p.muted
        textSize = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_SP, 11f, resources.displayMetrics)
    }
    private val path = Path()
    private val rect = RectF()
    private val plotHeight = context.dp(160)
    private val axisBand = context.dp(22)
    private var plotLeft = 0f
    private var slotWidth = 0f

    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        setMeasuredDimension(MeasureSpec.getSize(widthMeasureSpec), plotHeight + axisBand + context.dp(8))
    }

    override fun onDraw(canvas: Canvas) {
        if (slots.isEmpty()) return
        val top = context.dp(8).toFloat()
        val base = top + plotHeight
        val (max, step) = scale(slots.maxOf { it.wifi + it.mobile })

        // Y gridlines with their labels on the left; the gutter fits the widest label.
        val ticks = generateSequence(0L) { it + step }.takeWhile { it <= max }.toList()
        val tickText = ticks.map { if (it == 0L) "0" else Formatter.formatShortFileSize(context, it) }
        plotLeft = tickText.maxOf { label.measureText(it) } + context.dp(8)
        val right = width.toFloat()
        for ((i, t) in ticks.withIndex()) {
            val y = base - plotHeight * t / max.toFloat()
            line.color = if (t == 0L) p.baseline else p.grid
            canvas.drawLine(plotLeft, y, right, y, line)
            canvas.drawText(tickText[i], 0f, y + label.textSize / 3, label)
        }

        slotWidth = (right - plotLeft) / slots.size
        val barW = minOf(context.dp(24).toFloat(), slotWidth * 0.6f)
        val gap = context.dp(2).toFloat()
        val radius = context.dp(4).toFloat()
        label.textAlign = Paint.Align.CENTER
        for ((i, s) in slots.withIndex()) {
            val cx = plotLeft + slotWidth * (i + 0.5f)
            val l = cx - barW / 2
            val r = cx + barW / 2
            val dim = selected >= 0 && i != selected
            val wifiH = plotHeight * s.wifi / max.toFloat()
            val mobileH = plotHeight * s.mobile / max.toFloat()
            var y = base
            if (s.wifi > 0) {
                segment(canvas, l, y - wifiH, r, y, if (s.mobile > 0) 0f else radius, p.wifi, dim)
                y -= wifiH + gap
            }
            if (s.mobile > 0) segment(canvas, l, y - mobileH, r, y, radius, p.mobile, dim)
            s.axisLabel?.let { canvas.drawText(it, cx, base + axisBand - context.dp(6), label) }
        }
        label.textAlign = Paint.Align.LEFT
    }

    /** One stacked segment; only the top one gets the rounded data-end. Never thinner than 1dp. */
    private fun segment(c: Canvas, l: Float, t: Float, r: Float, b: Float, radius: Float, color: Int, dim: Boolean) {
        val top = minOf(t, b - context.dp(1))
        val rad = minOf(radius, (b - top) / 2, (r - l) / 2)
        fill.color = color
        fill.alpha = if (dim) 90 else 255
        rect.set(l, top, r, b)
        path.reset()
        path.addRoundRect(rect, floatArrayOf(rad, rad, rad, rad, 0f, 0f, 0f, 0f), Path.Direction.CW)
        c.drawPath(path, fill)
    }

    /** A "nice" axis top and step (1, 2 or 5 × 10ⁿ) giving 3–5 gridlines. */
    private fun scale(peak: Long): Pair<Long, Long> {
        if (peak <= 0) return 1_000_000L to 500_000L // empty: 0 and 1 MB
        val rough = peak / 4.0
        val mag = 10.0.pow(floor(log10(rough)))
        val step = listOf(1.0, 2.0, 5.0, 10.0).first { it * mag >= rough } * mag
        val top = ceil(peak / step) * step
        return top.toLong() to step.toLong()
    }

    private fun select(i: Int) {
        if (i == selected || i !in slots.indices) return
        selected = i
        onSelect?.invoke(i)
        contentDescription = slots[i].let {
            "${it.label}: Wi-Fi ${Formatter.formatShortFileSize(context, it.wifi)}, " +
                "mobile data ${Formatter.formatShortFileSize(context, it.mobile)}"
        }
        invalidate()
    }

    override fun onTouchEvent(e: MotionEvent): Boolean {
        if (slots.isEmpty() || slotWidth <= 0f) return false
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE -> {
                // The whole column is the hit target, not just the bar.
                select(((e.x - plotLeft) / slotWidth).toInt().coerceIn(0, slots.lastIndex))
                parent?.requestDisallowInterceptTouchEvent(true)
            }
            MotionEvent.ACTION_UP -> performClick()
        }
        return true
    }

    override fun performClick(): Boolean = super.performClick()
}

/**
 * One app's use as a thin horizontal bar: Wi-Fi then a 2dp gap then mobile data. Its length is the
 * app's total against the biggest app's, so the list reads as both ranking and split.
 */
internal class SplitBar(context: Context, private val p: Palette) : View(context) {
    var wifi = 0L
    var mobile = 0L
    var max = 1L

    private val fill = Paint(Paint.ANTI_ALIAS_FLAG)
    private val path = Path()
    private val rect = RectF()

    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        setMeasuredDimension(MeasureSpec.getSize(widthMeasureSpec), context.dp(6))
    }

    override fun onDraw(canvas: Canvas) {
        val total = wifi + mobile
        if (total <= 0 || max <= 0) return
        val h = height.toFloat()
        val len = maxOf(width * total.toFloat() / max, context.dp(2).toFloat())
        val gap = if (wifi > 0 && mobile > 0) context.dp(2).toFloat() else 0f
        val wifiLen = (len - gap) * wifi / total
        val rad = minOf(context.dp(4).toFloat(), h / 2)
        // Baseline (left) square, data-end (right) rounded.
        if (wifi > 0) bar(canvas, 0f, wifiLen, h, if (mobile > 0) 0f else rad, p.wifi)
        if (mobile > 0) bar(canvas, wifiLen + gap, len, h, rad, p.mobile)
    }

    private fun bar(c: Canvas, l: Float, r: Float, h: Float, rad: Float, color: Int) {
        fill.color = color
        rect.set(l, 0f, maxOf(r, l + 1f), h)
        path.reset()
        path.addRoundRect(rect, floatArrayOf(0f, 0f, rad, rad, rad, rad, 0f, 0f), Path.Direction.CW)
        c.drawPath(path, fill)
    }
}
