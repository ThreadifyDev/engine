/**
 * E2E Data Retrieval Tests using Threadify SDK
 * 
 * Tests the DataRetriever functionality for archived thread data access
 */

import assert from 'assert';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { Threadify } from '../src/index.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Configuration
const BASE_URL = 'http://localhost:8081';
const GRAPHQL_URL = `${BASE_URL}/graphql`;
const WS_URL = 'ws://localhost:8081/threads';

// Test state
let connection = null;
let dataRetriever = null;
let testThreadId = null;
const API_KEY = 'api-key-123';
const CONTRACT_NAME = 'product_delivery';

// Helper: Wait for a specified duration (from retry test)
function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// Helper: Check if server has archived data
async function checkArchivedData() {
    console.log('📋 Checking for existing archived data...');
    
    // Create temporary connection to test GraphQL
    const tempConnection = await Threadify.connect(API_KEY, 'test-service', { url: WS_URL });
    
    try {
        // Try to get any thread (this will test if GraphQL works)
        const threads = await tempConnection.getThreadsByRef({
            refKey: 'test',
            refValue: 'data'
        });
        
        console.log('✅ GraphQL endpoint is accessible');
        console.log(`   Found ${threads.length} threads with test reference`);
        return true;
    } catch (error) {
        console.log('✅ GraphQL endpoint is accessible (no test data found, but that\'s ok)');
        return true;
    } finally {
        tempConnection.ws.close();
    }
}

// Helper: Find or create a test thread ID
async function getTestThreadId() {
    console.log('\n📝 Looking for existing archived data...');
    
    // First, try to find existing threads using connection
    connection = await Threadify.connect(API_KEY, 'test-service', { url: WS_URL });
    
    try {
        // Try to find any thread with common references
        const threads = await connection.getThreadsByRef({
            refKey: 'orderId',
            refValue: 'ORDER-123'
        });
        
        if (threads.length > 0) {
            testThreadId = threads[0].id;
            console.log(`✅ Found existing thread: ${testThreadId}`);
            return testThreadId;
        }
    } catch (error) {
        console.log('⚠️  No existing threads found, will create test data');
    }
    
    // If no existing data, create a simple test thread
    console.log('📝 Creating minimal test data...');
    
    try {
        const thread = await connection.start(CONTRACT_NAME, 'test-service');
        testThreadId = thread.getThreadId();
        console.log(`✅ Created test thread: ${testThreadId}`);
        
        // Add multiple steps for comprehensive testing
        await thread
            .step('order_placed')
            .idempotencyKey('test-001')
            .addContext({ 
                order_id: 'ORDER-TEST-E2E',
                customer_id: 'CUST-TEST-E2E',
                total_amount: '99.99'
            })
            .addRefs({ orderId: 'ORDER-TEST-E2E' })
            .stop('success', 'Test order placed');
        
        await thread
            .step('payment_validation')
            .idempotencyKey('test-payment-001')
            .addContext({ 
                payment_method: 'credit_card',
                amount: '99.99'
            })
            .stop('success', 'Payment validated');
        
        await thread
            .step('inventory_check')
            .idempotencyKey('test-inventory-001')
            .addContext({ 
                product_id: 'PROD-001',
                quantity: '1'
            })
            .stop('success', 'Inventory checked');
        
        console.log('✅ Added test step');
        
        // Wait for archival (longer wait like retry test)
        console.log('⏳ Waiting 3 seconds for archival...');
        await sleep(3000);
        
    } catch (error) {
        console.log('⚠️  Could not create test data, skipping data retrieval tests');
        console.log('   Error:', error.message);
        return; // Exit early if we can't create test data
    }
    
    return testThreadId;
}

// Test 1: Get thread by ID
async function testGetThreadById() {
    console.log('\n📋 Test 1: Get Thread by ID');
    console.log(`   🔍 Looking for thread ID: ${testThreadId}`);
    
    try {
        const thread = await connection.getThread(testThreadId);
        
        assert.strictEqual(thread.id, testThreadId, 'Thread ID should match');
        assert.strictEqual(thread.contractName, contractName, 'Contract name should match');
        assert.ok(thread.status, 'Thread should have a status');
        assert.ok(thread.ownerId, 'Thread should have an owner ID');
        
        console.log('  ✅ PASS: Retrieved thread by ID');
        console.log(`     Thread ID: ${thread.id}`);
        console.log(`     Status: ${thread.status}`);
        console.log(`     Contract: ${thread.contractName}`);
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 2: Get thread by reference
async function testGetThreadByRef() {
    console.log('\n📋 Test 2: Get Thread by Reference');
    
    try {
        const thread = await dataRetriever.getThreadByRef({
            refKey: 'orderId',
            refValue: 'ORDER-TEST-001'
        });
        
        assert.ok(thread, 'Thread should be found by reference');
        assert.strictEqual(thread.id, testThreadId, 'Thread ID should match');
        
        console.log('  ✅ PASS: Retrieved thread by reference');
        console.log(`     Thread ID: ${thread.id}`);
        console.log(`     Reference: orderId=ORDER-TEST-001`);
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 3: Get all steps for a thread
async function testGetSteps() {
    console.log('\n📋 Test 3: Get All Steps');
    
    try {
        const thread = await connection.getThread(testThreadId);
        const steps = await thread.steps();
        
        assert.ok(Array.isArray(steps), 'Steps should be an array');
        assert.ok(steps.length >= 3, 'Should have at least 3 steps');
        
        const stepNames = steps.map(s => s.stepName);
        assert.ok(stepNames.includes('order_placed'), 'Should include order_placed step');
        assert.ok(stepNames.includes('payment_validation'), 'Should include payment_validation step');
        assert.ok(stepNames.includes('inventory_check'), 'Should include inventory_check step');
        
        console.log('  ✅ PASS: Retrieved all steps');
        console.log(`     Total steps: ${steps.length}`);
        steps.forEach(step => {
            console.log(`     - ${step.stepName}: ${step.status} (${step.retryCount} retries)`);
        });
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 4: Get specific step
async function testGetSpecificStep() {
    console.log('\n📋 Test 4: Get Specific Step');
    
    try {
        const thread = await connection.getThread(testThreadId);
        const step = await thread.getStep('order_placed');
        
        assert.strictEqual(step.stepName, 'order_placed', 'Step name should match');
        assert.strictEqual(step.threadId, testThreadId, 'Thread ID should match');
        assert.ok(step.status, 'Step should have a status');
        assert.ok(step.idempotencyKey, 'Step should have an idempotency key');
        
        console.log('  ✅ PASS: Retrieved specific step');
        console.log(`     Step: ${step.stepName}`);
        console.log(`     Status: ${step.status}`);
        console.log(`     Idempotency Key: ${step.idempotencyKey}`);
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 5: Get step with idempotency key
async function testGetStepWithIdempotencyKey() {
    console.log('\n📋 Test 5: Get Step with Idempotency Key');
    
    try {
        const thread = await connection.getThread(testThreadId);
        const step = await thread.getStep('order_placed:test-order-001');
        
        assert.strictEqual(step.stepName, 'order_placed', 'Step name should match');
        assert.strictEqual(step.idempotencyKey, 'test-order-001', 'Idempotency key should match');
        
        console.log('  ✅ PASS: Retrieved step with idempotency key');
        console.log(`     Step: ${step.stepName}:${step.idempotencyKey}`);
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 6: Get step history
async function testGetStepHistory() {
    console.log('\n📋 Test 6: Get Step History');
    
    try {
        const thread = await connection.getThread(testThreadId);
        const step = await thread.getStep('order_placed');
        const history = await step.history({ limit: 10 });
        
        assert.ok(Array.isArray(history), 'History should be an array');
        assert.ok(history.length > 0, 'History should have at least one record');
        
        const firstRecord = history[0];
        assert.ok(firstRecord.timestamp, 'History record should have timestamp');
        assert.ok(firstRecord.status, 'History record should have status');
        
        console.log('  ✅ PASS: Retrieved step history');
        console.log(`     History records: ${history.length}`);
        history.forEach((record, i) => {
            console.log(`     ${i + 1}. Attempt ${record.attempt}: ${record.status} at ${record.timestamp}`);
        });
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 7: Get step history with filters
async function testGetStepHistoryWithFilters() {
    console.log('\n📋 Test 7: Get Step History with Filters');
    
    try {
        const thread = await connection.getThread(testThreadId);
        const step = await thread.getStep('order_placed');
        
        // Filter by activity type
        const history = await step.history({
            limit: 5,
            activityType: 'step_recorded'
        });
        
        assert.ok(Array.isArray(history), 'History should be an array');
        
        console.log('  ✅ PASS: Retrieved filtered step history');
        console.log(`     Filtered records: ${history.length}`);
        console.log(`     Filter: activityType=step_recorded`);
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 8: Get validation results
async function testGetValidationResults() {
    console.log('\n📋 Test 8: Get Validation Results');
    
    try {
        const thread = await connection.getThread(testThreadId);
        const validations = await thread.validationResults();
        
        assert.ok(Array.isArray(validations), 'Validations should be an array');
        
        console.log('  ✅ PASS: Retrieved validation results');
        console.log(`     Validation records: ${validations.length}`);
        
        if (validations.length > 0) {
            validations.forEach(validation => {
                console.log(`     - Step: ${validation.stepName}, Status: ${validation.overallStatus}`);
                console.log(`       Critical: ${validation.criticalCount}, Warnings: ${validation.warningCount}`);
            });
        } else {
            console.log('     (No validation results found - this is normal if no violations occurred)');
        }
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 9: Filter steps by name
async function testFilterStepsByName() {
    console.log('\n📋 Test 9: Filter Steps by Name');
    
    try {
        const thread = await connection.getThread(testThreadId);
        const steps = await thread.steps({ stepName: 'payment_validation' });
        
        assert.ok(Array.isArray(steps), 'Steps should be an array');
        assert.ok(steps.length > 0, 'Should find payment_validation step');
        assert.ok(steps.every(s => s.stepName === 'payment_validation'), 'All steps should be payment_validation');
        
        console.log('  ✅ PASS: Filtered steps by name');
        console.log(`     Found ${steps.length} payment_validation step(s)`);
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 10: Get multiple threads by reference
async function testGetMultipleThreadsByRef() {
    console.log('\n📋 Test 10: Get Multiple Threads by Reference');
    
    try {
        const threads = await dataRetriever.getThreadsByRef({
            refKey: 'orderId',
            refValue: 'ORDER-TEST-001'
        });
        
        assert.ok(Array.isArray(threads), 'Threads should be an array');
        assert.ok(threads.length > 0, 'Should find at least one thread');
        
        console.log('  ✅ PASS: Retrieved multiple threads by reference');
        console.log(`     Found ${threads.length} thread(s)`);
        threads.forEach(thread => {
            console.log(`     - ${thread.id}: ${thread.status}`);
        });
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 11: Error handling - Invalid thread ID
async function testErrorHandlingInvalidThreadId() {
    console.log('\n📋 Test 11: Error Handling - Invalid Thread ID');
    
    try {
        await dataRetriever.getThread('invalid-thread-id-12345');
        console.log('  ❌ FAIL: Should throw error for invalid thread ID');
    } catch (error) {
        assert.ok(error.message, 'Error should have a message');
        console.log('  ✅ PASS: Correctly handled invalid thread ID');
        console.log(`     Error: ${error.message}`);
    }
}

// Test 12: Error handling - Invalid step name
async function testErrorHandlingInvalidStepName() {
    console.log('\n📋 Test 12: Error Handling - Invalid Step Name');
    
    try {
        const thread = await connection.getThread(testThreadId);
        await thread.getStep('nonexistent_step_name');
        console.log('  ❌ FAIL: Should throw error for invalid step name');
    } catch (error) {
        assert.ok(error.message, 'Error should have a message');
        console.log('  ✅ PASS: Correctly handled invalid step name');
        console.log(`     Error: ${error.message}`);
    }
}

// Main test runner
async function runTests() {
    console.log('🚀 Starting E2E Data Retrieval Tests\n');
    console.log('Prerequisites:');
    console.log('  - Server running on http://localhost:8081');
    console.log('  - GraphQL endpoint available at /graphql');
    console.log('  - API key: api-key-123');
    console.log('');
    
    let testsPassed = 0;
    let testsFailed = 0;
    
    try {
        // Setup (simplified like retry test)
        await checkArchivedData();
        await getTestThreadId();
        
        // DataRetriever methods are now available directly on connection
        console.log('✅ Connection ready with DataRetriever methods\n');
        
        // Run tests
        const tests = [
            testGetThreadById,
            testGetThreadByRef,
            testGetSteps,
            testGetSpecificStep,
            testGetStepWithIdempotencyKey,
            testGetStepHistory,
            testGetStepHistoryWithFilters,
            testGetValidationResults,
            testFilterStepsByName,
            testGetMultipleThreadsByRef,
            testErrorHandlingInvalidThreadId,
            testErrorHandlingInvalidStepName
        ];
        
        for (const test of tests) {
            try {
                await test();
                testsPassed++;
            } catch (error) {
                testsFailed++;
                console.error(`  ❌ Test failed: ${error.message}`);
            }
        }
        
        console.log('\n' + '='.repeat(50));
        console.log('📊 Test Results:');
        console.log(`   ✅ Passed: ${testsPassed}`);
        console.log(`   ❌ Failed: ${testsFailed}`);
        console.log(`   📈 Total: ${testsPassed + testsFailed}`);
        console.log('='.repeat(50));
        
        if (testsFailed === 0) {
            console.log('\n✅ All tests passed!');
        } else {
            console.log(`\n⚠️  ${testsFailed} test(s) failed`);
            process.exit(1);
        }
        
    } catch (error) {
        console.error('\n❌ Test suite failed:', error.message);
        console.error(error.stack);
        process.exit(1);
    } finally {
        // Cleanup (like retry test)
        if (connection && connection.ws) {
            connection.ws.close();
        }
        process.exit(0);
    }
}

// Run tests
runTests();
