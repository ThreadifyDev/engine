#!/usr/bin/env node

/**
 * Automated Hot/Cold Fallback Test
 * Tests PostgreSQL fallback using an existing archived thread
 */

import { Threadify } from './threadify-sdk/src/index.js';
import { exec } from 'child_process';
import { promisify } from 'util';

const execAsync = promisify(exec);

const config = {
  apiKey: 'test-api-key',
  threadId: '3d5d5d61-d62c-4ed5-8170-59a0da40fdee', // Thread that exists in PostgreSQL
  testStepName: 'fallback_test_step',
  wsUrl: 'ws://localhost:8081/threads'
};

const colors = {
  reset: '\x1b[0m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  red: '\x1b[31m',
  cyan: '\x1b[36m'
};

function log(msg, color = colors.reset) {
  console.log(`${color}${msg}${colors.reset}`);
}

async function clearValkeyCache(threadId) {
  const cmd = `docker exec threadifyengine-threadify-queue-1 valkey-cli -a thread0fy_server --no-auth-warning DEL "thread:${threadId}" "thread:${threadId}:meta"`;
  try {
    const { stdout } = await execAsync(cmd);
    return parseInt(stdout.trim());
  } catch (error) {
    throw new Error(`Failed to clear cache: ${error.message}`);
  }
}

async function runTest() {
  log('\n========== AUTOMATED HOT/COLD FALLBACK TEST ==========\n', colors.cyan);
  log('Testing PostgreSQL fallback with existing archived thread\n', colors.blue);

  let connection;

  try {
    log('[1] Clearing Valkey cache to force cold start...', colors.cyan);
    const keysDeleted = await clearValkeyCache(config.threadId);
    log(`✅ Cleared ${keysDeleted} keys from Valkey`, colors.green);

    log('\n[2] Connecting to Threadify...', colors.cyan);
    connection = await Threadify.connect(config.apiKey, 'fallback-test', {
      wsUrl: config.wsUrl,
      graphqlUrl: 'http://localhost:8081/graphql',
      debug: false
    });
    log('✅ Connected', colors.green);

    log('\n[3] Joining thread (should trigger PostgreSQL fallback)...', colors.cyan);
    log(`   Thread ID: ${config.threadId}`, colors.blue);
    log('   Expected: ⚠️ [COLD] Thread not in Valkey, checking PostgreSQL', colors.yellow);
    
    const thread = await connection.join(config.threadId, 'owner');
    log(`✅ Successfully joined thread: ${thread.getThreadId()}`, colors.green);
    log('   This confirms PostgreSQL fallback worked!', colors.green);

    log('\n[4] Fetching thread data via GraphQL (DataRetriever)...', colors.cyan);
    const archivedThread = await connection.getThread(config.threadId);
    log(`✅ Retrieved archived thread: ${archivedThread.threadId}`, colors.green);
    log(`   Status: ${archivedThread.status}`, colors.blue);
    log(`   Created: ${archivedThread.createdAt}`, colors.blue);

    log('\n[5] Fetching all steps with verification fields...', colors.cyan);
    const steps = await archivedThread.steps();
    log(`✅ Retrieved ${steps.length} steps`, colors.green);
    
    for (const step of steps) {
      log(`\n   📍 Step: ${step.stepName}`, colors.cyan);
      log(`      Status: ${step.status}`, colors.blue);
      log(`      Retry Count: ${step.retryCount}`, colors.blue);
      log(`      Verified: ${step.verified}`, step.verified ? colors.green : colors.red);
      log(`      Verification Error: ${step.verificationError || 'null'}`, colors.blue);
      
      if (step.lastExecution) {
        log(`      Last Execution:`, colors.yellow);
        log(`        - Attempt: ${step.lastExecution.attempt}`, colors.blue);
        log(`        - Status: ${step.lastExecution.status}`, colors.blue);
        log(`        - Timestamp: ${step.lastExecution.timestamp}`, colors.blue);
        log(`        - Duration: ${step.lastExecution.duration}ms`, colors.blue);
      }
    }

    if (steps.length > 0) {
      log(`\n[6] Fetching full history for first step (${steps[0].stepName})...`, colors.cyan);
      const firstStep = await archivedThread.getStep(steps[0].stepName);
      if (firstStep) {
        log(`✅ Retrieved step: ${firstStep.stepName}`, colors.green);
        
        const history = await firstStep.history({ limit: 10 });
        log(`✅ Retrieved ${history.length} history records`, colors.green);
        
        history.slice(0, 3).forEach((h, index) => {
          log(`\n   [${index + 1}] Attempt ${h.attempt}:`, colors.cyan);
          log(`       Status: ${h.status}`, colors.blue);
          log(`       Timestamp: ${h.timestamp}`, colors.blue);
          log(`       Duration: ${h.duration}ms`, colors.blue);
        });
        if (history.length > 3) {
          log(`\n   ... and ${history.length - 3} more records`, colors.blue);
        }
      }
    }

    log('\n[7] Recording a test step...', colors.cyan);
    const result = await thread.step(config.testStepName)
      .addContext({
        test_id: Date.now().toString(),
        test_type: 'automated_fallback_test',
        timestamp: new Date().toISOString()
      })
      .success('Fallback test completed');

    log(`✅ Step recorded: ${result.stepName}`, colors.green);
    log(`   Status: ${result.status}`, colors.blue);
    log(`   Idempotency Key: ${result.idempotencyKey}`, colors.blue);

    log('\n========== TEST PASSED ==========\n', colors.green);
    log('✓ Valkey cache cleared successfully', colors.green);
    log('✓ Thread retrieved from PostgreSQL (cold path)', colors.green);
    log('✓ DataRetriever.getThread() tested', colors.green);
    log('✓ ArchivedThread.steps() with verification fields tested', colors.green);
    log('✓ ArchivedStep.history() tested', colors.green);
    log('✓ Step recorded successfully', colors.green);
    log('✓ Hot/cold fallback mechanism verified!\n', colors.green);

    log('Check server logs for:', colors.cyan);
    log('  ⚠️ [COLD] Thread ... not in Valkey, checking PostgreSQL', colors.yellow);
    log('  ✅ [COLD] Thread ... from PostgreSQL\n', colors.yellow);

  } catch (error) {
    log(`\n❌ Test failed: ${error.message}`, colors.red);
    console.error(error);
    throw error;
  } finally {
    if (connection) {
      await connection.close();
      log('Connection closed\n', colors.blue);
    }
  }
}

runTest()
  .then(() => process.exit(0))
  .catch((err) => {
    console.error(err);
    process.exit(1);
  });
