import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "io.github.dewil95.netmerge"
    compileSdk = 36

    defaultConfig {
        applicationId = "io.github.dewil95.netmerge"
        minSdk = 26
        targetSdk = 36
        // Each CI build counts up, so Android always sees a newer build as an upgrade.
        versionCode = System.getenv("GITHUB_RUN_NUMBER")?.toIntOrNull() ?: 1
        versionName = "0.1.0 (build $versionCode)"
    }

    signingConfigs {
        // A fixed debug key (Android's standard debug passwords), so every CI build installs over
        // the previous one. Debug builds only; publishing needs a real, private key.
        getByName("debug") {
            storeFile = file("debug.keystore")
            storePassword = "android"
            keyAlias = "androiddebugkey"
            keyPassword = "android"
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
    }
}

dependencies {
    // The Go core, built by scripts/build-core.sh (gomobile bind of core/mobile).
    implementation(files("libs/netmerge-core.aar"))
    testImplementation("junit:junit:4.13.2")
}
