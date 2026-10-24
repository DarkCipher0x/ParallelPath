import org.jetbrains.compose.desktop.application.dsl.TargetFormat

plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.compose.multiplatform)
    alias(libs.plugins.compose.compiler)
}

kotlin {
    jvmToolchain(21)
    jvm("desktop")
    sourceSets.getByName("desktopMain").dependencies {
        implementation(project(":shared"))
        // Shots renders feature screens directly
        implementation(project(":feature:calendar"))
        implementation(project(":feature:expenses"))
        implementation(project(":feature:files"))
        implementation(project(":feature:records"))
        implementation(project(":feature:chat"))
        implementation(compose.desktop.currentOs)
        implementation(compose.material3)
        implementation(libs.kotlinx.coroutines.swing)
    }
}

// Development: render the real app to PNGs (see Shots.kt).  -PshotArgs="exe@@dataDir@@pass@@outDir@@freshDir"
tasks.register<JavaExec>("shots") {
    val main = kotlin.jvm("desktop").compilations.getByName("main")
    dependsOn(main.compileTaskProvider)
    mainClass.set("systems.darkcipher.parallelpath.desktop.ShotsKt")
    classpath = files(main.output.allOutputs, main.runtimeDependencyFiles)
    args = (project.findProperty("shotArgs") as String? ?: "").split("@@")
}

compose.desktop {
    application {
        mainClass = "systems.darkcipher.parallelpath.desktop.MainKt"
        // The Go engine (ppengine.exe) ships inside the app: resources/windows/ppengine.exe (tools/build.ps1 puts it there).
        nativeDistributions {
            appResourcesRootDir.set(project.layout.projectDirectory.dir("resources"))
            targetFormats(TargetFormat.Msi, TargetFormat.Exe)
            packageName = "ParallelPath"
            packageVersion = "0.0.1"
            description = "ParallelPath - post-quantum secure co-parenting"
            copyright = "DarkCipher0x at DarkCipher Systems - AGPL-3.0"
            vendor = "DarkCipher Systems"
            modules("java.naming", "jdk.crypto.ec")
            windows {
                iconFile.set(project.layout.projectDirectory.file("icon.ico"))
                menuGroup = "ParallelPath"
                shortcut = true
                perUserInstall = true
                upgradeUuid = "4c7b6a1e-6f2d-4d3b-9b8a-7e2f0c5d9a11"
            }
        }
        buildTypes.release.proguard { isEnabled.set(false) }
    }
}
