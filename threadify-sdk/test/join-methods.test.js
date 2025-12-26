/**
 * Tests for enhanced join methods - token and direct join
 */

import assert from 'assert';
import { Threadify } from '../src/index.js';

// Test the new join functionality
async function runJoinMethodTests() {
  console.log('🧪 Running Join Method Tests...\n');

  // Test 1: Token-based join (existing functionality)
  await asyncTest('token-based join works', async () => {
    try {
      const hostThread = await Threadify.connect('test-api-key');
      await hostThread.start('payment_flow', 'merchant', { customer_id: '12345' });
      
      const token = await hostThread.inviteParty({
        role: 'auditor',
        permissions: 'read'
      });
      
      const guestThread = await Threadify.connect('test-api-key');
      await guestThread.join(token); // Token-based join
      
      assert(guestThread.threadId === hostThread.threadId, 'Should join same thread');
      assert(guestThread.role === 'auditor', 'Should have correct role');
      
      await hostThread.close();
      await guestThread.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  // Test 2: Direct join with threadId and role
  await asyncTest('direct join with threadId and role', async () => {
    try {
      const hostThread = await Threadify.connect('test-api-key');
      await hostThread.start('payment_flow', 'merchant', { customer_id: '12345' });
      
      const internalService = await Threadify.connect('test-api-key');
      await internalService.join(hostThread.threadId, 'payment_processor'); // Direct join
      
      assert(internalService.threadId === hostThread.threadId, 'Should join same thread');
      // Note: role will be validated and set by server
      
      await hostThread.close();
      await internalService.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  // Test 3: Error handling for invalid join parameters
  await asyncTest('join() validates parameters', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      
      // Should throw error for missing parameters
      try {
        await thread.join();
        assert(false, 'Should throw error for missing parameters');
      } catch (error) {
        assert(error.message.includes('Token or threadId'), 'Should mention token/threadId in error');
      }
      
      // Should throw error for invalid parameter combination
      try {
        await thread.join('short_token', 'some_role'); // Short token with role should fail
        assert(false, 'Should throw error for invalid parameter combination');
      } catch (error) {
        assert(error.message.includes('Invalid join parameters'), 'Should mention invalid parameters');
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

  // Test 4: Join requires connection
  await asyncTest('join() requires connection', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      await thread.close(); // Close connection
      
      try {
        await thread.join('some_token');
        assert(false, 'Should throw error when not connected');
      } catch (error) {
        assert(error.message.includes('connected'), 'Should mention connection requirement');
      }
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  console.log('\n📊 Join Method Tests Completed');
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
  runJoinMethodTests().catch(console.error);
}

export { runJoinMethodTests };
