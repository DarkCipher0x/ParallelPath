plugins {
    `kotlin-dsl`
}

kotlin { jvmToolchain(21) }

dependencies {
    compileOnly(libs.gradle.android)
    compileOnly(libs.gradle.kotlin)
    compileOnly(libs.gradle.compose)
    compileOnly(libs.gradle.compose.compiler)
}

gradlePlugin {
    plugins {
        register("kmpLibrary") {
            id = "pp.kmp.library"
            implementationClass = "KmpLibraryPlugin"
        }
        register("kmpCompose") {
            id = "pp.kmp.compose"
            implementationClass = "KmpComposePlugin"
        }
    }
}
