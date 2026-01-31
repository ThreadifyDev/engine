// GraphQL client for Threadify Engine (via Web API proxy)

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
  error?: string;
  actor: string;
  actorService: string;
  companyId: string;
  companyName: string;
  hash?: string;
  prevHash?: string;
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
  validations: ValidationIssue[];
  overallStatus: string;
  hasCriticalViolation: boolean;
  criticalCount: number;
  warningCount: number;
  minorCount: number;
  infoCount: number;
  totalValidations: number;
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
}

class GraphQLClient {
  private async request<T>(query: string, variables?: Record<string, any>): Promise<T> {
    // Make GraphQL request through the Web API proxy
    const token = typeof window !== 'undefined' ? localStorage.getItem('auth_token') : null;
    // Use Web API URL (port 3001, configurable via VITE_API_URL)
    const apiUrl = typeof window !== 'undefined' 
      ? (import.meta.env.VITE_API_URL || 'http://localhost:3001')
      : 'http://localhost:3001';
    
    const response = await fetch(`${apiUrl}${GRAPHQL_ENDPOINT}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ query, variables }),
    });

    if (!response.ok) {
      throw new Error(`GraphQL request failed: ${response.statusText}`);
    }

    const result: GraphQLResponse<T> = await response.json();

    if (result.errors) {
      throw new Error(result.errors[0]?.message || 'GraphQL request failed');
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
        }
      }
    `;

    const data = await this.request<{ thread: Thread }>(query, { id: threadId });
    return data.thread;
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
  }): Promise<Thread[]> {
    const query = `
      query GetThreads(
        $contractName: String
        $status: String
        $limit: Int
        $offset: Int
      ) {
        threads(
          contractName: $contractName
          status: $status
          limit: $limit
          offset: $offset
        ) {
          id
          contractName
          contractVersion
          status
          startedAt
          completedAt
          error
        }
      }
    `;

    const data = await this.request<{ threads: Thread[] }>(query, options);
    return data.threads;
  }
}

export const graphqlClient = new GraphQLClient();
