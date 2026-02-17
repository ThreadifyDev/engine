import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '~/lib/api';

interface ServiceAccount {
  id: string;
  name: string;
  description?: string;
  role: string;
  is_active: boolean;
  last_used_at?: string;
  created_at: string;
}

interface CreateServiceAccountRequest {
  name: string;
  description?: string;
  role: string;
}

// Query hook for fetching service accounts
export function useServiceAccounts() {
  return useQuery({
    queryKey: ['serviceAccounts'],
    queryFn: async () => {
      const response = await api.listServiceAccounts();
      return response.service_accounts as ServiceAccount[];
    },
    staleTime: 30 * 1000, // 30 seconds
  });
}

// Mutation hook for creating a service account
export function useCreateServiceAccount() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CreateServiceAccountRequest) => {
      return await api.createServiceAccount(data);
    },
    onSuccess: () => {
      // Invalidate and refetch service accounts
      queryClient.invalidateQueries({ queryKey: ['serviceAccounts'] });
    },
  });
}

// Mutation hook for toggling service account active status
export function useToggleServiceAccount() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ id, isActive }: { id: string; isActive: boolean }) => {
      return await api.updateServiceAccount(id, { is_active: !isActive });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['serviceAccounts'] });
    },
  });
}

// Mutation hook for deleting a service account
export function useDeleteServiceAccount() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: string) => {
      return await api.deleteServiceAccount(id);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['serviceAccounts'] });
    },
  });
}
