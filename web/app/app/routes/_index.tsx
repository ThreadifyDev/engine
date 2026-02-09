import { useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import NewHeroSection from '~/components/homepage/NewHeroSection';
import PillarSections from '~/components/homepage/PillarSections';
import CommonPatterns from '~/components/homepage/CommonPatterns';
import PurposeBuilt from '~/components/homepage/PurposeBuilt';
import FinalCTA from '~/components/homepage/FinalCTA';
import FooterSection from '~/components/homepage/FooterSection';

export const meta: MetaFunction = () => {
  return [
    { title: "Threadify - Real-Time Execution Graph Infrastructure" },
    { name: "description", content: "See execution as it happens, not hours later. Capture customer requests as connected graphs with full context. Enforce business rules at runtime. Build systems that heal themselves." },
  ];
};

export default function Index() {
  const navigate = useNavigate();

  useEffect(() => {
    // Redirect to dashboard if already authenticated
    if (api.isAuthenticated()) {
      navigate('/dashboard');
    }
  }, [navigate]);

  return (
    <div className="min-h-screen bg-black">
      <NewHeroSection />
      
      {/* Four Pillars */}
      <PillarSections />

      {/* Common Patterns */}
      <CommonPatterns />

      {/* Purpose Built */}
      <PurposeBuilt />

      {/* Final CTA */}
      <FinalCTA />

      {/* Footer */}
      <FooterSection />
    </div>
  );
}
