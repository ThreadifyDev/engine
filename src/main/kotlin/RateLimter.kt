package dev.threadify

import io.ktor.http.*
import io.ktor.server.application.*
import io.ktor.server.plugins.ratelimit.*
import io.ktor.server.plugins.statuspages.*
import io.ktor.server.response.*
import io.ktor.server.routing.*
import kotlin.time.Duration.Companion.seconds

fun Application.configureRateLimiting() {
  install(RateLimit) {
    register {
        rateLimiter(limit = 10000, refillPeriod = 60.seconds)
    }
    register(RateLimitName("protected")) {
        rateLimiter(limit = 1000, refillPeriod = 60.seconds)
    }
   }
}