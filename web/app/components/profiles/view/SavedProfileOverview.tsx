import { useEffect, useState } from 'react';
import { api, type EntityProfile } from '~/lib/api';
import OverviewTab from '../OverviewTab';
import ProfileViewRenderer from './ProfileViewRenderer';
import { defaultView, withMetricPresentations, validateProfileView, type ProfileView } from './profile-view';

export default function SavedProfileOverview({ profile, type }: { profile: EntityProfile; type: string }) {
  const [state, setState] = useState<{ definition: ProfileView | null; loading: boolean; error: string }>({ definition: null, loading: true, error: '' });
  useEffect(() => {
    let active = true;
    setState({ definition: null, loading: true, error: '' });
    api.getProfileView(profile.profileTypeId).then(result => {
      const definition = result.data.definition ? validateProfileView(result.data.definition) : null;
      if (active) setState({ definition, loading: false, error: '' });
    }).catch(() => {
      if (active) setState({ definition: null, loading: false, error: 'The shared view could not be loaded. Showing the standard overview. Reload to try again.' });
    });
    return () => { active = false; };
  }, [profile.companyId, profile.profileTypeId]);
  if (state.loading) return <p role="status" className="py-8 text-sm text-stone-500">Loading shared profile view…</p>;
  const metrics = (profile.profileType?.metricsConfig ?? []).map(metric => ({ id: metric.id, name: metric.name, custom_definition: metric.customDefinition ? { ...metric.customDefinition, group_by: metric.customDefinition.groupBy } : undefined }));
  const definition = state.definition || metrics.length ? withMetricPresentations(state.definition ?? defaultView(), metrics) : null;
  return <>
    {state.error && <p role="alert" className="mb-4 rounded-lg bg-amber-50 p-3 text-sm text-amber-900">{state.error}</p>}
    {definition ? <><p className="mb-4 text-xs text-stone-500">Shared profile view</p><ProfileViewRenderer definition={definition} profile={profile} type={type} /></> : <OverviewTab profile={profile} metrics={profile.metrics ?? {}} />}
  </>;
}
