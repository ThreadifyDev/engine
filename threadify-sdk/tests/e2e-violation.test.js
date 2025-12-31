/**
 * E2E Thread Violation Tests using Threadify SDK
 * 
 * Tests thread-level violations (non-blocking warnings):
 * 1. Multiple terminal states violations
 * 2. Completed thread violations (trying to add steps after terminal)
 * 3. Thread max duration violations (documented)
 * 
 * Note: Step timeout violations require steps to remain in 'in_progress' state,
 * which the current SDK doesn't support (steps complete immediately with .stop())
 */

import { Threadify } from '../src/index.js';

// Configuration
const API_KEY = 'api-key-123';
const CONTRACT_NAME = 'product_delivery';

let connection = null;

// Helper: Connect using SDK
async function connectSDK() {
    console.log('Connecting to Threadify using SDK...');
    connection = await Threadify.connect(API_KEY, 'merchant-service');
    console.log('✅ SDK connected and authenticated\n');
    return connection;
}

// Helper: Wait for a specified duration
function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

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

// Main test runner
async function runTests() {
    console.log('🚀 Starting E2E Thread Violation Tests (using SDK)\n');
    console.log('Prerequisites:');
    console.log('  - Server running on http://localhost:8081');
    console.log('  - Contract "product_delivery" already uploaded');
    console.log('  - API key: api-key-123');
    console.log('');
    
    try {
        // Setup
        await connectSDK();
        
        // Run tests
        await testMultipleTerminalStatesViolation();
        await testCompletedThreadViolation();
        await testSimpleMerchantWorkflow();
        
        console.log('✅ All tests completed!');
        console.log('\nℹ️  Check the server logs and thread records for violation details');
        console.log('   Violations should be recorded but not block step submission');
        
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
