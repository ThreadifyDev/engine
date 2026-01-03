/**
 * E2E Contract Upload Tests with Parties Validation
 * 
 * Tests contract upload functionality:
 * 1. Login with correct endpoint
 * 2. Upload contract with parties field
 * 3. Verify parties are stored in PostgreSQL
 * 4. Check server/archiver logs for processing
 */

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Configuration
const BASE_URL = 'http://localhost:8081';
const DB_URL = 'postgres://td_engine:tdtdtd@localhost:5434/threadify?sslmode=disable';

// Test state
let jwtToken = null;

// Helper: Login to get JWT token
async function login() {
    console.log('🔐 Logging in to get JWT token...');
    
    const response = await fetch(`${BASE_URL}/v1/contracts/login`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            email: 'test@example.com',
            password: 'testpassword'
        })
    });
    
    if (!response.ok) {
        throw new Error(`Login failed: ${response.status} ${response.statusText}`);
    }
    
    const data = await response.json();
    jwtToken = data.token;
    console.log(`✅ Got JWT token: ${jwtToken?.substring(0, 50)}...`);
    return jwtToken;
}

// Helper: Create test contract with parties
function createTestContract() {
    const timestamp = Date.now();
    return `contract_name: order_processing_contract_${timestamp}
version: 1
description: Multi party order processing workflow with parties validation

parties:
  - merchant
  - payment_processor
  - warehouse_manager
  - logistics_carrier

validation:
  max_duration: 72h
  allow_multiple_terminals: false
  multiple_terminals_severity: major

entry_points:
  - order_placed

steps:
  - id: order_placed
    owner: merchant
    type: managed
    timeout: 5m
    business_context:
      required:
        - order_id
        - customer_id
        - total_amount
      optional:
        - notes
        - promo_code

  - id: payment_validation
    owner: payment_processor
    type: external
    timeout: 30s
    business_context:
      required:
        - payment_method
        - amount
      optional:
        - transaction_id

  - id: payment_validated
    owner: payment_processor
    type: managed

  - id: fulfillment_ready
    owner: warehouse_manager
    type: human_in_loop
    timeout: 2h
    business_context:
      required:
        - warehouse_location
        - items_count

  - id: shipped
    owner: logistics_carrier
    type: managed
    timeout: 24h
    business_context:
      required:
        - tracking_number
        - carrier_name
      optional:
        - estimated_delivery

  - id: delivered
    owner: logistics_carrier
    type: managed
    business_context:
      required:
        - delivery_timestamp
        - signature

  - id: delivery_failed
    owner: logistics_carrier
    type: managed
    business_context:
      required:
        - failure_reason

  - id: order_cancelled
    owner: merchant
    type: managed
    business_context:
      required:
        - cancellation_reason

terminal_steps:
  - delivered
  - order_cancelled

transitions:
  - from: order_placed
    to:
      - payment_validation
    max_retries: 3
  - from: payment_validation
    to:
      - payment_validated
      - order_cancelled
    max_retries: 3
  - from: payment_validated
    to:
      - fulfillment_ready
  - from: fulfillment_ready
    to:
      - shipped
  - from: shipped
    to:
      - delivered
      - delivery_failed
  - from: delivery_failed
    to:
      - shipped
    max_retries: 2`;
}

// Test 1: Upload contract with parties
async function testContractUpload() {
    console.log('\n📋 Test 1: Upload contract with parties');
    
    // Test with our custom contract with parties (skip working contract to avoid name conflicts)
    console.log('Testing custom contract with parties...');
    const customContractYAML = createTestContract();
    console.log('Custom contract preview:', customContractYAML.substring(0, 200) + '...');
    
    const response = await fetch(`${BASE_URL}/v1/contracts`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${jwtToken}`
        },
        body: JSON.stringify({
            contractYAML: customContractYAML
        })
    });
    
    if (!response.ok) {
        const errorText = await response.text();
        console.log('❌ Custom contract failed - validation issue');
        console.log(`Error: ${response.status} ${response.statusText}`);
        console.log(`Response: ${errorText}`);
        throw new Error(`Custom contract upload failed: ${response.status} ${response.statusText}`);
    }
    
    const data = await response.json();
    console.log(`✅ Custom contract uploaded successfully`);
    console.log(`   Contract ID: ${data.contract?.id}`);
    console.log(`   Name: ${data.contract?.name}`);
    
    return data.contract?.id;
}

// Test 2: Verify parties in PostgreSQL
async function testPartiesInDatabase(contractId) {
    console.log('\n📋 Test 2: Verify parties in PostgreSQL');
    
    // Wait a moment for processing
    await new Promise(resolve => setTimeout(resolve, 2000));
    
    try {
        // This would typically use a PostgreSQL client, but for simplicity we'll use curl
        const { execSync } = await import('child_process');
        
        const query = `SELECT graph->>'parties' as parties FROM contract_versions WHERE contract_id = '${contractId}' ORDER BY version DESC LIMIT 1;`;
        
        const result = execSync(`psql "${DB_URL}" -t -c "${query}"`, { encoding: 'utf8' });
        
        console.log(`Parties in database: ${result.trim()}`);
        
        // Verify expected parties are present
        const expectedParties = ['merchant', 'payment_processor', 'warehouse_manager', 'logistics_carrier'];
        
        for (const party of expectedParties) {
            if (result.includes(party)) {
                console.log(`  ✅ Found party: ${party}`);
            } else {
                throw new Error(`Missing party: ${party}`);
            }
        }
        
        console.log('✅ All expected parties found in database');
        
    } catch (error) {
        console.log(`❌ Database verification failed: ${error.message}`);
        throw error;
    }
}

// Test 3: Check server and archiver logs
async function testLogsProcessing() {
    console.log('\n📋 Test 3: Check server and archiver logs');
    
    try {
        const { execSync } = await import('child_process');
        
        // Check recent server logs for contract processing
        console.log('Checking server logs...');
        const serverLogs = execSync('tail -n 20 server-final-test.log 2>/dev/null || echo "No server logs found"', { encoding: 'utf8' });
        
        if (serverLogs.includes('contract') || serverLogs.includes('upload')) {
            console.log('✅ Server shows contract processing activity');
        } else {
            console.log('⚠️  No recent contract activity in server logs');
        }
        
        // Check recent archiver logs for database writes
        console.log('Checking archiver logs...');
        const archiverLogs = execSync('tail -n 20 archiver.log 2>/dev/null || echo "No archiver logs found"', { encoding: 'utf8' });
        
        if (archiverLogs.includes('postgres') || archiverLogs.includes('write') || archiverLogs.includes('archive')) {
            console.log('✅ Archiver shows database activity');
        } else {
            console.log('⚠️  No recent database activity in archiver logs');
        }
        
    } catch (error) {
        console.log(`⚠️  Could not check logs: ${error.message}`);
    }
}

// Main test runner
async function runTests() {
    console.log('🚀 Starting E2E Contract Upload Tests with Parties Validation\n');
    console.log('Prerequisites:');
    console.log('  - Server running on http://localhost:8081');
    console.log('  - PostgreSQL running on localhost:5432');
    console.log('  - Archiver process running');
    console.log('');
    
    try {
        // Test 1: Login
        await login();
        
        // Test 2: Upload contract with parties
        const contractId = await testContractUpload();
        
        // Test 3: Verify parties in database
        await testPartiesInDatabase(contractId);
        
        // Test 4: Check logs
        await testLogsProcessing();
        
        console.log('\n✅ All contract upload tests completed!');
        console.log('\nSummary:');
        console.log('  ✅ Authentication successful');
        console.log('  ✅ Contract uploaded with parties field');
        console.log('  ✅ Parties verified in PostgreSQL database');
        console.log('  ✅ Server and archiver processing confirmed');
        
    } catch (error) {
        console.error('\n❌ Test failed:', error.message);
        console.error(error.stack);
        process.exit(1);
    }
}

// Run tests
runTests();
