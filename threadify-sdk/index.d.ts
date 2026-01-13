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

export type StepStatus = 'success' | 'failed' | 'error' | 'skipped';

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

export interface ArchivedThread {
  id: string;
  contractId: string;
  status: string;
  createdAt: string;
  updatedAt: string;
  context: StepContext;
  refs: ThreadRefs;
}

export interface ArchivedStep {
  id: string;
  threadId: string;
  stepName: string;
  status: string;
  startedAt: string;
  finishedAt?: string;
  context: StepContext;
  refs: ThreadRefs;
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
   * Get archived step by thread ID and step name
   * @param threadId - Thread ID
   * @param stepName - Step name
   * @returns Promise resolving to archived step
   */
  getStep(threadId: string, stepName: string): Promise<ArchivedStep>;
  
  /**
   * Get step history
   * @param threadId - Thread ID
   * @param stepName - Step name
   * @param options - Query options
   * @returns Promise resolving to step history
   */
  getStepHistory(threadId: string, stepName: string, options?: { limit?: number }): Promise<ArchivedStep[]>;
  
  /**
   * Get validation results
   * @param threadId - Thread ID
   * @param options - Query options
   * @returns Promise resolving to validation results
   */
  getValidationResults(threadId: string, options?: { limit?: number }): Promise<any[]>;
  
  /**
   * Add notification handlers
   * @param handlers - Notification handlers
   */
  addNotificationHandlers(handlers: NotificationHandlers): void;
  
  /**
   * Remove notification handlers
   * @param handlers - Notification handlers to remove
   */
  removeNotificationHandlers(handlers: NotificationHandlers): void;
  
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
