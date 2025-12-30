import { ThreadStep } from './ThreadStep.js';

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
   * Close this thread instance
   * @returns {Promise<void>}
   */
  async close() {
    // For now, just resolve. In future, we might send a close message
    return Promise.resolve();
  }
}
