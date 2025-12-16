import { Threadify } from '../src/index.js';

async function testCloseSpeed() {
  console.log('🚀 Testing WebSocket close speed...\n');

  try {
    // Initialize Threadify
    console.log('📡 Connecting to Threadify...');
    const thread = await Threadify.connect('test-api-key-123', 'test-service');
    console.log('✅ Connected successfully!\n');

    // Start thread
    console.log('🏁 Starting thread...');
    const threadInstance = await thread.start(null, {
      environment: 'test',
      serviceName: 'test-service'
    });
    console.log(`✅ Thread started: ${threadInstance.getThreadId()}\n`);

    // Test closing speed
    console.log('🔌 Testing connection close speed...');
    const startTime = Date.now();
    await threadInstance.close();
    const endTime = Date.now();
    const closeTime = endTime - startTime;
    
    console.log(`✅ Connection closed in ${closeTime}ms\n`);
    
    if (closeTime < 3000) {
      console.log('✅ Close speed is excellent (< 3 seconds)');
    } else if (closeTime < 5000) {
      console.log('⚠️  Close speed is acceptable (< 5 seconds)');
    } else {
      console.log('❌ Close speed is too slow (> 5 seconds)');
    }

  } catch (error) {
    console.error('❌ Error:', error.message);
    console.error('Stack:', error.stack);
  }
}

testCloseSpeed();
