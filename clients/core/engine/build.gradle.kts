plugins {
    id("pp.kmp.library")
    alias(libs.plugins.kotlin.serialization)
}

kotlin {
    sourceSets.commonMain.dependencies {
        api(project(":core:model"))
    }
}
