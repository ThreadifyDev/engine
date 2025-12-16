/**
 * ThreadStep - Represents a step in a thread execution with fluent API
 */
export class ThreadStep {
  constructor(stepName, thread, serviceName = null) {
    this.stepName = stepName;
    this.thread = thread;
    this.serviceName = serviceName;
    
    // Build event locally, send on stop()
    this.event = {
      action: 'recordThreadEvent',
      threadId: thread.threadId,
      stepName: stepName,
      startedAt: new Date().toISOString(),
      finishedAt: null,
      context: {},
      status: 'in_progress',
      metadata: {},
      serviceName: serviceName
    };
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
   * Add metadata to this step
   * @param {Object} metadataData - Key-value pairs to add to step metadata
   * @returns {ThreadStep} - Returns this for method chaining
   */
  addMetadata(metadataData) {
    if (typeof metadataData !== 'object' || metadataData === null) {
      throw new Error('Metadata data must be an object');
    }
    
    // Convert all values to strings as expected by server schema
    const stringifiedMetadata = {};
    for (const [key, value] of Object.entries(metadataData)) {
      stringifiedMetadata[key] = String(value);
    }
    
    this.event.metadata = { ...this.event.metadata, ...stringifiedMetadata };
    return this;
  }

  /**
   * Stop the step and send the event to server
   * @param {string} status - Final status ('success', 'failed', 'skipped')
   * @param {string} message - Optional message for the step completion
   * @param {Object} finalMetadata - Optional final metadata
   * @returns {Promise<ThreadStep>} - Returns this for method chaining
   */
  async stop(status = 'success', message = '', finalMetadata = {}) {
    // Set final state
    this.event.finishedAt = new Date().toISOString();
    this.event.status = status;
    
    // Add final metadata if provided
    if (Object.keys(finalMetadata).length > 0) {
      this.addMetadata(finalMetadata);
    }
    
    // Add message to metadata if provided
    if (message) {
      this.event.metadata.message = String(message);
    }
    
    // Send the complete event to server
    try {
      await this._sendEvent();
    } catch (error) {
      console.error('Failed to send step event:', error);
      throw error;
    }
    
    return this;
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
            reject(new Error(data.message || 'Failed to record step event'));
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
}
