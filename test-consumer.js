#!/usr/bin/env node

/**
 * Consumer Test Script
 * 
 * This script ONLY connects and listens for notifications.
 * Use this with test-publisher.js to test HPA scenarios.
 * 
 * Usage:
 *   node test-consumer.js
 *   
 * Environment variables:
 *   WS_URL - WebSocket URL (default: wss://eng.threadify.dev/threads)
 *   API_KEY - API key for authentication (default: test-api-key)
 *   MAX_IN_FLIGHT - Flow control limit (default: 20)
 *   CONSUMER_ID - Unique consumer ID (default: random UUID)
 */

import { Threadify } from '@threadify/sdk';
import { v4 as uuidv4 } from 'uuid';

// Configuration
const WS_URL = process.env.WS_URL || 'ws://localhost:8081/threads';
const API_KEY = 'td_oqyuBOZtddPLxbcTUO1VxkRRLW47PwRvxvq-zTQsFB8' || process.env.API_KEY || 'api-key-123';
const SERVICE_NAME = process.env.SERVICE_NAME || 'test-service';
const MAX_IN_FLIGHT = parseInt(process.env.MAX_IN_FLIGHT) || 20;
const CONSUMER_ID = process.env.CONSUMER_ID || uuidv4();

// Test state
let connection = null;
let receivedNotifications = [];
let latencies = [];

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

function logNotification(message) {
  log(`📬 ${message}`, 'magenta');
}

// Connect using SDK
async function connect() {
  log('\n╔════════════════════════════════════════════════════════╗', 'cyan');
  log(`║  Consumer ${CONSUMER_ID.substring(0, 8)}... Starting          ║`, 'cyan');
  log('╚════════════════════════════════════════════════════════╝', 'cyan');
  
  logInfo('Connecting to Threadify...');
  connection = await Threadify.connect(API_KEY, SERVICE_NAME, {
    url: WS_URL,
    maxInFlight: MAX_IN_FLIGHT,
    debug: true, // Set to true for detailed SDK logs
  });
  
  logSuccess('Connected to Threadify');
  logInfo(`Consumer ID: ${CONSUMER_ID}`);
  logInfo(`MaxInFlight: ${MAX_IN_FLIGHT}`);
  
  return connection;
}

// Subscribe to notifications
async function subscribe() {
  logInfo('Subscribing to notifications...');
  
  // Subscribe to order_placed violations for product_delivery contract
  connection.onViolation('product_delivery@order_placed', async (notification) => {
    const receiveTime = Date.now();
    console.log(`Notification received: thread: ${notification.threadId}`);
    
    // Calculate latency if timestamp is available
    if (notification.timestamp) {
      const sentTime = new Date(notification.timestamp).getTime();
      const latency = receiveTime - sentTime;
      latencies.push(latency);
      logNotification(`[VIOLATION] ${notification.stepName} (thread: ${notification.threadId.substring(0, 8)}...) [Latency: ${latency}ms]`);
    } else {
      logNotification(`[VIOLATION] ${notification.stepName} (thread: ${notification.threadId.substring(0, 8)}...)`);
    }
    
    logInfo(`   Notification ID: ${notification.notificationId}`);
    logInfo(`   Status: ${notification.status}`);
    logInfo(`   Message: ${notification.message}`);
    
    receivedNotifications.push(notification);
    await notification.ack();
    logSuccess(`   ACKed notification ${notification.notificationId}`);
  });
  logSuccess('Subscribed to: product_delivery@order_placed [violations]');
  
  // Subscribe to order_placed completions for product_delivery contract
  connection.onCompleted('product_delivery@order_placed', async (notification) => {
    const receiveTime = Date.now();
    console.log(`Notification received: thread: ${notification.threadId}`);
    
    // Calculate latency if timestamp is available
    if (notification.timestamp) {
      const sentTime = new Date(notification.timestamp).getTime();
      const latency = receiveTime - sentTime;
      latencies.push(latency);
      logNotification(`[COMPLETED] ${notification.stepName} (thread: ${notification.threadId.substring(0, 8)}...) [Latency: ${latency}ms]`);
    } else {
      logNotification(`[COMPLETED] ${notification.stepName} (thread: ${notification.threadId.substring(0, 8)}...)`);
    }
    
    logInfo(`   Notification ID: ${notification.notificationId}`);
    logInfo(`   Status: ${notification.status}`);
    logInfo(`   Message: ${notification.message}`);
    
    receivedNotifications.push(notification);
    await notification.ack();
    logSuccess(`   ACKed notification ${notification.notificationId}`);
  });
  logSuccess('Subscribed to: product_delivery@order_placed [completions]');
  
  // Subscribe to order_placed failures for product_delivery contract
  connection.onFailed('product_delivery@order_placed', async (notification) => {
    console.log(`Notification received: thread: ${notification.threadId}`);
    logNotification(`[FAILED] ${notification.stepName} (thread: ${notification.threadId.substring(0, 8)}...)`);
    logInfo(`   Notification ID: ${notification.notificationId}`);
    logInfo(`   Status: ${notification.status}`);
    logInfo(`   Message: ${notification.message}`);
    
    receivedNotifications.push(notification);
    await notification.ack();
    logSuccess(`   ACKed notification ${notification.notificationId}`);
  });
  logSuccess('Subscribed to: product_delivery@order_placed [failures]');
  
  // Subscribe to payment_received (all event types)
  connection.onViolation('payment_received', async (notification) => {
    console.log(`Notification received: thread: ${notification.threadId}`);
    logNotification(`[VIOLATION] ${notification.stepName} (thread: ${notification.threadId.substring(0, 8)}...)`);
    receivedNotifications.push(notification);
    await notification.ack();
    logSuccess(`   ACKed notification ${notification.notificationId}`);
  });
  
  connection.onCompleted('payment_received', async (notification) => {
    console.log(`Notification received: thread: ${notification.threadId}`);
    logNotification(`[COMPLETED] ${notification.stepName} (thread: ${notification.threadId.substring(0, 8)}...)`);
    receivedNotifications.push(notification);
    await notification.ack();
    logSuccess(`   ACKed notification ${notification.notificationId}`);
  });
  
  connection.onFailed('payment_received', async (notification) => {
    console.log(`Notification received: thread: ${notification.threadId}`);
    logNotification(`[FAILED] ${notification.stepName} (thread: ${notification.threadId.substring(0, 8)}...)`);
    receivedNotifications.push(notification);
    await notification.ack();
    logSuccess(`   ACKed notification ${notification.notificationId}`);
  });
  logSuccess('Subscribed to: payment_received [all event types]');
}

// Print summary
function printSummary() {
  log('\n╔════════════════════════════════════════════════════════╗', 'cyan');
  log(`║  Consumer ${CONSUMER_ID.substring(0, 8)}... Summary          ║`, 'cyan');
  log('╚════════════════════════════════════════════════════════╝', 'cyan');
  
  log('\n📊 Statistics:', 'cyan');
  logInfo(`   • Consumer ID: ${CONSUMER_ID}`);
  logInfo(`   • Service: ${SERVICE_NAME}`);
  logInfo(`   • Notifications received: ${receivedNotifications.length}`);
  logInfo(`   • MaxInFlight: ${MAX_IN_FLIGHT}`);
  
  // Calculate latency statistics
  if (latencies.length > 0) {
    const avgLatency = (latencies.reduce((a, b) => a + b, 0) / latencies.length).toFixed(2);
    const minLatency = Math.min(...latencies);
    const maxLatency = Math.max(...latencies);
    const p95Latency = latencies.sort((a, b) => a - b)[Math.floor(latencies.length * 0.95)];
    
    log('\n⏱️  Latency Metrics:', 'cyan');
    logInfo(`   • Average: ${avgLatency}ms`);
    logInfo(`   • Min: ${minLatency}ms`);
    logInfo(`   • Max: ${maxLatency}ms`);
    logInfo(`   • P95: ${p95Latency}ms`);
  }
  
  log('\n✅ Consumer completed successfully!', 'green');
  
  if (receivedNotifications.length > 0) {
    log(`\n📬 Received notifications:`, 'cyan');
    receivedNotifications.forEach((notif, idx) => {
      log(`   ${idx + 1}. ${notif.stepName} - ${notif.status} (${notif.threadId.substring(0, 8)}...)`, 'blue');
    });
  }
}

// Main
async function main() {
  try {
    await connect();
    await subscribe();
    
    log('\n✅ Consumer ready and listening for notifications...', 'green');
    log('   Press Ctrl+C to stop\n', 'yellow');
    
    // Keep alive
    setInterval(() => {
      // Heartbeat
    }, 30000);
    
  } catch (error) {
    logError(`\n❌ Consumer failed: ${error.message}`);
    log(error.stack, 'red');
    
    if (connection) {
      await connection.close();
    }
    
    process.exit(1);
  }
}

// Handle process signals
process.on('SIGINT', async () => {
  log('\n\n⚠️  Consumer interrupted by user', 'yellow');
  if (connection) {
    await connection.close();
  }
  setTimeout(() => process.exit(0), 1000);
});

// Run
main();
