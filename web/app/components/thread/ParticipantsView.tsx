import { useQuery } from '@tanstack/react-query';
import { Code, Users } from 'lucide-react';
import { graphqlClient, type StepStateInfo } from '~/lib/graphql';

export function ParticipantsView({ threadId, steps, stepHistory }: { threadId: string; steps: StepStateInfo[]; stepHistory?: any[] }) {
  // Extract unique services and actor IDs from StepHistory objects
  const services = new Set<string>();
  const actorIds = new Set<string>();
  
  stepHistory?.forEach(item => {
    if (item.actorService) services.add(item.actorService);
    if (item.actor) actorIds.add(item.actor);
  });

  // Fetch resolved actor names - MUST be called before any conditional returns
  const { data: resolvedActors, isLoading: isLoadingActors } = useQuery({
    queryKey: ['resolveActors', Array.from(actorIds)],
    queryFn: () => graphqlClient.resolveActors(Array.from(actorIds)),
    enabled: actorIds.size > 0,
  });

  if (!stepHistory) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-600"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Services */}
      <div>
        <h5 className="font-semibold text-gray-900 mb-3 flex items-center gap-2 text-sm">
          <Code className="w-4 h-4" />
          Services ({services.size})
        </h5>
        {services.size > 0 ? (
          <div className="space-y-2">
            {Array.from(services).map(service => (
              <div key={service} className="bg-gray-50 border border-gray-200 rounded-md p-3">
                <div className="font-mono text-sm text-gray-900">{service}</div>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-gray-500">No service information available</p>
        )}
      </div>

      {/* Actors/Service Accounts */}
      <div>
        <h5 className="font-semibold text-gray-900 mb-3 flex items-center gap-2 text-sm">
          <Users className="w-4 h-4" />
          Actors ({actorIds.size})
        </h5>
        {isLoadingActors ? (
          <p className="text-sm text-gray-500">Loading actor information...</p>
        ) : actorIds.size > 0 && resolvedActors ? (
          <div className="space-y-2">
            {resolvedActors.map(actor => (
              <div key={actor.id} className="bg-gray-50 border border-gray-200 rounded-md p-3">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="font-medium text-sm text-gray-900">{actor.name}</div>
                    {actor.companyName && (
                      <div className="text-xs text-gray-500 mt-1">{actor.companyName}</div>
                    )}
                  </div>
                  <span className={`text-xs px-2 py-1 rounded-full ${
                    actor.type === 'user' 
                      ? 'bg-blue-100 text-blue-700' 
                      : 'bg-purple-100 text-purple-700'
                  }`}>
                    {actor.type === 'user' ? 'User' : 'Service Account'}
                  </span>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-gray-500">No actor information available</p>
        )}
      </div>

      {/* Step Count */}
      <div className="border-t border-gray-200 pt-6">
        <div className="bg-gray-900 text-white p-4 rounded-md">
          <div className="text-2xl font-semibold">{steps.length}</div>
          <div className="text-sm text-gray-300">Total Steps</div>
        </div>
      </div>
    </div>
  );
}
