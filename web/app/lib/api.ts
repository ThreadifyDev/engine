// API client for backend communication

// Get API URL from window.__ENV__ (injected by Remix root loader)
// Falls back to localhost for development
const getApiBaseUrl = () => {
  if (typeof window !== 'undefined' && (window as any).__ENV__?.API_URL) {
    return `${(window as any).__ENV__.API_URL}/api`;
  }
  return 'http://localhost:3001/api';
};

const API_BASE_URL = getApiBaseUrl();

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
  email: string;
  password: string;
  full_name: string;
  job_role: string;
  industry?: string;
  company_size?: string;
  use_case?: string;
  invitation_token?: string;
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

export interface GetCurrentPlanResponse {
  credit_account: CreditAccountDTO | null;
}

class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  private async request<T = any>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    const token = typeof window !== 'undefined' ? localStorage.getItem('auth_token') : null;

    const headers: Record<string, string> = {
      ...(options.headers as Record<string, string>),
    };

    // Only set default Content-Type if not already specified
    if (!headers['Content-Type']) {
      headers['Content-Type'] = 'application/json';
    }

    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const response = await fetch(url, {
      ...options,
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
      let errorMessage = data.message || data.error || 'An error occurred';

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
      const hasAuthHeader = headers['Authorization'];
      if ((errorMessage === 'Invalid token' || response.status === 401) && hasAuthHeader) {
        if (typeof window !== 'undefined') {
          localStorage.removeItem('auth_token');
          localStorage.removeItem('user');
          window.location.href = '/login';
        }
      }

      // If we have validation details, throw ValidationError
      if (data.details && Array.isArray(data.details)) {
        throw new ValidationError(errorMessage, data.details);
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

    // Store token in localStorage
    if (typeof window !== 'undefined' && response.token) {
      localStorage.setItem('auth_token', response.token);
      localStorage.setItem('user', JSON.stringify(response.user));
    }

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

  async logout(): Promise<void> {
    try {
      await this.request('/auth/logout', { method: 'POST' });
    } catch {
      // Ignore backend errors — we still clear local state
    } finally {
      if (typeof window !== 'undefined') {
        localStorage.removeItem('auth_token');
        localStorage.removeItem('user');
      }
    }
  }

  getStoredUser(): User | null {
    if (typeof window === 'undefined') return null;
    const userStr = localStorage.getItem('user');
    return userStr ? JSON.parse(userStr) : null;
  }

  getStoredToken(): string | null {
    if (typeof window === 'undefined') return null;
    return localStorage.getItem('auth_token');
  }

  isAuthenticated(): boolean {
    return !!this.getStoredToken();
  }

  setUser(user: User) {
    if (typeof window !== 'undefined') {
      localStorage.setItem('user', JSON.stringify(user));
    }
  }

  setToken(token: string) {
    if (typeof window !== 'undefined') {
      localStorage.setItem('auth_token', token);
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

  async getTeamMembers(): Promise<{ members: User[] }> {
    return this.request('/team/members', {
      method: 'GET',
    });
  }

  async removeTeamMember(id: string): Promise<any> {
    return this.request(`/team/members/${id}`, {
      method: 'DELETE',
    });
  }

  async listInvitations(): Promise<{ invitations: any[] }> {
    return this.request('/team/invitations', {
      method: 'GET',
    });
  }

  async sendTeamInvitation(data: { email: string; role: string }): Promise<any> {
    return this.request('/team/invitations', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async resendInvitation(id: string): Promise<any> {
    return this.request(`/team/invitations/${id}/resend`, {
      method: 'POST',
    });
  }

  async cancelInvitation(id: string): Promise<any> {
    return this.request(`/team/invitations/${id}`, {
      method: 'DELETE',
    });
  }

  // Contract Management (proxy to ThreadifyEngine)
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
    // Engine expects raw YAML in body, not JSON
    const token = this.getStoredToken();
    const response = await fetch(`${this.baseUrl}/contracts`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/x-yaml',
      },
      body: data.yaml, // Send raw YAML
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.message || 'Failed to create contract');
    }

    return response.json();
  }

  async previewContract(data: { yaml: string }): Promise<any> {
    return this.request('/contracts/preview', {
      method: 'POST',
      body: JSON.stringify(data),
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

  async validateInvitation(token: string): Promise<{ company_name: string; email: string }> {
    return this.request('/team/invitation/validate', {
      method: 'POST',
      body: JSON.stringify({ token }),
    });
  }

  async listServiceAccounts(): Promise<any> {
    return this.request('/service-accounts');
  }

  async getServiceAccount(id: string): Promise<any> {
    return this.request(`/service-accounts/${id}`);
  }

  async updateServiceAccount(id: string, data: { name?: string; description?: string; is_active?: boolean }): Promise<any> {
    return this.request(`/service-accounts/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
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

  async getChatConversations(): Promise<{ conversations: any[]; credits_available?: boolean }> {
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
    return this.request(`/code-samples?codeType=${codeType}`);
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
  async createEntityProfileType(data: { name: string; type: string[]; description?: string }): Promise<any> {
    return this.post('/entity-profile-types', data);
  }

  async listEntityProfileTypes(): Promise<any> {
    return this.request('/entity-profile-types');
  }

  async updateEntityProfileType(id: string, data: { name: string; type: string[]; description?: string }): Promise<any> {
    return this.request(`/entity-profile-types/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data)
    });
  }

  async archiveEntityProfileType(id: string): Promise<any> {
    return this.delete(`/entity-profile-types/${id}`);
  }

  async listEntityProfileTypesProxy(): Promise<any> {
    return this.request('/entity-profiles/types');
  }

  async getEntityProfile(refKey: string, type: string): Promise<any> {
    // Note: Use encodeURIComponent to safely pass refKey and type
    return this.request(`/entity-profiles?refKey=${encodeURIComponent(refKey)}&type=${encodeURIComponent(type)}`);
  }

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
  profileType?: EntityProfileType;
  name: string;
  createdAt: string;
  lastActiveAt: string;
  metrics: EntityProfileMetrics;
}

export const api = new ApiClient(API_BASE_URL);
