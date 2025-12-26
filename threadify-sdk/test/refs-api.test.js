/**
 * Tests for refs API - External system references
 */

import assert from 'assert';
import { Threadify } from '../src/index.js';

// Test the new refs functionality
async function runRefsApiTests() {
  console.log('🧪 Running Refs API Tests...\n');

  // Test 1: Thread creation with refs
  await asyncTest('Thread creation with refs', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      
      // Test new signature: start(contract_name, role, { refs })
      const startedThread = await thread.start('payment_flow', 'payment_processor', {
        refs: {
          customer_id: '12345',
          stripeId: 'ch_abc123',
          nhsId: 'NHS-789'
        }
      });
      
      assert(startedThread.threadId, 'Thread should have ID');
      assert(startedThread.contractId === 'payment_flow', 'Should store contract name');
      assert(startedThread.refs.customer_id === '12345', 'Should store refs');
      assert(startedThread.refs.stripeId === 'ch_abc123', 'Should store stripeId');
      
      await thread.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  // Test 2: Step creation with external_refs
  await asyncTest('Step creation with external_refs', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      
      // Test step with external_refs in options
      const step = thread.step('loan_application', {
        external_refs: {
          application_id: 'app_12345',
          credit_score: '750'
        }
      });
      
      assert(step.stepName === 'loan_application', 'Should have correct step name');
      assert(step.event.refs.application_id === 'app_12345', 'Should store external refs');
      assert(step.event.refs.credit_score === '750', 'Should store credit score');
      
      await thread.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  // Test 3: Chain addRefs method
  await asyncTest('Chain addRefs method', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      
      // Test chaining addRefs
      const step = thread.step('payment')
        .context({ amount: '$49.99' })
        .addRefs({ stripe_payment_id: 'pi_12345' })
        .addRefs({ merchant_id: 'mer_67890' });
      
      assert(step.event.context.amount === '$49.99', 'Should store context');
      assert(step.event.refs.stripe_payment_id === 'pi_12345', 'Should add first ref');
      assert(step.event.refs.merchant_id === 'mer_67890', 'Should add second ref');
      
      await thread.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  // Test 4: Parameter overloading for step()
  await asyncTest('step() parameter overloading', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      
      // Test step(name, options) overload
      const step1 = thread.step('payment', {
        external_refs: { payment_id: 'pay_123' }
      });
      assert(step1.event.refs.payment_id === 'pay_123', 'Should handle (name, options)');
      
      // Test step(name, serviceName, options) overload
      const step2 = thread.step('fraud_check', 'fraud-service', {
        external_refs: { fraud_token: 'token_456' }
      });
      assert(step2.event.refs.fraud_token === 'token_456', 'Should handle (name, service, options)');
      assert(step2.serviceName === 'fraud-service', 'Should use provided service name');
      
      await thread.close();
    } catch (error) {
      if (error.message.includes('WebSocket error') || error.message.includes('ECONNREFUSED')) {
        console.log('⚠️  Server not running - skipping test');
        return;
      }
      throw error;
    }
  });

  // Test 5: Error handling for invalid refs
  await asyncTest('refs validation', async () => {
    try {
      const thread = await Threadify.connect('test-api-key');
      
      const step = thread.step('test_step');
      
      // Should throw error for invalid refs data
      try {
        step.addRefs(null);
        assert(false, 'Should throw error for null refs');
      } catch (error) {
        assert(error.message.includes('Refs data must be an object'), 'Should validate refs type');
      }
      
      try {
        step.addRefs('invalid');
        assert(false, 'Should throw error for string refs');
      } catch (error) {
        assert(error.message.includes('Refs data must be an object'), 'Should validate refs type');
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

  console.log('\n📊 Refs API Tests Completed');
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
  runRefsApiTests().catch(console.error);
}

export { runRefsApiTests };
