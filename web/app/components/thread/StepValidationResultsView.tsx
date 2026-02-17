import { formatDistanceToNow } from 'date-fns';
import {
  CheckCircle2,
  AlertTriangle,
  Info,
  Calendar,
  Hash,
} from 'lucide-react';
import type { StepStateInfo, ValidationResultInfo } from '~/lib/graphql';

export function StepValidationResultsView({ 
  step, 
  validations
}: { 
  step: StepStateInfo; 
  validations: ValidationResultInfo[];
}) {
  const stepValidations = validations.filter(
    v => v.stepName === step.stepName && v.idempotencyKey === step.idempotencyKey
  );

  // Determine severity for each validation
  const getSeverity = (validation: ValidationResultInfo): string => {
    if (validation.hasCriticalViolation || validation.criticalCount > 0) return 'critical';
    if (validation.warningCount > 0) return 'warning';
    return 'info';
  };

  const getStatusBadge = (validation: ValidationResultInfo) => {
    const severity = getSeverity(validation);
    
    if (validation.overallStatus === 'success') {
      return (
        <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-green-100 text-green-700 text-xs font-medium">
          <CheckCircle2 className="w-3 h-3" />
          success
        </div>
      );
    }
    
    switch (severity) {
      case 'critical':
        return (
          <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-red-100 text-red-700 text-xs font-medium">
            <AlertTriangle className="w-3 h-3" />
            critical
          </div>
        );
      case 'warning':
        return (
          <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-yellow-100 text-yellow-700 text-xs font-medium">
            <AlertTriangle className="w-3 h-3" />
            warning
          </div>
        );
      default:
        return (
          <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-blue-100 text-blue-700 text-xs font-medium">
            <Info className="w-3 h-3" />
            info
          </div>
        );
    }
  };

  return (
    <div className="space-y-6">
      {/* Step Header - Matching Step Details style */}
      <div>
        <h2 className="text-2xl font-bold text-gray-900 mb-2">{step.stepName}</h2>
        {stepValidations.length > 0 && (
          <div className="mb-3">
            {getStatusBadge(stepValidations[0])}
          </div>
        )}
      </div>

      {/* Timeline Section - Matching Step Details style */}
      <div className="border-t border-gray-200 pt-6">
        <div className="flex items-center gap-2 mb-4">
          <Calendar className="w-4 h-4 text-gray-700" />
          <h3 className="text-sm font-semibold text-gray-900">Timeline</h3>
        </div>
        
        <div className="space-y-3 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-600">Validated:</span>
            <div className="text-right">
              <div className="text-gray-900">
                {formatDistanceToNow(new Date(step.lastUpdatedAt), { addSuffix: true })}
              </div>
              <div className="text-xs text-gray-500">
                {new Date(step.lastUpdatedAt).toLocaleString('en-US', {
                  day: '2-digit',
                  month: '2-digit',
                  year: 'numeric',
                  hour: '2-digit',
                  minute: '2-digit',
                  second: '2-digit'
                })}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Identifiers Section - Matching Step Details style */}
      <div className="border-t border-gray-200 pt-6">
        <div className="flex items-center gap-2 mb-4">
          <Hash className="w-4 h-4 text-gray-700" />
          <h3 className="text-sm font-semibold text-gray-900">Identifiers</h3>
        </div>
        
        <div className="space-y-3">
          <div>
            <div className="text-xs text-gray-600 mb-1">Idempotency Key:</div>
            <div className="bg-gray-50 border border-gray-200 rounded px-3 py-2 text-sm font-mono text-gray-900">
              {step.idempotencyKey}
            </div>
          </div>
        </div>
      </div>

      {/* Validation Results */}
      {stepValidations.length === 0 ? (
        <div className="border-t border-gray-200 pt-6">
          <div className="flex flex-col items-center justify-center py-8">
            <CheckCircle2 className="w-12 h-12 text-green-600 mb-3" />
            <h3 className="text-base font-semibold text-gray-900 mb-1">All Validations Passed</h3>
            <p className="text-sm text-gray-600">This step has no validation issues</p>
          </div>
        </div>
      ) : (
        <div className="border-t border-gray-200 pt-6">
          <h3 className="text-sm font-semibold text-gray-900 mb-4">Validation Details</h3>
          
          <div className="space-y-4">
            {stepValidations.map((validation) => (
              <div key={validation.validationId} className="space-y-3">
                {/* Summary Stats */}
                {validation.totalValidations > 0 && (
                  <div className="flex gap-4 text-xs">
                    {validation.criticalCount > 0 && (
                      <div className="flex items-center gap-1 text-red-700">
                        <AlertTriangle className="w-3 h-3" />
                        <span className="font-medium">{validation.criticalCount} critical</span>
                      </div>
                    )}
                    {validation.warningCount > 0 && (
                      <div className="flex items-center gap-1 text-yellow-700">
                        <AlertTriangle className="w-3 h-3" />
                        <span className="font-medium">{validation.warningCount} warnings</span>
                      </div>
                    )}
                    {validation.infoCount > 0 && (
                      <div className="flex items-center gap-1 text-blue-700">
                        <Info className="w-3 h-3" />
                        <span className="font-medium">{validation.infoCount} info</span>
                      </div>
                    )}
                  </div>
                )}

                {/* Individual Issues */}
                {validation.validations.length > 0 ? (
                  <div className="space-y-2">
                    {validation.validations.map((issue, idx) => (
                      <div key={idx} className="bg-gray-50 border border-gray-200 rounded-lg p-3">
                        <div className="text-sm text-gray-900 font-medium mb-1">{issue.message}</div>
                        {issue.field && (
                          <div className="text-xs text-gray-600 mb-1">
                            Field: <span className="font-mono bg-white px-1.5 py-0.5 rounded border border-gray-200">{issue.field}</span>
                          </div>
                        )}
                        {issue.expected && issue.actual && (
                          <div className="text-xs text-gray-600 space-y-0.5">
                            <div>Expected: <span className="font-mono">{issue.expected}</span></div>
                            <div>Actual: <span className="font-mono">{issue.actual}</span></div>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="bg-blue-50 border border-blue-200 rounded-lg p-3">
                    <div className="flex items-start gap-2">
                      <Info className="w-4 h-4 text-blue-600 flex-shrink-0 mt-0.5" />
                      <div className="text-sm text-blue-900">
                        Validation completed with status: <span className="font-medium">{validation.overallStatus}</span>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
