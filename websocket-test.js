#!/usr/bin/env node

/**
 * WebSocket Test Script for ThreadifyEngine
 * 
 * Usage:
 *   node websocket-test.js
 * 
 * Requirements:
 *   npm install ws
 */

const WebSocket = require('ws');

const WS_URL = 'ws://localhost:8080/threads';
const API_KEY = 'test-api-key-123';
const OWNER_ID = 'test-user-' + Date.now();

let ws;
let threadId;

// Colors for console output
const colors = {
    reset: '\x1b[0m',
    green: '\x1b[32m',
    blue: '\x1b[34m',
    yellow: '\x1b[33m',
    red: '\x1b[31m',
    cyan: '\x1b[36m'
};

function log(message, color = 'reset') {
    const timestamp = new Date().toISOString();
    console.log(`${colors[color]}[${timestamp}] ${message}${colors.reset}`);
}

function send(message) {
    const json = JSON.stringify(message, null, 2);
    log(`\n📤 SENDING:\n${json}`, 'blue');
    ws.send(JSON.stringify(message));
}

function connect() {
    return new Promise((resolve, reject) => {
        log(`Connecting to ${WS_URL}...`, 'cyan');
        
        ws = new WebSocket(WS_URL);
        
        ws.on('open', () => {
            log('✅ WebSocket connected', 'green');
            
            // Send connect message
            send({
                action: 'connect',
                apiKey: API_KEY,
                ownerId: OWNER_ID,
                subscribedEvents: ['onSuccess', 'onError', 'onViolation', 'onStepProgress']
            });
            
            resolve();
        });
        
        ws.on('message', (data) => {
            const message = data.toString();
            log(`\n📥 RECEIVED:\n${message}`, 'green');
            
            try {
                const parsed = JSON.parse(message);
                
                // Save threadId if present
                if (parsed.threadId) {
                    threadId = parsed.threadId;
                    log(`💾 Saved threadId: ${threadId}`, 'yellow');
                }
                
                // Check for errors
                if (parsed.status === 'error') {
                    log(`❌ Error: ${parsed.message}`, 'red');
                }
            } catch (e) {
                log(`⚠️  Could not parse message: ${e.message}`, 'yellow');
            }
        });
        
        ws.on('error', (error) => {
            log(`❌ WebSocket error: ${error.message}`, 'red');
            reject(error);
        });
        
        ws.on('close', () => {
            log('🔌 WebSocket disconnected', 'yellow');
        });
    });
}

function wait(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

async function runTests() {
    try {
        // Test 1: Connect
        log('\n=== TEST 1: Connect ===', 'cyan');
        await connect();
        await wait(1000);
        
        // Test 2: Start Thread without contract
        log('\n=== TEST 2: Start Thread (no contract) ===', 'cyan');
        send({
            action: 'startThread'
        });
        await wait(1000);
        
        // Test 3: Start Thread with contract
        log('\n=== TEST 3: Start Thread (with contract) ===', 'cyan');
        send({
            action: 'startThread',
            contractId: 'contract-789',
            metadata: {
                description: 'Test thread with contract',
                priority: 'high'
            }
        });
        await wait(1000);
        
        // Test 4: Record Thread Event - Started
        if (threadId) {
            log('\n=== TEST 4: Record Thread Event (started) ===', 'cyan');
            send({
                action: 'recordThreadEvent',
                threadId: threadId,
                startedAt: new Date().toISOString(),
                status: 'started',
                context: {
                    step: 'payment_initiated',
                    amount: '100.00'
                }
            });
            await wait(1000);
            
            // Test 5: Record Thread Event - Completed
            log('\n=== TEST 5: Record Thread Event (completed) ===', 'cyan');
            send({
                action: 'recordThreadEvent',
                threadId: threadId,
                finishedAt: new Date().toISOString(),
                status: 'completed',
                context: {
                    step: 'payment_completed'
                },
                metadata: {
                    transactionId: 'txn-' + Date.now()
                }
            });
            await wait(1000);
        } else {
            log('⚠️  Skipping event tests - no threadId available', 'yellow');
        }
        
        // Test 6: Error Case - Missing threadId
        log('\n=== TEST 6: Error Case (missing threadId) ===', 'cyan');
        send({
            action: 'recordThreadEvent',
            status: 'started'
        });
        await wait(1000);
        
        // Test 7: Error Case - Unknown action
        log('\n=== TEST 7: Error Case (unknown action) ===', 'cyan');
        send({
            action: 'unknownAction',
            data: 'test'
        });
        await wait(1000);
        
        // Test 8: Close Connection
        log('\n=== TEST 8: Close Connection ===', 'cyan');
        send({
            action: 'closeConnection'
        });
        await wait(1000);
        
        log('\n✅ All tests completed!', 'green');
        ws.close();
        
    } catch (error) {
        log(`\n❌ Test failed: ${error.message}`, 'red');
        process.exit(1);
    }
}

// Run tests
log('🚀 Starting WebSocket tests...', 'cyan');
runTests();
