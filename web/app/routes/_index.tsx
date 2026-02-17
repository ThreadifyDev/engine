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

      {/* Use Case Sections */}
      <div className="bg-white">
        <FeatureSection
          tagline="For Support & Operations Teams"
          title={`Answer "what happened?" instantly`}
          description={`
            A customer tickets you: "Where's my order?" A workflow is stuck: "Why hasn't this loan been approved?" You dig through logs, ping engineering, wait for answers.
Ask Threadify-"Where's Sarah's order stuck?" or "Why hasn't loan #4729 been approved?"—and get instant answers with full execution context. See which step failed, why it failed, what happened before.
Support answers customers in seconds. Operations debugs without engineering. Everyone understands what actually happened.
          `}
          footerTagline="Ask questions in plain English • Get instant answers"
          visual="right"
        />

        <FeatureSection
          tagline="For Intelligent Systems"
          title="Validate and react with context"
          description={`
            Services stay up while business logic silently breaks—payment before inventory check, disbursement before identity verification. Automation retries blindly.
            Threadify validates execution against business rules and sends events when things deviate. Systems react with full context: failed payments check if inventory is reserved before retrying, fraud systems see complete transaction history, AI agents know what led to this moment.
            Prevention replaces reaction. Context replaces guessing.
          `}
          footerTagline="Real-time validation • Context-aware automation"
          visual="left"
        />

        <FeatureSection
          tagline="For Execs"
          title="Turn execution into business intelligence"
          description={`
            Payments feel slow today but you don't know if it's isolated or systemic. Workflows stall and you can't pinpoint bottlenecks. Questions like "which request types take longest?" require engineering to write queries.
Ask Threadify questions—"Show me all failed payments in the last hour" or "Which workflows are stuck at manual review?"—and surface patterns across execution. Find where processes bottleneck, identify what's blocking customer journeys, understand execution behavior without digging through logs. Product finds friction points. Operations spots issues early. Finance sees operational patterns.
          `}
          footerTagline="Execution becomes intelligence. Questions get answers."
          visual="right"
        />

        <FeatureSection
          tagline='For LLM Agents'
          title="Build execution context"
          description={`
            AI agents make decisions without understanding what led here. A refund gets approved without seeing payment history. A support ticket gets routed without knowing this is the customer's fifth escalation.
          Threadify gives agents complete execution context. When handling a request, agents see what already happened—which steps succeeded, what failed, what decisions were made. Refund agents check transaction history before approving. Routing agents see escalation patterns. Orchestration triggers the next step only when conditions are actually met, not guessed.
          `}
          visual="left"
        />
      </div>

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
