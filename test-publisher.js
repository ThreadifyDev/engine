#!/usr/bin/env node

/**
 * Publisher Test Script
 * 
 * This script ONLY publishes notifications (creates threads and records steps).
 * Use this with test-consumer.js to test HPA scenarios.
 * 
 * Usage:
 *   node test-publisher.js
 *   node test-publisher.js --count 10  # Publish 10 notifications
 *   node test-publisher.js --step order_placed  # Specific step
 *   
 * Environment variables:
 *   WS_URL - WebSocket URL (default: wss://eng.threadify.dev/threads)
 *   API_KEY - API key for authentication (default: test-api-key)
 *   NOTIFICATION_COUNT - Number of notifications to publish (default: 1)
 *   STEP_NAME - Step name to trigger (default: order_placed)
 */

import { Threadify } from '@threadify/sdk';
import { v4 as uuidv4 } from 'uuid';

// Configuration
const WS_URL = process.env.WS_URL || 'ws://localhost:8081/threads';
const API_KEY = process.env.API_KEY || 'api-key-123';
const SERVICE_NAME = 'test-publisher';
const NOTIFICATION_COUNT = parseInt(process.env.NOTIFICATION_COUNT) || parseInt(process.argv.find(arg => arg.startsWith('--count='))?.split('=')[1]) || 1;
const STEP_NAME = process.env.STEP_NAME || process.argv.find(arg => arg.startsWith('--step='))?.split('=')[1] || 'order_placed';

// Test state
let connection = null;
let publishedCount = 0;
let failedCount = 0;
let publishLatencies = [];

// Colors for console output
const colors = {
  reset: '\x1b[0m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  red: '\x1b[31m',
  cyan: '\x1b[36m',
  magenta: '\x1b[35m',
};

function log(message, color = 'reset') {
  console.log(`${colors[color]}${message}${colors.reset}`);
}

function logSuccess(message) {
  log(`✅ ${message}`, 'green');
}

function logError(message) {
  log(`❌ ${message}`, 'red');
}

function logInfo(message) {
  log(`ℹ️  ${message}`, 'blue');
}

function logPublish(message) {
  log(`📤 ${message}`, 'magenta');
}

// Connect using SDK
async function connect() {
  log('\n╔════════════════════════════════════════════════════════╗', 'cyan');
  log('║  Publisher Starting                                   ║', 'cyan');
  log('╚════════════════════════════════════════════════════════╝', 'cyan');
  
  logInfo('Connecting to Threadify...');
  connection = await Threadify.connect(API_KEY, SERVICE_NAME, {
    url: WS_URL,
    debug: true,
  });
  
  logSuccess('Connected to Threadify');
  return connection;
}

// Publish a notification
async function publishNotification(index) {
  try {
    const startTime = Date.now();
    
    logInfo(`[${index + 1}/${NOTIFICATION_COUNT}] Starting thread...`);
    const thread = await connection.start('product_delivery', 'merchant');
    
    logInfo(`[${index + 1}/${NOTIFICATION_COUNT}] Recording step: ${STEP_NAME}`);
    await thread.step(STEP_NAME)
      .addContext({
        order_id: `ORDER-${uuidv4().substring(0, 8)}`,
        customer_id: `CUST-${uuidv4().substring(0, 8)}`,
        total_amount: `${Math.floor(Math.random() * 1000) + 10}`,
      })
      .success();
    
    const latency = Date.now() - startTime;
    publishLatencies.push(latency);
    
    publishedCount++;
    logPublish(`Published notification ${index + 1}: ${STEP_NAME} (thread: ${thread.id.substring(0, 8)}...) [${latency}ms]`);
  } catch (error) {
    failedCount++;
    logError(`Failed to publish notification ${index + 1}: ${error.message}`);
  }
}

// Publish multiple notifications
async function publishNotifications() {
  log(`\n📤 Publishing ${NOTIFICATION_COUNT} notification(s) for step: ${STEP_NAME}\n`, 'cyan');
  
  for (let i = 0; i < NOTIFICATION_COUNT; i++) {
    await publishNotification(i);
  }
}

// Print summary
function printSummary() {
  log('\n╔════════════════════════════════════════════════════════╗', 'cyan');
  log('║  Publisher Summary                                    ║', 'cyan');
  log('╚════════════════════════════════════════════════════════╝', 'cyan');
  
  log('\n📊 Statistics:', 'cyan');
  logInfo(`   • Service: ${SERVICE_NAME}`);
  logInfo(`   • Step name: ${STEP_NAME}`);
  logInfo(`   • Notifications published: ${publishedCount}/${NOTIFICATION_COUNT}`);
  logInfo(`   • Success rate: ${((publishedCount / NOTIFICATION_COUNT) * 100).toFixed(1)}%`);
  
  // Calculate publish latency statistics
  if (publishLatencies.length > 0) {
    const avgLatency = (publishLatencies.reduce((a, b) => a + b, 0) / publishLatencies.length).toFixed(2);
    const minLatency = Math.min(...publishLatencies);
    const maxLatency = Math.max(...publishLatencies);
    const p95Latency = publishLatencies.sort((a, b) => a - b)[Math.floor(publishLatencies.length * 0.95)];
    
    log('\n⏱️  Publish Latency:', 'cyan');
    logInfo(`   • Average: ${avgLatency}ms`);
    logInfo(`   • Min: ${minLatency}ms`);
    logInfo(`   • Max: ${maxLatency}ms`);
    logInfo(`   • P95: ${p95Latency}ms`);
  }
  
  log('\n✅ Publisher completed successfully!', 'green');
}

// Main
async function main() {
  try {
    await connect();
    await publishNotifications();
    
    printSummary();
    
    log('\n✅ Publisher completed successfully!', 'green');
    
    // Close connection
    await connection.close();
    
    setTimeout(() => process.exit(0), 1000);
    
  } catch (error) {
    logError(`\n❌ Publisher failed: ${error.message}`);
    log(error.stack, 'red');
    
    if (connection) {
      await connection.close();
    }
    
    process.exit(1);
  }
}

// Handle process signals
process.on('SIGINT', async () => {
  log('\n\n⚠️  Publisher interrupted by user', 'yellow');
  printSummary();
  if (connection) {
    await connection.close();
  }
  setTimeout(() => process.exit(0), 1000);
});

// Run
main();
