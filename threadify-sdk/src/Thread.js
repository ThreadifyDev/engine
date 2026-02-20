import { ThreadStep } from './ThreadStep.js';
import { Notification } from './Notification.js';
import { DataRetriever, ArchivedThread, ArchivedStep } from './DataRetriever.js';

/**
 * Connection - Represents a WebSocket connection to Threadify Engine
 */
export class Connection {
  constructor(ws, apiKey, serviceName = null, graphqlUrl = null, debug = false, maxInFlight = 10) {
    this.ws = ws;
    this.apiKey = apiKey;
    this.serviceName = serviceName;
    this.graphqlUrl = graphqlUrl;
    this.debug = debug;
    this.maxInFlight = maxInFlight; // Maximum unACKed notifications
    this.isConnected = false;
    this.activeThreads = new Map(); // Map of threadId -> thread info
    this.threads = new Map(); // Map of threadId -> ThreadInstance (for notification routing)
    
    // Global notification handlers (step-specific) - v2.0.0 API
    // Maps event pattern to stepName to handlers
    // e.g., 'step.success' -> 'order_placed' -> [handler1, handler2]
    this.notificationHandlers = new Map();
    
    this.processedNotifications = new Set(); // Track processed notification IDs
    this.maxProcessedSize = 10000; // Prevent memory leak
    this._dataRetriever = null; // Lazy-initialized DataRetriever
    this._activeSubscriptions = new Map(); // Track active subscriptions for merging
    
    this._setupNotificationListener();
  }

  /**
   * Debug logging utility
   * @private
   * @param {...any} args - Arguments to log
   */
  _debugLog(...args) {
    if (this.debug) {
      console.log('[DEBUG]', ...args);
    }
  }

  /**
   * Get lazy initialized DataRetriever instance
   * @private
   * @returns {DataRetriever} - DataRetriever instance
   */
  _getDataRetriever() {
    if (!this._dataRetriever) {
      if (!this.graphqlUrl) {
        throw new Error('GraphQL URL not configured. Pass graphqlUrl to Threadify.connect() or use wsUrl for auto-derivation.');
      }
      
      this._dataRetriever = new DataRetriever(this.graphqlUrl, this.apiKey);
    }
    return this._dataRetriever;
  }

  /**
   * Get archived thread by ID
   * @param {string} threadId - Thread ID
   * @returns {Promise<ArchivedThread>} - Archived thread
   */
  async getThread(threadId) {
    return this._getDataRetriever().getThread(threadId);
  }

  /**
   * Get archived thread by reference
   * @param {Object} refQuery - Reference query {refKey, refValue}
   * @returns {Promise<ArchivedThread[]>} - Array of archived threads
   */
  async getThreadByRef(refQuery) {
    return this._getDataRetriever().getThreadByRef(refQuery);
  }

  /**
   * Get multiple threads by reference
   * @param {Object} refQuery - Reference query {refKey, refValue}
   * @returns {Promise<ArchivedThread[]>} - Array of archived threads
   */
  async getThreadsByRef(refQuery) {
    return this._getDataRetriever().getThreadsByRef(refQuery);
  }


  /**
   * Get validation results for a thread
   * @param {string} threadId - Thread ID
   * @param {string} stepName - Optional step name filter
   * @returns {Promise<Array>} - Validation results
   */
  async getValidationResults(threadId, stepName = null) {
    return this._getDataRetriever().getValidationResults(threadId, stepName);
  }

  /**
   * Get thread chain starting from any thread
   * @param {string} startThreadId - Starting thread ID (can be any thread in the chain)
   * @param {number} maxDepth - Maximum depth to traverse downward (default: 3)
   * @returns {Promise<Array<ArchivedThread>>} - Thread chain from starting thread to descendants
   */
  async getThreadChain(startThreadId, maxDepth = 3) {
    return this._getDataRetriever().getThreadChain(startThreadId, maxDepth);
  }


  /**
   * Start a new thread (returns a ThreadInstance)
   * @param {...any} args - Variable arguments:
   *   - start() - Non-contract workflow
   *   - start(serviceName) - Non-contract with specific service
   *   - start(contractName, serviceName) - Contract workflow (contractName can be "name:version")
   * @returns {Promise<ThreadInstance>} - Returns a ThreadInstance for fluent API
   */
  async start(...args) {
    if (!this.isConnected) {
      throw new Error('Not connected. Call Threadify.connect() first.');
    }

    let contractName = null;
    let serviceName = null;

    if (args.length === 0) {
      // Non-contract workflow
      serviceName = this.serviceName;
      contractName = null;
    } else if (args.length === 1) {
      // Non-contract with specific service
      serviceName = args[0];
      contractName = null;
    } else if (args.length === 2) {
      // Contract workflow (contractName, serviceName)
      [contractName, serviceName] = args;
    } else {
      throw new Error('Invalid arguments. Use start(), start(serviceName), or start(contractName, serviceName)');
    }

    // Validate parameters
    if (contractName && typeof contractName !== 'string') {
      throw new Error('Contract name must be a string');
    }
    if (serviceName && typeof serviceName !== 'string') {
      throw new Error('Service name must be a string');
    }

    return new Promise((resolve, reject) => {
      const message = {
        action: 'startThread',
        contractName,
        refs: {
          serviceName: serviceName || this.serviceName
        }
      };

      // Only include role for contract-based workflows
      if (contractName) {
        // Extract role from service name (e.g., "merchant-service" -> "merchant")
        const effectiveServiceName = serviceName || this.serviceName;
        message.role = effectiveServiceName ? effectiveServiceName.replace(/-service$/, '') : 'participant';
      }

      // Set up one-time listener for response
      const responseHandler = (data) => {
        this._debugLog('[start] Response handler called with:', data.action, data.status);
        if (data.action === 'startThread') {
          if (data.status === 'success') {
            const threadInstance = new ThreadInstance(this, data.threadId, contractName, null, {});
            // Register thread for notification routing
            this.threads.set(data.threadId, threadInstance);
            this._debugLog(`Thread started: ${data.threadId}`);
            resolve(threadInstance);
          } else {
            this._debugLog('[start] Failed:', data.message);
            reject(new Error(data.message || 'Failed to start thread'));
          }
        } else {
          this._debugLog('[start] Ignoring message with action:', data.action);
        }
      };

      this._debugLog('[start] Setting up response handler and sending message:', message);
      this._onceResponse(responseHandler);
      this._send(message);
    });
  }

  /**
   * Internal method to record events (called by ThreadStep)
   * @private
   */
  _recordEvent(eventData) {
    if (!this.threadId) {
      console.warn('Thread not started. Call thread.start() first.');
      return;
    }

    const message = {
      action: 'recordThreadEvent',
      ...eventData
    };

    this._send(message);
  }

  /**
   * Get a step by name
   * @param {string} stepName - Name of the step
   * @returns {ThreadStep|undefined} - The step if found
   */
  getStep(stepName) {
    return this.steps.get(stepName);
  }

  /**
   * Get all steps
   * @returns {Array<ThreadStep>} - Array of all steps
   */
  getAllSteps() {
    return Array.from(this.steps.values());
  }

  /**
   * Close the thread connection
   */
  async close() {
    return new Promise((resolve) => {
      const message = { action: 'closeConnection' };
      
      const responseHandler = (data) => {
        if (data.action === 'closeConnection') {
          this.isConnected = false;
          this.ws.close(); // Close WebSocket immediately after receiving response
          resolve();
        }
      };

      this._onceResponse(responseHandler);
      this._send(message);
    });
  }

  /**
   * Send subscription to server (internal)
   * @private
   */
  _sendSubscription(stepName, eventTypes) {
    if (!this.isConnected) {
      console.warn('[Thread] Cannot subscribe - not connected');
      return;
    }

    const existing = this._activeSubscriptions.get(stepName) || [];
    const merged = [...new Set([...existing, ...eventTypes])];
    
    // Only send if changed
    if (JSON.stringify(existing.sort()) !== JSON.stringify(merged.sort())) {
      this._send({
        action: 'subscribe',
        stepName: stepName,
        eventTypes: merged
      });
      
      this._activeSubscriptions.set(stepName, merged);
    }
  }

  /**
   * Send unsubscription to server (internal)
   * @private
   */
  _sendUnsubscription(stepName) {
    if (!this.isConnected) return;

    this._send({
      action: 'unsubscribe',
      stepName: stepName
    });

    this._activeSubscriptions.delete(stepName);
  }

  /**
   * Resubscribe to all active subscriptions (for reconnection)
   * @private
   */
  _resubscribeAll() {
    for (const [stepName, eventTypes] of this._activeSubscriptions.entries()) {
      this._send({
        action: 'subscribe',
        stepName: stepName,
        eventTypes: eventTypes
      });
    }
  }

  /**
   * Internal method to send messages
   * @private
   */
  _send(message) {
    if (this.ws.readyState === 1) { // WebSocket.OPEN
      this.ws.send(JSON.stringify(message));
    } else {
      throw new Error('WebSocket is not connected');
    }
  }

  /**
   * Internal method to set up one-time response handler
   * @private
   */
  _onceResponse(handler) {
    const wrapper = (data) => {
      try {
        const message = JSON.parse(data.toString());
        this._debugLog('[_onceResponse] Received message:', message.action, message.status);
        handler(message);
        this.ws.off('message', wrapper);
      } catch (e) {
        console.error('Failed to parse WebSocket message:', e);
      }
    };
    this.ws.on('message', wrapper);
    this._debugLog('[_onceResponse] Handler registered, waiting for response...');
  }

  /**
   * Get thread ID
   * @returns {string|null} - Current thread ID
   */
  getThreadId() {
    return this.threadId;
  }

  /**
   * Get contract ID
   * @returns {string|null} - Current contract ID
   */
  getContractId() {
    return this.contractId;
  }

  /**
   * Register a notification handler for specific events (v2.0.0 API)
   * Supports both 2-param (thread-level) and 3-param (step-level) signatures
   * @param {string} event - Event pattern: 'thread.cancelled', 'thread.completed', 'step.success', 'rule.violated', etc.
   * @param {string|Function} stepNameOrHandler - Step name (3-param) or handler function (2-param)
   * @param {Function} [handler] - Handler function (only for 3-param signature)
   * @returns {Connection} - Returns this for chaining
   * 
   * @example
   * // Thread-level events (2 params - no stepName)
   * thread.on('thread.cancelled', (notif) => { ... });
   * thread.on('thread.completed', (notif) => { ... });
   * thread.on('thread.*', (notif) => { ... }); // All thread events
   * 
   * // Step-level events (3 params - with stepName)
   * thread.on('step.success', 'order_placed', (notif) => { ... });
   * thread.on('step.failed', 'order_placed', (notif) => { ... });
   * thread.on('rule.violated', 'order_placed', (notif) => { ... });
   * thread.on('rule.passed', 'order_placed', (notif) => { ... });
   * 
   * // Wildcards
   * thread.on('step.*', 'order_placed', (notif) => { ... }); // All step events for this step
   * thread.on('*', 'order_placed', (notif) => { ... });      // All events for this step
   */
  on(event, stepNameOrHandler, handler) {
    // Determine if this is 2-param (thread-level) or 3-param (step-level) signature
    let stepName;
    let actualHandler;
    
    if (typeof stepNameOrHandler === 'function') {
      // 2-param signature: on(event, handler)
      // Thread-level events - use empty string for stepName
      stepName = '';
      actualHandler = stepNameOrHandler;
    } else {
      // 3-param signature: on(event, stepName, handler)
      stepName = stepNameOrHandler;
      actualHandler = handler;
    }
    
    if (typeof actualHandler !== 'function') {
      throw new Error('Handler must be a function');
    }
    
    // Parse event to get source and type for NATS subscription
    const [source, type] = this._parseEvent(event);
    
    // Build event types array for subscription
    const eventTypes = this._buildEventTypes(source, type);
    
    // Send subscription to server
    this._sendSubscription(stepName, eventTypes);
    
    // Store handler
    const handlerKey = `${event}:${stepName}`;
    if (!this.notificationHandlers.has(handlerKey)) {
      this.notificationHandlers.set(handlerKey, []);
    }
    this.notificationHandlers.get(handlerKey).push(actualHandler);
    
    return this;
  }

  /**
   * Unsubscribe from notification events
   * Supports both 1-param (thread-level) and 2-param (step-level) signatures
   * @param {string} event - Event pattern
   * @param {string} [stepName] - Step name (optional, defaults to empty string for thread-level)
   * @returns {Connection} - Returns this for chaining
   * 
   * @example
   * // Thread-level events (1 param)
   * thread.off('thread.cancelled');
   * thread.off('thread.*');
   * 
   * // Step-level events (2 params)
   * thread.off('step.success', 'order_placed');
   */
  off(event, stepName = '') {
    const handlerKey = `${event}:${stepName}`;
    this.notificationHandlers.delete(handlerKey);
    
    // Check if any handlers remain for this step
    const hasHandlers = Array.from(this.notificationHandlers.keys())
      .some(key => key.endsWith(`:${stepName}`));
    
    if (!hasHandlers) {
      this._sendUnsubscription(stepName);
    }
    
    return this;
  }

  /**
   * Parse event string into source and type
   * @private
   * @param {string} event - Event pattern like 'step.success' or 'rule.violated'
   * @returns {[string, string]} - [source, type]
   * 
   * Examples:
   *   'step.success' → ['execution', 'success']
   *   'rule.violated' → ['validation', 'violated']
   *   'step.*' → ['execution', '*']
   *   '*' → ['*', '*']
   */
  _parseEvent(event) {
    // Replace semantic names with NATS source names
    const normalized = event
      .replace('step', 'execution')
      .replace('rule', 'validation');
    
    // Split on dot
    const parts = normalized.split('.');
    
    return [
      parts[0] || '*',  // source: execution | validation | *
      parts[1] || '*'   // type: success | failed | passed | violated | *
    ];
  }

  /**
   * Build event types array for subscription based on source and type
   * @private
   * @param {string} source - Source: execution | validation | *
   * @param {string} type - Type: success | failed | passed | violated | *
   * @returns {string[]} - Array of event types for subscription
   */
  _buildEventTypes(source, type) {
    const eventTypes = [];
    
    // Handle wildcards
    if (source === '*' && type === '*') {
      // Subscribe to all events
      return ['execution.success', 'execution.failed', 'validation.passed', 'validation.violated'];
    }
    
    if (source === 'execution' && type === '*') {
      return ['execution.success', 'execution.failed'];
    }
    
    if (source === 'validation' && type === '*') {
      return ['validation.passed', 'validation.violated'];
    }
    
    // Specific event
    return [`${source}.${type}`];
  }

  /**
   * Setup notification listener for WebSocket messages
   * @private
   */
  _setupNotificationListener() {
    this.ws.on('message', (data) => {
      try {
        const message = JSON.parse(data.toString());
        
        // Handle single notification (push-based with ackToken)
        if (message.action === 'notification') {
          this._handleNotification(message.notification, message.ackToken);
        }
        
        // Handle notification batch
        if (message.action === 'notification_batch') {
          message.notifications.forEach(notif => {
            this._handleNotification(notif);
          });
        }
      } catch (e) {
        // Ignore parse errors for non-JSON messages
      }
    });

    // Setup reconnection handling
    this.ws.on('close', () => {
      this._debugLog('[Thread] WebSocket closed');
      this.isConnected = false;
    });

    this.ws.on('error', (error) => {
      console.error('[Thread] WebSocket error:', error);
    });
  }

  /**
   * Reconnect and resubscribe to all active subscriptions
   * @returns {Promise<void>}
   */
  async reconnect() {
    if (this.isConnected) {
      this._debugLog('[Thread] Already connected');
      return;
    }

    return new Promise((resolve, reject) => {
      // Reconnect logic would need to be handled by creating a new WebSocket
      // For now, just resubscribe if connection is restored
      if (this.isConnected) {
        this._resubscribeAll();
        resolve();
      } else {
        reject(new Error('Not connected'));
      }
    });
  }

  /**
   * Handle incoming notification (v2.0.0)
   * @private
   * @param {Object} notificationData - Notification data
   * @param {string} ackToken - Opaque ACK token for stateless ACK
   */
  _handleNotification(notificationData, ackToken = null) {
    const notifID = notificationData.notificationId;
    
    // Deduplicate notifications
    if (this.processedNotifications.has(notifID)) {
      this._debugLog('[Notification] Duplicate ignored');
      // Still send ACK (idempotent)
      this._sendAck(notifID, notificationData.threadId, ackToken);
      return;
    }
    
    // Add to processed set
    this.processedNotifications.add(notifID);
    
    // Prevent memory leak - remove oldest if too large
    if (this.processedNotifications.size > this.maxProcessedSize) {
      const firstItem = this.processedNotifications.values().next().value;
      this.processedNotifications.delete(firstItem);
    }
    
    const notification = new Notification(notificationData, this, ackToken);
    
    // Determine event pattern from notification
    // notification.source: 'execution' | 'validation' | 'thread'
    // notification.notificationType: 'execution.success', 'validation.violated', etc.
    const eventPattern = this._getEventPattern(notification);
    
    // Trigger handlers for this event pattern
    this._triggerHandlers(eventPattern, notification);

    // Route to thread-specific waitFor()
    const thread = this.threads.get(notification.threadId);
    if (thread) {
      thread._handleNotification(notification);
    }
  }

  /**
   * Get event pattern from notification for handler matching
   * @private
   * @param {Notification} notification - Notification object
   * @returns {string} - Event pattern like 'step.success' or 'rule.violated'
   */
  _getEventPattern(notification) {
    // Map notification source and type back to SDK event pattern
    // execution.success → step.success
    // validation.violated → rule.violated
    const source = notification.source || 'execution';
    const type = notification.notificationType ? notification.notificationType.split('.')[1] : 'success';
    
    const sourceMap = {
      'execution': 'step',
      'validation': 'rule',
      'thread': 'thread'
    };
    
    return `${sourceMap[source] || source}.${type}`;
  }

  /**
   * Trigger handlers for a specific event pattern (v2.0.0)
   * @private
   * @param {string} eventPattern - Event pattern like 'step.success' or 'rule.violated'
   * @param {Notification} notification - Notification object
   */
  _triggerHandlers(eventPattern, notification) {
    const stepName = notification.stepName;
    const contractName = notification.contractName;
    
    // Build possible handler keys to check
    const keysToCheck = [];
    
    // 1. Exact match: "event:contract@stepName"
    if (contractName) {
      keysToCheck.push(`${eventPattern}:${contractName}@${stepName}`);
    }
    
    // 2. Wildcard contract: "event:stepName"
    keysToCheck.push(`${eventPattern}:${stepName}`);
    
    // 3. Wildcard type: "source.*:stepName" (e.g., "step.*:order_placed")
    const [source] = eventPattern.split('.');
    keysToCheck.push(`${source}.*:${stepName}`);
    
    // 4. Full wildcard: "*:stepName"
    keysToCheck.push(`*:${stepName}`);
    
    // Trigger all matching handlers
    keysToCheck.forEach(key => {
      const handlers = this.notificationHandlers.get(key);
      if (handlers && handlers.length > 0) {
        handlers.forEach(handler => {
          try {
            handler(notification);
          } catch (error) {
            console.error('[Notification] Handler error:', error);
          }
        });
      }
    });
  }

  /**
   * Send ACK for a notification
   * @private
   * @param {string} notificationId - Notification ID
   * @param {string} threadId - Thread ID
   * @param {string} ackToken - Opaque ACK token for stateless ACK (required)
   */
  _sendAck(notificationId, threadId, ackToken) {
    if (!ackToken) {
      console.error('[Connection] Cannot ACK: ackToken is required');
      return;
    }

    try {
      const ackMessage = {
        action: 'ack_notification',
        notification_id: notificationId,
        thread_id: threadId,
        ackToken: ackToken,
        processed: true
      };
      
      this.ws.send(JSON.stringify(ackMessage));
      this._debugLog('[Connection] ACK sent');
    } catch (error) {
      console.error('[Connection] Failed to send ACK:', error);
    }
  }

  /**
   * Join a thread using token or direct join
   * @param {string} tokenOrThreadId - JWT invitation token OR threadId for direct join
   * @param {string} role - Role for direct join (internal services only)
   * @returns {Promise<ThreadInstance>} - Returns a new ThreadInstance for the joined thread
   */
  async join(tokenOrThreadId, role = null) {
    if (!tokenOrThreadId) {
      throw new Error("Token or threadId is required for join");
    }
    if (!this.isConnected) {
      throw new Error("Thread must be connected to join. Call Threadify.connect() first.");
    }

    // Determine if this is token-based or direct join
    const isTokenJoin = !role && typeof tokenOrThreadId === 'string' && tokenOrThreadId.length > 50;
    const isDirectJoin = role && typeof tokenOrThreadId === 'string';

    return new Promise((resolve, reject) => {
      // Set up one-time listener for join response
      const responseHandler = (data) => {
        if (data.action === 'joinThread') {
          if (data.status === 'success') {
            this._debugLog(`Joined thread: ${data.threadId}`);
            this._debugLog(`Role: ${data.role}, AccessLevel: ${data.accessLevel}`);
            
            // Create and return a ThreadInstance
            const threadInstance = new ThreadInstance(
              this,
              data.threadId,
              data.contractId,
              data.role,
              null // refs
            );
            
            resolve(threadInstance);
          } else {
            reject(new Error(data.message || 'Failed to join thread'));
          }
        }
      };

      this._onceResponse(responseHandler);

      // Send appropriate join message
      if (isTokenJoin) {
        // Token-based join (external parties)
        this._debugLog(`Joining thread with token: ${tokenOrThreadId.substring(0, 20)}...`);
        this._send({
          action: 'joinThread',
          threadToken: tokenOrThreadId
        });
      } else if (isDirectJoin) {
        this._debugLog(`Joining thread directly: ${tokenOrThreadId} as ${role}`);
        this._send({
          action: 'joinThread',
          threadId: tokenOrThreadId,
          role: role
        });
      } else {
        reject(new Error('Invalid join parameters. Use either token or (threadId, role)'));
      }

      // Timeout after 10 seconds
      setTimeout(() => {
        reject(new Error('Join thread timeout'));
      }, 10000);
    });
  }
}

/**
 * ThreadInstance - Represents a specific thread with its own context
 */
export class ThreadInstance {
  constructor(connection, threadId, contractId, role, refs) {
    this.connection = connection;
    this.threadId = threadId;
    this.id = threadId; // Alias for backward compatibility
    this.contractId = contractId;
    this.role = role;
    this.refs = refs;
    this.steps = new Map();
    this.pendingWaits = new Map(); // stepName -> { resolve, reject, timeoutId, statuses }
  }

  /**
   * Create a new step in this thread
   * @param {string} stepName - Name of the step
   * @param {string} serviceName - Optional service name for the step
   * @returns {ThreadStep} - New ThreadStep instance
   */
  step(stepName, serviceName = null) {
    if (!stepName || typeof stepName !== 'string') {
      throw new Error('Step name must be a non-empty string');
    }

    const step = new ThreadStep(stepName, this, serviceName || this.connection.serviceName);
    this.steps.set(stepName, step);
    return step;
  }

  /**
   * Get thread ID
   * @returns {string} - Thread ID
   */
  getThreadId() {
    return this.threadId;
  }

  /**
   * Get contract ID
   * @returns {string|null} - Contract ID or null for non-contract workflows
   */
  getContractId() {
    return this.contractId;
  }

  /**
   * Send a message through the WebSocket connection
   * @param {Object} message - Message to send
   */
  _send(message) {
    if (!this.connection.ws || this.connection.ws.readyState !== 1) {
      throw new Error('WebSocket not connected');
    }
    this.connection.ws.send(JSON.stringify(message));
  }

  /**
   * Register a one-time response handler
   * @param {Function} handler - Response handler function
   */
  _onceResponse(handler) {
    const wrapper = (data) => {
      try {
        const message = JSON.parse(data.toString());
        handler(message);
        this.connection.ws.removeListener('message', wrapper);
      } catch (e) {
        console.error('Failed to parse message:', e);
      }
    };
    this.connection.ws.on('message', wrapper);
  }

  /**
   * Create an invitation token for this thread
   * @param {Object} options - Invitation options
   * @param {string} options.role - Required business/contract role (e.g., "supplier", "merchant")
   * @param {string} [options.accessLevel="external"] - Optional access level (owner/participant/observer/external)
   * @param {string} [options.expiresIn="24h"] - Optional expiry duration
   * @returns {Promise<Object>} - Invitation response with token and metadata
   */
  async inviteParty(options = {}) {
    const {
      role,
      accessLevel = "external",
      expiresIn = "24h"
    } = options;
    
    if (!role) {
      throw new Error("Role is required for inviteParty");
    }
    
    return new Promise((resolve, reject) => {
      this._onceResponse((message) => {
        if (message.status === 'success') {
          resolve({
            token: message.threadToken,
            threadId: this.threadId,
            role: message.role,
            accessLevel: message.accessLevel,
            expiresAt: message.expiresAt
          });
        } else {
          reject(new Error(message.message || 'Failed to create invitation token'));
        }
      });
      
      this._send({
        action: 'inviteParty',
        role,
        accessLevel,
        expiresIn
      });
    });
  }

  /**
   * Wait for a notification for a specific step
   * @param {string} stepName - Name of the step to wait for
   * @param {Object} options - Wait options
   * @param {number} [options.timeout=5000] - Timeout in milliseconds
   * @param {Array<string>} [options.statuses] - Only resolve for these statuses (e.g., ['success', 'failed'])
   * @returns {Promise<Notification>} - Resolves with notification when it arrives
   */
  waitFor(stepName, options = {}) {
    const { timeout = 5000, statuses = null } = options;
    
    if (!stepName || typeof stepName !== 'string') {
      return Promise.reject(new Error('Step name must be a non-empty string'));
    }
    
    return new Promise((resolve, reject) => {
      const timeoutId = setTimeout(() => {
        this.pendingWaits.delete(stepName);
        reject(new Error(`Timeout waiting for step: ${stepName} (${timeout}ms)`));
      }, timeout);

      this.pendingWaits.set(stepName, {
        resolve,
        reject,
        timeoutId,
        statuses
      });
    });
  }
  
  /**
   * Add external references to this thread
   * @param {Object} refs - Key-value pairs of external references
   * @returns {Promise<Object>} - Response from server
   */
  async addRefs(refs) {
    if (!refs || typeof refs !== 'object' || Object.keys(refs).length === 0) {
      throw new Error('Refs must be a non-empty object');
    }

    return new Promise((resolve, reject) => {
      this._onceResponse((message) => {
        if (message.action === 'addRefs') {
          if (message.status === 'success') {
            // Update local refs
            this.refs = { ...this.refs, ...refs };
            resolve(message);
          } else {
            reject(new Error(message.message || 'Failed to add refs'));
          }
        }
      });

      this._send({
        action: 'addRefs',
        threadId: this.threadId,
        refs
      });
    });
  }

  /**
   * Link this thread to another thread
   * @param {string} threadId - Thread ID to link to
   * @param {string} relationship - Type of relationship (default: 'parent')
   * @returns {Promise<Object>} - Response from server
   */
  async linkThread(threadId, relationship = 'parent') {
    if (!threadId || typeof threadId !== 'string') {
      throw new Error('Thread ID must be a non-empty string');
    }
    
    // Validate UUID format
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    if (!uuidRegex.test(threadId)) {
      throw new Error('Invalid thread ID format');
    }
    
    // Use addRefs under the hood with special prefix
    const refKey = `linkedThread:${relationship}`;
    return this.addRefs({ [refKey]: threadId });
  }

  /**
   * End the thread on the server
   * Defaults to 'cancelled' status, can be marked as 'completed' for successful completion
   * For contract-linked threads, 'completed' status is only allowed when terminal state is reached
   * Requires appropriate permissions (owner or participant with thread.end permission)
   * @param {string} status - Thread end status: 'cancelled' (default) or 'completed'
   * @param {string|Object} reason - Optional reason for ending (string) or data object
   * @returns {Promise<Object>} - Server response
   * @example
   * // Cancel thread (default)
   * await thread.end();
   * await thread.end('cancelled', 'Order cancelled by customer');
   * 
   * // Complete thread (only for non-contract threads or when terminal state reached)
   * await thread.end('completed', 'All steps finished');
   * 
   * // With data object
   * await thread.end('cancelled', { reason: 'Order cancelled', cancelledBy: 'customer' });
   */
  async end(status = 'cancelled', reason = '') {
    return this._endThread(status, reason);
  }

  /**
   * Close the thread on the server
   * @deprecated Use thread.end('cancelled', reason) instead
   * Marks the thread as cancelled
   * @param {string|Object} reason - Optional reason for closure (string) or data object
   * @returns {Promise<Object>} - Server response
   */
  async close(reason = '') {
    return this._endThread('cancelled', reason);
  }

  /**
   * Mark the thread as completed on the server
   * @deprecated Use thread.end('completed', reason) instead
   * For contract-linked threads, this will be rejected unless terminal state is reached
   * @param {string|Object} reason - Optional reason for completion (string) or data object
   * @returns {Promise<Object>} - Server response
   */
  async complete(reason = '') {
    return this._endThread('completed', reason);
  }

  /**
   * Internal method to end a thread (cancel or complete)
   * @private
   */
  async _endThread(status, reason = '') {
    return new Promise((resolve, reject) => {
      this._onceResponse((message) => {
        // Support both old (closeThread) and new (threadEnd) action names
        if (message.action === 'closeThread' || message.action === 'threadEnd') {
          if (message.status === 'success') {
            // Cleanup local state after successful end
            this._cleanup();
            
            resolve({
              threadId: this.threadId,
              status: message.threadStatus,
              endedAt: message.closedAt || message.completedAt || message.cancelledAt,
              message: message.message
            });
          } else {
            reject(new Error(message.message || 'Failed to end thread'));
          }
        }
      });

      // Prepare end data
      const endData = { status };
      if (typeof reason === 'string' && reason) {
        endData.reason = reason;
      } else if (typeof reason === 'object' && reason !== null) {
        Object.assign(endData, reason);
      }

      // Use new action name (server supports both for backward compatibility)
      this._send({
        action: 'threadEnd',
        threadId: this.threadId,
        ...endData
      });
    });
  }

  /**
   * Cleanup local thread state (internal use)
   * @private
   */
  _cleanup() {
    // Reject any pending waitFor() promises
    this.pendingWaits.forEach((pending, stepName) => {
      clearTimeout(pending.timeoutId);
      pending.reject(new Error(`Thread closed while waiting for step: ${stepName}`));
    });
    this.pendingWaits.clear();
    
    // Remove from connection's thread registry
    this.connection.threads.delete(this.threadId);
  }
}
