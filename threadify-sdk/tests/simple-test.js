import { Threadify } from '../src/index.js';

/**
 * Simple test to verify thread creation works
 */

async function testThreadCreation() {
  console.log('🚀 Testing Thread Creation...\n');

  try {
    // Step 1: Connect to Threadify
    console.log('📡 Connecting to Threadify...');
    const thread = await Threadify.connect(
      'test-api-key-123',
      'test-service',
      {
        url: 'ws://localhost:8081/threads',
        subscribedEvents: ['onSuccess', 'onError']
      }
    );
    console.log('✅ Connected successfully!\n');

    // Step 2: Start a thread with a contract ID
    console.log('🏁 Starting thread with contract...');
    const threadId = await thread.start('product_deliveries_new1:2', {
      environment: 'test',
    });

    thread.step("step_name", "sample service").addContext({data: "sample data"}).addContext({privateRecord: "hello"}, isPrivate = true).stop("success", "message", metadata = {})
    console.log(`✅ Thread started successfully!`);
    console.log(`   Thread ID: ${threadId}`);
    console.log(`   Contract ID: ${thread.getContractId()}\n`);

    // Step 3: Close connection
    console.log('🔌 Closing connection...');
    await thread.close();
    console.log('✅ Connection closed\n');

    console.log('🎉 Test completed successfully!');
    process.exit(0);

  } catch (error) {
    console.error('❌ Error:', error.message);
    console.error('Stack:', error.stack);
    process.exit(1);
  }
}

// Run the test
testThreadCreation();
