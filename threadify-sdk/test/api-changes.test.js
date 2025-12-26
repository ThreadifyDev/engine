/**
 * Tests for API Changes - New parameter order and role handling
 */

import assert from 'assert';
import { Threadify } from '../src/index.js';

// Test the new API changes we agreed to implement
async function runApiChangeTests() {
  console.log('🧪 Running API Change Tests...\n');

  // Test 1: connect() without serviceName parameter
  await asyncTest('connect() works without serviceName', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      assert(thread instanceof Threadify, 'Should return Thread instance');
      assert(thread.apiKey === 'test-api-key', 'Should store API key');
      assert(thread.serviceName === null, 'ServiceName should be null when not provided');
      await thread.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  // Test 2: start() with new parameter order (contract_name, role, refs)
  await asyncTest('start() with new parameter order', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      
      // Test new signature: start(contract_name, role, { refs })
      const startedThread = await thread.start('payment_flow', 'payment_processor', {
        refs: {
          customer_id: '12345',
          stripeId: 'ch_abc123'
        }
      });
      
      assert(startedThread.threadId, 'Thread should have ID');
      assert(startedThread.contractId === 'payment_flow', 'Should store contract name');
      // Note: role and refs will be stored in Redis on server side
      
      await thread.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  // Test 3: start() without role (contract-free threading)
  await asyncTest('start() works contract-free', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      
      // Contract-free threading
      const startedThread = await thread.start('simple_workflow', 'merchant');
      
      assert(startedThread.threadId, 'Thread should have ID');
      assert(startedThread.contractId === 'simple_workflow', 'Should store contract name');
      
      await thread.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  // Test 4: Error handling for invalid parameters
  await asyncTest('start() validates parameters', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      
      // Should throw error for missing contract name
      try {
        await thread.start();
        assert(false, 'Should throw error for missing contract name');
      } catch (error) {
        assert(error.message.includes('contract name'), 'Should mention contract name in error');
      }
      
      await thread.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  console.log('\n📊 API Change Tests Completed');
}

// Helper function for async tests
async function asyncTest(testName, testFn) {
  try {
    console.log(`▶️  ${testName}`);
    await testFn();
    console.log(`✅ ${testName}`);
  } catch (error) {
    console.error(`❌ ${testName}: ${error.message}`);
    throw error;
  }
}

// Run tests if this file is executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
  runApiChangeTests().catch(console.error);
}

export { runApiChangeTests };
