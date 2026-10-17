package io.github.dewil95.netmerge

import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Typeface
import android.graphics.drawable.Drawable
import android.os.Build
import android.os.Bundle
import android.text.Editable
import android.text.TextWatcher
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.widget.BaseAdapter
import android.widget.CheckBox
import android.widget.EditText
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.TextView
import mobile.Mobile

/**
 * Picks apps that don't merge. A ticked app skips the VPN and uses one network, as if net-merge
 * weren't installed: the fix for an app that breaks or keeps logging out when merged.
 */
class AppsActivity : Activity() {

    private class App(val pkg: String, val label: String, val icon: Drawable)

    private class Holder(val icon: ImageView, val label: TextView, val pkg: TextView, val check: CheckBox)

    private val excluded = mutableSetOf<String>()
    private var applied: Set<String> = emptySet() // what the running VPN was built with
    private var all: List<App> = emptyList()
    private var shown: List<App> = emptyList()
    private var query = ""
    private val adapter = AppAdapter()
    private lateinit var summary: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        excluded += ExcludedApps.get(this)
        applied = excluded.toSet()

        val column = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(20), dp(24), dp(20), 0)
        }
        column.addView(text("Apps that don't merge", 24f).apply { setTypeface(typeface, Typeface.BOLD) })
        column.addView(
            text(
                "Ticked apps skip net-merge and use one network as usual. Tick an app if it " +
                    "doesn't work or keeps logging you out while merging.",
                14f,
            ).apply { alpha = 0.7f },
            below(8),
        )
        val search = EditText(this).apply {
            hint = "Search apps"
            isSingleLine = true
            addTextChangedListener(object : TextWatcher {
                override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
                override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
                override fun afterTextChanged(s: Editable?) {
                    query = s?.toString().orEmpty()
                    refresh()
                }
            })
        }
        column.addView(search, below(16))
        summary = text("Loading apps…", 14f)
        column.addView(summary, below(8))
        val list = ListView(this).apply {
            adapter = this@AppsActivity.adapter
            clipToPadding = false
        }
        column.addView(list, LinearLayout.LayoutParams(MATCH_PARENT, 0, 1f).apply { topMargin = dp(8) })
        column.padForSystemBars()
        setContentView(column)

        val first = excluded.toSet()
        Thread {
            val apps = loadApps(first)
            runOnUiThread {
                all = apps
                refresh()
            }
        }.start()
    }

    override fun onPause() {
        super.onPause()
        // Rebuilding the VPN restarts open connections, so do it once on leaving, not per tick.
        if (excluded != applied) {
            applied = excluded.toSet()
            if (MergeStatus.parse(Mobile.status()).running) {
                startService(MergeVpnService.intent(this, MergeVpnService.ACTION_RELOAD))
            }
        }
    }

    /** Apps with a launcher icon, ticked ones first. Slow (labels, icons): runs off the main thread. */
    private fun loadApps(ticked: Set<String>): List<App> {
        val pm = packageManager
        val launcher = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
        val infos = if (Build.VERSION.SDK_INT >= 33) {
            pm.queryIntentActivities(launcher, PackageManager.ResolveInfoFlags.of(0))
        } else {
            @Suppress("DEPRECATION")
            pm.queryIntentActivities(launcher, 0)
        }
        return infos.map { it.activityInfo.applicationInfo }
            .filter { it.packageName != packageName }
            .distinctBy { it.packageName }
            .map { App(it.packageName, it.loadLabel(pm).toString(), it.loadIcon(pm)) }
            .sortedWith(compareBy({ it.pkg !in ticked }, { it.label.lowercase() }))
    }

    private fun refresh() {
        val q = query.trim().lowercase()
        shown = if (q.isEmpty()) all else all.filter { q in it.label.lowercase() || q in it.pkg }
        adapter.notifyDataSetChanged()
        summary.text = when (excluded.size) {
            0 -> "Every app merges."
            1 -> "1 app doesn't merge."
            else -> "${excluded.size} apps don't merge."
        }
    }

    private fun toggle(app: App, check: CheckBox) {
        if (!excluded.remove(app.pkg)) excluded += app.pkg
        check.isChecked = app.pkg in excluded
        ExcludedApps.set(this, excluded)
        refresh()
    }

    private inner class AppAdapter : BaseAdapter() {
        override fun getCount(): Int = shown.size
        override fun getItem(position: Int): Any = shown[position]
        override fun getItemId(position: Int): Long = position.toLong()

        override fun getView(position: Int, convertView: View?, parent: ViewGroup): View {
            val row = convertView ?: newRow()
            val h = row.tag as Holder
            val app = shown[position]
            h.icon.setImageDrawable(app.icon)
            h.label.text = app.label
            h.pkg.text = app.pkg
            h.check.isChecked = app.pkg in excluded
            row.setOnClickListener { toggle(app, h.check) }
            return row
        }
    }

    private fun newRow(): View {
        val icon = ImageView(this)
        val label = text("", 16f)
        val pkg = text("", 12f).apply { alpha = 0.6f }
        val check = CheckBox(this).apply {
            isClickable = false // the whole row toggles
            isFocusable = false
        }
        val texts = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            addView(label)
            addView(pkg)
        }
        return LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(0, dp(10), 0, dp(10))
            addView(icon, LinearLayout.LayoutParams(dp(40), dp(40)))
            addView(texts, LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f).apply { marginStart = dp(16) })
            addView(check)
            tag = Holder(icon, label, pkg, check)
        }
    }

    private fun text(s: String, sizeSp: Float) = TextView(this).apply {
        text = s
        textSize = sizeSp
    }

    private fun below(topDp: Int) =
        LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT).apply { topMargin = dp(topDp) }
}
