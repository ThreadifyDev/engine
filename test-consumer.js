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
 *   WS_URL - WebSocket URL (default: ws://localhost:8081/threads)
 *   API_KEY - API key for authentication (default: test-api-key)
 *   MAX_IN_FLIGHT - Flow control limit (default: 20)
 *   CONSUMER_ID - Unique consumer ID (default: random UUID)
 */

import WebSocket from 'ws';
import { v4 as uuidv4 } from 'uuid';

// Configuration
const WS_URL = process.env.WS_URL || 'ws://localhost:8081/threads';
const API_KEY = process.env.API_KEY || 'test-api-key';
const SERVICE_NAME = process.env.SERVICE_NAME || 'test-service';
const MAX_IN_FLIGHT = parseInt(process.env.MAX_IN_FLIGHT) || 20;
const CONSUMER_ID = process.env.CONSUMER_ID || uuidv4();

// Test state
let ws;
let ownerID;
let sessionID;
let receivedNotifications = [];

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
  log(`║  Consumer ${CONSUMER_ID.substring(0, 8)}... Starting          ║`, 'cyan');
  log('╚════════════════════════════════════════════════════════╝', 'cyan');
  
  sessionID = `session-${uuidv4()}`;
  
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
    
    ws.on('message', (data) => {
      try {
        const message = JSON.parse(data);
        
        // Log all notifications
        if (message.action === 'notification') {
          const notif = message.notification;
          logNotification(`Notification received: ${notif.stepName} (thread: ${notif.threadId})`);
          logInfo(`   Notification ID: ${notif.notificationId}`);
          logInfo(`   Thread ID: ${notif.threadId}`);
          logInfo(`   Step ID: ${notif.stepId}`);
          logInfo(`   Contract: ${notif.contractName || 'none'}`);
          logInfo(`   Status: ${notif.status}`);
          logInfo(`   Message: ${notif.message}`);
          
          receivedNotifications.push(message);
          
          // Auto-ACK the notification
          if (message.ackToken) {
            ws.send(JSON.stringify({
              action: 'ack_notification',
              notificationId: notif.notificationId,
              ackToken: message.ackToken,
            }));
            logSuccess(`   ACKed notification ${notif.notificationId}`);
          }
        }
      } catch (err) {
        // Ignore parse errors
      }
    });
    
    ws.on('close', () => {
      log('\n🛑 WebSocket connection closed', 'yellow');
      printSummary();
    });
  });
}

// Authenticate
async function authenticate() {
  logInfo('Authenticating...');
  
  const response = await sendMessage('connect', {
    apiKey: API_KEY,
    serviceName: SERVICE_NAME,
    maxInFlight: MAX_IN_FLIGHT,
  });
  
  if (response.status !== 'success') {
    throw new Error(`Authentication failed: ${response.message}`);
  }
  
  ownerID = response.ownerId;
  logSuccess(`Authenticated as ownerID: ${ownerID}`);
  logInfo(`Consumer ID: ${CONSUMER_ID}`);
  logInfo(`MaxInFlight: ${MAX_IN_FLIGHT}`);
  
  return response;
}

// Subscribe to notifications
async function subscribe() {
  logInfo('Subscribing to notifications...');
  
  // Subscribe to order_placed (no contract)
  await sendMessage('subscribe', {
    stepName: 'order_placed',
    eventTypes: [],
  });
  logSuccess('Subscribed to: order_placed (all contracts)');
  
  // Subscribe to payment_received (no contract)
  await sendMessage('subscribe', {
    stepName: 'payment_received',
    eventTypes: [],
  });
  logSuccess('Subscribed to: payment_received (all contracts)');
  
  // Subscribe to shipment_created (no contract)
  await sendMessage('subscribe', {
    stepName: 'shipment_created',
    eventTypes: [],
  });
  logSuccess('Subscribed to: shipment_created (all contracts)');
}

// Print summary
function printSummary() {
  log('\n╔════════════════════════════════════════════════════════╗', 'cyan');
  log('║  Consumer Summary                                     ║', 'cyan');
  log('╚════════════════════════════════════════════════════════╝', 'cyan');
  
  log(`\n📊 Statistics:`, 'cyan');
  log(`   • Consumer ID: ${CONSUMER_ID}`, 'blue');
  log(`   • Owner ID: ${ownerID}`, 'blue');
  log(`   • Session ID: ${sessionID}`, 'blue');
  log(`   • Notifications received: ${receivedNotifications.length}`, 'blue');
  log(`   • MaxInFlight: ${MAX_IN_FLIGHT}`, 'blue');
  
  if (receivedNotifications.length > 0) {
    log(`\n📬 Received notifications:`, 'cyan');
    receivedNotifications.forEach((msg, idx) => {
      const notif = msg.notification;
      log(`   ${idx + 1}. ${notif.stepName} - ${notif.status} (${notif.threadId.substring(0, 8)}...)`, 'blue');
    });
  }
}

// Main
async function main() {
  try {
    await connect();
    await authenticate();
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
    
    if (ws) {
      ws.close();
    }
    
    process.exit(1);
  }
}

// Handle process signals
process.on('SIGINT', () => {
  log('\n\n⚠️  Consumer interrupted by user', 'yellow');
  if (ws) {
    ws.close();
  }
  setTimeout(() => process.exit(0), 1000);
});

// Run
main();
