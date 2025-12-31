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
  console.log('🧪 Testing SDK Convenience Methods...\n');

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

  // Test 1: success method exists
  test('ThreadStep has success method', () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    thread.threadId = 'test-thread';
    
    const step = thread.step('test_step');
    assert(typeof step.success === 'function', 'success method should exist');
    assert(typeof step.error === 'function', 'error method should exist');
    assert(typeof step.failed === 'function', 'failed method should exist');
  });

  // Test 2: success method calls stop with correct parameters
  asyncTest('success method calls stop with success status', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    thread.threadId = 'test-thread';
    
    const step = thread.step('test_step');
    
    // Mock the stop method
    let stopCalled = false;
    let stopStatus = '';
    let stopMessage = '';
    let stopResult = {};
    
    step.stop = async (status, message, result) => {
      stopCalled = true;
      stopStatus = status;
      stopMessage = message;
      stopResult = result;
      return { status: 'success' };
    };
    
    await step.success('Custom message', { data: 'test' });
    
    assert(stopCalled, 'stop should be called');
    assertEquals(stopStatus, 'success', 'status should be success');
    assertEquals(stopMessage, 'Custom message', 'message should be passed');
    assertEquals(stopResult.data, 'test', 'result should be passed');
  });

  // Test 3: error method calls stop with error status
  asyncTest('error method calls stop with error status', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    thread.threadId = 'test-thread';
    
    const step = thread.step('test_step');
    
    // Mock the stop method
    let stopCalled = false;
    let stopStatus = '';
    let stopMessage = '';
    let stopError = {};
    
    step.stop = async (status, message, error) => {
      stopCalled = true;
      stopStatus = status;
      stopMessage = message;
      stopError = error;
      return { status: 'error' };
    };
    
    await step.error('Something went wrong', { errorCode: 500 });
    
    assert(stopCalled, 'stop should be called');
    assertEquals(stopStatus, 'error', 'status should be error');
    assertEquals(stopMessage, 'Something went wrong', 'message should be passed');
    assertEquals(stopError.errorCode, 500, 'error should be passed');
  });

  // Test 4: failed method calls stop with failed status
  asyncTest('failed method calls stop with failed status', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    thread.threadId = 'test-thread';
    
    const step = thread.step('test_step');
    
    // Mock the stop method
    let stopCalled = false;
    let stopStatus = '';
    
    step.stop = async (status, message, error) => {
      stopCalled = true;
      stopStatus = status;
      return { status: 'failed' };
    };
    
    await step.failed();
    
    assert(stopCalled, 'stop should be called');
    assertEquals(stopStatus, 'failed', 'status should be failed');
  });

  // Test 5: success method uses default values
  asyncTest('success method uses default values', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    thread.threadId = 'test-thread';
    
    const step = thread.step('test_step');
    
    // Mock the stop method
    let stopMessage = '';
    let stopResult = {};
    
    step.stop = async (status, message, result) => {
      stopMessage = message;
      stopResult = result;
      return { status: 'success' };
    };
    
    await step.success();
    
    assertEquals(stopMessage, 'Step completed successfully', 'default message should be used');
    assertEquals(Object.keys(stopResult).length, 0, 'default result should be empty object');
  });

  // Test 6: error method uses default values
  asyncTest('error method uses default values', async () => {
    const thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    thread.threadId = 'test-thread';
    
    const step = thread.step('test_step');
    
    // Mock the stop method
    let stopMessage = '';
    let stopError = {};
    
    step.stop = async (status, message, error) => {
      stopMessage = message;
      stopError = error;
      return { status: 'error' };
    };
    
    await step.error();
    
    assertEquals(stopMessage, 'Step failed with error', 'default message should be used');
    assertEquals(Object.keys(stopError).length, 0, 'default error should be empty object');
  });

  // Wait for async tests to complete
  await new Promise(resolve => setTimeout(resolve, 100));

  console.log(`\n📊 Test Results: ${passedCount}/${testCount} passed`);
  
  if (passedCount === testCount) {
    console.log('🎉 All convenience method tests passed!');
    process.exit(0);
  } else {
    console.log('❌ Some tests failed!');
    process.exit(1);
  }
}

// Run tests
runTests().catch(console.error);
