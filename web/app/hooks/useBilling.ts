import { useQuery } from '@tanstack/react-query';
import { api } from '~/lib/api';

export function useCurrentPlan() {
  return useQuery({
    queryKey: ['billing', 'plan'],
    queryFn: () => api.getBillingInfo(),
    staleTime: 30 * 1000, // 30 seconds
    refetchOnWindowFocus: true,
  });
}
