import { ThreadStep } from './ThreadStep.js';

/**
 * Thread - Represents a thread instance with WebSocket connection
 */
export class Thread {
  constructor(ws, apiKey, ownerId, serviceName = null) {
    this.ws = ws;
    this.apiKey = apiKey;
    this.ownerId = ownerId;
    this.serviceName = serviceName;
    this.threadId = null;
    this.contractId = null;
    this.isConnected = false;
    this.steps = new Map();
    this.eventHandlers = {
      onSuccess: [],
      onError: [],
      onViolation: [],
      onStepProgress: []
    };
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

    const step = new ThreadStep(stepName, this, serviceName || this.serviceName);
    this.steps.set(stepName, step);
    return step;
  }

  /**
   * Start the thread (creates thread on server)
   * @param {string} contractId - Contract ID (optional, can be empty or null for threads without contracts)
   * @param {Object} metadata - Optional metadata
   * @returns {Promise<Thread>} - Returns this Thread instance for fluent API
   */
  async start(contractId, metadata = {}) {
    if (!this.isConnected) {
      throw new Error('Not connected. Call Threadify.connect() first.');
    }

    // contractId is optional - allow empty string or null for threads without contracts
    if (contractId === undefined) {
      contractId = '';
    }

    return new Promise((resolve, reject) => {
      const message = {
        action: 'startThread',
        contractId,
        metadata: {
          ...metadata,
          serviceName: this.serviceName
        }
      };

      // Set up one-time listener for response
      const responseHandler = (data) => {
        if (data.action === 'startThread') {
          if (data.status === 'success') {
            this.threadId = data.threadId;
            this.contractId = data.contractId;
            console.log(`[DEBUG] Thread started: ${data.threadId}`);
            resolve(this); // Return the thread instance for fluent API
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
   * Subscribe to thread events
   * @param {string} eventType - Event type (onSuccess, onError, onViolation, onStepProgress)
   * @param {Function} handler - Event handler function
   */
  on(eventType, handler) {
    if (!this.eventHandlers[eventType]) {
      throw new Error(`Unknown event type: ${eventType}`);
    }
    this.eventHandlers[eventType].push(handler);
  }

  /**
   * Unsubscribe from thread events
   * @param {string} eventType - Event type
   * @param {Function} handler - Event handler function to remove
   */
  off(eventType, handler) {
    if (!this.eventHandlers[eventType]) {
      return;
    }
    const index = this.eventHandlers[eventType].indexOf(handler);
    if (index > -1) {
      this.eventHandlers[eventType].splice(index, 1);
    }
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
   * Create an invitation token for this thread
   * @param {Object} options - Invitation options
   * @param {string} options.role - Required role for the invitation
   * @param {string} [options.permissions="read,write"] - Optional permissions
   * @param {string} [options.expiresIn="24h"] - Optional expiry duration
   * @returns {Promise<string>} - JWT invitation token
   */
  async inviteParty(options = {}) {
    const {
      role,                    // Required
      permissions = "read,write", // Optional with default
      expiresIn = "24h"         // Optional with default
    } = options;
    
    // Validate required role
    if (!role) {
      throw new Error("Role is required for inviteParty");
    }
    
    // Validate thread is connected and has threadId
    if (!this.isConnected || !this.threadId) {
      throw new Error("Thread must be connected and started to create invitations");
    }
    
    return new Promise((resolve, reject) => {
      // Set up one-time response handler
      this._onceResponse((message) => {
        if (message.status === 'success') {
          resolve(message.threadToken);
        } else {
          reject(new Error(message.message || 'Failed to create invitation token'));
        }
      });
      
      // Send inviteParty message
      this._send({
        action: 'inviteParty',
        role,
        permissions,
        expiresIn
      });
    });
  }

  /**
   * Join a thread using an invitation token (instance method)
   * @param {string} threadToken - JWT invitation token
   * @returns {Promise<Thread>} - Returns this Thread instance with updated context
   */
  async join(threadToken) {
    if (!threadToken) {
      throw new Error("Thread token is required for join");
    }
    if (!this.isConnected) {
      throw new Error("Thread must be connected to join. Call Threadify.connect() first.");
    }

    return new Promise((resolve, reject) => {
      // Set up one-time listener for join response
      const responseHandler = (data) => {
        if (data.action === 'joinThread') {
          if (data.status === 'success') {
            // Update thread context with joined thread info
            this.threadId = data.threadId;
            this.contractId = data.contractId;
            this.role = data.role;
            this.permissions = data.permissions;
            
            console.log(`[DEBUG] Joined thread: ${data.threadId}`);
            console.log(`[DEBUG] Role: ${data.role}, Permissions: ${data.permissions}`);
            
            resolve(this);
          } else {
            reject(new Error(data.message || 'Failed to join thread'));
          }
        }
      };

      // Add response handler using existing method
      this._onceResponse(responseHandler);

      // Send join thread message
      const joinMessage = {
        action: 'joinThread',
        threadToken: threadToken
      };

      console.log(`[DEBUG] Joining thread with token: ${threadToken.substring(0, 20)}...`);
      this._send(joinMessage);

      // Timeout after 10 seconds
      setTimeout(() => {
        reject(new Error('Join thread timeout'));
      }, 10000);
    });
  }
}
