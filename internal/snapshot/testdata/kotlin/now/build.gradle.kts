plugins {
    kotlin("jvm") version "2.2.0"
}

dependencies {
    implementation(platform("io.ktor:ktor-bom:3.2.0"))
    implementation("io.ktor:ktor-client-core")
    implementation(libs.kotlinx.coroutines.core)
    api(project(":shared"))
    runtimeOnly("org.slf4j:slf4j-simple:2.0.17") {
        because("logs to the console")
    }
    compileOnly("org.jetbrains:annotations:26.0.2")
    testImplementation(kotlin("test"))
}
