/**
 * DataRetriever - GraphQL-based data retrieval for archived threads
 * Provides read-only access to historical thread data
 */

// GraphQL Fragments for reusability
const THREAD_FIELDS = `
  id
  contractId
  contractName
  contractVersion
  ownerId
  companyId
  status
  lastHash
  startedAt
  completedAt
  error
  refs
`;

const STEP_FIELDS = `
  threadId
  stepName
  idempotencyKey
  status
  retryCount
  firstSeenAt
  lastUpdatedAt
  latestStepID
  previousStep
  verified
  verificationError
`;

const STEP_HISTORY_FIELDS = `
  attempt
  timestamp
  status
  context
  duration
  error
`;

const SUB_STEP_FIELDS = `
  id
  threadId
  stepId
  name
  status
  payload
  recordedAt
`;

const VALIDATION_RESULT_FIELDS = `
  validationId
  threadId
  stepId
  stepName
  idempotencyKey
  timestamp
  validations {
    type
    message
    field
    expected
    actual
    rule
  }
  overallStatus
  hasCriticalViolation
  criticalCount
  warningCount
  minorCount
  infoCount
  totalValidations
`;

/**
 * ArchivedThread - Represents a historical thread with read-only access
 */
export class ArchivedThread {
  constructor(threadData, graphqlClient) {
    this.id = threadData.id;
    this.contractId = threadData.contractId;
    this.contractName = threadData.contractName;
    this.contractVersion = threadData.contractVersion;
    this.ownerId = threadData.ownerId;
    this.companyId = threadData.companyId;
    this.status = threadData.status;
    this.startedAt = threadData.startedAt;
    this.completedAt = threadData.completedAt;
    this.error = threadData.error;
    this.refs = threadData.refs ? JSON.parse(threadData.refs) : null;
    this.graphqlClient = graphqlClient;
  }

  /**
   * Get thread ID (alias for id property for API consistency)
   */
  get threadId() {
    return this.id;
  }

  /**
   * Get creation timestamp (alias for startedAt)
   */
  get createdAt() {
    return this.startedAt;
  }

  /**
   * Get all steps for this thread
   * @param {string} stepIdentifier - Optional filter: "stepName" or "stepName:idempotencyKey"
   * @param {Object} options - Optional query options
   * @param {string} options.status - Filter by status (success, failed, error)
   * @returns {Promise<Array<ArchivedStep>>}
   */
  async steps(stepIdentifier = null, options = {}) {
    let stepName = null;
    let idempotencyKey = null;

    // Parse stepIdentifier if provided
    if (stepIdentifier) {
      [stepName, idempotencyKey] = stepIdentifier.split(':');
    }

    const status = options.status || null;

    const query = `
      query GetThreadSteps($threadId: ID!, $stepName: String, $idempotencyKey: String, $status: String) {
        thread(id: $threadId) {
          steps(stepName: $stepName, idempotencyKey: $idempotencyKey, status: $status) {
            ${STEP_FIELDS}
            history(limit: 1) {
              ${STEP_HISTORY_FIELDS}
            }
          }
        }
      }
    `;

    const variables = {
      threadId: this.id,
      stepName,
      idempotencyKey,
      status
    };

    const data = await this.graphqlClient.query(query, variables);
    
    if (!data.thread || !data.thread.steps) {
      return [];
    }

    return data.thread.steps.map(stepData => 
      new ArchivedStep(stepData, this.graphqlClient)
    );
  }


  /**
   * Get validation results for this thread
   * @param {Object} options - Query options
   * @returns {Promise<Array<ValidationResult>>}
   */
  async validationResults(options = {}) {
    const query = `
      query GetThreadValidations($threadId: ID!, $options: ValidationQueryOptions) {
        thread(id: $threadId) {
          validationResults(options: $options) {
            ${VALIDATION_RESULT_FIELDS}
          }
        }
      }
    `;

    const variables = {
      threadId: this.id,
      options: options || null
    };

    const data = await this.graphqlClient.query(query, variables);
    
    if (!data.thread || !data.thread.validationResults) {
      return [];
    }

    return data.thread.validationResults;
  }

  /**
   * Get complete thread picture with all nested data in a single GraphQL query
   * This is more efficient than making multiple separate queries
   * @param {Object} options - Query options
   * @param {number} options.stepHistoryLimit - Limit for step history records per step (default: 50)
   * @param {number} options.validationLimit - Limit for validation results (default: 10)
   * @param {string} options.stepName - Filter steps by name (optional)
   * @param {string} options.idempotencyKey - Filter steps by idempotency key (optional)
   * @param {string} options.status - Filter steps by status (optional)
   * @returns {Promise<Object>} Complete thread data with steps, history, and validations
   */
  async getCompleteData(options = {}) {
    const query = `
      query GetCompleteThread(
        $id: ID!
        $stepName: String
        $idempotencyKey: String
        $status: String
        $stepHistoryLimit: Int
        $validationLimit: Int
      ) {
        thread(id: $id) {
          ${THREAD_FIELDS}
          steps(stepName: $stepName, idempotencyKey: $idempotencyKey, status: $status) {
            ${STEP_FIELDS}
            history(limit: $stepHistoryLimit) {
              ${STEP_HISTORY_FIELDS}
            }
          }
          validationResults(options: {limit: $validationLimit}) {
            ${VALIDATION_RESULT_FIELDS}
          }
        }
      }
    `;

    const variables = {
      id: this.id,
      stepName: options.stepName || null,
      idempotencyKey: options.idempotencyKey || null,
      status: options.status || null,
      stepHistoryLimit: options.stepHistoryLimit || 50,
      validationLimit: options.validationLimit || 10
    };

    const data = await this.graphqlClient.query(query, variables);
    
    if (!data.thread) {
      throw new Error(`Thread not found: ${this.id}`);
    }

    return data.thread;
  }
}

/**
 * ArchivedStep - Represents a historical step with read-only access
 */
export class ArchivedStep {
  constructor(stepData, graphqlClient) {
    this.threadId = stepData.threadId;
    this.stepName = stepData.stepName;
    this.idempotencyKey = stepData.idempotencyKey;
    this.status = stepData.status;
    this.retryCount = stepData.retryCount;
    this.firstSeenAt = stepData.firstSeenAt;
    this.lastUpdatedAt = stepData.lastUpdatedAt;
    this.latestStepID = stepData.latestStepID;
    this.previousStep = stepData.previousStep;
    this.verified = stepData.verified;
    this.verificationError = stepData.verificationError;
    this.lastExecution = stepData.history && stepData.history.length > 0 ? stepData.history[0] : null;
    this.graphqlClient = graphqlClient;
  }

  /**
   * Get sub-steps for this step
   * @returns {Promise<Array<SubStep>>}
   */
  async subSteps() {
    const query = `
      query GetStepSubSteps(
        $threadId: String!
        $stepName: String!
        $idempotencyKey: String
      ) {
        thread(id: $threadId) {
          steps(stepName: $stepName, idempotencyKey: $idempotencyKey) {
            subSteps {
              ${SUB_STEP_FIELDS}
            }
          }
        }
      }
    `;

    const variables = {
      threadId: this.threadId,
      stepName: this.stepName,
      idempotencyKey: this.idempotencyKey || null
    };

    const data = await this.graphqlClient.query(query, variables);
    
    if (!data.thread || !data.thread.steps || data.thread.steps.length === 0) {
      return [];
    }

    return data.thread.steps[0].subSteps || [];
  }

  /**
   * Get history for this step
   * @param {Object} options - History query options
   * @param {number} options.limit - Maximum number of records (default: 100)
   * @param {number} options.offset - Offset for pagination (default: 0)
   * @param {string} options.startAt - ISO timestamp to filter from
   * @param {string} options.endAt - ISO timestamp to filter to
   * @param {string} options.activityType - Filter by activity type
   * @param {string} options.actor - Filter by actor
   * @returns {Promise<Array<StepHistory>>}
   */
  async history(options = {}) {
    const query = `
      query GetStepHistory(
        $threadId: String!
        $stepName: String!
        $idempotencyKey: String
        $limit: Int
        $offset: Int
        $startAt: String
        $endAt: String
        $activityType: String
        $actor: String
      ) {
        stepHistory(
          threadId: $threadId
          stepName: $stepName
          idempotencyKey: $idempotencyKey
          limit: $limit
          offset: $offset
          startAt: $startAt
          endAt: $endAt
          activityType: $activityType
          actor: $actor
        ) {
          ${STEP_HISTORY_FIELDS}
        }
      }
    `;

    const variables = {
      threadId: this.threadId,
      stepName: this.stepName,
      idempotencyKey: this.idempotencyKey || null,
      limit: options.limit || 100,
      offset: options.offset || 0,
      startAt: options.startAt || null,
      endAt: options.endAt || null,
      activityType: options.activityType || null,
      actor: options.actor || null
    };

    const data = await this.graphqlClient.query(query, variables);
    
    if (!data.stepHistory) {
      return [];
    }

    return data.stepHistory;
  }
}

/**
 * GraphQLClient - Simple GraphQL client with authentication
 */
export class GraphQLClient {
  constructor(graphqlUrl, apiKey) {
    this.graphqlUrl = graphqlUrl;
    this.apiKey = apiKey;
  }

  /**
   * Execute a GraphQL query
   * @param {string} query - GraphQL query string
   * @param {Object} variables - Query variables
   * @returns {Promise<Object>} - Query result data
   */
  async query(query, variables = {}) {
    try {
      const response = await fetch(this.graphqlUrl, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-API-Key': this.apiKey
        },
        body: JSON.stringify({
          query,
          variables
        })
      });

      if (!response.ok) {
        throw new Error(`GraphQL request failed: ${response.status} ${response.statusText}`);
      }

      const result = await response.json();

      if (result.errors) {
        throw new Error(`GraphQL errors: ${JSON.stringify(result.errors)}`);
      }

      return result.data;
    } catch (error) {
      console.error('[GraphQL] Query failed:', error);
      throw error;
    }
  }
}

/**
 * DataRetriever - Main entry point for archived data access
 */
export class DataRetriever {
  constructor(graphqlUrl, apiKey) {
    this.graphqlClient = new GraphQLClient(graphqlUrl, apiKey);
  }

  /**
   * Get a thread by ID
   * @param {string} threadId - Thread ID
   * @returns {Promise<ArchivedThread>}
   */
  async getThread(threadId) {
    const query = `
      query GetThread($id: ID!) {
        thread(id: $id) {
          ${THREAD_FIELDS}
        }
      }
    `;

    const data = await this.graphqlClient.query(query, { id: threadId });
    
    if (!data.thread) {
      throw new Error(`Thread not found: ${threadId}`);
    }

    return new ArchivedThread(data.thread, this.graphqlClient);
  }

  /**
   * Get thread(s) by reference key-value pair
   * @param {Object} ref - Reference object
   * @param {string} ref.refKey - Reference key (e.g., "orderId")
   * @param {string} ref.refValue - Reference value (e.g., "ORD-12345")
   * @returns {Promise<ArchivedThread|null>} - First matching thread or null
   */
  async getThreadByRef({ refKey, refValue }) {
    const query = `
      query GetThreadByRef($refKey: String!, $refValue: String!) {
        threadsByRef(refKey: $refKey, refValue: $refValue) {
          ${THREAD_FIELDS}
        }
      }
    `;

    const data = await this.graphqlClient.query(query, { refKey, refValue });
    
    if (!data.threadsByRef || data.threadsByRef.length === 0) {
      return null;
    }

    // Return first match (could be extended to return all matches)
    return new ArchivedThread(data.threadsByRef[0], this.graphqlClient);
  }

  /**
   * Get multiple threads by reference
   * @param {Object} ref - Reference object
   * @param {string} ref.refKey - Reference key
   * @param {string} ref.refValue - Reference value
   * @param {string} ref.status - Optional status filter (e.g., "active", "completed")
   * @param {string} ref.startedAfter - Optional ISO timestamp filter
   * @param {string} ref.startedBefore - Optional ISO timestamp filter
   * @param {number} ref.limit - Optional limit (default: 50)
   * @param {number} ref.offset - Optional offset for pagination (default: 0)
   * @returns {Promise<Array<ArchivedThread>>} - All matching threads
   */
  async getThreadsByRef({ refKey, refValue, status, startedAfter, startedBefore, limit, offset }) {
    const query = `
      query GetThreadsByRef(
        $refKey: String!
        $refValue: String!
        $status: String
        $startedAfter: String
        $startedBefore: String
        $limit: Int
        $offset: Int
      ) {
        threadsByRef(
          refKey: $refKey
          refValue: $refValue
          status: $status
          startedAfter: $startedAfter
          startedBefore: $startedBefore
          limit: $limit
          offset: $offset
        ) {
          ${THREAD_FIELDS}
        }
      }
    `;

    const variables = {
      refKey,
      refValue,
      status: status || null,
      startedAfter: startedAfter || null,
      startedBefore: startedBefore || null,
      limit: limit || 50,
      offset: offset || 0
    };

    const data = await this.graphqlClient.query(query, variables);
    
    if (!data.threadsByRef) {
      return [];
    }

    return data.threadsByRef.map(threadData => 
      new ArchivedThread(threadData, this.graphqlClient)
    );
  }

  /**
   * Get thread chain starting from any thread
   * @param {string} startThreadId - Starting thread ID (can be any thread in the chain)
   * @param {number} maxDepth - Maximum depth to traverse downward (default: 3)
   * @returns {Promise<Array<ArchivedThread>>} - Thread chain from starting thread to descendants
   */
  async getThreadChain(startThreadId, maxDepth = 3) {
    const query = `
      query GetThreadChain($rootId: ID!, $maxDepth: Int) {
        threadChain(rootId: $rootId, maxDepth: $maxDepth) {
          ${THREAD_FIELDS}
        }
      }
    `;

    const data = await this.graphqlClient.query(query, { rootId: startThreadId, maxDepth });
    
    if (!data.threadChain) {
      return [];
    }

    return data.threadChain.map(threadData => 
      new ArchivedThread(threadData, this.graphqlClient)
    );
  }
}
