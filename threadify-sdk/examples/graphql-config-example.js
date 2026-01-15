/**
 * Example: Using GraphQL URL Configuration in Threadify SDK
 * 
 * This demonstrates the new unified API for configuring both WebSocket and GraphQL endpoints
 */

import { Threadify } from '../src/index.js';

// Example 1: Custom GraphQL URL with WebSocket URL
async function example1_CustomGraphqlUrl() {
  console.log('🚀 Example 1: Custom GraphQL URL Configuration');
  
  const connection = await Threadify.connect('test-api-key', 'my-service', {
    wsUrl: 'wss://api.mycompany.com/threads',
    graphqlUrl: 'https://api.mycompany.com/graphql'
  });
  
  console.log('✅ Connected with custom GraphQL endpoint');
  
  // Use GraphQL methods through connection
  const thread = await connection.getThread('thread-id-123');
  console.log('📋 Retrieved thread:', thread.id);
  
  connection.ws.close();
}

// Example 2: Automatic GraphQL URL derivation
async function example2_AutoDerivedGraphqlUrl() {
  console.log('\n🚀 Example 2: Auto-derived GraphQL URL');
  
  const connection = await Threadify.connect('test-api-key', 'my-service', {
    wsUrl: 'wss://threadify.example.com/threads'
    // graphqlUrl will be automatically derived as: https://threadify.example.com/graphql
  });
  
  console.log('✅ Connected with auto-derived GraphQL endpoint');
  
  // Use GraphQL convenience methods
  const steps = await connection.getStep('thread-id-123', 'payment_step');
  console.log('📋 Retrieved step:', steps.stepName);
  
  connection.ws.close();
}

// Example 3: Using Threadify.create() with configuration
async function example3_ConfigurationObject() {
  console.log('\n🚀 Example 3: Configuration Object Pattern');
  
  const threadify = Threadify.create({
    apiKey: 'test-api-key',
    serviceName: 'payment-service',
    wsUrl: 'wss://payments.myapp.com/threads',
    graphqlUrl: 'https://payments.myapp.com/graphql'
  });
  
  const connection = await threadify.connect();
  
  console.log('✅ Connected using configuration object');
  
  // Query archived data
  const history = await connection.getStepHistory('thread-456', 'order_step', {
    limit: 10
  });
  console.log('📋 Retrieved step history:', history.length, 'records');
  
  connection.ws.close();
}

// Example 4: Backward compatibility (existing API still works)
async function example4_BackwardCompatibility() {
  console.log('\n🚀 Example 4: Backward Compatibility');
  
  // Old API still works - GraphQL URL is auto-derived
  const connection = await Threadify.connect('test-api-key', 'my-service', {
    url: 'ws://localhost:8081/threads'
  });
  
  console.log('✅ Connected using legacy API');
  
  // GraphQL methods work seamlessly
  const validationResults = await connection.getValidationResults('thread-789');
  console.log('📋 Retrieved validation results:', validationResults.length);
  
  connection.ws.close();
}

// Run examples (commented out for demo)
/*
async function runExamples() {
  try {
    await example1_CustomGraphqlUrl();
    await example2_AutoDerivedGraphqlUrl();
    await example3_ConfigurationObject();
    await example4_BackwardCompatibility();
    
    console.log('\n🎉 All examples completed successfully!');
  } catch (error) {
    console.error('❌ Example failed:', error);
  }
}

runExamples();
*/

export {
  example1_CustomGraphqlUrl,
  example2_AutoDerivedGraphqlUrl,
  example3_ConfigurationObject,
  example4_BackwardCompatibility
};
