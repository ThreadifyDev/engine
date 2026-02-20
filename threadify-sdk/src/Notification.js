/**
 * Notification - Wrapper class for validation notifications
 * Provides helper methods and ACK functionality
 */
export class Notification {
  /**
   * Create a new Notification
   * @param {Object} data - Raw notification data from server
   * @param {Object} connection - Connection instance for ACK
   * @param {string} ackToken - Opaque ACK token for stateless ACK
   */
  constructor(data, connection, ackToken = null) {
    // Core fields
    this.notificationId = data.notificationId;
    this.threadId = data.threadId;
    this.stepId = data.stepId;
    this.stepName = data.stepName;
    this.contractName = data.contractName || ''; // Contract name (empty for non-contract threads)
    
    // Status fields
    this.status = data.status; // 'passed', 'violated', 'none'
    this.stepStatus = data.stepStatus; // 'success', 'failed', 'error'
    this.severity = data.severity; // 'info', 'warning', 'critical'
    
    // Content fields
    this.message = data.message;
    this.details = data.details || {};
    this.timestamp = data.timestamp ? new Date(data.timestamp) : new Date();
    
    // Violation-specific
    this.violationType = data.violationType || null;
    this.ownerId = data.ownerId;
    
    // Push-based model fields
    this.ackToken = ackToken; // Opaque token for stateless ACK
    
    // Internal state
    this._connection = connection;
    this._acknowledged = false;
  }

  /**
   * Acknowledge this notification
   * Sends ACK message to server with ackToken for stateless ACK
   */
  ack() {
    if (this._acknowledged) {
      console.warn(`[Notification] Already acknowledged: ${this.notificationId}`);
      return;
    }

    // Require ackToken for push-based model
    if (!this.ackToken) {
      throw new Error(`Cannot ACK notification ${this.notificationId}: ackToken is required`);
    }

    this._acknowledged = true;

    // Send ACK to server with ackToken
    const ackMessage = {
      action: 'ack_notification',
      notification_id: this.notificationId,
      thread_id: this.threadId,
      ackToken: this.ackToken,
      processed: true
    };

    try {
      this._connection.ws.send(JSON.stringify(ackMessage));
      this._connection._debugLog('[Notification] ACK sent');
    } catch (error) {
      console.error(`[Notification] Failed to send ACK:`, error);
      this._acknowledged = false; // Reset so user can retry
    }
  }

  /**
   * Check if this notification represents a violation
   * @returns {boolean}
   */
  isViolated() {
    return this.status === 'violated';
  }

  /**
   * Check if validation passed
   * @returns {boolean}
   */
  isPassed() {
    return this.status === 'passed';
  }

  /**
   * Check if this is a critical notification
   * @returns {boolean}
   */
  isCritical() {
    return this.severity === 'critical';
  }

  /**
   * Check if this is a warning notification
   * @returns {boolean}
   */
  isWarning() {
    return this.severity === 'warning';
  }

  /**
   * Check if this is an info notification
   * @returns {boolean}
   */
  isInfo() {
    return this.severity === 'info';
  }

  /**
   * Check if step completed successfully
   * @returns {boolean}
   */
  isSuccess() {
    return this.stepStatus === 'success';
  }

  /**
   * Check if step failed
   * @returns {boolean}
   */
  isFailed() {
    return this.stepStatus === 'failed';
  }

  /**
   * Check if step had an error
   * @returns {boolean}
   */
  isError() {
    return this.stepStatus === 'error';
  }

  /**
   * Get a human-readable description of the notification
   * @returns {string}
   */
  toString() {
    return `[${this.severity.toUpperCase()}] ${this.stepName}: ${this.message}`;
  }

  /**
   * Convert to plain object (for logging/serialization)
   * @returns {Object}
   */
  toJSON() {
    return {
      notificationId: this.notificationId,
      threadId: this.threadId,
      stepId: this.stepId,
      stepName: this.stepName,
      contractName: this.contractName,
      status: this.status,
      stepStatus: this.stepStatus,
      severity: this.severity,
      message: this.message,
      details: this.details,
      timestamp: this.timestamp,
      violationType: this.violationType,
      ownerId: this.ownerId,
      acknowledged: this._acknowledged
    };
  }
}

export default Notification;
