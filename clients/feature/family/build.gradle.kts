plugins {
    id("pp.kmp.compose")
}

kotlin {
    sourceSets.commonMain.dependencies {
        implementation(project(":core:engine"))
        implementation(project(":core:designsystem"))
    }
}
