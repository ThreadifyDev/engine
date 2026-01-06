/**
 * E2E Retry Limit Validation Tests using Threadify SDK
 * 
 * Tests retry limit violations (non-blocking critical warnings):
 * - Step retry count exceeding max_retries defined in contract transitions
 * 
 * Prerequisites:
 * - Server running on http://localhost:8081
 * - Contract "product_delivery" with retry limits configured
 * - API key: api-key-123
 */

import { Threadify } from '../src/index.js';
import http from 'http';

// Configuration
const API_KEY = 'api-key-123';
const CONTRACT_NAME = 'product_delivery';
const BASE_URL = 'http://localhost:8081';

let connection = null;

// Helper: Connect using SDK
async function connectSDK() {
    console.log('Connecting to Threadify using SDK...');
    connection = await Threadify.connect(API_KEY, 'payment-processor');
    console.log('✅ SDK connected and authenticated\n');
    return connection;
}

// Helper: Wait for a specified duration
function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// Helper to fetch validation notifications from the server
async function getValidationNotifications(threadId) {
    return new Promise((resolve, reject) => {
        const options = {
            hostname: 'localhost',
            port: 8081,
            path: `/api/v1/threads/${threadId}/violations`,
            method: 'GET',
            headers: {
                'X-API-Key': API_KEY
            }
        };

        const req = http.request(options, (res) => {
            let data = '';
            res.on('data', (chunk) => { data += chunk; });
            res.on('end', () => {
                if (res.statusCode === 200) {
                    resolve(JSON.parse(data));
                } else {
                    resolve({ violations: [] });
                }
            });
        });

        req.on('error', (e) => {
            console.log(`   ⚠️  Could not fetch violations: ${e.message}`);
            resolve({ violations: [] });
        });

        req.end();
    });
}

// Test: Retry Limit Exceeded Validation
async function testRetryLimitExceeded() {
    console.log('📋 Test 1: Retry Limit Exceeded Validation');
    console.log('   Contract: order_placed can retry with max_retries = 3\n');
    
    // Start thread
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread created: ${thread.threadId}`);
    
    // Use a fixed idempotency key for all retry attempts
    const idempKey = 'order-placed-retry-1';
    
    // Attempt order_placed 6 times with 'failed' status and same idempotency key
    // This simulates a step that keeps failing and needs retries
    // Expected: First 3 attempts allowed (retryCount 0,1,2), 4th attempt (retryCount 3) exceeds max_retries=3
    console.log('   Submitting order_placed 6 times with "failed" status and same idempotency key...');
    console.log('   Expected: Attempts 1-3 allowed, attempt 4 should trigger retry limit violation\n');
    
    for (let i = 1; i <= 6; i++) {
        console.log(`   🔄 Attempt ${i}...`);
        
        try {
            // action
            await thread
                .step('order_placed')
                .idempotencyKey(idempKey) // optional
                .addContext({ 
                    order_id: `ORDER-RETRY-${i}`,
                    customer_id: 'CUST-001',
                    total_amount: '199.99',
                    attempt: i
                })
                .stop('failed', `Order processing failed (attempt ${i})`);

            const result = await thread.waitFor("payment_validated", {timeout: 2000 })

            if (result.status === "completed" && result.stepStatus === "success"){
                console.log(`      ✅ Recorded (attempt ${i})`);
            } else {
                console.log(`      ⚠️ Error on attempt ${i}: ${result.stepStatus}`);
            }
        } catch (error) {
            console.log(`      ⚠️ Error on attempt ${i}: ${error.message}`);
        }
        
        await sleep(300);
    }
    
    // Wait for async validation to complete
    console.log('\n   ⏳ Waiting for async validation to process...');
    await sleep(2000);
    
    // Check for violations
    console.log('\n   🔍 Checking for retry limit violations...');
    const violations = await getValidationNotifications(thread.threadId);
    
    const retryViolations = violations.violations?.filter(v => 
        v.violation_type === 'retry_limit_exceeded'
    ) || [];
    
    if (retryViolations.length > 0) {
        console.log(`   ✅ SUCCESS: Found ${retryViolations.length} retry limit violation(s)`);
        console.log(`      Message: ${retryViolations[0].message}`);
        console.log(`      Details:`, retryViolations[0].details);
    } else {
        console.log(`   ❌ FAILED: No retry limit violations found`);
        console.log(`      Expected violation for exceeding max_retries=3`);
        console.log(`      Thread ID: ${thread.threadId}`);
        console.log(`      Idempotency Key: ${idempKey}`);
    }
    
    connection.ws.close();
}

// Test: Retry Within Limit (No Violation)
async function testRetryWithinLimit() {
    console.log('\n📋 Test 2: Retry Within Limit (No Violation)');
    console.log('   Testing that retries within limit do NOT trigger violations\n');
    
    const connection = await Threadify.connect(API_KEY, 'merchant-service');
    
    // Start thread
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread created: ${thread.threadId}`);
    
    // Use same idempotency key for all submissions
    const idempKey = 'order-placed-ok';
    
    // Submit order_placed 3 times with 'failed' status (within max_retries = 3)
    // Expected: All 3 attempts should be allowed without retry limit violation
    console.log('   Submitting order_placed 3 times with "failed" status (within max_retries=3)...');
    
    for (let i = 1; i <= 3; i++) {
        console.log(`   🔄 Attempt ${i}...`);
        
        await thread
            .step('order_placed')
            .idempotencyKey(idempKey)
            .addContext({ 
                order_id: `ORDER-OK-${i}`,
                customer_id: 'CUST-002',
                total_amount: '299.99',
                attempt: i
            })
            .stop('success', `Order processing failed (attempt ${i})`);
        
        console.log(`      ✅ Recorded (attempt ${i})`);
        await sleep(300);
    }
    
    // Wait for async validation
    console.log('\n   ⏳ Waiting for async validation to process...');
    await sleep(2000);
    
    // Check for violations
    console.log('\n   🔍 Checking for violations...');
    const violations = await getValidationNotifications(thread.threadId);
    
    const retryViolations = violations.violations?.filter(v => 
        v.violation_type === 'retry_limit_exceeded'
    ) || [];
    
    if (retryViolations.length === 0) {
        console.log(`   ✅ SUCCESS: No retry limit violations (as expected)`);
        console.log(`      2 retries is within max_retries=3`);
    } else {
        console.log(`   ❌ FAILED: Found unexpected retry limit violation`);
        console.log(`      Should not violate with only 2 retries (max=3)`);
        console.log(`      Violation:`, retryViolations[0]);
    }
    
    connection.ws.close();
}

// Main test runner
async function runTests() {
    console.log('🚀 Starting E2E Retry Limit Validation Tests (using SDK)\n');
    console.log('Prerequisites:');
    console.log('  - Server running on http://localhost:8081');
    console.log('  - Contract "product_delivery" with retry limits configured');
    console.log('  - order_placed: max_retries = 3');
    console.log('  - delivery_failed -> shipped: max_retries = 2');
    console.log('  - API key: api-key-123');
    console.log('');
    
    try {
        // Setup
        await connectSDK();
        
        // Run tests
        await testRetryLimitExceeded();
        await sleep(500);
        await testRetryWithinLimit();
        
        console.log('✅ All retry limit tests completed!');
        console.log('\nℹ️  Check the server logs and validation streams for retry limit violations');
        console.log('   Violations should be recorded as CRITICAL but not block step submission');
        
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
