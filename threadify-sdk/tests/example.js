import { Threadify } from '../src/index.js';

/**
 * Example usage of the Threadify SDK
 * This demonstrates the basic workflow of connecting, creating steps, and tracking progress
 */

async function runExample() {
  console.log('🚀 Starting Threadify SDK Example...\n');

  try {
    // Step 1: Connect to Threadify
    console.log('📡 Connecting to Threadify...');
    const thread = await Threadify.connect(
      'test-api-key-123',
      'payment-service',
      {
        url: 'ws://localhost:8081/threads',
        subscribedEvents: ['onSuccess', 'onError', 'onStepProgress']
      }
    );
    console.log('✅ Connected successfully!\n');

    // Step 2: Subscribe to events
    thread.on('onSuccess', (data) => {
      console.log('🎉 Success event:', data);
    });

    thread.on('onError', (data) => {
      console.error('❌ Error event:', data);
    });

    thread.on('onStepProgress', (data) => {
      console.log('📊 Progress event:', data);
    });

    // Step 3: Start the thread
    console.log('🏁 Starting thread...');
    const threadId = await thread.start('contract-payment-v1', {
      environment: 'test',
      version: '1.0.0'
    });
    console.log(`✅ Thread started with ID: ${threadId}\n`);

    // Step 4: Create and execute steps
    console.log('📝 Executing payment workflow...\n');

    // Step 4a: Initialize payment (auto-starts when created)
    const initStep = thread.step('initialize_payment');
    initStep.addContext({
      amount: 100.00,
      currency: 'USD',
      customerId: 'cust_123'
    });
    console.log('  ⏳ Step: initialize_payment - Started (auto)');
    
    // Simulate processing
    await sleep(1000);
    initStep.stop('success', { transactionId: 'txn_init_456' });
    console.log('  ✅ Step: initialize_payment - Completed\n');

    // Step 4b: Validate payment (auto-starts when created)
    const validateStep = thread.step('validate_payment');
    validateStep.addContext({
      transactionId: 'txn_init_456',
      validationRules: ['fraud_check', 'balance_check']
    });
    console.log('  ⏳ Step: validate_payment - Started (auto)');
    
    await sleep(1500);
    validateStep.stop('success', { 
      validationResult: 'passed',
      fraudScore: 0.02
    });
    console.log('  ✅ Step: validate_payment - Completed\n');

    // Step 4c: Process payment (auto-starts when created)
    const processStep = thread.step('process_payment');
    processStep.addContext({
      paymentMethod: 'card',
      last4: '4242'
    });
    console.log('  ⏳ Step: process_payment - Started (auto)');
    
    await sleep(2000);
    processStep.stop('success', { 
      status: 'succeeded',
      chargeId: 'ch_789'
    });
    console.log('  ✅ Step: process_payment - Completed\n');

    // Step 5: Summary
    console.log('📊 Thread Summary:');
    console.log(`  Thread ID: ${thread.getThreadId()}`);
    console.log(`  Contract ID: ${thread.getContractId()}`);
    console.log(`  Total Steps: ${thread.getAllSteps().length}`);
    console.log('  Steps:');
    thread.getAllSteps().forEach(step => {
      console.log(`    - ${step.stepName}: ${step.getStatus()}`);
    });

    // Step 6: Close connection
    console.log('\n🔌 Closing connection...');
    await thread.close();
    console.log('✅ Connection closed\n');

    console.log('🎉 Example completed successfully!');
    process.exit(0);

  } catch (error) {
    console.error('❌ Error:', error.message);
    process.exit(1);
  }
}

// Helper function to simulate async operations
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

// Run the example
runExample();
