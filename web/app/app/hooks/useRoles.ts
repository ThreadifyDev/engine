import { useQuery } from '@tanstack/react-query';
import { api } from '~/lib/api';

interface Role {
  name: string;
  description: string;
  permissions: string[];
}

interface RolesResponse {
  level: string;
  roles: Record<string, Role>;
}

export function useRoles(level: 'app_level' | 'api_level' | 'runtime_level') {
  return useQuery({
    queryKey: ['roles', level],
    queryFn: async () => {
      const response = await api.getRolesByLevel(level);
      return response as RolesResponse;
    },
    staleTime: 5 * 60 * 1000, // 5 minutes - roles don't change often
  });
}

export function useServiceAccountRoles() {
  const query = useRoles('api_level');
  
  return {
    ...query,
    roles: query.data?.roles 
      ? Object.entries(query.data.roles).map(([key, role]) => ({
          value: key,
          label: role.name,
          description: role.description,
        }))
      : [],
  };
}
