import { useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import Nav from '~/components/homepage/Nav';
import LiveThreadDemo from '~/components/homepage/LiveThreadDemo';
import FeatureSection from '~/components/homepage/FeatureSection';
import Footer from '~/components/homepage/Footer';
import { Zap, Link2, Lock, Radio, MessageSquare, Clock } from 'lucide-react';

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
                Intro to Threadify
              </div>
              
              <h1 className="text-6xl font-bold mb-8 leading-tight">
                Every customer request tells a business story.
                <br />
                Threadify captures it
              </h1>

              <p className="text-xl text-gray-600 mb-8 leading-relaxed">
                Customer requests execute complex business processes across distributed systems. Threadify translates this journey into a live execution graph you can query, enforce, and react to in real-time.
              </p>

              <div className="flex gap-4">
                <button onClick={() => navigate('/signup')} className="px-6 py-3 bg-black text-white font-semibold rounded-lg hover:bg-gray-800 transition">
                  Get Started
                </button>
                <button className="px-6 py-3 border border-gray-300 text-black font-semibold rounded-lg hover:border-gray-500 transition">
                  See demo
                </button>
              </div>

              <p className="text-xs text-gray-500 mt-8">
                Real-time • Cryptographically verified • For teams and AI
              </p>
            </div>

            {/* Right: Live Thread Demo */}
            <div className="flex justify-center lg:justify-end">
              <LiveThreadDemo />
            </div>
          </div>
        </div>
      </section>

      {/* Feature Sections */}
      <div className="bg-white">
        <FeatureSection
          title={`Answer "what happened?" in seconds, not hours.`}
          description={`
            Support gets a ticket: "Where's my order?" They dig through logs, contact engineering, wait 45 minutes.

            Threadify captures every customer request as a complete execution graph. Query by customer ID, order number, or support ticket from Zendesk, Jira, Salesforce—get answers instantly.

            Support answers in 30 seconds. Engineering stops context-switching. Everyone understands execution.
          `}
          visual="right"
        />

        {/* How It Works Section */}
        <section className="bg-white py-24 px-6">
          <div className="max-w-6xl mx-auto">
            <div className="text-center mb-20">
              <h2 className="text-4xl font-light text-black mb-4">
                Here's exactly what you do
              </h2>
              <p className="text-xl text-gray-700">
                From "what happened?" to answers in seconds - here's how in 4 simple steps
              </p>
            </div>

            {/* Steps with Cards and Connecting Lines */}
            <div className="grid md:grid-cols-2 gap-0 relative">
              {/* Step 1 */}
              <div className="border border-gray-300 rounded-lg p-6 relative m-4">
                <div className="flex items-start gap-4 mb-3">
                  <div className="w-10 h-10 rounded-full bg-black flex items-center justify-center text-white font-semibold flex-shrink-0">
                    1
                  </div>
                  <h3 className="text-lg font-semibold text-black pt-2">
                    Add SDK (create thread)
                  </h3>
                </div>
                <p className="text-gray-600 text-sm leading-relaxed">
                  Drop our lightweight SDK into your applications, agents, etc. Create threads to capture execution flows with minimal overhead (&lt;30ms).
                </p>
                {/* Line with arrow to next step */}
                <div className="hidden md:block absolute top-1/2 right-0" style={{ transform: 'translate(calc(100% + 8px), -50%)' }}>
                  <div className="flex items-center">
                    <div className="w-4 h-0.5 bg-gradient-to-r from-purple-600 to-pink-600"></div>
                    <div className="w-0 h-0 border-t-4 border-t-transparent border-b-4 border-b-transparent border-l-4 border-l-pink-600"></div>
                  </div>
                </div>
              </div>

              {/* Step 2 */}
              <div className="border border-gray-300 rounded-lg p-6 relative m-4">
                <div className="flex items-start gap-4 mb-3">
                  <div className="w-10 h-10 rounded-full bg-black flex items-center justify-center text-white font-semibold flex-shrink-0">
                    2
                  </div>
                  <h3 className="text-lg font-semibold text-black pt-2">
                    Instrument your business process with context
                  </h3>
                </div>
                <p className="text-gray-600 text-sm leading-relaxed">
                  Add business context to each step: customer data, reasoning, decisions, and timing. Capture what matters to validate against your business rules.
                </p>
                {/* Line with arrow down to next step */}
                <div className="hidden md:block absolute bottom-0 right-1/2" style={{ transform: 'translateX(50%) translateY(calc(100% + 8px))' }}>
                  <div className="flex flex-col items-center">
                    <div className="w-0.5 h-4 bg-gradient-to-b from-pink-600 to-orange-500"></div>
                    <div className="w-0 h-0 border-l-4 border-l-transparent border-r-4 border-r-transparent border-t-4 border-t-orange-500"></div>
                  </div>
                </div>
              </div>

              {/* Step 3 */}
              <div className="border border-gray-300 rounded-lg p-6 relative m-4 md:col-start-1">
                <div className="flex items-start gap-4 mb-3">
                  <div className="w-10 h-10 rounded-full bg-black flex items-center justify-center text-white font-semibold flex-shrink-0">
                    3
                  </div>
                  <h3 className="text-lg font-semibold text-black pt-2">
                    Link to external system
                  </h3>
                </div>
                <p className="text-gray-600 text-sm leading-relaxed">
                  Connect threads to external systems using references like paymentId, ticketId, etc. Enforce rules across your entire ecosystem with full context.
                </p>
                {/* Line with arrow to next step */}
                <div className="hidden md:block absolute top-1/2 right-0" style={{ transform: 'translate(calc(100% + 8px), -50%)' }}>
                  <div className="flex items-center">
                    <div className="w-4 h-0.5 bg-gradient-to-r from-orange-500 to-yellow-500"></div>
                    <div className="w-0 h-0 border-t-4 border-t-transparent border-b-4 border-b-transparent border-l-4 border-l-yellow-500"></div>
                  </div>
                </div>
              </div>

              {/* Step 4 */}
              <div className="border border-gray-300 rounded-lg p-6 m-4">
                <div className="flex items-start gap-4 mb-3">
                  <div className="w-10 h-10 rounded-full bg-black flex items-center justify-center text-white font-semibold flex-shrink-0">
                    4
                  </div>
                  <h3 className="text-lg font-semibold text-black pt-2">
                    Realtime execution graph
                  </h3>
                </div>
                <p className="text-gray-600 text-sm leading-relaxed">
                  Query execution graph as data comes in. Get live notifications for state changes and contract violations. Build intelligent automation with full execution memory.
                </p>
              </div>
            </div>
          </div>
        </section>

        <FeatureSection
          title="Detect patterns before they become problems."
          description={`
            Payments are slow today. Are they all slow? Is it systemic or isolated? You dig through dashboards, losing hours.

            Threadify's AI analyzes execution patterns across thousands of threads. Spot systemic issues: "80% of payments timing out since 2 PM." Find inefficiencies: "Loan apps with missing docs take 3x longer." Discover impacts: "When inventory check >2s, 40% abandon checkout."

            Operations detects issues before customers complain. Business finds optimization opportunities. Engineering fixes root causes.
          `}
          visual="right"
        />


        <FeatureSection
          title="Enforce business rules in real-time, not after the fact."
          description={`
            Your services are "up" but your business logic is broken. Payment processes before inventory is checked. Funds disburse before identity is verified. You discover violations after customers complain.

            Threadify validates execution against your business rules. Define contracts: "inventory must succeed before payment," "identity verified before disbursement." Violations trigger WebSocket events to your application—while you can still intervene.

            Prevent bad state before customers see it. Prove compliance with cryptographic audit trails. Start enforcing what should happen.
          `}
          visual="left"
        />

        {/* Workflow Discovery Section */}
        <section className="relative bg-gray-100 py-24 px-6 overflow-hidden">
          {/* Hexagon Background Pattern */}
          <svg className="absolute inset-0 w-full h-full opacity-10 pointer-events-none" xmlns="http://www.w3.org/2000/svg">
            <defs>
              <pattern id="hexagons" x="0" y="0" width="60" height="52" patternUnits="userSpaceOnUse">
                <path d="M30 0 L60 15 L60 37 L30 52 L0 37 L0 15 Z" fill="none" stroke="currentColor" strokeWidth="1"/>
              </pattern>
            </defs>
            <rect width="100%" height="100%" fill="url(#hexagons)" className="text-gray-400"/>
          </svg>
          
          <div className="max-w-6xl mx-auto relative z-10">
            <div className="text-center mb-8">
              <h2 className="text-4xl font-semibold text-black mb-6">
                Don't know your process yet? Start here.
              </h2>
              <p className="text-xl text-black/95 max-w-4xl mx-auto leading-relaxed mb-8">
                No contract required. Start capturing execution data, let Threadify detect your patterns, formalize them into contracts, and enforce rules automatically.
              </p>
              <div className="flex gap-3 justify-center flex-wrap">
                <button className="px-5 py-2.5 bg-white text-black text-sm font-medium rounded hover:bg-white transition">
                  Code Samples
                </button>
                <button onClick={() => navigate('/signup')} className="px-5 py-2.5 bg-black text-white text-sm font-medium rounded hover:bg-gray-800 transition">
                  Get Started
                </button>
              </div>
            </div>
          </div>
        </section>

        <FeatureSection
          title="React in real-time with complete context."
          description={`
            A payment fails. Your system retries blindly. A customer abandons checkout. Your system doesn't know why. An LLM agent approves a refund without knowing the customer's history. Automation without context causes more problems.

            Threadify sends real-time events to your application when execution changes. Your systems query the complete thread before reacting: "Payment failed, but inventory is still reserved—don't retry yet, escalate to ops." LLM agents query history: "This customer's last refund was denied for fraud. Escalate instead of approving." Non-LLM systems make context-aware decisions too.

            Automation becomes intelligent. Retries become strategic. Escalations become informed. Your systems react with perfect execution memory.
          `}
          visual="right"
        />

        {/* <FeatureSection
          title="React."
          description="Build intelligent systems that respond to execution events with full context. Automate responses and enable self-healing workflows."
          visual="right"
        /> */}
      </div>

      {/* Features Grid */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-7xl mx-auto">
          <h2 className="text-4xl font-light text-black mb-16">
            Built for scale and intelligence
          </h2>
          
          <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
            {/* Feature 1 */}
            <div className="border border-zinc-200 rounded-lg p-8 hover:border-zinc-300 transition">
              <Zap className="w-6 h-6 text-zinc-600 mb-4" />
              <h3 className="text-lg font-semibold text-black mb-3">
                Ultra-low latency instrumentation
              </h3>
              <p className="text-gray-600">
                Instrument agents, microservices, and UI in &lt;30ms. Capture execution without slowing down your systems.
              </p>
            </div>

            {/* Feature 2 */}
            <div className="border border-zinc-200 rounded-lg p-8 hover:border-zinc-300 transition">
              <Link2 className="w-6 h-6 text-zinc-600 mb-4" />
              <h3 className="text-lg font-semibold text-black mb-3">
                Unified execution graphs
              </h3>
              <p className="text-gray-600">
                Capture processes across agents, microservices, databases, and UI. One connected graph per customer request.
              </p>
            </div>

            {/* Feature 3 */}
            <div className="border border-zinc-200 rounded-lg p-8 hover:border-zinc-300 transition">
              <Lock className="w-6 h-6 text-zinc-600 mb-4" />
              <h3 className="text-lg font-semibold text-black mb-3">
                Cryptographically verified chains
              </h3>
              <p className="text-gray-600">
                Tamper-proof execution graphs. Every step is hashed and linked. Legally defensible audit trails.
              </p>
            </div>

            {/* Feature 4 */}
            <div className="border border-zinc-200 rounded-lg p-8 hover:border-zinc-300 transition">
              <Radio className="w-6 h-6 text-zinc-600 mb-4" />
              <h3 className="text-lg font-semibold text-black mb-3">
                Real-time event reactions
              </h3>
              <p className="text-gray-600">
                React to state changes, business rule violations, and thread lifecycle events. Build intelligent automation with complete execution context.
              </p>
            </div>

            {/* Feature 5 */}
            <div className="border border-zinc-200 rounded-lg p-8 hover:border-zinc-300 transition">
              <MessageSquare className="w-6 h-6 text-zinc-600 mb-4" />
              <h3 className="text-lg font-semibold text-black mb-3">
                LLM-powered support
              </h3>
              <p className="text-gray-600">
                Support teams ask natural language questions about customer journeys. Get instant answers with full execution context. No log diving required.
              </p>
            </div>

            {/* Feature 6 */}
            <div className="border border-zinc-200 rounded-lg p-8 hover:border-zinc-300 transition">
              <Clock className="w-6 h-6 text-zinc-600 mb-4" />
              <h3 className="text-lg font-semibold text-black mb-3">
                Built for long-running workflows
              </h3>
              <p className="text-gray-600">
                Track processes that span days, weeks, or months. No progress loss. No added complexity.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Footer */}
      <Footer />
    </div>
  );
}
