import { useState, useEffect } from 'react';
import { useNavigate } from '@remix-run/react';
import type { MetaFunction } from "@remix-run/node";
import { api, type EntityProfileType } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import ProfileTypesTab from '~/components/profiles/ProfileTypesTab';

export const meta: MetaFunction = () => {
  return [
    { title: "Entity Profiles - Threadify" },
    { name: "description", content: "Manage Entity Profile Schemas and View Profiles" },
  ];
};

export default function EntityProfiles() {
  const navigate = useNavigate();
  const [profileTypes, setProfileTypes] = useState<EntityProfileType[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const token = api.isAuthenticated();
    if (!token) {
      navigate('/login');
      return;
    }
    fetchProfileTypes();
  }, [navigate]);

  const fetchProfileTypes = async () => {
    try {
      setIsLoading(true);
      const res = await api.listEntityProfileTypes();
      setProfileTypes(res.data || []);
      setError(null);
    } catch (err: any) {
      console.error(err);
      setError(err.message || 'Failed to load profile types');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <AppLayout>
      <div className="p-8">
        <ProfileTypesTab
          profileTypes={profileTypes}
          isLoading={isLoading}
          error={error}
          onRefresh={fetchProfileTypes}
        />
      </div>
    </AppLayout>
  );
}
