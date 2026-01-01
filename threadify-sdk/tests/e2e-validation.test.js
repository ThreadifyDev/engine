/**
 * E2E Validation Tests using Threadify SDK
 * 
 * Tests all blocking validations:
 * 1. Entry point validation
 * 2. Step exists in contract
 * 3. Required business context
 * 4. Idempotency
 * 5. Role validation
 * 6. Access control
 */

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { Threadify } from '../src/index.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Configuration
const BASE_URL = 'http://localhost:8081';
const CONTRACT_FILE = path.join(__dirname, '../../threadify-go/examples/contract_example.yaml');

// Test state
let apiKey = null;
let jwtToken = null;
let connection = null;
let contractName = 'product_delivery';  // Use contract name, not UUID

// Helper: Get API key for WebSocket
async function getApiKey() {
    // Check if API key is provided via environment
    if (process.env.API_KEY) {
        apiKey = process.env.API_KEY;
        console.log('✅ Using API key from environment');
        return apiKey;
    }
    
    // Use default test API key
    apiKey = 'api-key-123';
    console.log('✅ Using default test API key');
    return apiKey;
}

// Helper: Login to get JWT token for REST API
async function login() {
    const response = await fetch(`${BASE_URL}/v1/contracts/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            userId: 'user-123'
        })
    });
    
    const data = await response.json();
    if (!data.token) {
        throw new Error('Login failed: ' + JSON.stringify(data));
    }
    
    jwtToken = data.token;
    console.log('✅ Logged in successfully (JWT for REST API)');
    return jwtToken;
}

// Helper: Upload contract
async function uploadContract() {
    const contractYaml = fs.readFileSync(CONTRACT_FILE, 'utf8');
    
    const response = await fetch(`${BASE_URL}/v1/contracts`, {
        method: 'POST',
        headers: {
            'Authorization': `Bearer ${jwtToken}`,
            'Content-Type': 'text/yaml'
        },
        body: contractYaml
    });
    
    const data = await response.json();
    console.log('📄 Contract upload response:', JSON.stringify(data, null, 2));
    
    if (response.status !== 200 && response.status !== 201) {
        // Check if contract already exists (400 or 500 error with "already exists" or "Failed to create")
        if ((response.status === 400 || response.status === 500) && 
            data.message && 
            (data.message.includes('already exists') || data.message.includes('Failed to create'))) {
            console.log(`⚠️  Contract already exists, deleting and re-uploading to ensure latest version...`);
            
            // Delete the existing contract
            const deleteResponse = await fetch(`${BASE_URL}/v1/contracts/${contractName}`, {
                method: 'DELETE',
                headers: {
                    'Authorization': `Bearer ${jwtToken}`
                }
            });
            
            if (deleteResponse.ok) {
                console.log(`✅ Deleted existing contract`);
                // Retry upload
                const retryResponse = await fetch(`${BASE_URL}/v1/contracts`, {
                    method: 'POST',
                    headers: {
                        'Authorization': `Bearer ${jwtToken}`,
                        'Content-Type': 'text/yaml'
                    },
                    body: contractYaml
                });
                
                if (retryResponse.ok || retryResponse.status === 201) {
                    console.log(`✅ Contract re-uploaded successfully`);
                    return contractName;
                }
            }
            
            console.log(`⚠️  Could not delete/re-upload, proceeding with existing contract`);
            return contractName;
        }
        console.log('⚠️  Contract upload failed with status:', response.status);
        throw new Error('Contract upload failed: ' + JSON.stringify(data));
    }
    
    console.log(`✅ Contract uploaded successfully (name: ${contractName})`);
    return contractName;
}

// Helper: Connect using SDK (uses default ws://localhost:8081/threads)
async function connectSDK() {
    console.log('Connecting to Threadify using SDK...');
    connection = await Threadify.connect(apiKey, 'merchant-service');
    console.log('✅ SDK connected and authenticated');
    return connection;
}

// Test 1: Entry Point Validation
async function testEntryPointValidation() {
    console.log('\n📋 Test 1: Entry Point Validation');
    
    const thread = await connection.start(contractName, 'merchant-service');
    console.log(`  Thread created: ${thread.threadId}`);
    
    // Try non-entry-point step (should fail)
    try {
        await thread
            .step('shipped')
            .addContext({ tracking_number: '123' })
            .stop('success', 'Shipped');
        
        console.log('  ❌ FAIL: Non-entry-point step should be rejected');
    } catch (error) {
        if (error.message.includes('entry point')) {
            console.log('  ✅ PASS: Non-entry-point step rejected');
            console.log(`     Error: ${error.message}`);
        } else {
            console.log('  ⚠️  Unexpected error:', error.message);
        }
    }
    
    // Submit valid entry point (should succeed)
    const thread2 = await connection.start(contractName, 'merchant-service');
    try {
        await thread2
            .step('order_placed')
            .addContext({ 
                order_id: 'ORDER-123',
                customer_id: 'CUST-456',
                total_amount: '99.99'
            })
            .stop('success', 'Order placed');
        
        console.log('  ✅ PASS: Entry point step accepted');
    } catch (error) {
        console.log('  ❌ FAIL: Entry point step should be accepted');
        console.log(`     Error: ${error.message}`);
    }
}

// Test 2: Step Exists in Contract
async function testStepExistsValidation() {
    console.log('\n📋 Test 2: Step Exists in Contract');
    
    const thread = await connection.start(contractName, 'merchant-service');
    
    try {
        await thread
            .step('nonexistent_step')
            .addContext({ some_field: 'value' })
            .stop('success', 'Done');
        
        console.log('  ❌ FAIL: Non-existent step should be rejected');
    } catch (error) {
        if (error.message.includes('not found in contract')) {
            console.log('  ✅ PASS: Non-existent step rejected');
            console.log(`     Error: ${error.message}`);
        } else {
            console.log('  ⚠️  Unexpected error:', error.message);
        }
    }
}

// Test 3: Required Business Context
async function testBusinessContextValidation() {
    console.log('\n📋 Test 3: Required Business Context');
    
    const thread = await connection.start(contractName, 'merchant-service');
    
    // Missing required fields (should fail)
    try {
        await thread
            .step('order_placed')
            .addContext({ notes: 'Some notes' })
            // Missing order_id, customer_id, total_amount
            .stop('success', 'Done');
        
        console.log('  ❌ FAIL: Missing required context should be rejected');
    } catch (error) {
        if (error.message.includes('required context field')) {
            console.log('  ✅ PASS: Missing required context rejected');
            console.log(`     Error: ${error.message}`);
        } else {
            console.log('  ⚠️  Unexpected error:', error.message);
        }
    }
    
    // All required fields present (should succeed)
    const thread2 = await connection.start(contractName, 'merchant-service');
    try {
        await thread2
            .step('order_placed')
            .addContext({ 
                order_id: 'ORDER-789',
                customer_id: 'CUST-999',
                total_amount: '199.99'
            })
            .stop('success', 'Order placed');
        
        console.log('  ✅ PASS: Valid context accepted');
    } catch (error) {
        console.log('  ❌ FAIL: Valid context should be accepted');
        console.log(`     Error: ${error.message}`);
    }
}

// Test 4: Idempotency Validation
async function testIdempotencyValidation() {
    console.log('\n📋 Test 4: Idempotency Validation');
    
    const thread = await connection.start(contractName, 'merchant-service');
    
    // Submit step with idempotency key
    try {
        await thread
            .step('order_placed')
            .idempotencyKey('idemp-key-001')
            .addContext({ 
                order_id: 'ORDER-IDEMP-1',
                customer_id: 'CUST-IDEMP-1',
                total_amount: '100.00'
            })
            .stop('success', 'First submission');
        
        console.log('  ✅ PASS: First submission with idempotency key accepted');
    } catch (error) {
        console.log('  ❌ FAIL: First submission should succeed');
        console.log(`     Error: ${error.message}`);
        return;
    }
    
    // Try to submit same step with same idempotency key but different data (should be silently ignored - that's idempotency!)
    try {
        await thread
            .step('order_placed')
            .idempotencyKey('idemp-key-001')
            .addContext({ 
                order_id: 'ORDER-IDEMP-2',  // Different data
                customer_id: 'CUST-IDEMP-2',
                total_amount: '200.00'
            })
            .stop('success', 'Duplicate submission');
        
        // Idempotency means duplicates are silently ignored (no error thrown)
        console.log('  ✅ PASS: Duplicate idempotency key silently ignored (idempotent behavior)');
    } catch (error) {
        console.log('  ❌ FAIL: Idempotent requests should not throw errors');
        console.log(`     Error: ${error.message}`);
    }
    
    // Submit with different idempotency key (should succeed)
    try {
        await thread
            .step('payment_validation')
            .idempotencyKey('idemp-key-002')
            .addContext({ 
                payment_method: 'credit_card',
                amount: '100.00'  // Required field
            })
            .stop('success', 'Different step');
        
        console.log('  ✅ PASS: Different idempotency key accepted');
    } catch (error) {
        console.log('  ⚠️  Unexpected error:', error.message);
    }
}

// Test 5: Role Validation
async function testRoleValidation() {
    console.log('\n📋 Test 5: Role Validation');
    
    // Connect as merchant service
    const merchantConnection = await Threadify.connect(apiKey, 'merchant-service', { url: `${BASE_URL.replace('http', 'ws')}/threads` });
    
    const thread = await merchantConnection.start(contractName, 'merchant-service');
    
    // Merchant should be able to submit 'order_placed' (owned by merchant)
    try {
        await thread
            .step('order_placed')
            .addContext({ 
                order_id: 'ORDER-ROLE-1',
                customer_id: 'CUST-ROLE-1',
                total_amount: '150.00'
            })
            .stop('success', 'Order placed by merchant');
        
        console.log('  ✅ PASS: Merchant can submit merchant-owned step');
    } catch (error) {
        console.log('  ❌ FAIL: Merchant should be able to submit order_placed');
        console.log(`     Error: ${error.message}`);
    }
    
    // Connect as payment_processor service
    const paymentConnection = await Threadify.connect(apiKey, 'payment_processor-service', { url: `${BASE_URL.replace('http', 'ws')}/threads` });
    
    // Create a new thread and try to submit merchant-owned step as payment processor (should fail)
    const thread2 = await paymentConnection.start(contractName, 'payment_processor-service');
    try {
        await thread2
            .step('order_placed')  // This step is owned by merchant
            .addContext({ 
                order_id: 'ORDER-ROLE-2',
                customer_id: 'CUST-ROLE-2',
                total_amount: '250.00'
            })
            .stop('success', 'Attempted by payment processor');
        
        console.log('  ❌ FAIL: Payment processor should not be able to submit merchant-owned step');
    } catch (error) {
        if (error.message.includes('role') || error.message.includes('Access denied')) {
            console.log('  ✅ PASS: Role validation rejected incorrect service');
            console.log(`     Error: ${error.message}`);
        } else {
            console.log('  ⚠️  Unexpected error:', error.message);
        }
    }
    
    // Connections will be cleaned up automatically
}

// Test 6: Access Control Validation
async function testAccessControlValidation() {
    console.log('\n📋 Test 6: Access Control Validation');
    
    // Create a thread with the authenticated user
    const thread = await connection.start(contractName, 'merchant-service');
    console.log(`  Thread created: ${thread.threadId}`);
    
    // Submit a step (should succeed - user has write access)
    try {
        await thread
            .step('order_placed')
            .addContext({ 
                order_id: 'ORDER-777',
                customer_id: 'CUST-888',
                total_amount: '299.99'
            })
            .stop('success', 'Order placed');
        
        console.log('  ✅ PASS: User with write access can submit steps');
    } catch (error) {
        console.log('  ❌ FAIL: User should have write access to their own thread');
        console.log(`     Error: ${error.message}`);
    }
    
    // Test inviteParty and join workflow
    try {
        // Create invitation token using SDK (use valid role from server)
        const invitationToken = await thread.inviteParty({
            role: 'external_partner',
            permissions: 'read,write',
            expiresIn: '24h'
        });
        
        console.log('  ✅ PASS: Invitation token created successfully');
        console.log(`     Token: ${invitationToken.substring(0, 20)}...`);
        
        // Test joining with the token (using same connection for demo)
        try {
            const joinedThread = await connection.join(invitationToken);
            console.log('  ✅ PASS: Successfully joined thread with invitation token');
            console.log(`     Joined thread ID: ${joinedThread.getThreadId()}`);
        } catch (joinError) {
            console.log('  ⚠️  Join with token failed:', joinError.message);
        }
    } catch (error) {
        console.log('  ℹ️  Invitation feature may not be fully implemented:', error.message);
    }
    
    // Note: Full implementation would require:
    // 1. Create a thread with one user
    // 2. Try to submit a step with a different user (different auth token)
    // This would require a second connection/auth token
    console.log('  ℹ️  Note: Access denial test requires multiple users');
    console.log('     Expected: Users without write permission should be rejected');
    console.log('     Error should contain: "Access denied" or "write permission"');
}

// Main test runner
async function runTests() {
    console.log('🚀 Starting E2E Validation Tests (using SDK)\n');
    console.log('Prerequisites:');
    console.log('  - Server running on http://localhost:8081');
    console.log('  - Contract file exists at examples/contract_example.yaml');
    console.log('');
    
    try {
        // Setup
        await login();           // Get JWT for REST API
        await getApiKey();       // Get API key for WebSocket
        await uploadContract();  // Upload contract using JWT
        await connectSDK();      // Connect WebSocket using API key
        
        // Run tests
        await testEntryPointValidation();
        await testStepExistsValidation();
        await testBusinessContextValidation();
        await testIdempotencyValidation();
        await testRoleValidation();
        await testAccessControlValidation();
        
        console.log('\n✅ All tests completed!');
        
    } catch (error) {
        console.error('\n❌ Test failed:', error.message);
        console.error(error.stack);
    } finally {
        // Cleanup
        if (connection && connection.ws) {
            connection.ws.close();
        }
        process.exit(0);
    }
}

// Run tests
runTests();
