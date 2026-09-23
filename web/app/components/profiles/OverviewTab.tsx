import { formatDistanceToNow } from 'date-fns';
import type { EntityProfile } from '~/lib/api';

export default function OverviewTab({ profile, metrics, hideHeading = false }: { profile: EntityProfile; metrics: any; hideHeading?: boolean }) {
  return (
    <>
      {/* <div className="grid grid-cols-4 gap-8 py-6 border-b border-gray-200 mb-10">
        <div>
          <p className="text-xs text-gray-500 mb-1">Total Activities</p>
          <p className="text-sm text-gray-900 font-medium">{metrics.totalDeliveries}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500 mb-1">Successful</p>
          <p className="text-sm text-green-700 font-medium">{metrics.completedSuccessfully}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500 mb-1">Violations</p>
          <p className="text-sm text-red-600 font-medium">{metrics.validationViolations}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500 mb-1">Avg Duration</p>
          <p className="text-sm text-gray-900 font-medium">
            {metrics.averageDeliveryTimeMs > 0
              ? `${(metrics.averageDeliveryTimeMs / 1000).toFixed(1)}s`
              : 'N/A'}
          </p>
        </div>
      </div> */}

      {!hideHeading && <h2 className="text-lg font-semibold text-gray-900 mb-4">Profile Details</h2>}
      <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
        <div className="divide-y divide-gray-200">
          <div className="px-4 py-4 hover:bg-gray-50 sm:px-6">
            <dt className="text-sm font-medium text-gray-500 mb-1">Record ID</dt>
            <dd className="break-all font-mono text-sm text-gray-900">{profile.id}</dd>
          </div>
          <div className="px-4 py-4 hover:bg-gray-50 sm:px-6">
            <dt className="text-sm font-medium text-gray-500 mb-1">First Observed</dt>
            <dd className="text-sm text-gray-900">
              {new Date(profile.createdAt).toLocaleString()}
              <span className="block text-gray-500 sm:ml-2 sm:inline">
                ({formatDistanceToNow(new Date(profile.createdAt), { addSuffix: true })})
              </span>
            </dd>
          </div>
          <div className="px-4 py-4 hover:bg-gray-50 sm:px-6">
            <dt className="text-sm font-medium text-gray-500 mb-1">Last Active</dt>
            <dd className="text-sm text-gray-900">
              {new Date(profile.lastActiveAt).toLocaleString()}
              <span className="block text-gray-500 sm:ml-2 sm:inline">
                ({formatDistanceToNow(new Date(profile.lastActiveAt), { addSuffix: true })})
              </span>
            </dd>
          </div>
          {metrics.lastCalculatedAt && (
            <div className="px-4 py-4 hover:bg-gray-50 sm:px-6">
              <dt className="text-sm font-medium text-gray-500 mb-1">Metrics Last Calculated</dt>
              <dd className="text-sm text-gray-900">
                {new Date(metrics.lastCalculatedAt).toLocaleString()}
              </dd>
            </div>
          )}
          {profile.profileType?.metricsConfig && profile.profileType.metricsConfig.length > 0 && (
            <div className="px-4 py-4 hover:bg-gray-50 sm:px-6">
              <dt className="text-sm font-medium text-gray-500 mb-2">Configured Metrics</dt>
              <dd className="text-sm text-gray-900">
                <ul className="list-disc pl-5 space-y-2">
                  {profile.profileType.metricsConfig.map((mc: any, i: number) => (
                    <li key={i}>
                      <div className="flex items-center flex-wrap gap-2">
                        {mc.name ? (
                          <span className="font-medium">{mc.name} <span className="text-gray-500 text-xs font-normal">({mc.templateId})</span></span>
                        ) : (
                          <span className="font-medium">{mc.templateId}</span>
                        )}
                        {mc.parameters && Object.keys(mc.parameters).length > 0 && (
                          <div className="flex flex-wrap gap-1">
                            {Object.entries(mc.parameters).map(([key, val]) => (
                              <span key={key} className="text-[10px] bg-gray-100 text-gray-600 px-1.5 py-0.5 rounded border border-gray-200 font-mono">
                                @{key}: {String(val)}
                              </span>
                            ))}
                          </div>
                        )}
                      </div>
                    </li>
                  ))}
                </ul>
              </dd>
            </div>
          )}
        </div>
      </div>
    </>
  );
}
