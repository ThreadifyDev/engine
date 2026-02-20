import { useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import Nav from '~/components/homepage/Nav';
import LiveThreadDemo from '~/components/homepage/LiveThreadDemo';
import FeatureSection from '~/components/homepage/FeatureSection';
import Footer from '~/components/homepage/Footer';
import { Zap, Link2, Lock, Radio, MessageSquare, Clock, Database, ShieldCheck, Webhook, Brain } from 'lucide-react';

export const meta: MetaFunction = () => {
  return [
    { title: "Threadify - Real-Time Execution Graph Infrastructure" },
    { name: "description", content: "Understand how your systems execute customer requests" },
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

  return (
    <div className="min-h-screen">
      {/* Navigation */}
      <Nav />

      {/* Hero Section */}
      <section className="bg-white text-black min-h-screen flex items-center">
        <div className="max-w-7xl mx-auto px-6 py-20 w-full">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            {/* Left: Hero Content */}
            <div>
              <div className="text-sm font-semibold tracking-wider mb-8">
                Realtime Execution Graph
              </div>
              
              <h1 className="text-6xl font-bold mb-8 leading-tight">
                Every customer request tells a story.
                <br />
                Turn it into intelligence.
              </h1>

              <p className="text-xl text-gray-600 mb-8 leading-relaxed">
                Threadify turns customer requests into live execution graphs. Support answers "what happened?" in seconds. Operations validates business logic in real-time. AI agents act with complete context.
              </p>

              <div className="flex gap-4">
                <button onClick={() => navigate('/signup')} className="px-6 py-3 bg-black text-white font-semibold rounded-lg hover:bg-gray-800 transition">
                  Get Started
                </button>
                <button onClick={() => navigate('https://docs.threadify.dev')} className="px-6 py-3 border border-gray-300 text-black font-semibold rounded-lg hover:border-gray-500 transition">
                  View Docs
                </button>
              </div>

              <p className="text-xs text-gray-500 mt-8">
                Real-time • Cryptographically verified • Drives revenue, reduces cost, manages risk
              </p>
            </div>

            {/* Right: Live Thread Demo */}
            <div className="flex justify-center lg:justify-end">
              <LiveThreadDemo />
            </div>
          </div>
        </div>
      </section>

      {/* How It Works Section - Scribe Style */}
      <section className="bg-gradient-to-br from-purple-50 via-blue-50 to-white py-24 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-4xl font-bold text-black mb-4">
              Here's how Threadify works
            </h2>
            <p className="text-lg text-gray-600">
              Hint: It's incredibly easy!
            </p>
          </div>

          <div className="grid lg:grid-cols-2 gap-16 items-start">
            {/* Left: Steps */}
            <div className="space-y-12">
              {/* Step 1 */}
              <div className="flex gap-6 items-start group cursor-pointer">
                <div className="relative flex-shrink-0">
                  <div className="w-10 h-10 rounded-full bg-black flex items-center justify-center relative">
                    <div className="absolute inset-0 rounded-full bg-black animate-pulse opacity-75"></div>
                    <div className="relative w-3 h-3 rounded-full bg-white"></div>
                  </div>
                  <div className="absolute top-10 left-5 w-0.5 h-24 bg-gray-200"></div>
                </div>
                <div>
                  <h3 className="text-xl font-semibold text-black mb-2">
                    Step 1: Capture execution
                  </h3>
                  <p className="text-gray-600 leading-relaxed">
                    Install our SDK and instrument your services. Every customer request becomes a live execution graph with complete business context.
                  </p>
                </div>
              </div>

              {/* Step 2 */}
              <div className="flex gap-6 items-start group cursor-pointer">
                <div className="relative flex-shrink-0">
                  <div className="w-10 h-10 rounded-full bg-gray-200 flex items-center justify-center">
                    <div className="w-3 h-3 rounded-full bg-gray-400"></div>
                  </div>
                  <div className="absolute top-10 left-5 w-0.5 h-24 bg-gray-200"></div>
                </div>
                <div>
                  <h3 className="text-xl font-semibold text-black mb-2">
                    Step 2: Validate business logic
                  </h3>
                  <p className="text-gray-600 leading-relaxed">
                    Define contracts that ensure workflows follow the right process. Get notified when execution violates what should happen.
                  </p>
                </div>
              </div>

              {/* Step 3 */}
              <div className="flex gap-6 items-start group cursor-pointer">
                <div className="relative flex-shrink-0">
                  <div className="w-10 h-10 rounded-full bg-gray-200 flex items-center justify-center">
                    <div className="w-3 h-3 rounded-full bg-gray-400"></div>
                  </div>
                  <div className="absolute top-10 left-5 w-0.5 h-24 bg-gray-200"></div>
                </div>
                <div>
                  <h3 className="text-xl font-semibold text-black mb-2">
                    Step 3: Query and analyze
                  </h3>
                  <p className="text-gray-600 leading-relaxed">
                    Ask questions about execution in natural language. Surface insights like "which request types are most expensive" or "where are workflows getting stuck"—without writing queries.
                  </p>
                </div>
              </div>

              {/* Step 4 */}
              <div className="flex gap-6 items-start group cursor-pointer">
                <div className="relative flex-shrink-0">
                  <div className="w-10 h-10 rounded-full bg-gray-200 flex items-center justify-center">
                    <div className="w-3 h-3 rounded-full bg-gray-400"></div>
                  </div>
                </div>
                <div>
                  <h3 className="text-xl font-semibold text-black mb-2">
                    Step 4: React in real-time
                  </h3>
                  <p className="text-gray-600 leading-relaxed">
                    Get notified when execution changes—violations, completions, state transitions. Your systems query the full thread before acting, turning blind automation into intelligent decisions.
                  </p>
                </div>
              </div>
            </div>

            {/* Right: Video Placeholder */}
            <div className="sticky top-24">
              <div className="bg-white rounded-2xl shadow-2xl p-8 border border-gray-200">
                <div className="h-[600px] bg-gradient-to-br from-gray-100 to-gray-200 rounded-lg flex items-center justify-center">
                  <div className="text-center">
                    <div className="w-20 h-20 mx-auto mb-4 rounded-full bg-black/10 flex items-center justify-center">
                      <svg className="w-10 h-10 text-gray-400" fill="currentColor" viewBox="0 0 20 20">
                        <path d="M6.3 2.841A1.5 1.5 0 004 4.11V15.89a1.5 1.5 0 002.3 1.269l9.344-5.89a1.5 1.5 0 000-2.538L6.3 2.84z" />
                      </svg>
                    </div>
                    <p className="text-gray-500 font-medium">Video Placeholder</p>
                    <p className="text-sm text-gray-400 mt-1">Step 1: Capture execution</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Use Cases Section */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="mb-12">
            <p className="text-sm font-semibold text-gray-500 uppercase tracking-wider mb-3">USE CASES</p>
            <h2 className="text-4xl font-light text-black mb-4">
              Built for every team.<br />For any workflow.
            </h2>
            <p className="text-lg text-gray-600 max-w-2xl">
              Turn processes into playbooks to train colleagues, assist customers, and drive software adoption.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {/* Top Row - 2 larger cards */}
            {/* Card 1: Resolve support tickets */}
            <div className="group relative overflow-hidden rounded-lg aspect-[4/3] cursor-pointer">
              <img 
                src="https://images.unsplash.com/photo-1556761175-b413da4baf72?w=800&q=80" 
                alt="Support team"
                className="absolute inset-0 w-full h-full object-cover"
              />
              <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/30 to-transparent"></div>
              <div className="relative h-full p-5 flex flex-col justify-end">
                <div className="w-7 h-7 rounded-md bg-blue-600 flex items-center justify-center mb-2">
                  <MessageSquare className="w-3.5 h-3.5 text-white" />
                </div>
                <h3 className="text-base font-semibold text-white">Resolve support tickets</h3>
              </div>
            </div>

            {/* Card 2: Prevent business logic failures */}
            <div className="group relative overflow-hidden rounded-lg aspect-[4/3] cursor-pointer">
              <img 
                src="https://images.unsplash.com/photo-1522071820081-009f0129c71c?w=800&q=80" 
                alt="Team collaboration"
                className="absolute inset-0 w-full h-full object-cover"
              />
              <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/30 to-transparent"></div>
              <div className="relative h-full p-5 flex flex-col justify-end">
                <div className="w-7 h-7 rounded-md bg-purple-600 flex items-center justify-center mb-2">
                  <ShieldCheck className="w-3.5 h-3.5 text-white" />
                </div>
                <h3 className="text-base font-semibold text-white">Prevent business logic failures</h3>
              </div>
            </div>

            {/* Bottom Row - 2 cards */}
            {/* Card 3: Optimize operational costs */}
            <div className="group relative overflow-hidden rounded-lg aspect-[4/3] cursor-pointer">
              <img 
                src="https://images.unsplash.com/photo-1551434678-e076c223a692?w=800&q=80" 
                alt="Team working"
                className="absolute inset-0 w-full h-full object-cover"
              />
              <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/30 to-transparent"></div>
              <div className="relative h-full p-5 flex flex-col justify-end">
                <div className="w-7 h-7 rounded-md bg-green-600 flex items-center justify-center mb-2">
                  <Zap className="w-3.5 h-3.5 text-white" />
                </div>
                <h3 className="text-base font-semibold text-white">Optimize operational costs</h3>
              </div>
            </div>

            {/* Card 4: Build smarter AI agents */}
            <div className="group relative overflow-hidden rounded-lg aspect-[4/3] cursor-pointer">
              <img 
                src="https://images.unsplash.com/photo-1600880292203-757bb62b4baf?w=800&q=80" 
                alt="Team meeting"
                className="absolute inset-0 w-full h-full object-cover"
              />
              <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/30 to-transparent"></div>
              <div className="relative h-full p-5 flex flex-col justify-end">
                <div className="w-7 h-7 rounded-md bg-purple-600 flex items-center justify-center mb-2">
                  <Brain className="w-3.5 h-3.5 text-white" />
                </div>
                <h3 className="text-base font-semibold text-white">Build smarter AI agents</h3>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Features Grid */}
      <section className="bg-gradient-to-b from-white to-gray-50 py-24 px-6">
        <div className="max-w-7xl mx-auto">
          <h2 className="text-4xl font-light text-black mb-16">
            Built for scale and intelligence
          </h2>
          
          <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-x-12 gap-y-8">
            {/* Feature 1 */}
            <div className="group cursor-pointer">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-black/5 group-hover:bg-black/10 transition-colors flex-shrink-0">
                  <Zap className="w-4 h-4 text-black" />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-black group-hover:text-gray-700 transition-colors">
                    Ultra-low latency instrumentation
                  </h3>
                </div>
              </div>
            </div>

            {/* Feature 2 */}
            <div className="group cursor-pointer">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-black/5 group-hover:bg-black/10 transition-colors flex-shrink-0">
                  <Link2 className="w-4 h-4 text-black" />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-black group-hover:text-gray-700 transition-colors">
                    Unified execution graphs
                  </h3>
                </div>
              </div>
            </div>

            {/* Feature 3 */}
            <div className="group cursor-pointer">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-black/5 group-hover:bg-black/10 transition-colors flex-shrink-0">
                  <Lock className="w-4 h-4 text-black" />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-black group-hover:text-gray-700 transition-colors">
                    Immutable audit trails
                  </h3>
                </div>
              </div>
            </div>

            {/* Feature 4 */}
            <div className="group cursor-pointer">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-black/5 group-hover:bg-black/10 transition-colors flex-shrink-0">
                  <Radio className="w-4 h-4 text-black" />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-black group-hover:text-gray-700 transition-colors">
                    Event-driven automation
                  </h3>
                </div>
              </div>
            </div>

            {/* Feature 5 */}
            <div className="group cursor-pointer">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-black/5 group-hover:bg-black/10 transition-colors flex-shrink-0">
                  <MessageSquare className="w-4 h-4 text-black" />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-black group-hover:text-gray-700 transition-colors">
                    AI-powered queries
                  </h3>
                </div>
              </div>
            </div>

            {/* Feature 6 */}
            <div className="group cursor-pointer">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-black/5 group-hover:bg-black/10 transition-colors flex-shrink-0">
                  <Clock className="w-4 h-4 text-black" />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-black group-hover:text-gray-700 transition-colors">
                    Persistent workflow tracking
                  </h3>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Footer */}
      <Footer />
    </div>
  );
}
