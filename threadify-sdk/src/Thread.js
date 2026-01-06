import { ThreadStep } from './ThreadStep.js';
import { Notification } from './Notification.js';

/**
 * Connection - Represents a WebSocket connection to Threadify Engine
 */
export class Connection {
  constructor(ws, apiKey, serviceName = null) {
    this.ws = ws;
    this.apiKey = apiKey;
    this.serviceName = serviceName;
    this.isConnected = false;
    this.activeThreads = new Map(); // Map of threadId -> thread info
    this.threads = new Map(); // Map of threadId -> ThreadInstance (for notification routing)
    
    // Global notification handlers (step-specific)
    this.notificationHandlers = {
      violation: new Map(),  // stepName -> [handlers]
      completed: new Map(),
      failed: new Map()
    };
    
    this.processedNotifications = new Set(); // Track processed notification IDs
    this.maxProcessedSize = 10000; // Prevent memory leak
    
    this._setupNotificationListener();
  }

  /**
   * Create a new step in this thread
   * @param {string} stepName - Name of the step
   * @param {string} serviceName - Optional service name for the step
   * @param {Object} options - Step options (optional)
   * @param {Object} options.external_refs - External system references
   * @returns {ThreadStep} - New ThreadStep instance
   */
  step(stepName, serviceName = null, options = {}) {
    if (!stepName || typeof stepName !== 'string') {
      throw new Error('Step name must be a non-empty string');
    }

    // Handle overloading: step(name, options) or step(name, serviceName, options)
    if (typeof serviceName === 'object' && serviceName !== null) {
      options = serviceName;
      serviceName = null;
    }

    const step = new ThreadStep(stepName, this, serviceName || this.serviceName, options);
    return step;
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
        if (data.action === 'startThread') {
          if (data.status === 'success') {
            const threadInstance = new ThreadInstance(this, data.threadId, contractName, null, {});
            // Register thread for notification routing
            this.threads.set(data.threadId, threadInstance);
            console.log(`[DEBUG] Thread started: ${data.threadId}`);
            resolve(threadInstance);
          } else {
            reject(new Error(data.message || 'Failed to start thread'));
          }
        }
      };

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
        handler(message);
        this.ws.off('message', wrapper);
      } catch (e) {
        console.error('Failed to parse WebSocket message:', e);
      }
    };
    this.ws.on('message', wrapper);
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
   * Register a global violation handler for a specific step
   * @param {string} stepName - Name of the step to listen for
   * @param {Function} handler - Handler function (receives Notification)
   * @returns {Connection} - Returns this for chaining
   */
  onViolation(stepName, handler) {
    if (typeof handler !== 'function') {
      throw new Error('Handler must be a function');
    }
    if (!this.notificationHandlers.violation.has(stepName)) {
      this.notificationHandlers.violation.set(stepName, []);
    }
    this.notificationHandlers.violation.get(stepName).push(handler);
    return this;
  }

  /**
   * Register a global completion handler for a specific step
   * @param {string} stepName - Name of the step to listen for
   * @param {Function} handler - Handler function (receives Notification)
   * @returns {Connection} - Returns this for chaining
   */
  onCompleted(stepName, handler) {
    if (typeof handler !== 'function') {
      throw new Error('Handler must be a function');
    }
    if (!this.notificationHandlers.completed.has(stepName)) {
      this.notificationHandlers.completed.set(stepName, []);
    }
    this.notificationHandlers.completed.get(stepName).push(handler);
    return this;
  }

  /**
   * Register a global failure handler for a specific step
   * @param {string} stepName - Name of the step to listen for
   * @param {Function} handler - Handler function (receives Notification)
   * @returns {Connection} - Returns this for chaining
   */
  onFailed(stepName, handler) {
    if (typeof handler !== 'function') {
      throw new Error('Handler must be a function');
    }
    if (!this.notificationHandlers.failed.has(stepName)) {
      this.notificationHandlers.failed.set(stepName, []);
    }
    this.notificationHandlers.failed.get(stepName).push(handler);
    return this;
  }

  /**
   * Setup notification listener for WebSocket messages
   * @private
   */
  _setupNotificationListener() {
    this.ws.on('message', (data) => {
      try {
        const message = JSON.parse(data.toString());
        
        // Handle single notification
        if (message.action === 'notification') {
          this._handleNotification(message.notification);
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
  }

  /**
   * Handle incoming notification
   * @private
   */
  _handleNotification(notificationData) {
    const notifID = notificationData.notificationId;
    
    // Deduplicate notifications
    if (this.processedNotifications.has(notifID)) {
      console.log(`[Notification] Duplicate ignored: ${notifID}`);
      // Still send ACK (idempotent)
      this._sendAck(notifID, notificationData.threadId);
      return;
    }
    
    // Add to processed set
    this.processedNotifications.add(notifID);
    
    // Prevent memory leak - remove oldest if too large
    if (this.processedNotifications.size > this.maxProcessedSize) {
      const firstItem = this.processedNotifications.values().next().value;
      this.processedNotifications.delete(firstItem);
    }
    
    const notification = new Notification(notificationData, this);
    const stepName = notification.stepName;
    
    // Trigger global handlers based on notification type
    if (notification.isViolated()) {
      this._triggerHandlers(this.notificationHandlers.violation, stepName, notification);
    } else if (notification.isSuccess()) {
      this._triggerHandlers(this.notificationHandlers.completed, stepName, notification);
    } else if (notification.isFailed() || notification.isError()) {
      this._triggerHandlers(this.notificationHandlers.failed, stepName, notification);
    }

    // Route to thread-specific waitFor()
    const thread = this.threads.get(notification.threadId);
    if (thread) {
      thread._handleNotification(notification);
    }
  }

  /**
   * Trigger handlers for a specific step name
   * @private
   */
  _triggerHandlers(handlerMap, stepName, notification) {
    const handlers = handlerMap.get(stepName);
    if (handlers && handlers.length > 0) {
      handlers.forEach(handler => {
        try {
          handler(notification);
        } catch (error) {
          console.error(`[Notification] Handler error for ${stepName}:`, error);
        }
      });
    }
  }

  /**
   * Send ACK for a notification
   * @private
   */
  _sendAck(notificationId, threadId) {
    try {
      const ackMessage = {
        action: 'ack_notification',
        notification_id: notificationId,
        thread_id: threadId,
        processed: true
      };
      this.ws.send(JSON.stringify(ackMessage));
      console.log(`[Connection] ACK sent for notification: ${notificationId}`);
    } catch (error) {
      console.error(`[Connection] Failed to send ACK for ${notificationId}:`, error);
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
            console.log(`[DEBUG] Joined thread: ${data.threadId}`);
            console.log(`[DEBUG] Role: ${data.role}, Permissions: ${data.permissions}`);
            
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
        console.log(`[DEBUG] Joining thread with token: ${tokenOrThreadId.substring(0, 20)}...`);
        this._send({
          action: 'joinThread',
          threadToken: tokenOrThreadId
        });
      } else if (isDirectJoin) {
        // Direct join (internal services)
        console.log(`[DEBUG] Joining thread directly: ${tokenOrThreadId} as ${role}`);
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
    this.contractId = contractId;
    this.role = role;
    this.refs = refs;
    this.steps = new Map();
    this.pendingWaits = new Map(); // stepName -> { resolve, reject, timeoutId, statuses }
  }

  /**
   * Create a new step in this thread instance
   * @param {string} stepName - Name of the step
   * @param {string} serviceName - Optional service name for the step
   * @param {Object} options - Step options (optional)
   * @returns {ThreadStep} - New ThreadStep instance
   */
  step(stepName, serviceName = null, options = {}) {
    if (!stepName || typeof stepName !== 'string') {
      throw new Error('Step name must be a non-empty string');
    }

    // Handle overloading: step(name, options) or step(name, serviceName, options)
    if (typeof serviceName === 'object' && serviceName !== null) {
      options = serviceName;
      serviceName = null;
    }

    const step = new ThreadStep(stepName, this, serviceName || this.connection.serviceName, options);
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
    const listener = (data) => {
      handler(data);
      this.connection.ws.removeListener('message', listener);
    };
    this.connection.ws.on('message', (data) => {
      try {
        const message = JSON.parse(data.toString());
        listener(message);
      } catch (e) {
        console.error('Failed to parse message:', e);
      }
    });
  }

  /**
   * Create an invitation token for this thread
   * @param {Object} options - Invitation options
   * @param {string} options.role - Required role for the invitation
   * @param {string} [options.permissions="read,write"] - Optional permissions
   * @param {string} [options.expiresIn="24h"] - Optional expiry duration
   * @returns {Promise<string>} - JWT invitation token
   */
  async inviteParty(options = {}) {
    const {
      role,
      permissions = "read,write",
      expiresIn = "24h"
    } = options;
    
    if (!role) {
      throw new Error("Role is required for inviteParty");
    }
    
    return new Promise((resolve, reject) => {
      this._onceResponse((message) => {
        if (message.status === 'success') {
          resolve(message.threadToken);
        } else {
          reject(new Error(message.message || 'Failed to create invitation token'));
        }
      });
      
      this._send({
        action: 'inviteParty',
        role,
        permissions,
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
   * Handle incoming notification for this thread
   * @private
   */
  _handleNotification(notification) {
    const stepName = notification.stepName;
    const pending = this.pendingWaits.get(stepName);
    
    if (pending) {
      // Check if status matches filter (if provided)
      if (!pending.statuses || pending.statuses.includes(notification.stepStatus)) {
        clearTimeout(pending.timeoutId);
        this.pendingWaits.delete(stepName);
        
        // ✅ AUTO-ACK for waitFor() - promise fulfilled means notification received
        notification.ack();
        
        pending.resolve(notification);
      }
    }
  }

  /**
   * Close this thread instance
   * @returns {Promise<void>}
   */
  async close() {
    // Reject any pending waitFor() promises
    this.pendingWaits.forEach((pending, stepName) => {
      clearTimeout(pending.timeoutId);
      pending.reject(new Error(`Thread closed while waiting for step: ${stepName}`));
    });
    this.pendingWaits.clear();
    
    // Remove from connection's thread registry
    this.connection.threads.delete(this.threadId);
    
    return Promise.resolve();
  }
}
