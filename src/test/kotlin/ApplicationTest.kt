package dev.threadify

import io.ktor.client.request.*
import io.ktor.http.*
import io.ktor.server.config.*
import io.ktor.server.testing.*
import kotlin.test.Test
import kotlin.test.assertEquals

class ApplicationTest {

    @Test
    fun testRoot() = testApplication {
        environment {
            config = MapApplicationConfig(
                "ktor.environment" to "test",
                "postgres.url" to "jdbc:h2:mem:test;DB_CLOSE_DELAY=-1;MODE=PostgreSQL",
                "postgres.user" to "sa",
                "postgres.password" to "",
                "queue.master.host" to "localhost",
                "queue.master.port" to "6379",
                "queue.master.password" to "test",
                "queue.pool.maxSize" to "8",
                "queue.pool.timeout" to "500",
                "queue.ttl.default" to "7776000",
                "jwt.secret" to "test-secret-key-for-testing-min-32-characters-long",
                "jwt.issuer" to "threadify-test",
                "jwt.audience" to "threadify-test-users",
                "jwt.realm" to "Threadify Test API",
                "jwt.expirationMs" to "3600000"
            )
        }
        
        application {
            module()
        }
        client.get("/v1/").apply {
            assertEquals(HttpStatusCode.OK, status)
        }
    }

}
