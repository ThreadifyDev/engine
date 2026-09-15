import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '~/lib/api';

export function useCurrentPlan() {
  return useQuery({
    queryKey: ['billing', 'plan'],
    queryFn: () => api.getBillingInfo(),
    staleTime: 30 * 1000, // 30 seconds
    refetchOnWindowFocus: true,
  });
}

export function useTiers() {
  return useQuery({
    queryKey: ['billing', 'tiers'],
    queryFn: () => api.getTiers(),
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}

export function useCreateCheckout() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ tier, billingCycle }: { tier: string; billingCycle: string }) =>
      api.createBillingCheckout(tier, billingCycle),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['billing', 'plan'] });
    },
  });
}

export function useCancelSubscription() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.cancelSubscription(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['billing', 'plan'] });
    },
  });
}
