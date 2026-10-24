import org.jetbrains.kotlin.gradle.ExperimentalWasmDsl

// The web app: the same Compose UI as the Windows and Android apps, compiled to WebAssembly and served by ppserver.
// The Go engine runs beside it in the page (engine/cmd/ppweb), so all cryptography happens in the browser.
plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.compose.multiplatform)
    alias(libs.plugins.compose.compiler)
    alias(libs.plugins.kotlin.serialization)
}

kotlin {
    @OptIn(ExperimentalWasmDsl::class)
    wasmJs {
        outputModuleName.set("parallelpath")
        browser { commonWebpackConfig { outputFileName = "parallelpath.js" } }
        binaries.executable()
    }
    compilerOptions {
        allWarningsAsErrors.set(true)
        optIn.add("kotlin.js.ExperimentalWasmJsInterop")
    }
    sourceSets.getByName("wasmJsMain").dependencies {
        implementation(project(":shared"))
        implementation(compose.runtime)
        implementation(compose.foundation)
        implementation(compose.material3)
        implementation(compose.ui)
        implementation(compose.components.resources)
    }
}

// Noto Color Emoji (SIL Open Font License, NotoColorEmoji-OFL.txt): the browser canvas has no emoji font of its own.
compose.resources {
    packageOfResClass = "systems.darkcipher.parallelpath.web.res"
    publicResClass = false
    generateResClass = always
}
