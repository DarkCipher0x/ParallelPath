pluginManagement {
    includeBuild("build-logic")
    repositories {
        google {
            content {
                includeGroupByRegex("com\\.android.*")
                includeGroupByRegex("com\\.google.*")
                includeGroupByRegex("androidx.*")
            }
        }
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google {
            content {
                includeGroupByRegex("com\\.android.*")
                includeGroupByRegex("com\\.google.*")
                includeGroupByRegex("androidx.*")
            }
        }
        mavenCentral()
        // Build tools for the web app (Kotlin/Wasm): Node.js and Binaryen, fetched by the Kotlin Gradle plugin.
        ivy("https://nodejs.org/dist") {
            name = "Node.js"
            patternLayout { artifact("v[revision]/[artifact](-v[revision]-[classifier]).[ext]") }
            metadataSources { artifact() }
            content { includeModule("org.nodejs", "node") }
        }
        ivy("https://github.com/WebAssembly/binaryen/releases/download") {
            name = "Binaryen"
            patternLayout { artifact("version_[revision]/binaryen-version_[revision]-[classifier].[ext]") }
            metadataSources { artifact() }
            content { includeModule("com.github.webassembly", "binaryen") }
        }
        ivy("https://github.com/yarnpkg/yarn/releases/download") {
            name = "Yarn"
            patternLayout { artifact("v[revision]/[artifact](-v[revision]).[ext]") }
            metadataSources { artifact() }
            content { includeModule("com.yarnpkg", "yarn") }
        }
    }
}

rootProject.name = "ParallelPathClients"

// Dependencies point one way only:  apps -> shared -> feature:* -> core:*.  A feature never imports another feature.
// All cryptography lives in the Go engine (../engine); these modules only ever see plain data.

include(":core:model")        // the data the engine returns (messages, spaces, audit rows)
include(":core:engine")       // typed calls to the engine over a transport each app supplies
include(":core:designsystem") // the ParallelPath theme (dark, light, high contrast) and shared components

include(":feature:onboarding") // connect to a server, confirm its fingerprint, create keys, unlock
include(":feature:chat")       // family spaces list, conversation, reactions, edits/retractions with reasons
include(":feature:family")     // members, invitations, approval by safety number, removal
include(":feature:audit")      // the verified audit log
include(":feature:calendar")   // parenting-time schedule, events, change requests and check-ins
include(":feature:expenses")   // shared costs, answers, payments and balances
include(":feature:files")      // documents: folders, versioned files, who opened what
include(":feature:records")    // the info bank and the private journal

include(":shared")             // App() root: navigation and state
include(":apps:android")       // Android shell: Activity, gomobile engine, photo picker
include(":apps:desktop")       // Windows shell: window, engine process, file dialogs, packaging
include(":apps:web")           // web app: the same UI in the browser (Kotlin/Wasm) beside the engine in WebAssembly
