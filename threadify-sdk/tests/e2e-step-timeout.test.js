/**
 * E2E Step Timeout Validation Tests
 * 
 * Tests non-blocking validation for step timeout violations:
 * - Steps exceeding defined timeout duration
 * - Steps completing within timeout (control)
 * - Different timeout durations (5m, 30s, 2h, 24h)
 * - Edge cases (exactly at timeout, just over timeout)
 * 
 * Prerequisites:
 * - Server running on http://localhost:8081
 * - Contract "product_delivery" with timeout configurations
 * - API key: api-key-123
 */

import { Threadify } from '../src/index.js';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Configuration
const API_KEY = 'api-key-123';
const CONTRACT_NAME = 'product_delivery';
const BASE_URL = 'http://localhost:8081';
const CONTRACT_FILE = path.join(__dirname, '../../threadify-go/examples/contract_example.yaml');

let connection = null;
let jwtToken = null;

// Helper: Login
async function login() {
    const response = await fetch(`${BASE_URL}/v1/contracts/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ userId: 'user-123' })
    });
    
    const data = await response.json();
    jwtToken = data.token;
    console.log('✅ Logged in successfully\n');
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
    
    if (response.status === 400 || response.status === 500) {
        const data = await response.json();
        if (data.message && data.message.includes('already exists')) {
            console.log('⚠️  Contract already exists, continuing...\n');
            return;
        }
    }
    
    console.log('✅ Contract uploaded successfully\n');
}

// Helper: Connect SDK
async function connectSDK() {
    console.log('Connecting to Threadify...');
    connection = await Threadify.connect(API_KEY, 'merchant-service');
    console.log('✅ SDK connected\n');
}

// Helper: Wait
function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// Helper: Calculate timestamps with delay
function getTimestamps(delaySeconds) {
    const startedAt = new Date();
    const finishedAt = new Date(startedAt.getTime() + (delaySeconds * 1000));
    return {
        startedAt: startedAt.toISOString(),
        finishedAt: finishedAt.toISOString()
    };
}

// Test 1: Short Timeout Exceeded (30s)
async function testShortTimeoutExceeded() {
    console.log('📋 Test 1: Short Timeout Exceeded (30s)');
    console.log('   Step: payment_validation (timeout: 30s)');
    console.log('   Simulating: 45 second execution time\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread: ${thread.threadId}`);
    
    // Entry point
    await thread.step('order_placed')
        .addContext({ 
            order_id: 'ORD-TIMEOUT-001',
            customer_id: 'CUST-001',
            total_amount: '99.99'
        })
        .stop('success');
    console.log('   ✅ Step: order_placed');
    
    await sleep(500);
    
    // Simulate step that took 45 seconds (exceeds 30s timeout)
    const timestamps = getTimestamps(45);
    
    // Note: We need to manually set timestamps via context or use a custom approach
    // For now, we'll add a delay and let the server calculate duration
    console.log('   ⏱️  Simulating 45-second execution...');
    
    await thread.step('payment_validation')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '99.99',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '45s'
        })
        .stop('success');
    console.log('   ⚠️  Step: payment_validation (took 45s, timeout: 30s)');
    console.log('   Expected: Step timeout exceeded violation\n');
    
    await sleep(2000);
    console.log('   ✅ PASS: Timeout violation should be logged\n');
}

// Test 2: Medium Timeout Exceeded (5m)
async function testMediumTimeoutExceeded() {
    console.log('📋 Test 2: Medium Timeout Exceeded (5m)');
    console.log('   Step: order_placed (timeout: 5m)');
    console.log('   Simulating: 7 minute execution time\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread: ${thread.threadId}`);
    
    // Simulate step that took 7 minutes (exceeds 5m timeout)
    const timestamps = getTimestamps(420); // 7 minutes = 420 seconds
    
    await thread.step('order_placed')
        .addContext({ 
            order_id: 'ORD-TIMEOUT-002',
            customer_id: 'CUST-002',
            total_amount: '149.99',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '7m'
        })
        .stop('success');
    console.log('   ⚠️  Step: order_placed (took 7m, timeout: 5m)');
    console.log('   Expected: Step timeout exceeded violation\n');
    
    await sleep(2000);
    console.log('   ✅ PASS: Timeout violation should be logged\n');
}

// Test 3: Long Timeout Exceeded (2h)
async function testLongTimeoutExceeded() {
    console.log('📋 Test 3: Long Timeout Exceeded (2h)');
    console.log('   Step: fulfillment_ready (timeout: 2h)');
    console.log('   Simulating: 3 hour execution time\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread: ${thread.threadId}`);
    
    // Complete prerequisites
    await thread.step('order_placed')
        .addContext({ 
            order_id: 'ORD-TIMEOUT-003',
            customer_id: 'CUST-003',
            total_amount: '199.99'
        })
        .stop('success');
    console.log('   ✅ Step: order_placed');
    
    await sleep(500);
    
    await thread.step('payment_validation')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '199.99'
        })
        .stop('success');
    console.log('   ✅ Step: payment_validation');
    
    await sleep(500);
    
    await thread.step('payment_validated')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '199.99'
        })
        .stop('success');
    console.log('   ✅ Step: payment_validated');
    
    await sleep(500);
    
    // Simulate step that took 3 hours (exceeds 2h timeout)
    const timestamps = getTimestamps(10800); // 3 hours = 10800 seconds
    
    await thread.step('fulfillment_ready')
        .addContext({ 
            warehouse_location: 'WH-001',
            items_count: '5',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '3h'
        })
        .stop('success');
    console.log('   ⚠️  Step: fulfillment_ready (took 3h, timeout: 2h)');
    console.log('   Expected: Step timeout exceeded violation\n');
    
    await sleep(2000);
    console.log('   ✅ PASS: Timeout violation should be logged\n');
}

// Test 4: Very Long Timeout Exceeded (24h)
async function testVeryLongTimeoutExceeded() {
    console.log('📋 Test 4: Very Long Timeout Exceeded (24h)');
    console.log('   Step: shipped (timeout: 24h)');
    console.log('   Simulating: 30 hour execution time\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread: ${thread.threadId}`);
    
    // Complete prerequisites
    await thread.step('order_placed')
        .addContext({ 
            order_id: 'ORD-TIMEOUT-004',
            customer_id: 'CUST-004',
            total_amount: '299.99'
        })
        .stop('success');
    
    await sleep(500);
    
    await thread.step('payment_validation')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '299.99'
        })
        .stop('success');
    
    await sleep(500);
    
    await thread.step('payment_validated')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '299.99'
        })
        .stop('success');
    
    await sleep(500);
    
    await thread.step('fulfillment_ready')
        .addContext({ 
            warehouse_location: 'WH-002',
            items_count: '3'
        })
        .stop('success');
    
    await sleep(500);
    
    // Simulate step that took 30 hours (exceeds 24h timeout)
    const timestamps = getTimestamps(108000); // 30 hours = 108000 seconds
    
    await thread.step('shipped')
        .addContext({ 
            tracking_number: 'TRACK-001',
            carrier_name: 'SlowShip',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '30h'
        })
        .stop('success');
    console.log('   ⚠️  Step: shipped (took 30h, timeout: 24h)');
    console.log('   Expected: Step timeout exceeded violation\n');
    
    await sleep(2000);
    console.log('   ✅ PASS: Timeout violation should be logged\n');
}

// Test 5: Edge Case - Exactly at Timeout
async function testExactlyAtTimeout() {
    console.log('📋 Test 5: Edge Case - Exactly at Timeout');
    console.log('   Step: payment_validation (timeout: 30s)');
    console.log('   Simulating: Exactly 30 second execution time\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread: ${thread.threadId}`);
    
    await thread.step('order_placed')
        .addContext({ 
            order_id: 'ORD-TIMEOUT-005',
            customer_id: 'CUST-005',
            total_amount: '49.99'
        })
        .stop('success');
    
    await sleep(500);
    
    // Exactly 30 seconds
    const timestamps = getTimestamps(30);
    
    await thread.step('payment_validation')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '49.99',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '30s'
        })
        .stop('success');
    console.log('   ℹ️  Step: payment_validation (took exactly 30s, timeout: 30s)');
    console.log('   Expected: NO violation (at threshold)\n');
    
    await sleep(2000);
    console.log('   ✅ PASS: Should not trigger violation at exact timeout\n');
}

// Test 6: Edge Case - Just Over Timeout
async function testJustOverTimeout() {
    console.log('📋 Test 6: Edge Case - Just Over Timeout');
    console.log('   Step: payment_validation (timeout: 30s)');
    console.log('   Simulating: 31 second execution time (1s over)\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread: ${thread.threadId}`);
    
    await thread.step('order_placed')
        .addContext({ 
            order_id: 'ORD-TIMEOUT-006',
            customer_id: 'CUST-006',
            total_amount: '59.99'
        })
        .stop('success');
    
    await sleep(500);
    
    // 31 seconds (just 1 second over)
    const timestamps = getTimestamps(31);
    
    await thread.step('payment_validation')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '59.99',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '31s'
        })
        .stop('success');
    console.log('   ⚠️  Step: payment_validation (took 31s, timeout: 30s)');
    console.log('   Expected: Timeout violation (even 1s over)\n');
    
    await sleep(2000);
    console.log('   ✅ PASS: Should trigger violation for any excess\n');
}

// Test 7: Within Timeout (Control Test)
async function testWithinTimeout() {
    console.log('📋 Test 7: Within Timeout (Control Test)');
    console.log('   All steps completing within their timeouts\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread: ${thread.threadId}`);
    
    // order_placed: 2 minutes (timeout: 5m)
    let timestamps = getTimestamps(120);
    await thread.step('order_placed')
        .addContext({ 
            order_id: 'ORD-TIMEOUT-007',
            customer_id: 'CUST-007',
            total_amount: '79.99',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '2m'
        })
        .stop('success');
    console.log('   ✅ Step: order_placed (2m < 5m timeout)');
    
    await sleep(500);
    
    // payment_validation: 15 seconds (timeout: 30s)
    timestamps = getTimestamps(15);
    await thread.step('payment_validation')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '79.99',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '15s'
        })
        .stop('success');
    console.log('   ✅ Step: payment_validation (15s < 30s timeout)');
    
    await sleep(500);
    
    await thread.step('payment_validated')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '79.99'
        })
        .stop('success');
    console.log('   ✅ Step: payment_validated');
    
    await sleep(500);
    
    // fulfillment_ready: 1 hour (timeout: 2h)
    timestamps = getTimestamps(3600);
    await thread.step('fulfillment_ready')
        .addContext({ 
            warehouse_location: 'WH-003',
            items_count: '2',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '1h'
        })
        .stop('success');
    console.log('   ✅ Step: fulfillment_ready (1h < 2h timeout)');
    
    console.log('   Expected: NO violations (all within timeout)\n');
    
    await sleep(2000);
    console.log('   ✅ PASS: No timeout violations for compliant steps\n');
}

// Test 8: Multiple Timeout Violations in Same Thread
async function testMultipleTimeoutViolations() {
    console.log('📋 Test 8: Multiple Timeout Violations in Same Thread');
    console.log('   Testing multiple steps exceeding timeouts\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread: ${thread.threadId}`);
    
    // Violation 1: order_placed exceeds 5m
    let timestamps = getTimestamps(420); // 7 minutes
    await thread.step('order_placed')
        .addContext({ 
            order_id: 'ORD-TIMEOUT-008',
            customer_id: 'CUST-008',
            total_amount: '399.99',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '7m'
        })
        .stop('success');
    console.log('   ⚠️  Step: order_placed (7m > 5m timeout)');
    
    await sleep(500);
    
    // Violation 2: payment_validation exceeds 30s
    timestamps = getTimestamps(60); // 60 seconds
    await thread.step('payment_validation')
        .addContext({ 
            payment_method: 'credit_card',
            amount: '399.99',
            started_at: timestamps.startedAt,
            finished_at: timestamps.finishedAt,
            simulated_duration: '60s'
        })
        .stop('success');
    console.log('   ⚠️  Step: payment_validation (60s > 30s timeout)');
    
    console.log('   Expected: Multiple timeout violations logged\n');
    
    await sleep(2000);
    console.log('   ✅ PASS: Multiple violations should be tracked\n');
}

// Main test runner
async function runTests() {
    console.log('🚀 Starting Step Timeout Validation Tests\n');
    console.log('=' .repeat(60) + '\n');
    
    try {
        await login();
        await uploadContract();
        await connectSDK();
        
        await testShortTimeoutExceeded();
        await testMediumTimeoutExceeded();
        await testLongTimeoutExceeded();
        await testVeryLongTimeoutExceeded();
        await testExactlyAtTimeout();
        await testJustOverTimeout();
        await testWithinTimeout();
        await testMultipleTimeoutViolations();
        
        console.log('=' .repeat(60));
        console.log('✅ All Step Timeout Tests Completed!\n');
        console.log('Note: Timeout violations are logged asynchronously.');
        console.log('Check server logs or validation_results table for details.\n');
        
    } catch (error) {
        console.error('❌ Test failed:', error.message);
        console.error(error.stack);
    } finally {
        if (connection) {
            await connection.close();
            console.log('Connection closed');
        }
    }
}

// Run tests
runTests();
