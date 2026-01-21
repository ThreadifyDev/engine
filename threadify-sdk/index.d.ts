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
   * Mark step as successful
   * @param message - Optional success message
   * @param finalContext - Optional final context data
   * @returns Promise resolving to step result (without internal details)
   */
  success(message?: string, finalContext?: StepContext): Promise<StepResult>;
  
  /**
   * Mark step as failed
   * @param message - Failure message
   * @param finalContext - Optional final context data
   * @returns Promise resolving to step result (without internal details)
   */
  failed(message: string, finalContext?: StepContext): Promise<StepResult>;
  
  /**
   * Skip this step
   * @param message - Skip reason
   * @param finalContext - Optional final context data
   * @returns Promise resolving to step result (without internal details)
   */
  skip(message: string, finalContext?: StepContext): Promise<StepResult>;
}

export class ThreadInstance {
  /** Thread ID */
  readonly threadId: string;
  /** Contract ID */
  readonly contractId: string;
  
  /**
   * Create a new step in this thread
   * @param stepName - Name of the step
   * @param options - Thread options
   * @returns New ThreadStep instance
   */
  step(stepName: string, options?: ThreadOptions): ThreadStep;
  
  /**
   * Get thread metadata
   * @returns Promise resolving to thread metadata
   */
  getMetadata(): Promise<any>;
  
  /**
   * Complete the thread
   * @param message - Optional completion message
   * @returns Promise resolving when thread is completed
   */
  complete(message?: string): Promise<void>;
  
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
   * Subscribe to violation notifications for a specific step
   * @param stepIdentifier - Step name or "contractName@stepName"
   * @param handler - Notification handler function
   */
  onViolation(stepIdentifier: string, handler: (notification: any) => void): void;
  
  /**
   * Subscribe to completion notifications for a specific step
   * @param stepIdentifier - Step name or "contractName@stepName"
   * @param handler - Notification handler function
   */
  onCompleted(stepIdentifier: string, handler: (notification: any) => void): void;
  
  /**
   * Subscribe to failure notifications for a specific step
   * @param stepIdentifier - Step name or "contractName@stepName"
   * @param handler - Notification handler function
   */
  onFailed(stepIdentifier: string, handler: (notification: any) => void): void;
  
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
