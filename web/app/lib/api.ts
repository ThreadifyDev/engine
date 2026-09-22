// API client for backend communication

import { getConfig } from '../config.client';

import yaml from 'js-yaml';
import { browserHeaders, csrfToken, purgeLegacyToken } from './browser-session';

// Patterns that indicate internal error details which should not reach users.
const INTERNAL_ERROR_PATTERNS = [
  /SQLSTATE\s+\d+/i,
  /violates\s+foreign\s+key/i,
  /syntax\s+error/i,
  /connection\s+refused/i,
  /at\s+\S+\.go:\d+/i,
  /goroutine\s+\d+/i,
  /internal\/\S+/i,
  /\/threadify-go\/\S+/i,
  /localhost:\d+/i,
  /https?:\/\/\S+/i,
  /tcp:\/\/\S+/i,
];

function sanitizeErrorMessage(raw: string): string {
  if (typeof raw !== 'string') return 'An error occurred';
  for (const pattern of INTERNAL_ERROR_PATTERNS) {
    if (pattern.test(raw)) {
      return 'An internal error occurred. Please try again or contact support.';
    }
  }
  return raw;
}

export class ValidationError extends Error {
  details?: Array<{ field: string; message: string }>;

  constructor(message: string, details?: Array<{ field: string; message: string }>) {
    super(message);
    this.name = 'ValidationError';
    this.details = details;
  }
}

export interface SignupData {
  company_name?: string;
  principal_type?: "user" | "service_account";
  email: string;
  password: string;
  full_name: string;
  job_role: string;
  industry?: string;
  company_size?: string;
  use_case?: string;
  invitation_token?: string;
  middle_name?: string;
}

export interface LoginData {
  email: string;
  password: string;
}

export interface VerifyOTPData {
  email: string;
  token: string;
}

export interface ForgotPasswordData {
  email: string;
}

export interface ResetPasswordData {
  token: string;
  password: string;
}

export interface User {
  id: string;
  company_id: string;
  company_name?: string;
  email: string;
  full_name?: string;
  job_role?: string;
  email_verified: boolean;
  onboarding_completed: boolean;
  first_instrumentation_done: boolean;
  created_at: string;
  updated_at: string;
  last_login_at?: string;
}

export interface AuthResponse {
  token: string;
  user: User;
  message?: string;
}

export interface LoginResponse {
  user?: User;
  otp_required?: boolean;
  email_verification_required?: boolean;
  message?: string;
}

export interface ApiError {
  error: string;
}

export interface CreditAccountDTO {
  id: string;
  company_id: string;
  billing_cycle_start: string;
  balance_millicents: number;
  min_balance_millicents: number;
  max_monthly_charge_millicents: number;
  auto_topup_millicents: number;
  monthly_charged_millicents: number;
  created_at: string;
  updated_at: string;
}

export interface PlanDTO {
  subscription_tier: string;
  status: string;
  billing_cycle: string;
  billing_end: string;
}

export interface UsageMeterDTO {
  bandwidth_ingress_balance: number;
  max_bandwidth_ingress: number;
  bandwidth_egress_balance: number;
  max_bandwidth_egress: number;
  max_team_seats: number;
  max_contract_limit: number;
  max_rate_limit: number;
  max_payload_bytes: number;
  hot_storage_days: number;
  cold_storage_days: number;
  support: string;
}

export interface GetCurrentPlanResponse {
  billing_source?: 'registry';
  account_id?: string;
  entitlements?: {
    revision: string;
    input_bandwidth_bytes: number;
    output_bandwidth_bytes: number;
    input_requests_per_second: number;
    entity_profile_limit: number;
  };
  plan: PlanDTO | null;
  usage_meter: UsageMeterDTO | null;
  credit_account: CreditAccountDTO | null;
}

class ApiClient {
  // Every dashboard request goes to the connected Engine.
  private getUrl(endpoint: string): string {
    return `${getConfig().engineUrl.replace(/\/+$/, '')}/v1${endpoint}`;
  }

  private async request<T = any>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = this.getUrl(endpoint);
    purgeLegacyToken();

    const headers: Record<string, string> = {
      ...browserHeaders(),
      ...(options.headers as Record<string, string>),
    };

    // Only set default Content-Type if not already specified
    if (!headers['Content-Type']) {
      headers['Content-Type'] = 'application/json';
    }


    const response = await fetch(url, {
      ...options,
      credentials: 'include',
      headers,
    });

    // Check if response is JSON
    const contentType = response.headers.get('content-type');
    const isJson = contentType && contentType.includes('application/json');

    let data;
    try {
      if (isJson) {
        data = await response.json();
      } else {
        const text = await response.text();
        // Try to parse as JSON anyway (some servers don't set content-type correctly)
        try {
          data = JSON.parse(text);
        } catch {
          // If not JSON, wrap the text in an error object
          data = { error: text || 'An error occurred' };
        }
      }
    } catch (error) {
      throw new Error('Failed to parse server response');
    }

    if (!response.ok) {
      // Prefer 'message' field for user-friendly errors, fallback to 'error' field
      let errorMessage = sanitizeErrorMessage(data.message || data.error || 'An error occurred');

      // Cleanup internal billing error prefixes
      if (typeof errorMessage === 'string' && errorMessage.startsWith('payment required: insufficient credits: {')) {
        const jsonPart = errorMessage.split('payment required: insufficient credits: ')[1];
        try {
          const parsed = JSON.parse(jsonPart);
          if (parsed.message) errorMessage = parsed.message;
        } catch (e) {
          // Keep original if parsing fails
        }
      }

      // Handle invalid token by logging out (only for authenticated requests)
      // Don't redirect on login failures (which also return 401)
      const hasAuthHeader = !endpoint.startsWith('/auth/');
      if ((errorMessage === 'Invalid token' || response.status === 401) && hasAuthHeader) {
        if (typeof window !== 'undefined') {
          localStorage.removeItem('auth_token');
          localStorage.removeItem('user');
          window.location.href = '/login';
        }
      }

      // If we have validation details, throw ValidationError
      const errorDetails = data.details || data.errors;
      if (errorDetails && Array.isArray(errorDetails)) {
        throw new ValidationError(errorMessage, errorDetails);
      }

      throw new Error(errorMessage);
    }

    return data;
  }

  async signup(data: SignupData): Promise<{ message: string }> {
    return this.request('/auth/signup', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async login(data: LoginData): Promise<LoginResponse> {
    return this.request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async verifyOTP(data: VerifyOTPData): Promise<AuthResponse> {
    const response = await this.request<AuthResponse>('/auth/verify-otp', {
      method: 'POST',
      body: JSON.stringify(data),
    });


    return response;
  }

  async forgotPassword(data: ForgotPasswordData): Promise<{ message: string }> {
    return this.request('/auth/forgot-password', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async resetPassword(data: ResetPasswordData): Promise<{ message: string }> {
    return this.request('/auth/reset-password', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async resendVerificationEmail(data: { email: string }): Promise<{ message: string }> {
    return this.request('/auth/resend-verification', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  private async browserAuth<T>(path: string, options: RequestInit = {}): Promise<T> {
    purgeLegacyToken();
    const response = await fetch(`${getConfig().engineUrl.replace(/\/+$/, '')}/auth/${path}`, {
      ...options, credentials: 'include',
      headers: { 'Content-Type': 'application/json', ...browserHeaders(), ...options.headers },
    });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || 'Sign-in failed');
    return body;
  }

  async session(): Promise<{ authenticated: boolean; user?: User }> {
    const session = await this.browserAuth<{ authenticated: boolean; user?: User }>('session');
    if (session.user) this.setUser(session.user);
    else if (typeof window !== 'undefined') localStorage.removeItem('user');
    return session;
  }

  getIngestionRules(): Promise<IngestionRules> {
    return this.request('/engine/ingestion-rules');
  }

  saveIngestionRules(filters: string[], revision: string): Promise<IngestionRules> {
    return this.request('/engine/ingestion-rules', { method: 'PUT', body: JSON.stringify({ filters, revision }) });
  }

  previewIngestionRules(filters: string[], span_names: string[]): Promise<IngestionPreview> {
    return this.request('/engine/ingestion-rules/preview', { method: 'POST', body: JSON.stringify({ filters, span_names }) });
  }

  getEngineSettings(): Promise<EngineSettings> {
    return this.request('/engine/settings');
  }

  saveEnginePublicURL(public_url: string): Promise<EngineSettings> {
    return this.request('/engine/settings', { method: 'PUT', body: JSON.stringify({ public_url }) });
  }

  resetEnginePublicURL(): Promise<EngineSettings> {
    return this.request('/engine/settings', { method: 'DELETE' });
  }

  approveCLILogin(transaction_id: string, browser_token: string): Promise<{ status: string }> {
    return this.browserAuth('cli/approve', { method: 'POST', body: JSON.stringify({ transaction_id, browser_token }) });
  }

  async exchangeAPIKey(apiKey: string): Promise<void> {
    await this.browserAuth('api-key/exchange', { method: 'POST', body: JSON.stringify({ api_key: apiKey }) });
    await this.session();
  }

  startManagedLogin(): Promise<{ transaction_id: string; poll_token: string; verification_url: string; expires_at: string }> {
    return this.browserAuth('managed/start', { method: 'POST', body: '{}' });
  }

  pollManagedLogin(transaction_id: string, poll_token: string, signal: AbortSignal): Promise<{ status: string }> {
    return this.browserAuth('managed/poll', { method: 'POST', body: JSON.stringify({ transaction_id, poll_token }), signal });
  }

  async logout(): Promise<void> {
    const result = await this.browserAuth<{ logout_url?: string }>('logout', { method: 'POST', body: '{}' });
    localStorage.removeItem('user');
    purgeLegacyToken();
    window.location.assign(result.logout_url || '/login');
  }

  getStoredUser(): User | null {
    if (typeof window === 'undefined') return null;
    const userStr = localStorage.getItem('user');
    return userStr ? JSON.parse(userStr) : null;
  }

  isAuthenticated(): boolean {
    // This only controls rendering; the Engine verifies session authority on every request.
    return !!csrfToken();
  }

  setUser(user: User) {
    if (typeof window !== 'undefined') {
      localStorage.setItem('user', JSON.stringify(user));
    }
  }

  async updateProfile(data: {
    full_name: string;
    job_role: string;
    industry: string;
    company_size: string;
    use_case: string;
  }): Promise<{ message: string; user: User }> {
    return this.request('/user/profile', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async markInstrumentationDone(): Promise<{ message: string; user: User }> {
    return this.request('/user/mark-instrumentation-done', {
      method: 'POST',
    });
  }

  async getUserProfile(minimal?: boolean): Promise<{ user: User; company: any }> {
    const params = minimal ? '?minimal=true' : '';
    return this.request(`/user/profile${params}`, {
      method: 'GET',
    });
  }

  async listEngineUsers(): Promise<{ users: EngineUser[]; can_manage: boolean; login_url: string }> {
    return this.request('/users');
  }

  async inviteEngineUser(data: { email: string; full_name: string; role: string }): Promise<{ user: EngineUser; created: boolean; login_url: string }> {
    return this.request('/users', { method: 'POST', body: JSON.stringify(data) });
  }

  async updateEngineUser(id: string, data: { full_name?: string; role?: string; status?: EngineUser['status'] }): Promise<{ user: EngineUser }> {
    return this.request(`/users/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(data) });
  }

  async getAllContracts(params?: { search?: string; limit?: number; offset?: number }): Promise<any> {
    const query = new URLSearchParams();
    if (params?.search) query.append('search', params.search);
    if (params?.limit) query.append('limit', params.limit.toString());
    if (params?.offset) query.append('offset', params.offset.toString());

    const queryString = query.toString();
    const endpoint = `/contracts${queryString ? `?${queryString}` : ''}`;
    return this.request(endpoint);
  }

  async createContract(data: { name: string; yaml: string }): Promise<any> {
    return this.request('/contracts', {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: data.yaml,
    });
  }

  async previewContract(data: { yaml: string }): Promise<any> {
    return this.request('/contracts/preview', {
      method: 'POST',
      headers: {
        'Content-Type': 'text/plain',
      },
      body: data.yaml,
    });
  }

  async getContract(id: string): Promise<any> {
    return this.request(`/contracts/${id}`);
  }

  async updateContract(id: string, data: { yaml: string }): Promise<any> {
    return this.request(`/contracts/${id}`, {
      method: 'PUT',
      body: data.yaml,
      headers: {
        'Content-Type': 'text/plain',
      },
    });
  }

  async deleteContract(id: string): Promise<any> {
    return this.request(`/contracts/${id}`, {
      method: 'DELETE',
    });
  }

  async getContractVersions(id: string): Promise<any> {
    return this.request(`/contracts/${id}/versions`);
  }

  async getContractVersion(id: string, version: string): Promise<any> {
    return this.request(`/contracts/${id}/versions/${version}`);
  }

  async deleteContractVersion(id: string, version: string): Promise<any> {
    return this.request(`/contracts/${id}/versions/${version}`, {
      method: 'DELETE',
    });
  }

  // Service Account Management
  async createServiceAccount(data: { name: string; description?: string; role: string }): Promise<any> {
    return this.request('/service-accounts', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async listServiceAccounts(): Promise<any> {
    return this.request('/service-accounts');
  }

  async getServiceAccount(id: string): Promise<any> {
    return this.request(`/service-accounts/${id}`);
  }

  async updateServiceAccount(id: string, data: { name?: string; description?: string; is_active?: boolean }): Promise<any> {
    const {service_account: current} = await this.getServiceAccount(id);
    const updated = {name: current.name, description: current.description || '', is_active: current.is_active, ...data};

    return this.request(`/service-accounts/${id}`, {
      method: 'PUT',
      body: JSON.stringify(updated),
    });
  }

  async deleteServiceAccount(id: string): Promise<any> {
    return this.request(`/service-accounts/${id}`, {
      method: 'DELETE',
    });
  }

  // Deprecated: Use role-based permissions from roles.json instead
  async getRolePermissions(role: string): Promise<any> {
    console.warn('getRolePermissions is deprecated - permissions are now managed via roles.json');
    return this.request(`/service-accounts/scopes/${role}/permissions`);
  }

  // Fetch all roles from backend
  async getRoles(): Promise<any> {
    return this.request('/roles');
  }

  // Fetch roles by level (app_level or runtime_level)
  async getRolesByLevel(level: string): Promise<any> {
    return this.request(`/roles/${level}`);
  }

  private async post<T>(endpoint: string, data: unknown): Promise<T> {
    return this.request(endpoint, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  private async delete<T>(endpoint: string): Promise<T> {
    return this.request(endpoint, {
      method: 'DELETE',
    });
  }

  // API Key Management
  async createAPIKey(data: {
    name: string;
    expires_in?: number;
    service_account_id?: string;
    create_service_account?: boolean;
    service_account_role?: string;
  }): Promise<{
    key: string;
    key_prefix: string;
    api_key: {
      id: string;
      name: string;
      key_prefix: string;
      created_at: string;
      expires_at?: string;
    };
  }> {
    return this.post('/api-keys', data);
  }

  async listAPIKeys(): Promise<{
    api_keys: Array<{
      id: string;
      name: string;
      key_prefix: string;
      last_used_at?: string;
      expires_at?: string;
      created_at: string;
    }>;
  }> {
    return this.request('/api-keys');
  }

  async revokeAPIKey(keyId: string): Promise<{ message: string }> {
    return this.delete(`/api-keys/${keyId}`);
  }

  async chatAsk(message: string, conversationId?: string): Promise<{ answer: string }> {
    return this.post('/chat/ask', { message, conversation_id: conversationId });
  }

  async getChatConversations(): Promise<{ conversations: any[]; credits_available?: boolean; max_tokens?: number; max_messages?: number }> {
    return this.request('/chat/conversations');
  }

  async getChatMessageHistory(conversationId: string) {
    return this.request<{ messages: any[] }>(`/chat/conversations/${conversationId}`);
  }

  async deleteChatConversation(conversationId: string) {
    return this.delete<{ message: string }>(`/chat/conversations/${conversationId}`);
  }

  async continueConversation(conversationId: string): Promise<{ conversation_id: string; title: string; parent_id: string }> {
    return this.post(`/chat/conversations/${conversationId}/continue`, {});
  }

  async getCodeSamples(codeType: string): Promise<{ code_type: string; samples: Record<string, string> }> {
    const result = await this.request<{code_type: string; samples: Record<string,string>}>(`/code-samples?codeType=${encodeURIComponent(codeType)}`);
    return {...result, samples: Object.fromEntries(Object.entries(result.samples).map(([language, sample]) => [language, sample.replaceAll('YOUR_ENGINE_URL', getConfig().engineUrl)]))};
  }

  async getBillingInfo(): Promise<GetCurrentPlanResponse> {
    return this.request('/billing/plan');
  }

  async createCheckoutSession(amountMillicents: number): Promise<{ url: string }> {
    return this.post('/billing/checkout', {
      amount_millicents: amountMillicents,
    });
  }

  async updateMonthlyLimit(maxMonthlyMillicents: number): Promise<{ status: string }> {
    return this.request('/billing/spending-limit', {
      method: 'PUT',
      body: JSON.stringify({
        max_monthly_millicents: maxMonthlyMillicents,
      }),
    });
  }

  // --- Entity Profile Management ---
  async listEntityProfileTypes(): Promise<any> {
    return this.request('/entity-profile-types');
  }

  async applyEntityProfileType(slug: string, data: EntityProfileTypeDeclaration, dryRun = false): Promise<ApplyEntityProfileTypeResponse> {
    const suffix = dryRun ? '?dry_run=true' : '';
    return this.request(`/entity-profile-types/${encodeURIComponent(slug)}${suffix}`, {
      method: 'PUT',
      body: JSON.stringify(data)
    });
  }

  async archiveEntityProfileType(slug: string): Promise<any> {
    return this.delete(`/entity-profile-types/${encodeURIComponent(slug)}`);
  }

  async listEntityProfileTypesProxy(): Promise<any> {
    return this.request('/entity-profiles/types');
  }

  async listMetricsTemplates(): Promise<{ data: MetricsTemplateResponse[] }> {
    return this.request('/metrics-templates');
  }

  async getPricing(): Promise<{ credit: {
    ingress_cost_millicents: number;
    egress_cost_millicents: number;
    seat_cost_millicents: number;
    contract_cost_millicents: number;
    llm_token_cost_millicents: number;
    rate_limit_tps: number;
    payload_limit_bytes: number;
    custom_metric_cost_per_complexity_millicents: number;
  } }> {
    return this.request('/pricing');
  }

  async getEntityProfile(refKey: string, type: string): Promise<any> {
    // Note: Use encodeURIComponent to safely pass refKey and type
    return this.request(`/entity-profiles?refKey=${encodeURIComponent(refKey)}&type=${encodeURIComponent(type)}`);
  }

}

export interface CustomMetricFilter {
  key: string;
  value: string;
}

export interface CustomMetricDefinition {
  name: string;
  target?: 'thread' | 'step';
  step_name?: string;
  operation: 'COUNT' | 'RATE' | 'AVG' | 'SUM' | 'MIN' | 'MAX';
  field: string;
  filters?: CustomMetricFilter[];
  group_by?: string;
  granularity?: string;
  visualisation?: string;
}

export interface EntityTypeMetric {
  id?: string;
  template_id?: string;
  name?: string;
  parameters?: Record<string, any>;
  custom_definition?: CustomMetricDefinition;
}

export interface EntityProfileTypeDeclaration {
  name: string;
  type: string[];
  description?: string;
  metrics: EntityTypeMetric[];
}

export interface ApplyEntityProfileTypeResponse {
  status: 'created' | 'updated' | 'unchanged';
  dry_run: boolean;
  config_hash: string;
  data: EntityProfileType;
  changes: {
    name_changed: boolean;
    description_changed: boolean;
    types_changed: boolean;
    metrics_added: string[];
    metrics_updated: string[];
    metrics_removed: string[];
  };
  backfill: { supported: boolean; applied: boolean };
}

export interface EntityProfileType {
  id: string;
  company_id: string;
  name: string;
  slug: string;
  type: string[];
  description: string;
  created_at: string;
  updated_at: string;
  metrics?: EntityTypeMetric[];
}

export interface ParameterDefinition {
  name: string;
  type: string;
  values?: string[];
  description?: string;
}

export interface MetricsTemplateResponse {
  id: string;
  metrics_name: string;
  parameter_definitions: ParameterDefinition[];
}

export interface EntityProfileMetrics {
  entityProfileId: string;
  totalDeliveries: number;
  completedSuccessfully: number;
  validationViolations: number;
  deliveryHealthScore: number | null;
  prevDeliveryHealthScore: number | null;
  healthTrendSlope: number | null;
  averageDeliveryTimeMs: number;
  lastCalculatedAt: string;
}

export interface EntityProfile {
  id: string;
  refKey: string;
  companyId: string;
  profileTypeId: string;
  profileType?: { name: string; type: string[]; description?: string; metricsConfig?: Array<{ name?: string; templateId?: string; parameters?: Record<string, unknown> }> };
  name: string;
  createdAt: string;
  lastActiveAt: string;
  metrics: EntityProfileMetrics;
}

export const api = new ApiClient();

export interface EngineSettings {
  public_url: string;
  config_public_url: string;
  source: 'config' | 'ui' | 'unset';
  can_manage: boolean;
  endpoints: Record<string, string>;
}

export interface EngineUser {
  id: string;
  email: string;
  full_name: string;
  status: 'invited' | 'active' | 'suspended' | 'archived';
  roles: string[];
}

export interface IngestionRules {
  filters: string[];
  revision: string;
  updated_at?: string;
  evaluated_spans: number;
  dropped_spans: number;
  can_manage: boolean;
}
export interface IngestionPreview {
  spans: { name: string; drop: boolean; pattern?: string }[];
  dropped: number;
  kept: number;
}
