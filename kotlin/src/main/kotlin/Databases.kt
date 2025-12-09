package dev.threadify

import io.ktor.server.application.*
import java.sql.Connection
import java.sql.DriverManager
import io.lettuce.core.*
import io.lettuce.core.api.StatefulRedisConnection
import io.lettuce.core.protocol.ProtocolVersion
import dev.threadify.Utilities.GlobalServices
import dev.threadify.Schemas.DBModels.Contracts
import dev.threadify.Schemas.DBModels.ContractVersions
import org.jetbrains.exposed.sql.Database
import org.jetbrains.exposed.sql.SchemaUtils
import org.jetbrains.exposed.sql.transactions.transaction

fun Application.configureDatabases() {
    // Connect to Valkey/Redis - skip if in test environment and connection fails
    try {
        GlobalServices.queueServer = connectToValkey()
        println("Connected Successfully to Valkey")
    } catch (e: Exception) {
        if (environment.config.propertyOrNull("ktor.environment")?.getString() == "test") {
            println("Warning: Valkey/Redis connection failed in test environment - continuing without it")
        } else {
            throw e
        }
    }
    
    GlobalServices.persistedServer = connectToPostgres()
    println("Connected Successfully to Postgres")

    // Create database tables
    initializeDatabaseTables()
}

/**
 * Makes a connection to a Postgres database and initializes Exposed ORM.
 *
 * In order to connect to your running Postgres process,
 * please specify the following parameters in your configuration file:
 * - postgres.url -- Url of your running database process.
 * - postgres.user -- Username for database connection
 * - postgres.password -- Password for database connection
 *
 * @return [Database] Exposed database instance
 * */
fun Application.connectToPostgres(): Database {
    Class.forName("org.postgresql.Driver")
    val url = environment.config.property("postgres.url").getString()
    val user = environment.config.property("postgres.user").getString()
    val password = environment.config.property("postgres.password").getString()

    // Initialize and return Exposed Database connection
    return Database.connect(
        url = url,
        driver = "org.postgresql.Driver",
        user = user,
        password = password
    )
}

/**
 * Initializes all database tables using Exposed ORM SchemaUtils.
 * Creates tables if they don't exist based on the defined model classes.
 */
fun Application.initializeDatabaseTables() {
    transaction {
        // SchemaUtils.drop(Contracts, ContractVersions)
        SchemaUtils.create(Contracts, ContractVersions)
        log.info("Database tables created successfully from Exposed models")
    }
}

/**
 * Makes a connection to a Valkey database.
 *
 * In order to connect to your running Valkey process,
 * please specify the following parameters in your configuration file:
 * - queue.master.host -- Url of your running database process.
 * - queue.master.port -- Username for database connection
 * - queue.master.password -- Password for database connection
 *
 * @return [Connection] that represent connection to the database. Please, don't forget to close this connection when
 * your application shuts down by calling [Connection.close]
 * */
fun Application.connectToValkey(): StatefulRedisConnection<String, String> {
    val host = environment.config.property("queue.master.host").getString()
    val port = environment.config.property("queue.master.port").getString().toInt()
    val password = environment.config.property("queue.master.password").getString()
    
   // Master URI
   val masterUri = RedisURI.Builder.redis(host, port).apply {
        if (!password.isNullOrEmpty()) {
            withPassword(password.toCharArray())
        }
    }.build()

    // Create master client with RESP2 protocol
    val redisClient = RedisClient.create(masterUri)
    redisClient?.setOptions(io.lettuce.core.ClientOptions.builder()
        .protocolVersion(ProtocolVersion.RESP2)
        .autoReconnect(true)
        .build())

    // Create simple connections (no pooling)
    val masterConnection = redisClient.connect()
    
    return masterConnection
}
