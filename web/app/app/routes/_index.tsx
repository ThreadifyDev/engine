import { useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import NewHeroSection from '~/components/homepage/NewHeroSection';
import PillarSections from '~/components/homepage/PillarSections';
import ContractsSection from '~/components/homepage/ContractsSection';
import OutcomesSection from '~/components/homepage/OutcomesSection';
import CTASection from '~/components/homepage/CTASection';
import FooterSection from '~/components/homepage/FooterSection';

export const meta: MetaFunction = () => {
  return [
    { title: "Threadify - Workflow Intelligence for Your Business" },
    { name: "description", content: "Build systems that understand your business workflow. Observe, Validate, Detect, and React to your business processes in real time." },
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
    <div className="min-h-screen bg-white">
      <NewHeroSection />
      
      {/* Section header */}
      <section className="py-16 bg-white">
        <div className="max-w-4xl mx-auto px-6 text-center">
          <h2 className="text-4xl font-bold mb-4">How Threadify Works</h2>
          <p className="text-lg text-gray-600">
            A workflow intelligence system that creates context graphs for your business processes
          </p>
        </div>
      </section>

      {/* Four Pillars as individual sections */}
      <PillarSections />

      {/* Outcomes */}
      <OutcomesSection />

      {/* Contracts Deep Dive */}
      <ContractsSection />

      {/* CTA */}
      <CTASection />

      {/* Footer */}
      <FooterSection />
    </div>
  );
}
