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
 *   WS_URL - WebSocket URL (default: ws://localhost:8081/threads)
 *   API_KEY - API key for authentication (default: test-api-key)
 *   NOTIFICATION_COUNT - Number of notifications to publish (default: 1)
 *   STEP_NAME - Step name to trigger (default: order_placed)
 *   DELAY_MS - Delay between notifications in ms (default: 1000)
 */

import WebSocket from 'ws';
import { v4 as uuidv4 } from 'uuid';

// Configuration
const WS_URL = process.env.WS_URL || 'ws://localhost:8081/threads';
const API_KEY = process.env.API_KEY || 'test-api-key';
const SERVICE_NAME = 'test-publisher';
const NOTIFICATION_COUNT = parseInt(process.env.NOTIFICATION_COUNT) || parseInt(process.argv.find(arg => arg.startsWith('--count='))?.split('=')[1]) || 1;
const STEP_NAME = process.env.STEP_NAME || process.argv.find(arg => arg.startsWith('--step='))?.split('=')[1] || 'order_placed';
const DELAY_MS = parseInt(process.env.DELAY_MS) || 1000;

// Test state
let ws;
let ownerID;
let publishedCount = 0;

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

// Send message and wait for response
function sendMessage(action, data = {}) {
  return new Promise((resolve, reject) => {
    const message = { action, ...data };
    
    const timeout = setTimeout(() => {
      reject(new Error(`Timeout waiting for response to ${action}`));
    }, 10000);

    const handler = (event) => {
      try {
        const response = JSON.parse(event.data);
        
        // Check if this is the response we're waiting for
        if (response.action === action) {
          clearTimeout(timeout);
          ws.removeEventListener('message', handler);
          resolve(response);
        }
      } catch (err) {
        // Ignore parse errors
      }
    };

    ws.addEventListener('message', handler);
    ws.send(JSON.stringify(message));
  });
}

// Connect to WebSocket
async function connect() {
  log('\n╔════════════════════════════════════════════════════════╗', 'cyan');
  log('║  Publisher Starting                                   ║', 'cyan');
  log('╚════════════════════════════════════════════════════════╝', 'cyan');
  
  return new Promise((resolve, reject) => {
    ws = new WebSocket(WS_URL);
    
    ws.on('open', () => {
      logSuccess('WebSocket connected');
      resolve();
    });
    
    ws.on('error', (error) => {
      logError(`WebSocket error: ${error.message}`);
      reject(error);
    });
    
    ws.on('close', () => {
      log('\n🛑 WebSocket connection closed', 'yellow');
    });
  });
}

// Authenticate
async function authenticate() {
  logInfo('Authenticating...');
  
  const response = await sendMessage('connect', {
    apiKey: API_KEY,
    serviceName: SERVICE_NAME,
    maxInFlight: 10,
  });
  
  if (response.status !== 'success') {
    throw new Error(`Authentication failed: ${response.message}`);
  }
  
  ownerID = response.ownerId;
  logSuccess(`Authenticated as ownerID: ${ownerID}`);
  
  return response;
}

// Start a thread
async function startThread() {
  const response = await sendMessage('startThread', {
    role: 'merchant',
  });
  
  if (response.status !== 'success') {
    throw new Error(`Failed to start thread: ${response.message}`);
  }
  
  const threadID = response.threadId || response.threadID || response.thread_id || response.id;
  return threadID;
}

// Record a step (triggers notification)
async function recordStep(threadID, stepName) {
  const now = new Date().toISOString();
  
  const response = await sendMessage('recordThreadEvent', {
    threadID,
    stepName,
    context: {
      orderId: `ORDER-${uuidv4().substring(0, 8)}`,
      amount: Math.floor(Math.random() * 1000) + 10,
      timestamp: now,
    },
    status: 'success',
    startedAt: now,
    finishedAt: now,
  });
  
  if (response.status !== 'success') {
    throw new Error(`Failed to record step: ${response.message}`);
  }
  
  return response;
}

// Publish a notification
async function publishNotification(index) {
  try {
    logInfo(`[${index + 1}/${NOTIFICATION_COUNT}] Starting thread...`);
    const threadID = await startThread();
    
    logInfo(`[${index + 1}/${NOTIFICATION_COUNT}] Recording step: ${STEP_NAME}`);
    await recordStep(threadID, STEP_NAME);
    
    publishedCount++;
    logPublish(`Published notification ${index + 1}: ${STEP_NAME} (thread: ${threadID.substring(0, 8)}...)`);
    
  } catch (error) {
    logError(`Failed to publish notification ${index + 1}: ${error.message}`);
  }
}

// Publish multiple notifications
async function publishNotifications() {
  log(`\n📤 Publishing ${NOTIFICATION_COUNT} notification(s) for step: ${STEP_NAME}`, 'cyan');
  log(`   Delay between notifications: ${DELAY_MS}ms\n`, 'blue');
  
  for (let i = 0; i < NOTIFICATION_COUNT; i++) {
    await publishNotification(i);
    
    // Delay between notifications (except for the last one)
    if (i < NOTIFICATION_COUNT - 1) {
      await new Promise(resolve => setTimeout(resolve, DELAY_MS));
    }
  }
}

// Print summary
function printSummary() {
  log('\n╔════════════════════════════════════════════════════════╗', 'cyan');
  log('║  Publisher Summary                                    ║', 'cyan');
  log('╚════════════════════════════════════════════════════════╝', 'cyan');
  
  log(`\n📊 Statistics:`, 'cyan');
  log(`   • Owner ID: ${ownerID}`, 'blue');
  log(`   • Step name: ${STEP_NAME}`, 'blue');
  log(`   • Notifications published: ${publishedCount}/${NOTIFICATION_COUNT}`, 'blue');
  log(`   • Success rate: ${((publishedCount / NOTIFICATION_COUNT) * 100).toFixed(1)}%`, 'blue');
}

// Main
async function main() {
  try {
    await connect();
    await authenticate();
    await publishNotifications();
    
    printSummary();
    
    log('\n✅ Publisher completed successfully!', 'green');
    
    // Close connection
    ws.close();
    
    setTimeout(() => process.exit(0), 1000);
    
  } catch (error) {
    logError(`\n❌ Publisher failed: ${error.message}`);
    log(error.stack, 'red');
    
    if (ws) {
      ws.close();
    }
    
    process.exit(1);
  }
}

// Handle process signals
process.on('SIGINT', () => {
  log('\n\n⚠️  Publisher interrupted by user', 'yellow');
  printSummary();
  if (ws) {
    ws.close();
  }
  setTimeout(() => process.exit(0), 1000);
});

// Run
main();
