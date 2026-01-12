/**
 * DataRetriever - GraphQL-based data retrieval for archived threads
 * Provides read-only access to historical thread data
 */

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
   * Get all steps for this thread
   * @param {Object} filters - Optional filters
   * @param {string} filters.stepName - Filter by step name
   * @param {string} filters.idempotencyKey - Filter by idempotency key
   * @returns {Promise<Array<ArchivedStep>>}
   */
  async steps(filters = {}) {
    const query = `
      query GetThreadSteps($threadId: ID!, $stepName: String, $idempotencyKey: String) {
        thread(id: $threadId) {
          steps(stepName: $stepName, idempotencyKey: $idempotencyKey) {
            threadId
            stepName
            idempotencyKey
            status
            retryCount
            firstSeenAt
            lastUpdatedAt
            latestStepID
            previousStep
          }
        }
      }
    `;

    const variables = {
      threadId: this.id,
      stepName: filters.stepName || null,
      idempotencyKey: filters.idempotencyKey || null
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
   * Get a specific step by name or name:idempotencyKey
   * @param {string} stepIdentifier - Step name or "stepName:idempKey"
   * @returns {Promise<ArchivedStep>}
   */
  async getStep(stepIdentifier) {
    const [stepName, idempotencyKey] = stepIdentifier.split(':');
    
    const steps = await this.steps({ 
      stepName, 
      idempotencyKey: idempotencyKey || null 
    });

    if (steps.length === 0) {
      throw new Error(`Step not found: ${stepIdentifier}`);
    }

    // If idempotencyKey provided, return exact match
    if (idempotencyKey) {
      return steps[0];
    }

    // If only stepName, return first (or could return all attempts)
    return steps[0];
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
   * @returns {Promise<Object>} Complete thread data with steps, history, and validations
   */
  async getCompleteData(options = {}) {
    const query = `
      query GetCompleteThread(
        $id: ID!
        $stepName: String
        $idempotencyKey: String
        $stepHistoryLimit: Int
        $validationLimit: Int
      ) {
        thread(id: $id) {
          id
          contractId
          contractVersion
          contractName
          ownerId
          companyId
          status
          lastHash
          refs
          startedAt
          completedAt
          error
          steps(stepName: $stepName, idempotencyKey: $idempotencyKey) {
            threadId
            stepName
            idempotencyKey
            status
            retryCount
            firstSeenAt
            lastUpdatedAt
            latestStepID
            previousStep
            history(limit: $stepHistoryLimit) {
              attempt
              timestamp
              status
              context
              duration
              error
            }
          }
          validationResults(options: {limit: $validationLimit}) {
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
          }
        }
      }
    `;

    const variables = {
      id: this.id,
      stepName: options.stepName || null,
      idempotencyKey: options.idempotencyKey || null,
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
    this.graphqlClient = graphqlClient;
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
          attempt
          timestamp
          status
          context
          duration
          error
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
          id
          contractId
          contractName
          contractVersion
          ownerId
          companyId
          status
          startedAt
          completedAt
          error
          refs
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
          id
          contractId
          contractName
          contractVersion
          ownerId
          companyId
          status
          startedAt
          completedAt
          error
          refs
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
   * @returns {Promise<Array<ArchivedThread>>} - All matching threads
   */
  async getThreadsByRef({ refKey, refValue }) {
    const query = `
      query GetThreadsByRef($refKey: String!, $refValue: String!) {
        threadsByRef(refKey: $refKey, refValue: $refValue) {
          id
          contractId
          contractName
          contractVersion
          ownerId
          companyId
          status
          startedAt
          completedAt
          error
          refs
        }
      }
    `;

    const data = await this.graphqlClient.query(query, { refKey, refValue });
    
    if (!data.threadsByRef) {
      return [];
    }

    return data.threadsByRef.map(threadData => 
      new ArchivedThread(threadData, this.graphqlClient)
    );
  }
}
