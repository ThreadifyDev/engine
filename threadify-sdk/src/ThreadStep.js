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
    this.subSteps = []; // Accumulate sub-steps
    
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
   * Add a sub-step to be sent when this step completes
   * @param {string} name - Sub-step name
   * @param {Object} data - Sub-step data (duration, metadata, error, etc.)
   * @param {string} status - Sub-step status: 'success' or 'failed' (default: 'success')
   * @returns {ThreadStep} - Returns this for method chaining
   * @example
   * step.subStep('validate_inventory', { itemsChecked: 5 });
   * step.subStep('calculate_tax', { taxAmount: 12.50 }, 'success');
   * step.subStep('apply_discount', { error: 'Invalid coupon' }, 'failed');
   */
  subStep(name, data = {}, status = 'success') {
    if (typeof name !== 'string' || name.trim() === '') {
      throw new Error('Sub-step name must be a non-empty string');
    }
    
    if (status !== 'success' && status !== 'failed') {
      throw new Error('Sub-step status must be either "success" or "failed"');
    }
    
    this.subSteps.push({
      name,
      status,
      payload: data,  // All user data goes in payload
      recordedAt: new Date().toISOString()
    });
    
    return this;
  }

  

  /**
   * Stop the step and send the event to server
   * @param {string} status - Final status ('success', 'failed', 'skipped')
   * @param {string|Object} messageOrData - Optional message (string) or data object
   * @returns {Promise<ThreadStep>} - Returns this for method chaining
   */
  async stop(status = 'success', messageOrData = '') {
    // Set final state
    this.event.finishedAt = new Date().toISOString();
    this.event.status = status;
    
    // Handle messageOrData - can be string or object
    if (typeof messageOrData === 'string') {
      // If string, add as message field in threadify_metadata
      if (messageOrData) {
        if (!this.event.threadify_metadata) {
          this.event.threadify_metadata = {};
        }
        this.event.threadify_metadata.message = messageOrData;
      }
    } else if (typeof messageOrData === 'object' && messageOrData !== null) {
      // If object, add to threadify_metadata (keep separate from context)
      if (Object.keys(messageOrData).length > 0) {
        if (!this.event.threadify_metadata) {
          this.event.threadify_metadata = {};
        }
        Object.assign(this.event.threadify_metadata, messageOrData);
      }
    }
    
    // Add sub-steps as array if any were recorded
    if (this.subSteps.length > 0) {
      this.event.subSteps = this.subSteps;
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
   * @param {string|Object} messageOrData - Success message (string) or data object
   * @returns {Promise<Object>} - Server response
   * @example
   * // With string message
   * await step.success('Order placed successfully');
   * 
   * // With data object
   * await step.success({ message: 'Order placed', orderId: 'ORD-123', total: 99.99 });
   * 
   * // Without data
   * await step.success();
   */
  async success(messageOrData = '') {
    return this.stop('success', messageOrData);
  }

  /**
   * Complete step with error status (convenience method)
   * @param {string|Object} messageOrData - Error message (string) or error data object
   * @returns {Promise<Object>} - Server response
   * @example
   * // With string message
   * await step.error('Service unavailable');
   * 
   * // With error object
   * await step.error({ message: 'Service unavailable', service: 'inventory-api', statusCode: 503 });
   * 
   * // Without data
   * await step.error();
   */
  async error(messageOrData = '') {
    return this.stop('error', messageOrData);
  }

  /**
   * Complete step with failed status (convenience method)
   * @param {string|Object} messageOrData - Failure message (string) or error data object
   * @returns {Promise<Object>} - Server response
   * @example
   * // With string message
   * await step.failed('Payment processing failed');
   * 
   * // With error object
   * await step.failed({ message: 'Payment processing failed', errorCode: 'TIMEOUT', retries: 2 });
   * 
   * // Without data
   * await step.failed();
   */
  async failed(messageOrData = '') {
    return this.stop('failed', messageOrData);
  }
}
