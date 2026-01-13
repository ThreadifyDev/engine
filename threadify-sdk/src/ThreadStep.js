/**
 * ThreadStep - Represents a step in a thread execution with fluent API
 * @example
 * const step = thread.step('order_placed');
 * await step
 *   .addContext({ orderId: 'ORD-12345' })
 *   .success();
 */
export class ThreadStep {
  constructor(stepName, thread, serviceName = null) {
    this.stepName = stepName;
    this.thread = thread;
    this.serviceName = serviceName;
    this.manualIdempotencyKey = null; // For manual override
    
    // Build event locally, send on stop()
    this.event = {
      action: 'recordThreadEvent',
      threadId: thread.threadId,
      stepName: stepName,
      startedAt: new Date().toISOString(),
      finishedAt: null,
      context: {},
      refs: {}, // Use addRefs() to populate
      status: 'in_progress',
      serviceName: serviceName
    };
  }

  /**
   * Set manual idempotency key (optional)
   * @param {string} key - Idempotency key for deduplication
   * @returns {ThreadStep} - Returns this for method chaining
   */
  idempotencyKey(key) {
    if (typeof key !== 'string' || key.trim() === '') {
      throw new Error('Idempotency key must be a non-empty string');
    }
    this.manualIdempotencyKey = key;
    return this;
  }

  /**
   * Generate idempotency key from step name and context
   * @returns {string} - Hash of stepName + context
   */
  _generateIdempotencyKey() {
    if (this.manualIdempotencyKey) {
      return this.manualIdempotencyKey;
    }
    
    // Create stable string representation of context
    const contextStr = JSON.stringify(this.event.context, Object.keys(this.event.context).sort());
    const input = this.stepName + contextStr;
    
    // Simple hash function (FNV-1a)
    let hash = 2166136261;
    for (let i = 0; i < input.length; i++) {
      hash ^= input.charCodeAt(i);
      hash = Math.imul(hash, 16777619);
    }
    
    // Convert to hex string
    return (hash >>> 0).toString(16).padStart(8, '0');
  }

  /**
   * Add references to external systems
   * @param {Object} refsData - Key-value pairs of external system references
   * @returns {ThreadStep} - Returns this for method chaining
   */
  addRefs(refsData) {
    if (typeof refsData !== 'object' || refsData === null) {
      throw new Error('Refs data must be an object');
    }
    
    // Convert all values to strings as expected by server schema
    const stringifiedRefs = {};
    for (const [key, value] of Object.entries(refsData)) {
      stringifiedRefs[key] = String(value);
    }
    
    this.event.refs = { ...this.event.refs, ...stringifiedRefs };
    return this;
  }

  /**
   * Add context data to this step
   * @param {Object} contextData - Key-value pairs to add to step context
   * @param {boolean} isPrivate - Whether this context is private (optional)
   * @returns {ThreadStep} - Returns this for method chaining
   */
  addContext(contextData, isPrivate = false) {
    if (typeof contextData !== 'object' || contextData === null) {
      throw new Error('Context data must be an object');
    }
    
    // Convert all values to strings as expected by server schema
    const stringifiedContext = {};
    for (const [key, value] of Object.entries(contextData)) {
      stringifiedContext[key] = String(value);
      
      // Mark private context with special prefix if needed
      if (isPrivate) {
        stringifiedContext[`private_${key}`] = String(value);
      }
    }
    
    this.event.context = { ...this.event.context, ...stringifiedContext };
    return this;
  }

  

  /**
   * Stop the step and send the event to server
   * @param {string} status - Final status ('success', 'failed', 'skipped')
   * @param {string} message - Optional message for the step completion
   * @param {Object} finalContext - Optional final context data
   * @returns {Promise<ThreadStep>} - Returns this for method chaining
   */
  async stop(status = 'success', message = '', finalContext = {}) {
    // Set final state
    this.event.finishedAt = new Date().toISOString();
    this.event.status = status;
    
    // Add final context if provided
    if (Object.keys(finalContext).length > 0) {
      this.addContext(finalContext);
    }
    
    // Add message to context if provided
    if (message) {
      this.event.context.message = String(message);
    }
    
    // Generate and add idempotency key
    this.event.idempotencyKey = this._generateIdempotencyKey();
    
    // Send the complete event to server
    try {
      await this._sendEvent();
    } catch (error) {
      // Check if it's a duplicate error
      if (error.isDuplicate) {
        console.warn('⚠️ Duplicate step detected:', error.message);
        // Don't throw - this is expected behavior
        return {
          stepName: this.stepName,
          threadId: this.thread.threadId,
          status: this.event.status,
          idempotencyKey: this.event.idempotencyKey,
          timestamp: this.event.finishedAt || this.event.startedAt,
          duplicate: true
        };
      }
      console.error('Failed to send step event:', error);
      throw error;
    }
    
    // Return a clean response object without internal details
    return {
      stepName: this.stepName,
      threadId: this.thread.threadId,
      status: this.event.status,
      idempotencyKey: this.event.idempotencyKey,
      timestamp: this.event.finishedAt || this.event.startedAt
    };
  }

  /**
   * Send the built event to the server
   * @private
   */
  _sendEvent() {
    return new Promise((resolve, reject) => {
      if (!this.thread.threadId) {
        reject(new Error('Thread not started. Call thread.start() first.'));
        return;
      }

      const message = { ...this.event };

      // Set up one-time listener for response
      const responseHandler = (data) => {
        if (data.action === 'recordThreadEvent') {
          if (data.status === 'success') {
            resolve(data);
          } else {
            // Create error object with isDuplicate flag
            const error = new Error(data.message || 'Failed to record step event');
            error.isDuplicate = data.isDuplicate || false;
            reject(error);
          }
        }
      };

      this.thread._onceResponse(responseHandler);
      this.thread._send(message);
    });
  }

  /**
   * Get the current event data (for debugging)
   * @returns {Object} - Current event data
   */
  getEventData() {
    return { ...this.event };
  }

  /**
   * Get step name
   * @returns {string} - Step name
   */
  getStepName() {
    return this.stepName;
  }

  /**
   * Get step status
   * @returns {string} - Current status
   */
  getStatus() {
    return this.event.status;
  }

  /**
   * Get the current context
   * @returns {Object} - Current context data
   */
  getContext() {
    return { ...this.event.context };
  }

  /**
   * Get the current metadata
   * @returns {Object} - Current metadata data
   */
  getMetadata() {
    return { ...this.event.metadata };
  }

  /**
   * Complete step with success status (convenience method)
   * @param {string} message - Success message (optional)
   * @param {Object} result - Result data (optional)
   * @returns {Promise<Object>} - Server response
   */
  async success(message = 'Step completed successfully', result = {}) {
    return this.stop('success', message, result);
  }

  /**
   * Complete step with error status (convenience method)
   * @param {string} message - Error message (optional)
   * @param {Object} error - Error data (optional)
   * @returns {Promise<Object>} - Server response
   */
  async error(message = 'Step failed with error', error = {}) {
    return this.stop('error', message, error);
  }

  /**
   * Complete step with failed status (convenience method)
   * @param {string} message - Failure message (optional)
   * @param {Object} error - Error data (optional)
   * @returns {Promise<Object>} - Server response
   */
  async failed(message = 'Step failed', error = {}) {
    return this.stop('failed', message, error);
  }
}
