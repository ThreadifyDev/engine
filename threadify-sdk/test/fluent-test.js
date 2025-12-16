import { Threadify } from '../src/index.js';

async function testFluentAPI() {
  console.log('🚀 Testing Fluent API...\n');

  try {
    // Initialize Threadify
    console.log('📡 Connecting to Threadify...');
    const thread = await Threadify.connect('test-api-key-123', 'test-service');
    console.log('✅ Connected successfully!\n');

    // Start thread without contract (should work)
    console.log('🏁 Starting thread without contract...');
    const startStartTime = Date.now();
    const threadInstance = await thread.start(null, {
      environment: 'test',
      serviceName: 'test-service'
    });
    const startEndTime = Date.now();
    console.log(`✅ Thread started successfully!`);
    console.log(`   Thread ID: ${threadInstance.getThreadId()}`);
    console.log(`   Contract ID: ${threadInstance.getContractId()}`);
    console.log(`   ⏱️  Start time: ${startEndTime - startStartTime}ms\n`);

    // Test fluent API
    console.log('🔄 Testing fluent step API...');
    const step1StartTime = Date.now();
    await threadInstance
      .step('test-step')
      .addContext({ data: 'sample data' })
      .addContext({ privateRecord: 'hello' }, true)
      .addMetadata({ version: '1.0', environment: 'test' })
      .stop('success', 'Step completed successfully', { result: 'passed' });
    const step1EndTime = Date.now();
    console.log('✅ Fluent API step completed successfully!');
    console.log(`   ⏱️  Step 1 time: ${step1EndTime - step1StartTime}ms\n`);

    // Test another step with failure
    console.log('❌ Testing step with failure...');
    await threadInstance
      .step('failing-step')
      .addContext({ attempt: '1' })
      .stop('failed', 'Step failed as expected', { error: 'test error' });

    await threadInstance
      .step('payment-step')
      .stop('error', 'Step failed', { error: 'payment failed' });

    await threadInstance
      .step('payment-step')
      .addContext({ amount: 100 })
      .stop('success', 'Payment successful', { result: 'passed' });
    
    console.log('✅ Failure step completed successfully!\n');

    // Close connection
    console.log('🔌 Closing connection...');
    const closeStartTime = Date.now();
    await threadInstance.close();
    const closeEndTime = Date.now();
    console.log(`✅ Connection closed!`);
    console.log(`   ⏱️  Close time: ${closeEndTime - closeStartTime}ms\n`);

    console.log('🎉 All fluent API tests passed!');

  } catch (error) {
    console.error('❌ Error:', error.message);
    console.error('Stack:', error.stack);
  }
}

testFluentAPI();
