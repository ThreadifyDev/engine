/**
 * TypeScript definitions for @threadify/sdk
 */

export interface ThreadifyConnectOptions {
  /** WebSocket URL (default: ws://localhost:8081/threads) */
  url?: string;
  /** WebSocket URL (alias for url) */
  wsUrl?: string;
  /** GraphQL URL (default: derived from wsUrl) */
  graphqlUrl?: string;
  /** Enable debug logging (default: false) */
  debug?: boolean;
}

export interface StepContext {
  [key: string]: string | number | boolean;
}

export interface ThreadRefs {
  [key: string]: string;
}

export type StepStatus = 'success' | 'failed' | 'error';

export interface SubStepData {
  /** Execution duration in milliseconds */
  duration?: number;
  /** Error message if sub-step failed */
  error?: string;
  /** Additional metadata */
  metadata?: Record<string, any>;
  /** Any other custom fields */
  [key: string]: any;
}

export interface StepResult {
  stepName: string;
  threadId: string;
  status: StepStatus;
  idempotencyKey: string;
  timestamp: string;
  duplicate?: boolean;
}

export interface ThreadOptions {
  external_refs?: ThreadRefs;
}

export interface ArchivedThreadData {
  id: string;
  contractId: string;
  contractName: string;
  contractVersion: string;
  ownerId: string;
  companyId: string;
  status: string;
  startedAt: string;
  completedAt?: string;
  error?: string;
  refs: string;
}

export interface ArchivedStepData {
  threadId: string;
  stepName: string;
  idempotencyKey: string;
  status: StepStatus;
  retryCount: number;
  firstSeenAt: string;
  lastUpdatedAt: string;
  latestStepID: string;
  previousStep?: string;
  verified: boolean;
  verificationError?: string;
}

export interface StepHistoryData {
  attempt: number;
  timestamp: string;
  status: StepStatus;
  context: string;
  duration: number;
  error?: string;
}

export interface ValidationResult {
  validationId: string;
  threadId: string;
  stepId: string;
  stepName: string;
  idempotencyKey?: string;
  timestamp: string;
  validations: Array<{
    type: string;
    severity: 'critical' | 'warning' | 'info';
    message: string;
    details?: any;
  }>;
  overallStatus: 'critical' | 'warning' | 'info';
  hasCriticalViolation: boolean;
  criticalCount: number;
  warningCount: number;
  infoCount: number;
}

export class ArchivedStep {
  readonly threadId: string;
  readonly stepName: string;
  readonly idempotencyKey: string;
  readonly status: StepStatus;
  readonly retryCount: number;
  readonly firstSeenAt: string;
  readonly lastUpdatedAt: string;
  readonly latestStepID: string;
  readonly previousStep?: string;
  readonly verified: boolean;
  readonly verificationError?: string;

  /**
   * Get execution history for this step
   * @param options - Query options
   * @returns Promise resolving to step history
   */
  history(options?: {
    limit?: number;
    activityType?: string;
    startAt?: string;
    endAt?: string;
  }): Promise<StepHistoryData[]>;
}

export class ArchivedThread {
  readonly id: string;
  readonly contractId: string;
  readonly contractName: string;
  readonly contractVersion: string;
  readonly ownerId: string;
  readonly companyId: string;
  readonly status: string;
  readonly startedAt: string;
  readonly completedAt?: string;
  readonly error?: string;
  readonly refs: any;

  /**
   * Get all steps for this thread, optionally filtered
   * @param stepIdentifier - Optional filter: "stepName" or "stepName:idempotencyKey"
   * @param options - Optional query options
   * @returns Promise resolving to array of archived steps
   */
  steps(stepIdentifier?: string, options?: { status?: StepStatus }): Promise<ArchivedStep[]>;

  /**
   * Get validation results for this thread
   * @param options - Query options
   * @returns Promise resolving to validation results
   */
  validationResults(options?: { limit?: number }): Promise<ValidationResult[]>;
}

export interface NotificationData {
  threadId: string;
  stepName: string;
  status: string;
  message?: string;
  context?: StepContext;
  timestamp: string;
}

export class ThreadStep {
  /** Step name */
  readonly stepName: string;
  
  /**
   * Set manual idempotency key
   * @param key - Idempotency key for deduplication
   * @returns This ThreadStep instance for chaining
   */
  idempotencyKey(key: string): ThreadStep;
  
  /**
   * Add external references
   * @param refs - Reference key-value pairs
   * @returns This ThreadStep instance for chaining
   */
  addRefs(refs: ThreadRefs): ThreadStep;
  
  /**
   * Add context data to this step
   * @param contextData - Key-value pairs to add to step context
   * @param isPrivate - Whether this context is private
   * @returns This ThreadStep instance for chaining
   */
  addContext(contextData: StepContext, isPrivate?: boolean): ThreadStep;
  
  /**
   * Add a sub-step to be sent when this step completes
   * @param name - Sub-step name
   * @param data - Sub-step data (duration, metadata, error, etc.)
   * @param status - Sub-step status: 'success' or 'failed' (default: 'success')
   * @returns This ThreadStep instance for chaining
   * @example
   * step.subStep('validate_inventory', { itemsChecked: 5 });
   * step.subStep('calculate_tax', { taxAmount: 12.50 }, 'success');
   * step.subStep('apply_discount', { error: 'Invalid coupon' }, 'failed');
   */
  subStep(name: string, data?: SubStepData, status?: 'success' | 'failed'): ThreadStep;
  
  /**
   * Mark step as successful
   * @param messageOrData - Success message (string) or data object
   * @returns Promise resolving to step result (without internal details)
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
  success(messageOrData?: string | StepContext): Promise<StepResult>;
  
  /**
   * Mark step as failed
   * @param messageOrData - Failure message (string) or error data object
   * @returns Promise resolving to step result (without internal details)
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
  failed(messageOrData?: string | StepContext): Promise<StepResult>;
  
  /**
   * Mark step as error
   * @param messageOrData - Error message (string) or error data object
   * @returns Promise resolving to step result (without internal details)
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
  error(messageOrData?: string | StepContext): Promise<StepResult>;
}

export interface InvitePartyOptions {
  /** Role for the invited user */
  role: string;
  /** Permissions (comma-separated: 'read', 'write', 'execute') */
  permissions?: string;
  /** Token expiration (e.g., '24h', '7d', '30d') */
  expiresIn?: string;
}

export interface InvitePartyResponse {
  /** JWT invitation token to share with invited user */
  token: string;
  /** Thread ID */
  threadId: string;
  /** Role assigned to invited user */
  role: string;
  /** Permissions granted */
  permissions: string;
  /** Token expiration timestamp */
  expiresAt: string;
}

export class ThreadInstance {
  /** Thread ID */
  readonly threadId: string;
  /** Contract ID */
  readonly contractId: string;
  /** User's role in this thread */
  readonly role?: string;
  /** User's permissions in this thread */
  readonly permissions?: string;
  
  /**
   * Create a new step in this thread
   * @param stepName - Name of the step
   * @param options - Thread options
   * @returns New ThreadStep instance
   */
  step(stepName: string, options?: ThreadOptions): ThreadStep;
  
  /**
   * Invite another user to join this thread
   * @param options - Invitation options (role, permissions, expiration)
   * @returns Promise resolving to invitation response with token
   */
  inviteParty(options: InvitePartyOptions): Promise<InvitePartyResponse>;
  
  /**
   * Get thread metadata
   * @returns Promise resolving to thread metadata
   */
  getMetadata(): Promise<any>;
  
  /**
   * Close the thread on the server (marks thread as closed)
   * Requires appropriate permissions (owner or participant with thread.close permission)
   * @param reason - Optional reason for closing (string) or data object
   * @returns Promise resolving to close response
   * @example
   * await thread.close('Order cancelled by customer');
   * await thread.close({ reason: 'Order cancelled', cancelledBy: 'customer' });
   */
  close(reason?: string | Record<string, any>): Promise<{
    threadId: string;
    status: string;
    closedAt: string;
    message: string;
  }>;
  
  /**
   * Mark the thread as completed on the server
   * Useful for threads without contracts
   * Requires appropriate permissions (owner or participant with thread.close permission)
   * @param reason - Optional reason for completion (string) or data object
   * @returns Promise resolving to completion response
   * @example
   * await thread.complete('All steps finished');
   * await thread.complete({ totalSteps: 5, duration: 120 });
   */
  complete(reason?: string | Record<string, any>): Promise<{
    threadId: string;
    status: string;
    closedAt: string;
    message: string;
  }>;
  
  /**
   * Add a notification handler
   * @param eventName - Event name to listen for
   * @param handler - Event handler function
   */
  on(eventName: string, handler: (data: NotificationData) => void): void;
  
  /**
   * Remove a notification handler
   * @param eventName - Event name
   * @param handler - Event handler function to remove
   */
  off(eventName: string, handler: (data: NotificationData) => void): void;
}

export interface NotificationHandlers {
  violation?: (data: NotificationData) => void;
  completed?: (data: NotificationData) => void;
  failed?: (data: NotificationData) => void;
}

export class Connection {
  /** WebSocket connection status */
  readonly isConnected: boolean;
  /** GraphQL endpoint URL */
  readonly graphqlUrl: string;
  
  /**
   * Start a new thread
   * @param contractName - Contract name (optional for non-contract workflows)
   * @param serviceName - Service name for role inference
   * @param options - Thread options
   * @returns Promise resolving to new ThreadInstance
   */
  start(contractName?: string, serviceName?: string, options?: ThreadOptions): Promise<ThreadInstance>;
  
  /**
   * Join an existing thread
   * @param tokenOrThreadId - Thread token or thread ID
   * @param role - Role for the thread (required if using thread ID)
   * @returns Promise resolving to ThreadInstance
   */
  join(tokenOrThreadId: string, role?: string): Promise<ThreadInstance>;
  
  /**
   * Get archived thread by ID
   * @param threadId - Thread ID
   * @returns Promise resolving to archived thread
   */
  getThread(threadId: string): Promise<ArchivedThread>;
  
  /**
   * Get archived thread by reference
   * @param refKey - Reference key
   * @param refValue - Reference value
   * @returns Promise resolving to archived thread
   */
  getThreadByRef(refKey: string, refValue: string): Promise<ArchivedThread>;
  
  /**
   * Get multiple threads by reference
   * @param refQuery - Reference query {refKey, refValue}
   * @returns Promise resolving to array of archived threads
   */
  getThreadsByRef(refQuery: { refKey: string; refValue: string }): Promise<ArchivedThread[]>;
  
  /**
   * Get thread chain starting from any thread
   * @param startThreadId - Starting thread ID (can be any thread in the chain)
   * @param maxDepth - Maximum depth to traverse downward (default: 3)
   * @returns Promise resolving to array of threads from starting thread to descendants
   */
  getThreadChain(startThreadId: string, maxDepth?: number): Promise<ArchivedThread[]>;
  
  /**
   * Subscribe to notification events for a specific step
   * @param event - Event pattern:
   *   - 'step.success' - Step executed successfully
   *   - 'step.failed' - Step execution failed
   *   - 'rule.violated' - Contract validation violated
   *   - 'rule.passed' - Contract validation passed
   *   - 'step.*' - All step execution events
   *   - 'rule.*' - All validation events
   *   - '*' - All events
   * @param stepIdentifier - Step name or "contractName@stepName"
   * @param handler - Notification handler function
   * @returns Connection instance for chaining
   * @example
   * connection.on('step.success', 'order_placed', (notif) => {
   *   console.log('Order placed successfully');
   *   notif.ack();
   * });
   * 
   * connection.on('rule.violated', 'product_delivery@order_placed', (notif) => {
   *   console.error('Validation failed:', notif.message);
   *   notif.ack();
   * });
   */
  on(event: string, stepIdentifier: string, handler: (notification: any) => void): Connection;
  
  /**
   * Unsubscribe from notification events
   * @param event - Event pattern to unsubscribe from
   * @param stepIdentifier - Step name or "contractName@stepName"
   * @returns Connection instance for chaining
   */
  off(event: string, stepIdentifier: string): Connection;
  
  /**
   * Close the WebSocket connection
   */
  close(): void;
}

export class Threadify {
  /**
   * Connect to Threadify Engine
   * @param apiKey - Your API key
   * @param serviceName - Optional service name for identification
   * @param options - Connection options
   * @returns Promise resolving to Connection instance
   */
  static connect(apiKey: string, serviceName?: string, options?: ThreadifyConnectOptions): Promise<Connection>;
  
  /**
   * Create a Threadify instance with configuration
   * @param config - Configuration object
   * @returns Configured Threadify instance
   */
  static create(config: {
    apiKey: string;
    serviceName?: string;
    wsUrl?: string;
    graphqlUrl?: string;
    debug?: boolean;
  }): {
    connect(): Promise<Connection>;
  };
}

export class Notification {
  /** Thread ID */
  readonly threadId: string;
  /** Step name */
  readonly stepName: string;
  /** Notification status */
  readonly status: string;
  /** Notification message */
  readonly message?: string;
  /** Notification context */
  readonly context?: StepContext;
  /** Timestamp */
  readonly timestamp: string;
}
