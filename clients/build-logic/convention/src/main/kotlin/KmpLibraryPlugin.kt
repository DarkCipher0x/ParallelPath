import com.android.build.gradle.LibraryExtension
import org.gradle.api.JavaVersion
import org.gradle.api.Plugin
import org.gradle.api.Project
import org.gradle.api.artifacts.VersionCatalogsExtension
import org.gradle.kotlin.dsl.configure
import org.gradle.kotlin.dsl.getByType
import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import org.jetbrains.kotlin.gradle.dsl.KotlinMultiplatformExtension

/**
 * `pp.kmp.library`: a Kotlin Multiplatform module built for Android, desktop (Windows) and the web (Kotlin/Wasm in
 * the browser). macOS/iOS targets are added later from the same code.
 *
 * - The Android namespace comes from the module path (":feature:chat" -> systems.darkcipher.parallelpath.feature.chat).
 * - Warnings are errors.
 */
class KmpLibraryPlugin : Plugin<Project> {
    override fun apply(target: Project) = with(target) {
        pluginManager.apply("org.jetbrains.kotlin.multiplatform")
        pluginManager.apply("com.android.library")

        val libs = extensions.getByType<VersionCatalogsExtension>().named("libs")

        extensions.configure<KotlinMultiplatformExtension> {
            jvmToolchain(21)
            androidTarget { compilerOptions { jvmTarget.set(JvmTarget.JVM_21) } }
            jvm("desktop")
            @OptIn(org.jetbrains.kotlin.gradle.ExperimentalWasmDsl::class)
            wasmJs { browser() }
            compilerOptions {
                allWarningsAsErrors.set(true)
                freeCompilerArgs.add("-Xexpect-actual-classes")
            }
            sourceSets.getByName("commonMain").dependencies {
                implementation(libs.findLibrary("kotlinx-coroutines-core").get())
            }
            sourceSets.getByName("commonTest").dependencies {
                implementation(kotlin("test"))
                implementation(libs.findLibrary("kotlinx-coroutines-test").get())
            }
        }

        extensions.configure<LibraryExtension> {
            namespace = "systems.darkcipher.parallelpath" + path.replace(':', '.').replace('-', '_')
            compileSdk = 36
            defaultConfig { minSdk = 29 }
            compileOptions {
                sourceCompatibility = JavaVersion.VERSION_21
                targetCompatibility = JavaVersion.VERSION_21
            }
        }
    }
}
