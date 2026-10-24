plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.compose.compiler)
}

android {
    namespace = "systems.darkcipher.parallelpath"
    compileSdk = 36

    defaultConfig {
        applicationId = "systems.darkcipher.parallelpath"
        minSdk = 29
        targetSdk = 36
        versionCode = 14
        versionName = "0.0.1"
    }

    signingConfigs {
        // Release signing comes from the environment; the keystore never lives in the repo.
        create("release") {
            System.getenv("PP_KEYSTORE")?.let { path ->
                storeFile = file(path)
                storePassword = System.getenv("PP_KEYSTORE_PASS")
                keyAlias = System.getenv("PP_KEY_ALIAS")
                keyPassword = System.getenv("PP_KEY_PASS")
            }
        }
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (System.getenv("PP_KEYSTORE") != null) signingConfig = signingConfigs.getByName("release")
            ndk { debugSymbolLevel = "NONE" }
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }
    kotlin { jvmToolchain(21) }
    buildFeatures { compose = true }
    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "x86_64")
            isUniversalApk = false
        }
    }
    packaging {
        resources.excludes += setOf("META-INF/*.version", "META-INF/LICENSE*", "META-INF/NOTICE*", "DebugProbesKt.bin", "kotlin-tooling-metadata.json")
    }
    dependenciesInfo {
        includeInApk = false
        includeInBundle = false
    }
}

dependencies {
    implementation(project(":shared"))
    // The Go engine (engine/mobile via gomobile; tools/build.ps1 rebuilds it). All cryptography is in here.
    implementation(files("../../libs/ppengine.aar"))
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.process)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.compose.foundation)
    implementation(libs.compose.runtime)
}
