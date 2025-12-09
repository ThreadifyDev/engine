package dev.threadify.Routers

import dev.threadify.getUserId
import dev.threadify.getClaim
import dev.threadify.Utilities.GlobalServices
import dev.threadify.Utilities.ContractValidator
import dev.threadify.Utilities.mapToHttpStatusCode
import dev.threadify.Services.ContractService
import io.ktor.server.application.*
import io.ktor.server.auth.*
import io.ktor.server.routing.*
import io.ktor.http.*
import io.ktor.server.response.*
import io.ktor.server.request.*

/**
 * Contracts routes with JWT authentication.
 * 
 * Technical details:
 * - Public routes (login) don't require authentication
 * - Protected routes use authenticate("auth-jwt") block
 * - User ID is extracted from JWT token via call.getUserId()
 * - Custom claims can be accessed via call.getClaim("claimName")
 */
fun Route.contracts() {
    // Public route - no authentication required
    // This endpoint generates a JWT token for testing
    val contractService = ContractService()
    post("/contracts/login") {
        val userId = call.receiveText()
        val authService = GlobalServices.authenticationService
        
        // Create JWT token with custom claims
        val token = authService.createToken(
            userId = userId,
            claims = mapOf(
                "role" to "user",
                "companyId" to "31d531c4-dce1-425c-8c09-86c4338369b4",
                "permissions" to listOf("read", "write")
            )
        )
        
        call.respond(
            HttpStatusCode.OK,
            mapOf(
                "token" to token,
                "userId" to userId,
                "message" to "Use this token in Authorization header as: Bearer <token>"
            )
        )
    }
    
    // Protected routes - require valid JWT token
    authenticate("auth-jwt") {
        // GET all contracts - requires authentication
        get("/contracts") {
            val userId = call.getUserId()
            val role = call.getClaim("role")
            
            call.respond(
                HttpStatusCode.OK,
                mapOf(
                    "message" to "Contracts list",
                    "authenticatedUser" to userId,
                    "userRole" to role
                )
            )
        }
        
        // GET specific contract by ID - requires authentication
        // Optional query parameter: version (to get specific version)
        get("/contracts/{id}") {
            val contractId = call.parameters["id"]
            val userId = call.getUserId()
            val companyId = call.getClaim("companyId").toString()
            val versionParam = call.request.queryParameters["version"]
            
            // Validate authentication
            if (userId.isNullOrBlank() || companyId.isNullOrBlank()) {
                call.respond(
                    HttpStatusCode.Unauthorized,
                    mapOf("message" to "Unauthorized")
                )
                return@get
            }
            
            // Validate contract ID
            if (contractId.isNullOrBlank()) {
                call.respond(
                    HttpStatusCode.BadRequest,
                    mapOf("message" to "Contract ID is required")
                )
                return@get
            }
            
            // Parse version if provided
            val version = versionParam?.toIntOrNull()
            
            val result = contractService.getContract(contractId, companyId, version)
            call.respond(mapToHttpStatusCode(result.first), result.second)
        }
        
        // POST create new contract - requires authentication
        post("/contracts") {
            val contentType = call.request.contentType()

            val userId = call.getUserId()
            val companyId = call.getClaim("companyId").toString()

            // If userId and companyId is empty in the JWT then we reject the request
            if (userId.isNullOrBlank() || companyId.isNullOrBlank()) {
              call.respond(
                HttpStatusCode.Unauthorized,
                mapOf(
                  "message" to "Unauthorized"
                )
              )
              return@post
            }

            // Validate data-type of content is either of type yaml or plain-text
            val yamlContent = when {
              contentType.match(ContentType.parse("application/x-yaml")) ||
              contentType.match(ContentType.parse("text/yaml")) ||
              contentType.match(ContentType.Text.Plain) -> call.receiveText()
              
              else -> {
                  call.respond(HttpStatusCode.UnsupportedMediaType)
                  return@post
              }
            }
            
            val result = contractService.createContract(companyId, userId, yamlContent)

            if (result.first != 200) {
              call.respond(
               HttpStatusCode.BadRequest,
               result.second
              )
             return@post
            }
            
            call.respond(
                HttpStatusCode.Created,
                result.second
            )
        }

        put("/contracts/{id}") {
         val contractId = call.parameters["id"]
         val contentType = call.request.contentType()

         val userId = call.getUserId()
         val companyId = call.getClaim("companyId").toString()

         // If userId and companyId is empty in the JWT then we reject the request
         if (userId.isNullOrBlank() || companyId.isNullOrBlank()) {
           call.respond(
             HttpStatusCode.Unauthorized,
             mapOf(
               "message" to "Unauthorized"
             )
           )
           return@put
         }

         if (contractId.isNullOrBlank()) {
           call.respond(
             HttpStatusCode.BadRequest,
             mapOf(
               "message" to "Contract ID is required"
             )
           )
           return@put
         }

         // Validate data-type of content is either of type yaml or plain-text
         val yamlContent = when {
           contentType.match(ContentType.parse("application/x-yaml")) ||
           contentType.match(ContentType.parse("text/yaml")) ||
           contentType.match(ContentType.Text.Plain) -> call.receiveText()
           
           else -> {
               call.respond(HttpStatusCode.UnsupportedMediaType)
               return@put
           }
         }
         
         val result = contractService.updateContract(contractId, companyId, userId, yamlContent)

         if (result.first != 200) {
           call.respond(
            HttpStatusCode.BadRequest,
            result.second
           )
          return@put
         }
         
         call.respond(
             HttpStatusCode.Created,
             result.second
         )
     }
     
        // DELETE contract by ID - soft delete
        delete("/contracts/{id}") {
            val contractId = call.parameters["id"]
            val userId = call.getUserId()
            val companyId = call.getClaim("companyId").toString()
            
            // Validate authentication
            if (userId.isNullOrBlank() || companyId.isNullOrBlank()) {
                call.respond(
                    HttpStatusCode.Unauthorized,
                    mapOf("message" to "Unauthorized")
                )
                return@delete
            }
            
            // Validate contract ID
            if (contractId.isNullOrBlank()) {
                call.respond(
                    HttpStatusCode.BadRequest,
                    mapOf("message" to "Contract ID is required")
                )
                return@delete
            }
            
            val result = contractService.deleteContract(contractId, companyId)
            call.respond(mapToHttpStatusCode(result.first), result.second)
        }
        
        // GET all versions metadata for a contract
        get("/contracts/{id}/versions") {
            val contractId = call.parameters["id"]
            val userId = call.getUserId()
            val companyId = call.getClaim("companyId").toString()
            
            // Validate authentication
            if (userId.isNullOrBlank() || companyId.isNullOrBlank()) {
                call.respond(
                    HttpStatusCode.Unauthorized,
                    mapOf("message" to "Unauthorized")
                )
                return@get
            }
            
            // Validate contract ID
            if (contractId.isNullOrBlank()) {
                call.respond(
                    HttpStatusCode.BadRequest,
                    mapOf("message" to "Contract ID is required")
                )
                return@get
            }
            
            val result = contractService.getAllContractVersions(contractId, companyId)
            call.respond(mapToHttpStatusCode(result.first), result.second)
        }
        
        // DELETE contract version by ID and version number - soft delete
        delete("/contracts/{id}/versions/{version}") {
            val contractId = call.parameters["id"]
            val versionParam = call.parameters["version"]
            val userId = call.getUserId()
            val companyId = call.getClaim("companyId").toString()
            
            // Validate authentication
            if (userId.isNullOrBlank() || companyId.isNullOrBlank()) {
                call.respond(
                    HttpStatusCode.Unauthorized,
                    mapOf("message" to "Unauthorized")
                )
                return@delete
            }
            
            // Validate contract ID
            if (contractId.isNullOrBlank()) {
                call.respond(
                    HttpStatusCode.BadRequest,
                    mapOf("message" to "Contract ID is required")
                )
                return@delete
            }
            
            // Validate and parse version
            val version = versionParam?.toIntOrNull()
            if (version == null) {
                call.respond(
                    HttpStatusCode.BadRequest,
                    mapOf("message" to "Valid version number is required")
                )
                return@delete
            }
            
            val result = contractService.deleteContractVersion(contractId, version, companyId)
            call.respond(mapToHttpStatusCode(result.first), result.second)
        }
    }
}
