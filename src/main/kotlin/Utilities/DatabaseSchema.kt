package dev.threadify.Utilities

import java.sql.Connection

object DatabaseSchema {
    
    /**
     * Creates all necessary tables in the database
     */
    fun initializeTables(connection: Connection) {
        createContractsTable(connection)
        createContractVersionsTable(connection)
    }
    
    /**
     * SQL to create the contracts table
     */
    private fun createContractsTable(connection: Connection) {
        val sql = """
            CREATE TABLE IF NOT EXISTS contracts (
                id VARCHAR(255) PRIMARY KEY,
                name VARCHAR(500) NOT NULL,
                description TEXT NOT NULL,
                is_deleted BOOLEAN DEFAULT FALSE,
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
            );
            
            -- Index for faster lookups on non-deleted contracts
            CREATE INDEX IF NOT EXISTS idx_contracts_not_deleted 
            ON contracts(is_deleted) WHERE is_deleted = FALSE;
            
            -- Index for name searches
            CREATE INDEX IF NOT EXISTS idx_contracts_name 
            ON contracts(name);
        """.trimIndent()
        
        connection.createStatement().use { statement ->
            statement.execute(sql)
        }
    }
    
    /**
     * SQL to create the contract_versions table
     */
    private fun createContractVersionsTable(connection: Connection) {
        val sql = """
            CREATE TABLE IF NOT EXISTS contract_versions (
                id VARCHAR(255) PRIMARY KEY,
                version INTEGER NOT NULL,
                content TEXT NOT NULL,
                content_hash VARCHAR(64) NOT NULL,
                contract_id VARCHAR(255) NOT NULL,
                is_deleted BOOLEAN DEFAULT FALSE,
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                
                -- Foreign key constraint
                CONSTRAINT fk_contract
                    FOREIGN KEY (contract_id)
                    REFERENCES contracts(id)
                    ON DELETE CASCADE,
                
                -- Unique constraint: one version number per contract
                CONSTRAINT unique_contract_version
                    UNIQUE (contract_id, version)
            );
            
            -- Index for faster lookups by contract_id
            CREATE INDEX IF NOT EXISTS idx_contract_versions_contract_id 
            ON contract_versions(contract_id);
            
            -- Index for version lookups
            CREATE INDEX IF NOT EXISTS idx_contract_versions_version 
            ON contract_versions(contract_id, version);
            
            -- Index for content hash (for deduplication checks)
            CREATE INDEX IF NOT EXISTS idx_contract_versions_hash 
            ON contract_versions(content_hash);
            
            -- Index for non-deleted versions
            CREATE INDEX IF NOT EXISTS idx_contract_versions_not_deleted 
            ON contract_versions(is_deleted) WHERE is_deleted = FALSE;
        """.trimIndent()
        
        connection.createStatement().use { statement ->
            statement.execute(sql)
        }
    }
    
    /**
     * Drop all tables (useful for testing/reset)
     */
    fun dropAllTables(connection: Connection) {
        val sql = """
            DROP TABLE IF EXISTS contract_versions CASCADE;
            DROP TABLE IF EXISTS contracts CASCADE;
        """.trimIndent()
        
        connection.createStatement().use { statement ->
            statement.execute(sql)
        }
    }
}
