/**
 * ThreadStep - Represents a step in a thread execution
 */
export class ThreadStep {
  constructor(stepName, thread) {
    this.stepName = stepName;
    this.thread = thread;
    this.context = {};
    this.startedAt = null;
    this.finishedAt = null;
    this.status = null;
  }

  /**
   * Add context data to this step
   * @param {Object} contextData - Key-value pairs to add to step context
   * @returns {ThreadStep} - Returns this for method chaining
   */
  addContext(contextData) {
    if (typeof contextData !== 'object' || contextData === null) {
      throw new Error('Context data must be an object');
    }
    
    this.context = { ...this.context, ...contextData };
    return this;
  }

  /**
   * Mark this step as started
   * @returns {ThreadStep} - Returns this for method chaining
   */
  start() {
    this.startedAt = new Date().toISOString();
    this.status = 'started';
    
    // Send event to server
    this.thread._recordEvent({
      threadId: this.thread.threadId,
      startedAt: this.startedAt,
      status: this.status,
      context: {
        step: this.stepName,
        ...this.context
      }
    });
    
    return this;
  }

  /**
   * Mark this step as completed
   * @param {Object} metadata - Optional metadata for completion
   * @returns {ThreadStep} - Returns this for method chaining
   */
  complete(metadata = {}) {
    this.finishedAt = new Date().toISOString();
    this.status = 'completed';
    
    // Send event to server
    this.thread._recordEvent({
      threadId: this.thread.threadId,
      startedAt: this.startedAt,
      finishedAt: this.finishedAt,
      status: this.status,
      context: {
        step: this.stepName,
        ...this.context
      },
      metadata
    });
    
    return this;
  }

  /**
   * Mark this step as failed
   * @param {Error|string} error - Error object or message
   * @param {Object} metadata - Optional metadata for failure
   * @returns {ThreadStep} - Returns this for method chaining
   */
  fail(error, metadata = {}) {
    this.finishedAt = new Date().toISOString();
    this.status = 'failed';
    
    const errorMessage = error instanceof Error ? error.message : error;
    
    // Send event to server
    this.thread._recordEvent({
      threadId: this.thread.threadId,
      startedAt: this.startedAt,
      finishedAt: this.finishedAt,
      status: this.status,
      context: {
        step: this.stepName,
        error: errorMessage,
        ...this.context
      },
      metadata
    });
    
    return this;
  }

  /**
   * Get the current context
   * @returns {Object} - Current context data
   */
  getContext() {
    return { ...this.context };
  }

  /**
   * Get step status
   * @returns {string|null} - Current status
   */
  getStatus() {
    return this.status;
  }
}
