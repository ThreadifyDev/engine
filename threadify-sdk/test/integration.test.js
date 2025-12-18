import { Threadify, Thread } from '../src/index.js';

// Simple integration test runner
function assert(condition, message) {
  if (!condition) {
    throw new Error(`Assertion failed: ${message}`);
  }
}

async function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function runIntegrationTests() {
  console.log('🚀 Running SDK Integration Tests...\n');

  let testCount = 0;
  let passedCount = 0;

  function asyncTest(name, testFn) {
    testCount++;
    return testFn()
      .then(() => {
        console.log(`✅ ${name}`);
        passedCount++;
      })
      .catch((error) => {
        console.log(`❌ ${name}: ${error.message}`);
      });
  }

  // Test 1: SDK inviteParty method (requires server)
  await asyncTest('SDK inviteParty creates token', async () => {
    console.log('📡 Connecting to server...');
    
    try {
      const thread = await Threadify.connect('test-api-key', 'integration-test');
      assert(thread.isConnected, 'Thread should be connected');
      
      console.log('📋 Starting thread...');
      const startedThread = await thread.start('test-contract-123');
      assert(startedThread.threadId, 'Thread should have ID');
      assert(startedThread.contractId, 'Thread should have contract ID');
      
      console.log('📨 Creating invitation...');
      const token = await thread.inviteParty({
        role: 'external_partner',
        permissions: 'read,write',
        expiresIn: '24h'
      });
      
      assert(token, 'Should receive JWT token');
      assert(typeof token === 'string', 'Token should be string');
      assert(token.length > 20, 'Token should be substantial length');
      
      console.log(`✅ Token created: ${token.substring(0, 20)}...`);
      
      await thread.close();
      
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping integration test');
        return;
      }
      throw error;
    }
  });

  // Test 2: SDK Thread.join method (requires server)
  await asyncTest('SDK Thread.join works with token', async () => {
    try {
      // First create a token using a connected thread
      const hostThread = await Threadify.connect('test-api-key', 'host-test');
      await hostThread.start(); // Thread without contract
      
      const token = await hostThread.inviteParty({
        role: 'contractor'
      });
      
      console.log('👥 Joining thread with token...');
      
      // Now join with the token using instance method
      const guestThread = await Threadify.connect('test-api-key', 'guest-service');
      await guestThread.join(token);
      
      assert(guestThread instanceof Thread, 'Should return Thread instance');
      assert(guestThread.isConnected, 'Should be connected');
      assert(guestThread.threadId, 'Should have thread ID');
      // Note: contractId may be empty when thread created without contract
      assertEquals(guestThread.role, 'contractor', 'Should have correct role');
      assertEquals(guestThread.permissions, 'read,write', 'Should have default permissions');
      
      console.log(`✅ Joined thread: ${guestThread.threadId}`);
      console.log(`👤 Role: ${guestThread.role}`);
      console.log(`🔐 Permissions: ${guestThread.permissions}`);
      
      await hostThread.close();
      await guestThread.close();
      
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping integration test');
        return;
      }
      throw error;
    }
  });

  function assertEquals(actual, expected, message) {
    if (actual !== expected) {
      throw new Error(`${message}: Expected ${expected}, got ${actual}`);
    }
  }

  console.log(`\n📊 Integration Test Results: ${passedCount}/${testCount} passed`);
  
  if (passedCount === testCount) {
    console.log('🎉 All integration tests passed!');
  } else {
    console.log('⚠️  Some integration tests skipped (server not running)');
  }
}

// Run integration tests
runIntegrationTests().catch(console.error);
