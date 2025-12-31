#!/usr/bin/env node

/**
 * Simple test to verify validation results are archived
 * Follows valid contract transitions
 */

import { Threadify } from '../src/index.js';

const API_KEY = 'api-key-123';
const ENGINE_URL = 'ws://localhost:8081/threads';

async function testValidationArchival() {
    console.log('🧪 Testing Validation Results Archival\n');

    let connection = null;
    let thread = null;
    
    try {
        // Connect
        console.log('📡 Connecting to Threadify...');
        connection = await Threadify.connect(API_KEY, 'merchant-service');
        console.log('✅ Connected\n');

        // Start thread
        console.log('🚀 Starting thread with product_delivery contract...');
        thread = await connection.start('product_delivery', 'merchant-service');
        console.log(`✅ Thread started: ${thread.threadId}\n`);

        // Step 1: order_placed (entry point)
        console.log('📝 Step 1: order_placed');
        await thread
            .step('order_placed')
            .idempotencyKey('ORDER-001')
            .addContext({
                order_id: 'ORD-12345',
                customer_id: 'CUST-67890',
                total_amount: '99.99'
            })
            .stop('success', 'Order placed');
        console.log('✅ order_placed recorded\n');

        // Wait for async validation
        await new Promise(resolve => setTimeout(resolve, 2000));

        console.log(`\n✅ Test completed!`);
        console.log(`Thread ID: ${thread.threadId}`);
        console.log(`\n💡 Now check:`);
        console.log(`   1. Valkey stream: XLEN streams:validation_results`);
        console.log(`   2. Postgres table: SELECT * FROM validation_results WHERE thread_id='${thread.threadId}';`);

        if (connection) connection.close();
        process.exit(0);

    } catch (error) {
        console.error('❌ Test failed:', error.message);
        if (connection) connection.close();
        process.exit(1);
    }
}

testValidationArchival();
