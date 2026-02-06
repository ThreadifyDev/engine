// GraphQL client for Threadify Engine (via Web API proxy)
import { getConfig } from '../config.client';

const GRAPHQL_ENDPOINT = '/api/graphql'; // Proxy endpoint on Web API

export interface GraphQLError {
  message: string;
  path?: string[];
  extensions?: Record<string, any>;
}

export interface GraphQLResponse<T> {
  data?: T;
  errors?: GraphQLError[];
}

// GraphQL Types based on schema
export interface StepHistory {
  attempt: number;
  timestamp: string;
  status: string;
  context: string;
  duration: number;
  startedAt?: string;
  finishedAt?: string;
  error?: string;
  actor: string;
  actorService: string;
  companyId: string;
  companyName: string;
  hash?: string;
  prevHash?: string;
}

export interface HashChainStatus {
  verified: boolean;
  lastVerifiedAt: string;
  totalEvents: number;
  brokenAt?: string;
  error?: string;
}

export interface ContractGraph {
  graph: {
    nodes: Record<string, {
      id: string;
      owner: string;
      type: string;
      mode?: string;
      required: boolean;
      next: string[];
      timeout?: string;
      maxDuration?: string;
      businessContext?: any;
    }>;
    entryPoints: string[];
    terminalSteps: string[];
  };
  transitions: Array<{
    From: string;
    To: string[];
    CanRetry?: boolean;
    MaxRetries?: number;
  }>;
  parties: string[];
  validation?: {
    MaxDuration?: string;
    AllowMultipleTerminals?: boolean;
    MultipleTerminalsSeverity?: string;
  };
  notificationConfig?: {
    DefaultScope?: string;
    RoleDefaults?: any;
  };
}

export interface StepStateInfo {
  threadId: string;
  stepName: string;
  idempotencyKey: string;
  status: string;
  retryCount: number;
  firstSeenAt: string;
  lastUpdatedAt: string;
  latestStepID: string;
  previousStep?: string;
  actor?: string;
  actorService?: string;
  latestContext?: string;
  hash?: string;
  prevHash?: string;
  verified?: boolean;
  verificationError?: string;
  history?: StepHistory[];
}

export interface ValidationIssue {
  type: string;
  message: string;
  field?: string;
  expected?: string;
  actual?: string;
  rule?: string;
}

export interface ValidationResultInfo {
  validationId: string;
  threadId: string;
  stepId: string;
  stepName: string;
  idempotencyKey: string;
  timestamp: string;
  message?: string; // Notification message
  validations: ValidationIssue[];
  overallStatus: string;
  hasCriticalViolation: boolean;
  criticalCount: number;
  warningCount: number;
  minorCount: number;
  infoCount: number;
  totalValidations: number;
}

export interface ThreadNotification {
  notificationId: string;
  threadId: string;
  stepId: string;
  stepName: string;
  idempotencyKey?: string;
  source: string; // 'execution', 'validation', 'thread'
  notificationType: string; // 'execution.success', 'validation.violated', etc.
  stepStatus?: string; // 'success', 'failed', 'error'
  validationStatus?: string; // 'passed', 'violated', 'none'
  violationType?: string;
  severity?: string; // 'critical', 'warning', 'info', 'major', 'minor'
  message: string;
  details?: Record<string, any>;
  timestamp: string;
}

export interface NotificationSummary {
  totalNotifications: number;
  criticalCount: number;
  warningCount: number;
  majorCount: number;
  minorCount: number;
  infoCount: number;
  executionCount: number;
  validationCount: number;
  hasCritical: boolean;
  hasWarnings: boolean;
}

export interface ActorInfo {
  id: string;
  name: string;
  type: string; // "user" or "service_account"
  companyName?: string;
}

export interface Thread {
  id: string;
  contractId?: string;
  contractVersion?: number;
  contractName?: string;
  ownerId: string;
  companyId: string;
  status: string;
  createdBy?: string;
  lastHash?: string;
  refs?: Record<string, any>;
  startedAt?: string;
  completedAt?: string;
  error?: string;
  steps?: StepStateInfo[];
  validationResults?: ValidationResultInfo[];
  notificationSummary?: NotificationSummary;
  notifications?: ThreadNotification[];
}

class GraphQLClient {
  private getApiUrl(): string {
    return getConfig().apiUrl;
  }

  private async request<T>(query: string, variables?: Record<string, any>): Promise<T> {
    // Make GraphQL request through the Web API proxy
    const token = typeof window !== 'undefined' ? localStorage.getItem('auth_token') : null;
    // Use Web API URL from runtime configuration
    const apiUrl = typeof window !== 'undefined' 
      ? this.getApiUrl()
      : 'http://localhost:3001';
    
    const response = await fetch(`${apiUrl}${GRAPHQL_ENDPOINT}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ query, variables }),
    });

    // Handle token expiration (401 Unauthorized)
    if (response.status === 401) {
      if (typeof window !== 'undefined') {
        localStorage.removeItem('auth_token');
        localStorage.removeItem('user');
        window.location.href = '/auth/login';
      }
      throw new Error('Token expired. Please log in again.');
    }

    if (!response.ok) {
      throw new Error(`GraphQL request failed: ${response.statusText}`);
    }

    const result: GraphQLResponse<T> = await response.json();

    if (result.errors) {
      // Check if error is due to authentication
      const errorMessage = result.errors[0]?.message || 'GraphQL request failed';
      if (errorMessage.toLowerCase().includes('unauthorized') || errorMessage.toLowerCase().includes('invalid token')) {
        if (typeof window !== 'undefined') {
          localStorage.removeItem('auth_token');
          localStorage.removeItem('user');
          window.location.href = '/auth/login';
        }
        throw new Error('Token expired. Please log in again.');
      }
      throw new Error(errorMessage);
    }

    if (!result.data) {
      throw new Error('No data returned from GraphQL');
    }

    return result.data;
  }

  async getThread(threadId: string): Promise<Thread> {
    const query = `
      query GetThread($id: ID!) {
        thread(id: $id) {
          id
          contractId
          contractVersion
          contractName
          ownerId
          companyId
          status
          createdBy
          lastHash
          refs
          startedAt
          completedAt
          error
          steps {
            threadId
            stepName
            idempotencyKey
            status
            retryCount
            firstSeenAt
            lastUpdatedAt
            latestStepID
            previousStep
            actor
            actorService
            latestContext
            hash
            prevHash
          }
          validationResults {
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
          notificationSummary {
            totalNotifications
            criticalCount
            warningCount
            majorCount
            minorCount
            infoCount
            executionCount
            validationCount
            hasCritical
            hasWarnings
          }
        }
      }
    `;

    const data = await this.request<{ thread: Thread }>(query, { id: threadId });
    return data.thread;
  }

  async getThreadNotifications(
    threadId: string,
    options?: {
      stepId?: string;
      stepName?: string;
      source?: string;
      severity?: string[];
      limit?: number;
      offset?: number;
    }
  ): Promise<ThreadNotification[]> {
    const query = `
      query GetThreadNotifications(
        $threadId: ID!
        $options: ThreadNotificationQueryOptions
      ) {
        thread(id: $threadId) {
          notifications(options: $options) {
            notificationId
            threadId
            stepId
            stepName
            idempotencyKey
            source
            notificationType
            stepStatus
            validationStatus
            violationType
            severity
            message
            details
            timestamp
          }
        }
      }
    `;

    const data = await this.request<{ thread: { notifications: ThreadNotification[] } }>(query, {
      threadId,
      options: options || {},
    });
    return data.thread.notifications;
  }

  async getStepHistory(
    threadId: string,
    stepName: string,
    idempotencyKey: string,
    limit = 10
  ): Promise<StepHistory[]> {
    const query = `
      query GetStepHistory(
        $threadId: String!
        $stepName: String!
        $idempotencyKey: String!
        $limit: Int
      ) {
        stepHistory(
          threadId: $threadId
          stepName: $stepName
          idempotencyKey: $idempotencyKey
          limit: $limit
        ) {
          attempt
          timestamp
          status
          context
          duration
          startedAt
          finishedAt
          error
          actor
          actorService
          companyId
          companyName
          hash
          prevHash
        }
      }
    `;

    const data = await this.request<{ stepHistory: StepHistory[] }>(query, {
      threadId,
      stepName,
      idempotencyKey,
      limit,
    });

    return data.stepHistory;
  }

  async resolveActors(ids: string[]): Promise<ActorInfo[]> {
    const query = `
      query ResolveActors($ids: [String!]!) {
        resolveActors(ids: $ids) {
          id
          name
          type
          companyName
        }
      }
    `;

    const data = await this.request<{ resolveActors: ActorInfo[] }>(query, { ids });
    return data.resolveActors;
  }

  async getThreads(options?: {
    contractName?: string;
    status?: string;
    limit?: number;
    offset?: number;
    startedAfter?: string;
    startedBefore?: string;
  }): Promise<{ threads: Thread[]; totalCount: number }> {
    const query = `
      query GetThreads(
        $contractName: String
        $status: String
        $limit: Int
        $offset: Int
        $startedAfter: String
        $startedBefore: String
      ) {
        threads(
          contractName: $contractName
          status: $status
          limit: $limit
          offset: $offset
          startedAfter: $startedAfter
          startedBefore: $startedBefore
        ) {
          threads {
            id
            contractName
            contractVersion
            status
            refs
            startedAt
            completedAt
            error
          }
          totalCount
        }
      }
    `;

    const data = await this.request<{ threads: { threads: Thread[]; totalCount: number } }>(query, options);
    return { threads: data.threads.threads, totalCount: data.threads.totalCount };
  }

  async getThreadsByContract(options: {
    contractName: string;
    contractVersion?: number;
    status?: string;
    limit?: number;
    offset?: number;
    startedAfter?: string;
    startedBefore?: string;
  }): Promise<{ threads: Thread[]; totalCount: number }> {
    const query = `
      query GetThreadsByContract(
        $contractName: String!
        $contractVersion: Int
        $status: String
        $limit: Int
        $offset: Int
        $startedAfter: String
        $startedBefore: String
      ) {
        threadsByContract(
          contractName: $contractName
          contractVersion: $contractVersion
          status: $status
          limit: $limit
          offset: $offset
          startedAfter: $startedAfter
          startedBefore: $startedBefore
        ) {
          threads {
            id
            contractName
            contractVersion
            status
            refs
            startedAt
            completedAt
            error
          }
          totalCount
        }
      }
    `;

    const data = await this.request<{ threadsByContract: { threads: Thread[]; totalCount: number } }>(query, options);
    return { threads: data.threadsByContract.threads, totalCount: data.threadsByContract.totalCount };
  }

  async getThreadsByRef(options: {
    refKey: string;
    refValue: string;
    status?: string;
    limit?: number;
    offset?: number;
    startedAfter?: string;
    startedBefore?: string;
  }): Promise<{ threads: Thread[]; totalCount: number }> {
    const query = `
      query GetThreadsByRef(
        $refKey: String!
        $refValue: String!
        $status: String
        $limit: Int
        $offset: Int
        $startedAfter: String
        $startedBefore: String
      ) {
        threadsByRef(
          refKey: $refKey
          refValue: $refValue
          status: $status
          limit: $limit
          offset: $offset
          startedAfter: $startedAfter
          startedBefore: $startedBefore
        ) {
          threads {
            id
            contractName
            contractVersion
            status
            refs
            startedAt
            completedAt
            error
          }
          totalCount
        }
      }
    `;

    const data = await this.request<{ threadsByRef: { threads: Thread[]; totalCount: number } }>(query, options);
    return { threads: data.threadsByRef.threads, totalCount: data.threadsByRef.totalCount };
  }

  async verifyThreadIntegrity(threadId: string): Promise<HashChainStatus> {
    const query = `
      query VerifyThreadIntegrity($threadId: String!) {
        verifyThreadIntegrity(threadId: $threadId) {
          verified
          lastVerifiedAt
          totalEvents
          brokenAt
          error
        }
      }
    `;

    const data = await this.request<{ verifyThreadIntegrity: HashChainStatus }>(query, { threadId });
    return data.verifyThreadIntegrity;
  }

  async getContractGraph(name: string, version?: number): Promise<any> {
    const query = `
      query GetContractGraph($name: String!, $version: Int) {
        contractGraph(name: $name, version: $version) {
          graph {
            nodes {
              id
              owner
              type
              mode
              required
              next
              timeout
              maxDuration
              businessContext
            }
            entryPoints
            terminalSteps
          }
          transitions {
            From
            To
            CanRetry
            MaxRetries
          }
          parties
        }
      }
    `;

    const variables: { name: string; version?: number } = { name };
    if (version !== undefined) {
      variables.version = version;
    }

    const response = await this.request<{ contractGraph: any }>(query, variables);
    
    // Convert nodes array back to map for our component
    const contractGraph = response.contractGraph;
    if (contractGraph?.graph?.nodes && Array.isArray(contractGraph.graph.nodes)) {
      const nodesMap: Record<string, any> = {};
      contractGraph.graph.nodes.forEach((node: any) => {
        nodesMap[node.id] = node;
      });
      contractGraph.graph.nodes = nodesMap;
    }
    
    return contractGraph;
  }
}

export const graphqlClient = new GraphQLClient();
