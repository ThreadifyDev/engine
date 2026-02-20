import { Threadify } from '../src/index.js';

/**
 * SDK Data Retriever Usage Examples
 * Demonstrates how to retrieve archived thread data using GraphQL
 */

async function main() {
  // Configuration
  const WS_URL = 'ws://localhost:8081/threads';
  const API_KEY = 'your-api-key-here';

  // Create connection (DataRetriever methods are now available directly on connection)
  const connection = await Threadify.connect(API_KEY, 'data-retriever-service', { url: WS_URL });

  console.log('=== SDK Data Retriever Examples ===\n');

  // Example 1: Get thread by ID
  console.log('Example 1: Get Thread by ID');
  try {
    const thread = await connection.getThread('thread-123');
    console.log('Thread ID:', thread.id);
    console.log('Status:', thread.status);
    console.log('Contract:', thread.contractName);
    console.log('Started:', thread.startedAt);
    console.log('Completed:', thread.completedAt);
    console.log('');
  } catch (error) {
    console.error('Error:', error.message);
  }

  // Example 2: Get thread by reference
  console.log('Example 2: Get Thread by Reference');
  try {
    const thread = await connection.getThreadByRef({
      refKey: 'orderId',
      refValue: 'ORD-12345'
    });
    
    if (thread) {
      console.log('Found thread:', thread.id);
      console.log('Status:', thread.status);
      console.log('');
    } else {
      console.log('No thread found with that reference');
    }
  } catch (error) {
    console.error('Error:', error.message);
  }

  // Example 3: Get all steps for a thread
  console.log('Example 3: Get All Steps');
  try {
    const thread = await connection.getThread('thread-123');
    const steps = await thread.steps();
    
    console.log(`Found ${steps.length} steps:`);
    steps.forEach(step => {
      console.log(`  - ${step.stepName}: ${step.status} (${step.retryCount} retries)`);
    });
    console.log('');
  } catch (error) {
    console.error('Error:', error.message);
  }

  // Example 4: Get specific step with history
  console.log('Example 4: Get Step History');
  try {
    const thread = await connection.getThread('thread-123');
    const step = await thread.getStep('order_placed');
    
    console.log('Step:', step.stepName);
    console.log('Status:', step.status);
    console.log('Retry count:', step.retryCount);
    
    // Get step history with filters
    const history = await step.history({
      limit: 10,
      activityType: 'step_recorded'
    });
    
    console.log(`\nHistory (${history.length} records):`);
    history.forEach((record, i) => {
      console.log(`  ${i + 1}. Attempt ${record.attempt}: ${record.status} at ${record.timestamp}`);
      if (record.error) {
        console.log(`     Error: ${record.error}`);
      }
    });
    console.log('');
  } catch (error) {
    console.error('Error:', error.message);
  }

  // Example 5: Get validation results
  console.log('Example 5: Get Validation Results');
  try {
    const thread = await connection.getThread('thread-123');
    const validations = await thread.validationResults();
    
    console.log(`Found ${validations.length} validation results:`);
    validations.forEach(validation => {
      console.log(`\n  Step: ${validation.stepName}`);
      console.log(`  Status: ${validation.overallStatus}`);
      console.log(`  Critical: ${validation.criticalCount}, Warnings: ${validation.warningCount}`);
      
      if (validation.hasCriticalViolation) {
        console.log('  Critical violations:');
        validation.validations
          .filter(v => v.type === 'critical')
          .forEach(v => console.log(`    - ${v.message}`));
      }
    });
    console.log('');
  } catch (error) {
    console.error('Error:', error.message);
  }

  // Example 6: Advanced filtering - Get failed steps only
  console.log('Example 6: Get Failed Steps Only');
  try {
    const thread = await connection.getThread('thread-123');
    const allSteps = await thread.steps();
    const failedSteps = allSteps.filter(step => step.status === 'failed');
    
    console.log(`Found ${failedSteps.length} failed steps:`);
    for (const step of failedSteps) {
      console.log(`\n  Step: ${step.stepName}`);
      console.log(`  Retries: ${step.retryCount}`);
      
      // Get error details from history
      const history = await step.history({ limit: 1 });
      if (history.length > 0 && history[0].error) {
        console.log(`  Last error: ${history[0].error}`);
      }
    }
    console.log('');
  } catch (error) {
    console.error('Error:', error.message);
  }

  // Example 7: Time-based filtering
  console.log('Example 7: Time-Based History Filtering');
  try {
    const thread = await dataRetriever.getThread('thread-123');
    const step = await thread.getStep('payment_processed');
    
    // Get history for last 24 hours
    const yesterday = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();
    const history = await step.history({
      startAt: yesterday,
      limit: 50
    });
    
    console.log(`Events in last 24 hours: ${history.length}`);
    console.log('');
  } catch (error) {
    console.error('Error:', error.message);
  }

  // Example 8: Actor-based filtering
  console.log('Example 8: Filter by Actor');
  try {
    const thread = await dataRetriever.getThread('thread-123');
    const step = await thread.getStep('order_placed');
    
    // Get history for specific service
    const history = await step.history({
      actor: 'merchant-service',
      limit: 20
    });
    
    console.log(`Events from merchant-service: ${history.length}`);
    console.log('');
  } catch (error) {
    console.error('Error:', error.message);
  }

  // Example 9: Multiple threads by reference
  console.log('Example 9: Get Multiple Threads by Reference');
  try {
    const threads = await dataRetriever.getThreadsByRef({
      refKey: 'customerId',
      refValue: 'CUST-456'
    });
    
    console.log(`Found ${threads.length} threads for customer:`);
    threads.forEach(thread => {
      console.log(`  - ${thread.id}: ${thread.status} (${thread.contractName})`);
    });
    console.log('');
  } catch (error) {
    console.error('Error:', error.message);
  }

  // Example 10: Automated retry logic based on history
  console.log('Example 10: Automated Retry Decision');
  try {
    const thread = await dataRetriever.getThread('thread-123');
    const steps = await thread.steps();
    
    for (const step of steps) {
      if (step.status === 'failed' && step.retryCount < 3) {
        const history = await step.history({ limit: 5 });
        
        // Check if errors are transient
        const hasTransientErrors = history.some(h => 
          h.error && (h.error.includes('timeout') || h.error.includes('network'))
        );
        
        if (hasTransientErrors) {
          console.log(`✅ Step ${step.stepName} should be retried (transient errors detected)`);
        } else {
          console.log(`❌ Step ${step.stepName} should NOT be retried (permanent errors)`);
        }
      }
    }
    console.log('');
  } catch (error) {
    console.error('Error:', error.message);
  }

  console.log('=== Examples Complete ===');
}

// Run examples
main().catch(console.error);
