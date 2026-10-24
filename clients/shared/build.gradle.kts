plugins {
    id("pp.kmp.compose")
}

kotlin {
    sourceSets.commonMain.dependencies {
        api(project(":core:engine"))
        api(project(":core:designsystem"))
        implementation(project(":feature:onboarding"))
        implementation(project(":feature:chat"))
        implementation(project(":feature:family"))
        implementation(project(":feature:audit"))
        implementation(project(":feature:calendar"))
        implementation(project(":feature:expenses"))
        implementation(project(":feature:files"))
        implementation(project(":feature:records"))
    }
}
