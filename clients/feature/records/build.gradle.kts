plugins {
    id("pp.kmp.compose")
}

kotlin {
    sourceSets.all { languageSettings.optIn("kotlin.time.ExperimentalTime") }
    sourceSets.commonMain.dependencies {
        implementation(project(":core:engine"))
        implementation(project(":core:designsystem"))
    }
}
