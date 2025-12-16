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
}
