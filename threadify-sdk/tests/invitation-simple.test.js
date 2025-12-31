import { Thread } from '../src/index.js';

// Simple test runner for ES modules
function assert(condition, message) {
  if (!condition) {
    throw new Error(`Assertion failed: ${message}`);
  }
}

function assertEquals(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}: Expected ${expected}, got ${actual}`);
  }
}

async function runTests() {
  console.log('🧪 Running SDK Invitation Tests...\n');

  let testCount = 0;
  let passedCount = 0;

  function test(name, testFn) {
    testCount++;
    try {
      testFn();
      console.log(`✅ ${name}`);
      passedCount++;
    } catch (error) {
      console.log(`❌ ${name}: ${error.message}`);
    }
  }

  function asyncTest(name, testFn) {
    testCount++;
    testFn()
      .then(() => {
        console.log(`✅ ${name}`);
        passedCount++;
      })
      .catch((error) => {
        console.log(`❌ ${name}: ${error.message}`);
      });
  }

  // Mock WebSocket for testing
  const mockWs = {
    send: () => {},
    on: () => {},
    close: () => {},
    readyState: 1 // OPEN
  };

  // Test 1: Thread creation
  test('Thread constructor creates instance', () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    assert(thread instanceof Thread, 'Thread should be instance of Thread');
    assertEquals(thread.apiKey, 'test-api-key', 'API key should be set');
    assertEquals(thread.ownerId, 'test-owner', 'Owner ID should be set');
  });

  // Test 2: inviteParty validation
  asyncTest('inviteParty throws error when role is missing', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    thread.threadId = 'test-thread';
    
    try {
      await thread.inviteParty({});
      throw new Error('Should have thrown error');
    } catch (error) {
      assertEquals(error.message, 'Role is required for inviteParty');
    }
  });

  // Test 3: inviteParty validation for connection
  asyncTest('inviteParty throws error when not connected', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = false;
    
    try {
      await thread.inviteParty({ role: 'contractor' });
      throw new Error('Should have thrown error');
    } catch (error) {
      assertEquals(error.message, 'Thread must be connected and started to create invitations');
    }
  });

  // Test 4: inviteParty with valid parameters
  asyncTest('inviteParty works with valid parameters', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    thread.threadId = 'test-thread';
    
    // Mock the _onceResponse method
    thread._onceResponse = (callback) => {
      setTimeout(() => callback({
        action: 'inviteParty',
        status: 'success',
        threadToken: 'test-jwt-token'
      }), 10);
    };
    
    thread._send = () => {};
    
    const token = await thread.inviteParty({ 
      role: 'contractor',
      permissions: 'read',
      expiresIn: '7d'
    });
    
    assertEquals(token, 'test-jwt-token');
  });

  // Test 5: Thread.join validation
  asyncTest('Thread.join throws error when token is missing', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    
    try {
      await thread.join('');
      throw new Error('Should have thrown error');
    } catch (error) {
      assertEquals(error.message, 'Thread token is required for join');
    }
  });

  // Test 6: Thread.join validation for connection
  asyncTest('Thread.join throws error when not connected', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = false;
    
    try {
      await thread.join('token');
      throw new Error('Should have thrown error');
    } catch (error) {
      assertEquals(error.message, 'Thread must be connected to join. Call Threadify.connect() first.');
    }
  });

  // Wait for async tests to complete
  await new Promise(resolve => setTimeout(resolve, 100));

  console.log(`\n📊 Test Results: ${passedCount}/${testCount} passed`);
  
  if (passedCount === testCount) {
    console.log('🎉 All tests passed!');
    process.exit(0);
  } else {
    console.log('❌ Some tests failed!');
    process.exit(1);
  }
}

// Run tests
runTests().catch(console.error);
