import { useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import HomePageStory from '~/components/HomePageStory';
import '~/styles/homepage-story.css';

export const meta: MetaFunction = () => {
  return [
    { title: "Threadify — Service-delivery Intelligence" },
    { name: "description", content: "Threadify captures and validates how your business delivers on every customer request — turning that into intelligence for your teams, systems, and agents." },
  ];
};

export default function Index() {
  const navigate = useNavigate();

  useEffect(() => {
    // Redirect to dashboard if already authenticated
    if (api.isAuthenticated()) {
      navigate('/u/dashboard');
    }
  }, [navigate]);

  return <HomePageStory />;
}
