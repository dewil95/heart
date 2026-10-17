package io.github.dewil95.netmerge

import android.content.Context

/** Apps the user chose not to merge (package names). They skip the VPN entirely. */
object ExcludedApps {
    private const val PREFS = "settings"
    private const val KEY = "excluded_apps"

    fun get(context: Context): Set<String> =
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getStringSet(KEY, null)?.toSet() ?: emptySet()

    fun set(context: Context, packages: Set<String>) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putStringSet(KEY, HashSet(packages))
            .apply()
    }
}
