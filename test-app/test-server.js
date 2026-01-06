#!/usr/bin/env node

/**
 * Threadify SDK Test Server
 * 
 * Express server that:
 * - Serves the test UI (HTML)
 * - Provides API endpoints to run SDK tests
 * - Uses the REAL Threadify SDK
 * - Displays results in the browser
 */

import express from 'express';
import path from 'path';
import { fileURLToPath } from 'url';
import { Threadify } from '../threadify-sdk/src/index.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();
const PORT = 3000;

// Middleware
app.use(express.json());
app.use(express.static(__dirname)); // Serve static files

// Test state (in-memory for demo)
const testSessions = new Map();

/**
 * API: Connect to Threadify
 */
app.post('/api/connect', async (req, res) => {
  const { apiKey, serviceName, url } = req.body;
  
  try {
    const connection = await Threadify.connect(apiKey, serviceName, { url });
    
    const sessionId = `session-${Date.now()}`;
    const sessionState = {
      connection,
      notifications: [],
      threads: new Map()
    };
    
    // Set up global handlers
    connection.onViolation('order_placed', (notification) => {
      sessionState.notifications.push({
        type: 'violation',
        ...notification
      });
      notification.ack();
    });
    
    connection.onCompleted('order_placed', (notification) => {
      sessionState.notifications.push({
        type: 'completed',
        ...notification
      });
      notification.ack();
    });
    
    connection.onFailed('order_placed', (notification) => {
      sessionState.notifications.push({
        type: 'failed',
        ...notification
      });
      notification.ack();
    });
    
    testSessions.set(sessionId, sessionState);
    
    res.json({
      success: true,
      sessionId,
      message: 'Connected to Threadify Engine'
    });
  } catch (error) {
    res.status(500).json({
      success: false,
      error: error.message
    });
  }
});

/**
 * API: Start Thread
 */
app.post('/api/start-thread', async (req, res) => {
  const { sessionId, contractName, role } = req.body;
  
  const session = testSessions.get(sessionId);
  if (!session) {
    return res.status(404).json({ success: false, error: 'Session not found' });
  }
  
  try {
    const thread = await session.connection.start(contractName, role);
    const threadId = thread.getThreadId();
    
    session.threads.set(threadId, thread);
    
    res.json({
      success: true,
      threadId,
      message: `Thread started: ${threadId}`
    });
  } catch (error) {
    res.status(500).json({
      success: false,
      error: error.message
    });
  }
});

/**
 * API: Record Step
 */
app.post('/api/record-step', async (req, res) => {
  const { sessionId, threadId, stepName, context, status } = req.body;
  
  const session = testSessions.get(sessionId);
  if (!session) {
    return res.status(404).json({ success: false, error: 'Session not found' });
  }
  
  const thread = session.threads.get(threadId);
  if (!thread) {
    return res.status(404).json({ success: false, error: 'Thread not found' });
  }
  
  try {
    await thread.step(stepName)
      .addContext(context)
      .stop(status || 'success');
    
    res.json({
      success: true,
      message: `Step ${stepName} recorded`
    });
  } catch (error) {
    res.status(500).json({
      success: false,
      error: error.message
    });
  }
});

/**
 * API: Wait For Notification
 */
app.post('/api/wait-for', async (req, res) => {
  const { sessionId, threadId, stepName, timeout } = req.body;
  
  const session = testSessions.get(sessionId);
  if (!session) {
    return res.status(404).json({ success: false, error: 'Session not found' });
  }
  
  const thread = session.threads.get(threadId);
  if (!thread) {
    return res.status(404).json({ success: false, error: 'Thread not found' });
  }
  
  try {
    const notification = await thread.waitFor(stepName, {
      timeout: timeout || 10000,
      statuses: ['success', 'failed']
    });
    
    res.json({
      success: true,
      notification: {
        notificationId: notification.notificationId,
        threadId: notification.threadId,
        stepName: notification.stepName,
        status: notification.status,
        stepStatus: notification.stepStatus,
        severity: notification.severity,
        message: notification.message,
        violationType: notification.violationType,
        details: notification.details,
        isPassed: notification.isPassed(),
        isViolated: notification.isViolated(),
        isCritical: notification.isCritical()
      }
    });
  } catch (error) {
    res.status(500).json({
      success: false,
      error: error.message
    });
  }
});

/**
 * API: Get Notifications
 */
app.get('/api/notifications/:sessionId', (req, res) => {
  const { sessionId } = req.params;
  
  const session = testSessions.get(sessionId);
  if (!session) {
    return res.status(404).json({ success: false, error: 'Session not found' });
  }
  
  res.json({
    success: true,
    notifications: session.notifications
  });
});

/**
 * API: Disconnect
 */
app.post('/api/disconnect', async (req, res) => {
  const { sessionId } = req.body;
  
  const session = testSessions.get(sessionId);
  if (!session) {
    return res.status(404).json({ success: false, error: 'Session not found' });
  }
  
  try {
    await session.connection.close();
    testSessions.delete(sessionId);
    
    res.json({
      success: true,
      message: 'Disconnected from Threadify Engine'
    });
  } catch (error) {
    res.status(500).json({
      success: false,
      error: error.message
    });
  }
});

/**
 * Serve the test UI
 */
app.get('/', (req, res) => {
  res.sendFile(path.join(__dirname, 'test-ui.html'));
});

// Start server
app.listen(PORT, () => {
  console.log(`
╔════════════════════════════════════════════════════════╗
║  Threadify SDK Test Server                            ║
╚════════════════════════════════════════════════════════╝

🚀 Server running on: http://localhost:${PORT}
📝 Test UI: http://localhost:${PORT}
🔌 WebSocket Backend: ws://localhost:8081/threads

Available API Endpoints:
  POST /api/connect          - Connect to Threadify
  POST /api/start-thread     - Start a new thread
  POST /api/record-step      - Record a step
  POST /api/wait-for         - Wait for notification
  GET  /api/notifications    - Get all notifications
  POST /api/disconnect       - Disconnect

Ready to test! 🎉
`);
});
