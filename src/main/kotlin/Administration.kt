package dev.threadify

import com.codahale.metrics.*
import dev.inmo.krontab.builder.*
import io.github.flaxoos.ktor.server.plugins.taskscheduling.*
import io.github.flaxoos.ktor.server.plugins.taskscheduling.managers.lock.database.*
import io.github.flaxoos.ktor.server.plugins.taskscheduling.managers.lock.redis.*
import io.ktor.http.*
import io.ktor.serialization.gson.*
import io.ktor.serialization.kotlinx.json.*
import io.ktor.server.application.*
import io.ktor.server.metrics.dropwizard.*
import io.ktor.server.metrics.micrometer.*
import io.ktor.server.plugins.calllogging.*
import io.ktor.server.plugins.contentnegotiation.*
import io.ktor.server.plugins.cors.routing.*
import io.ktor.server.plugins.openapi.*
import io.ktor.server.plugins.swagger.*
import io.ktor.server.request.*
import io.ktor.server.response.*
import io.ktor.server.routing.*
import io.ktor.server.sse.*
import io.ktor.server.websocket.*
import io.ktor.sse.*
import io.ktor.websocket.*
import io.micrometer.prometheus.*
import java.sql.Connection
import java.sql.DriverManager
import java.time.Duration
import java.util.concurrent.TimeUnit
import kotlin.time.Duration.Companion.seconds
import org.jetbrains.exposed.sql.SchemaUtils
import org.jetbrains.exposed.sql.transactions.transaction
import org.slf4j.event.*

fun Application.configureAdministration() {
    // Skip TaskScheduling configuration in test environments to avoid Redis connection issues
    val isTestEnvironment = environment.config.propertyOrNull("ktor.environment")?.getString() == "test"
    
    if (!isTestEnvironment) {
        install(TaskScheduling){
            // Choose task manager config based on your chosen task manager dependencies
            redis { // <-- given no name, this will be the default manager
                connectionPoolInitialSize = 1
                host = "localhost"
                port = 6379
                username = "my_username"
                password = "my_password"
                connectionAcquisitionTimeoutMs = 1_000
                lockExpirationMs = 60_000
            }
        
            task { // if no taskManagerName is provided, the task would be assigned to the default manager
                name = "My task"
                task = { taskExecutionTime ->
                    log.info("My task is running: $taskExecutionTime")
                }
                kronSchedule = {
                    seconds {
                        from(0).every(12)
                    }
                    minutes {
                        from(10).every(30)
                    }
                }
                concurrency = 2
            }
        }
    }
}
