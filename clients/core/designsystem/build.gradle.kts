plugins {
    id("pp.kmp.compose")
}

kotlin {
    sourceSets.commonMain.dependencies {
        api(project(":core:model"))
    }
    // Both apps run on the JVM: java.time formatting is written once and compiled into each.
    sourceSets.androidMain { kotlin.srcDir("src/jvmShared/kotlin") }
    sourceSets.getByName("desktopMain") { kotlin.srcDir("src/jvmShared/kotlin") }
}
