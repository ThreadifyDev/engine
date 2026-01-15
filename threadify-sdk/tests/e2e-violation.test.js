/**
 * E2E Thread and Step Data Tests using Threadify SDK
 * 
 * Tests retrieving thread-level and step-level data
 */

import { Threadify } from '../src/index.js';
import assert from 'assert';

// Configuration
const API_KEY = 'api-key-123';
const WS_URL = 'wss://eng.threadify.dev/threads';
const TEST_THREAD_ID = '64977480-56e7-4ea0-9318-dc2c0fd6a955'; // Use one of the thread IDs from previous test

let connection = null;

// Helper: Connect using SDK
async function connectSDK() {
    console.log('Connecting to Threadify using SDK...');
    connection = await Threadify.connect(API_KEY, 'test-service', { url: WS_URL });
    console.log('✅ SDK connected and authenticated\n');
    return connection;
}

// Helper: Wait for a specified duration
function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

// Test 1: Get complete thread picture using new getCompleteData() method
async function testGetCompleteThread() {
    console.log('📋 Test 1: Get Complete Thread Picture (Single Query)');
    console.log('   Using thread.getCompleteData() - retrieves everything in one GraphQL query\n');
    
    try {
        const thread = await connection.getThread(TEST_THREAD_ID);
        
        assert.strictEqual(thread.id, TEST_THREAD_ID, 'Thread ID should match');
        assert.ok(thread.ownerId, 'Thread should have owner ID');
        
        // Get complete data in a single query
        const completeData = await thread.getCompleteData({
            stepHistoryLimit: 50,
            validationLimit: 10
        });
        
        assert.ok(completeData, 'Complete data should be returned');
        assert.ok(Array.isArray(completeData.steps), 'Steps should be an array');
        assert.ok(completeData.steps.length > 0, 'Thread should have at least one step');
        
        console.log('  ✅ PASS: Retrieved complete thread picture in single query');
        console.log(`\n  📊 Thread Metadata:`);
        console.log(`     ID: ${completeData.id}`);
        console.log(`     Owner: ${completeData.ownerId}`);
        console.log(`     Company: ${completeData.companyId || 'N/A'}`);
        console.log(`     Contract: ${completeData.contractName || 'N/A'}`);
        console.log(`     Version: ${completeData.contractVersion || 'N/A'}`);
        console.log(`     Status: ${completeData.status || 'N/A'}`);
        console.log(`     Started: ${completeData.startedAt || 'N/A'}`);
        console.log(`     Completed: ${completeData.completedAt || 'N/A'}`);
        
        if (completeData.refs) {
            console.log(`     Refs:`, completeData.refs);
        }
        
        console.log(`\n  📝 Steps (${completeData.steps.length} total):`);
        
        // Display steps with their history (already included in the response)
        for (let i = 0; i < Math.min(completeData.steps.length, 3); i++) {
            const step = completeData.steps[i];
            console.log(`\n     ${i + 1}. ${step.stepName}:${step.idempotencyKey}`);
            console.log(`        Status: ${step.status}`);
            console.log(`        Retry Count: ${step.retryCount}`);
            console.log(`        First Seen: ${step.firstSeenAt}`);
            console.log(`        Last Updated: ${step.lastUpdatedAt}`);
            
            // History is already included in the response!
            if (step.history && step.history.length > 0) {
                console.log(`        History (${step.history.length} attempts):`);
                step.history.forEach((record) => {
                    console.log(`          - Attempt ${record.attempt}: ${record.status} at ${record.timestamp}`);
                });
            } else {
                console.log(`        History: No records`);
            }
        }
        
        if (completeData.steps.length > 3) {
            console.log(`\n     ... and ${completeData.steps.length - 3} more steps`);
        }
        
        // Validation results are also included in the response
        if (completeData.validationResults && completeData.validationResults.length > 0) {
            console.log(`\n  ⚠️  Validation Results (${completeData.validationResults.length} total):`);
            completeData.validationResults.forEach((val, i) => {
                console.log(`     ${i + 1}. ${val.stepName} - ${val.overallStatus}`);
                console.log(`        Critical: ${val.criticalCount}, Warnings: ${val.warningCount}, Info: ${val.infoCount}`);
            });
        } else {
            console.log(`\n  ⚠️  Validation Results: None found`);
        }
        
        console.log(`\n  💡 All data retrieved in a single GraphQL query!`);
        console.log(`     - Thread metadata: ✓`);
        console.log(`     - ${completeData.steps.length} steps with state: ✓`);
        console.log(`     - Step history for each step: ✓`);
        console.log(`     - ${completeData.validationResults?.length || 0} validation results: ✓`);
        
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 2: Get step history
async function testGetStepHistory() {
    console.log('\n📋 Test 2: Get Step History');
    
    try {
        const thread = await connection.getThread(TEST_THREAD_ID);
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

// COMMENTED OUT: Get Step Tests
/*
// Test 1: Get specific step by name
async function testGetSpecificStep() {
    console.log('📋 Test 1: Get Specific Step (by name only)');
    
    try {
        const thread = await connection.getThread(TEST_THREAD_ID);
        const step = await thread.getStep('order_placed');
        
        assert.strictEqual(step.stepName, 'order_placed', 'Step name should match');
        assert.strictEqual(step.threadId, TEST_THREAD_ID, 'Thread ID should match');
        assert.ok(step.status, 'Step should have a status');
        assert.ok(step.idempotencyKey, 'Step should have an idempotency key');
        
        console.log('  ✅ PASS: Retrieved specific step');
        console.log(`     Step: ${step.stepName}`);
        console.log(`     Status: ${step.status}`);
        console.log(`     Idempotency Key: ${step.idempotencyKey}`);
        console.log(`     Thread ID: ${step.threadId}`);
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}

// Test 2: Get step with idempotency key
async function testGetStepWithIdempotencyKey() {
    console.log('\n📋 Test 2: Get Step with Idempotency Key (stepName:idempKey)');
    
    try {
        const thread = await connection.getThread(TEST_THREAD_ID);
        const step = await thread.getStep('order_placed:test-001');
        
        assert.strictEqual(step.stepName, 'order_placed', 'Step name should match');
        assert.strictEqual(step.idempotencyKey, 'test-001', 'Idempotency key should match');
        
        console.log('  ✅ PASS: Retrieved step with idempotency key');
        console.log(`     Step: ${step.stepName}:${step.idempotencyKey}`);
        console.log(`     Status: ${step.status}`);
        console.log(`     Thread ID: ${step.threadId}`);
    } catch (error) {
        console.log('  ❌ FAIL:', error.message);
        throw error;
    }
}
*/

// COMMENTED OUT: Get Thread By Ref Test
/*
async function testGetThreadByRef() {
    console.log('📋 Test: Get Thread By Ref');
    console.log('   Testing retrieval of threads using external references\n');
    
    try {
        // Query using a known ref from e2e-data-retrieval test
        console.log('   🔍 Querying threads with refKey="orderId", refValue="ORDER-TEST-E2E"...');
        const threads = await connection.getThreadsByRef({
            refKey: 'orderId',
            refValue: 'ORDER-TEST-E2E'
        });
        
        console.log(`   ✅ Found ${threads.length} thread(s) with this ref`);
        
        if (threads.length > 0) {
            threads.forEach((thread, index) => {
                console.log(`\n   Thread ${index + 1}:`);
                console.log(`      ID: ${thread.id}`);
                console.log(`      Contract: ${thread.contractName || 'N/A'}`);
                console.log(`      Status: ${thread.status || 'N/A'}`);
                console.log(`      Owner: ${thread.ownerId || 'N/A'}`);
                console.log(`      Created: ${thread.createdAt || 'N/A'}`);
                
                if (thread.refs) {
                    console.log(`      Refs:`, thread.refs);
                }
            });
            
            console.log('\n   ✅ PASS: Successfully retrieved threads by ref');
        } else {
            console.log('   ℹ️  No threads found with this ref');
            console.log('   💡 Run e2e-data-retrieval.test.js first to create test data');
        }
        
    } catch (error) {
        console.log('   ❌ FAIL: Error retrieving threads by ref');
        console.log(`      Error: ${error.message}`);
        throw error;
    }
}
*/

// COMMENTED OUT TESTS - Uncomment if needed
/*
// Test 1: Multiple Terminal States Violation
async function testMultipleTerminalStatesViolation() {
    console.log('📋 Test 1: Multiple Terminal States Violation');
    console.log('   Contract: allow_multiple_terminals = false');
    console.log('   Terminal steps: delivered, order_cancelled\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread created: ${thread.threadId}`);
    
    // Submit order_placed (entry point)
    await thread
        .step('order_placed')
        .addContext({ 
            order_id: 'ORDER-TERMINAL-1',
            customer_id: 'CUST-001',
            total_amount: '299.99'
        })
        .stop('success', 'Order placed');
    console.log('   ✅ Step: order_placed');
    
    // Cancel the order (terminal state)
    await thread
        .step('order_cancelled')
        .addContext({ 
            cancellation_reason: 'Customer requested cancellation'
        })
        .stop('success', 'Order cancelled');
    console.log('   ✅ Step: order_cancelled (TERMINAL)');
    
    console.log('   ℹ️  Thread reached terminal state: order_cancelled');
    console.log('   ℹ️  Contract does not allow multiple terminal states');
    console.log(`      Thread ID: ${thread.threadId}\n`);
}

// Test 2: Completed Thread Violation (trying to add steps after terminal)
async function testCompletedThreadViolation() {
    console.log('📋 Test 2: Completed Thread Violation');
    console.log('   Attempting to add steps after thread reaches terminal state\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread created: ${thread.threadId}`);
    
    // Submit order_placed
    await thread
        .step('order_placed')
        .addContext({ 
            order_id: 'ORDER-COMPLETED-1',
            customer_id: 'CUST-002',
            total_amount: '399.99'
        })
        .stop('success', 'Order placed');
    console.log('   ✅ Step: order_placed');
    
    // Cancel the order (terminal state)
    await thread
        .step('order_cancelled')
        .addContext({ 
            cancellation_reason: 'Out of stock'
        })
        .stop('success', 'Order cancelled');
    console.log('   ✅ Step: order_cancelled (TERMINAL - thread completed)');
    
    // Wait for async validation to complete
    console.log('   ⏳ Waiting for async validation to complete...');
    await sleep(100); // Give async processing time to mark thread as completed
    
    // Try to add another merchant step after terminal (should be blocked)
    console.log('   ⚠️  Attempting to add step to completed thread...');
    try {
        await thread
            .step('order_placed')
            .addContext({ 
                order_id: 'ORDER-AFTER-TERMINAL',
                customer_id: 'CUST-999',
                total_amount: '999.99'
            })
            .stop('success', 'This should fail');
        
        console.log('   ❌ FAIL: Should not be able to add steps to completed thread\n');
    } catch (error) {
        if (error.message.includes('completed') || error.message.includes('terminal')) {
            console.log('   ✅ PASS: Completed thread check blocked the step');
            console.log(`      Error: ${error.message}\n`);
        } else {
            console.log('   ⚠️  Unexpected error:', error.message, '\n');
        }
    }
}

// Test 3: Simple Merchant-Only Workflow
async function testSimpleMerchantWorkflow() {
    console.log('📋 Test 3: Simple Merchant-Only Workflow');
    console.log('   Testing merchant steps only (order_placed -> order_cancelled)\n');
    
    const thread = await connection.start(CONTRACT_NAME, 'merchant-service');
    console.log(`   Thread created: ${thread.threadId}`);
    
    // Step 1: order_placed (entry point, merchant-owned)
    await thread
        .step('order_placed')
        .addContext({ 
            order_id: 'ORDER-MERCHANT-1',
            customer_id: 'CUST-003',
            total_amount: '499.99'
        })
        .stop('success', 'Order placed');
    console.log('   ✅ Step: order_placed');
    
    // Step 2: order_cancelled (terminal, merchant-owned)
    await thread
        .step('order_cancelled')
        .addContext({ 
            cancellation_reason: 'Merchant cancelled order'
        })
        .stop('success', 'Order cancelled by merchant');
    console.log('   ✅ Step: order_cancelled (TERMINAL)');
    
    console.log('   ℹ️  Thread completed successfully with terminal state: order_cancelled');
    console.log(`      Thread ID: ${thread.threadId}\n`);
}
*/

// Test: Contract-based notification filtering with actual notifications
async function testContractFiltering() {
    console.log('\n📋 Test: Contract-Based Notification Filtering with Real Notifications\n');
    
    let violationReceived = false;
    let completedReceived = false;
    let contractFilteredReceived = false;
    let wrongContractReceived = false;
    
    // Test 1: Subscribe to failed events for test_step
    console.log('  1️⃣  Subscribing to failed events for test_step...');
    connection.onFailed('test_step', (notification) => {
        console.log(`     ✅ Failed received for test_step: ${notification.stepName}, status: ${notification.stepStatus}`);
        console.log(`        Thread: ${notification.threadId}`);
        console.log(`        Contract: ${notification.contractName || 'none'}`);
        violationReceived = true;
        console.log(`        [DEBUG] violationReceived set to: ${violationReceived}`);
    });
    
    // Test 2: Subscribe to completed events for test_step
    console.log('  2️⃣  Subscribing to completed for test_step...');
    connection.onCompleted('test_step', (notification) => {
        console.log(`     ✅ Completed received for test_step: ${notification.stepName}, event: ${notification.eventType}`);
        console.log(`        Thread: ${notification.threadId}`);
        completedReceived = true;
    });
    
    // Test 3: Subscribe with contract filter
    console.log('  3️⃣  Subscribing to failed events for product_delivery@order_placed...');
    connection.onFailed('product_delivery@order_placed', (notification) => {
        console.log(`     ✅ Contract-filtered notification: ${notification.contractName}@${notification.stepName}`);
        console.log(`        Thread: ${notification.threadId}`);
        contractFilteredReceived = true;
    });
    
    // Test 4: Subscribe to a different contract to verify filtering
    console.log('  4️⃣  Subscribing to failed events for wrong_contract@order_placed (should NOT receive)...');
    connection.onFailed('wrong_contract@order_placed', (notification) => {
        console.log(`     ❌ FAIL: Received notification for wrong contract!`);
        wrongContractReceived = true;
    });
    
    console.log('\n  ℹ️  Subscriptions registered. Waiting for confirmations...\n');
    await sleep(500);
    
    // Now create threads and generate notifications
    console.log('  📝 Creating threads and generating notifications...\n');
    
    // Thread 1: No contract, test_step - should trigger violation handler
    console.log('  Creating Thread 1: No contract, test_step (will fail to trigger violation)');
    const thread1 = await connection.start();
    await thread1.step('test_step')
        .addContext({ test: 'data' })
        .stop('failed', 'Test failed');  // Failed status triggers violation
    
    await sleep(1500);
    
    // Thread 2: No contract, test_step completed
    console.log('  Creating Thread 2: No contract, test_step (success)');
    const thread2 = await connection.start();
    await thread2.step('test_step')
        .addContext({ test: 'data2' })
        .stop('success', 'Test complete');
    
    await sleep(1500);
    
    // Thread 3: product_delivery contract, order_placed - should trigger contract-filtered handler
    console.log('  Creating Thread 3: product_delivery contract, order_placed (will fail)');
    const thread3 = await connection.start('product_delivery', 'merchant-service');  // (contractName, serviceName with correct role)
    await thread3.step('order_placed')
        .addContext({ order_id: 'order-123', customer_id: 'cust-456', total_amount: '100.00', items: '2' })
        .stop('failed', 'Order failed');  // Failed status triggers violation
    
    await sleep(2000);
    
    console.log('\n  📊 Test Results:');
    console.log(`     Violation received (test_step): ${violationReceived ? '✅' : '❌'}`);
    console.log(`     Completed received (test_step): ${completedReceived ? '✅' : '❌'}`);
    console.log(`     Contract-filtered received (product_delivery@order_placed): ${contractFilteredReceived ? '✅' : '❌'}`);
    console.log(`     Wrong contract received (should be false): ${wrongContractReceived ? '❌ FAIL' : '✅ PASS'}`);
    
    if (violationReceived && completedReceived && contractFilteredReceived && !wrongContractReceived) {
        console.log('\n  ✅ PASS: All filtering tests passed!\n');
    } else {
        console.log('\n  ❌ FAIL: Some tests did not pass as expected\n');
    }
}

// Test: No subscription = no notifications
async function testNoSubscriptionNoNotifications() {
    console.log('\n📋 Test: No Subscription = No Notifications\n');
    
    // Create a fresh connection without any subscriptions
    const testConnection = await Threadify.connect(API_KEY, 'test-no-sub', { url: WS_URL });
    
    let notificationReceived = false;
    
    // Set up a listener to catch any notifications (shouldn't receive any)
    testConnection.ws.on('message', (data) => {
        try {
            const message = JSON.parse(data.toString());
            if (message.action === 'notification') {
                notificationReceived = true;
                console.log('  ❌ FAIL: Received notification without subscription!');
            }
        } catch (e) {
            // Ignore
        }
    });
    
    console.log('  ℹ️  Connected without any subscriptions.');
    console.log('  ℹ️  Should NOT receive any notifications.');
    
    await sleep(2000);
    
    if (!notificationReceived) {
        console.log('  ✅ PASS: No notifications received (as expected)\n');
    }
    
    testConnection.ws.close();
}

// Test: Thread Linking and Chain Traversal
async function testThreadLinkingAndChain() {
    console.log('🔗 Test: Thread Linking and Chain Traversal');
    console.log('   Testing linkThread() and threadChain GraphQL query\n');
    
    try {
        // Create fresh connection for this test
        const testConnection = await Threadify.connect(API_KEY, 'test-linking', { url: WS_URL });
        
        // Create root thread
        console.log('  Creating root thread...');
        const rootThread = await testConnection.start(null, 'order-service');
        await rootThread.step('order_placed').addContext({ 
            orderId: 'ORD-12345',
            amount: 99.99 
        }).success();
        
        console.log(`  ✅ Root thread created: ${rootThread.threadId}`);
        
        // Create child thread and link to root
        console.log('  Creating child thread and linking to root...');
        const childThread = await testConnection.start(null, 'payment-service');
        await childThread.linkThread(rootThread.threadId, 'parent');
        await childThread.step('payment_processed').addContext({ 
            paymentId: 'PAY-67890',
            amount: 99.99 
        }).success();
        
        console.log(`  ✅ Child thread created and linked: ${childThread.threadId}`);
        
        // Create grandchild thread and link to child
        console.log('  Creating grandchild thread and linking to child...');
        const grandchildThread = await testConnection.start(null, 'notification-service');
        await grandchildThread.linkThread(childThread.threadId, 'parent');
        await grandchildThread.step('notification_sent').addContext({ 
            notificationId: 'NOTIF-11111',
            recipient: 'customer@example.com'
        }).success();
        
        console.log(`  ✅ Grandchild thread created and linked: ${grandchildThread.threadId}`);
        
        // Wait for threads to be persisted
        await sleep(1000);
        
        // Test threadChain query from root
        console.log('  Testing threadChain query from root...');
        const chainThreads = await testConnection.getThreadChain(rootThread.threadId, 3);
        
        console.log(`  ✅ Thread chain retrieved: ${chainThreads.length} threads`);
        
        // Verify chain contains all threads
        const threadIds = chainThreads.map(t => t.id);
        assert.ok(threadIds.includes(rootThread.threadId), 'Root thread should be in chain');
        assert.ok(threadIds.includes(childThread.threadId), 'Child thread should be in chain');
        assert.ok(threadIds.includes(grandchildThread.threadId), 'Grandchild thread should be in chain');
        
        // Verify linkedThread refs are present (refs is JSON object parsed by ArchivedThread)
        const childThreadData = chainThreads.find(t => t.id === childThread.threadId);
        assert.ok(childThreadData.refs, 'Child thread should have refs');
        assert.ok(childThreadData.refs['linkedThread:parent'], 'Child thread should have linkedThread:parent ref');
        assert.strictEqual(childThreadData.refs['linkedThread:parent'], rootThread.threadId, 'Parent ref should point to root thread');
        
        const grandchildThreadData = chainThreads.find(t => t.id === grandchildThread.threadId);
        assert.ok(grandchildThreadData.refs['linkedThread:parent'], 'Grandchild should have linkedThread:parent ref');
        assert.strictEqual(grandchildThreadData.refs['linkedThread:parent'], childThread.threadId, 'Grandchild parent ref should point to child thread');
        
        console.log('  ✅ Thread chain structure verified');
        console.log('  ✅ LinkedThread refs correctly stored and retrieved');
        
        // Test threadChain with different depth
        console.log('  Testing threadChain with depth=2...');
        const shallowChain = await testConnection.getThreadChain(rootThread.threadId, 2);
        assert.ok(shallowChain.length >= 2, 'Should return at least root and child with depth=2');
        assert.ok(shallowChain.length <= 3, 'Should not exceed depth+1 threads (root + 2 descendants)');
        
        console.log(`  ✅ Depth-limited chain retrieved: ${shallowChain.length} threads`);
        
        console.log('\n  ✅ All thread linking and chain tests passed!\n');
        
    } catch (error) {
        console.error('  ❌ Thread linking test failed:', error.message);
        throw error;
    }
}

// Main test runner
async function runTests() {
    console.log('🚀 Starting E2E Threadify Tests (Notifications + Linking)\n');
    console.log('Prerequisites:');
    console.log('  - Server running on http://localhost:8081');
    console.log('  - API key: api-key-123');
    console.log('  - Contract filtering implemented');
    console.log('  - Thread linking and threadChain implemented');
    console.log('');
    
    try {
        // Setup
                // Create fresh connection for this test
        const testConnection = await Threadify.connect("test-api-key", 'test-linking', { url: WS_URL });
        
        // Create root thread
        console.log('  Creating root thread...');
        const rootThread = await testConnection.start("product_delivery", 'merchant');
        console.log("Got here")
        // Create a second thread to link to
        // const linkedThread = await testConnection.start("product_delivery", 'merchant');
        // await linkedThread.step('link_test').addContext({ 
        //     test: 'creating linked thread'
        // }).success();
        
        // // Link root thread to the valid linked thread
        // await rootThread.linkThread(linkedThread.threadId, 'parent');
        
        const t= await rootThread.step('order_placed').addContext({ 
            order_id: 'ORD-12345',
            customer_id: "hu",
            total_amount: 99.99 
        }).success();
        console.log("T", t)
        // await connectSDK();
        
        // // Run notification filtering tests
        // await testContractFiltering();
        // await testNoSubscriptionNoNotifications();
        
        // // Run thread linking tests
        // await testThreadLinkingAndChain();
        
        // console.log('\n✅ All tests completed!');
        // console.log('\n💡 Features Validated:');
        // console.log('   - Real-time notifications with filtering');
        // console.log('   - Thread linking via linkThread()');
        // console.log('   - Thread chain traversal via GraphQL');
        // console.log('   - Both query and field resolvers for threadChain');
        // console.log('   - Complete thread picture via nested queries');
        
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
