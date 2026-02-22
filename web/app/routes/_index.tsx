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
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            
            {/* Text Block (Inside Grid) */}
            <div className="flex flex-col items-start pr-6 pt-0 h-full">
              <div className="mb-6">
                <span className="text-[10px] font-bold text-gray-400 uppercase tracking-widest px-2.5 py-1.5 border border-gray-100 rounded inline-block">
                  USE CASES
                </span>
              </div>
              <h2 className="text-[2.5rem] font-bold text-black mb-4 leading-[1.15] tracking-tight">
                Built for intelligent systems.
              </h2>
              <p className="text-gray-600 mb-6 leading-relaxed pr-4">
                Turn execution into intelligence to answer questions instantly, prevent failures, and build smarter systems.
              </p>
              <button
                onClick={() => navigate('/signup')} 
                className="mt-auto w-max px-5 py-2.5 bg-[#4F46E5] text-white text-sm font-semibold rounded-lg hover:bg-[#4338CA] transition"
              >
                Try Threadify free
              </button>
            </div>

            {/* Card 1: Create documentation and SOPs */}
            <div className="group relative overflow-hidden rounded-xl aspect-[4/3] cursor-pointer shadow-sm hover:shadow-md transition">
              {/* Sleek Pattern Background */}
              {/* Sleek Pattern Background */}
              <div className="absolute inset-0 bg-indigo-950 group-hover:scale-105 transition duration-700 shadow-inner">
                <div className="absolute inset-0 opacity-20" style={{ backgroundImage: 'radial-gradient(circle at 2px 2px, white 1px, transparent 0)', backgroundSize: '16px 16px' }}></div>
                <div className="absolute -top-20 -right-20 w-80 h-80 bg-fuchsia-600 rounded-full mix-blend-screen filter blur-[80px] opacity-70 group-hover:opacity-90 transition duration-700"></div>
                <div className="absolute -bottom-20 -left-20 w-80 h-80 bg-blue-600 rounded-full mix-blend-screen filter blur-[80px] opacity-70 group-hover:opacity-90 transition duration-700"></div>
                <div className="absolute inset-0 bg-gradient-to-t from-indigo-950/90 to-transparent"></div>
              </div>
              <div className="relative h-full p-6 flex justify-between items-end">
                <div>
                  <div className="w-8 h-8 rounded bg-indigo-500 flex items-center justify-center mb-3">
                    <Zap className="w-4 h-4 text-white" />
                  </div>
                  <h3 className="text-lg font-medium text-white">Create documentation and SOPs</h3>
                </div>
                <div className="w-6 h-6 rounded-full bg-white/10 flex items-center justify-center group-hover:bg-white/20 transition">
                  <svg className="w-3.5 h-3.5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 5l7 7-7 7"></path></svg>
                </div>
              </div>
            </div>

            {/* Card 2: Train teammates */}
            <div className="group relative overflow-hidden rounded-xl aspect-[4/3] cursor-pointer shadow-sm hover:shadow-md transition">
              {/* Blueprint Pattern Background */}
              {/* Blueprint Pattern Background */}
              <div className="absolute inset-0 bg-slate-950 group-hover:scale-105 transition duration-700">
                <div className="absolute inset-0 opacity-20 top-2" style={{ backgroundImage: 'repeating-linear-gradient(45deg, #94a3b8 0, #94a3b8 1px, transparent 0, transparent 50%)', backgroundSize: '16px 16px' }}></div>
                <div className="absolute inset-x-0 bottom-0 h-px bg-gradient-to-r from-transparent via-cyan-400 to-transparent opacity-60"></div>
                <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-blue-400 to-transparent opacity-60"></div>
                <div className="absolute inset-0 bg-cyan-900/40 mix-blend-multiply"></div>
                <div className="absolute top-1/4 right-1/4 w-64 h-64 bg-cyan-500 rounded-full filter blur-[80px] opacity-40 group-hover:opacity-60 transition duration-700"></div>
                <div className="absolute inset-0 bg-gradient-to-t from-slate-950 via-slate-950/40 to-slate-950/80"></div>
              </div>
              <div className="relative h-full p-6 flex justify-between items-end">
                <div>
                  <div className="w-8 h-8 rounded bg-indigo-500 flex items-center justify-center mb-3">
                    <Database className="w-4 h-4 text-white" />
                  </div>
                  <h3 className="text-lg font-medium text-white">Train teammates</h3>
                </div>
                <div className="w-6 h-6 rounded-full bg-white/10 flex items-center justify-center group-hover:bg-white/20 transition">
                  <svg className="w-3.5 h-3.5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 5l7 7-7 7"></path></svg>
                </div>
              </div>
            </div>

            {/* Card 3: Implement software */}
            <div className="group relative overflow-hidden rounded-xl aspect-[4/3] cursor-pointer shadow-sm hover:shadow-md transition">
              {/* Server Striped Tech Pattern Background */}
              {/* Server Striped Tech Pattern Background */}
              <div className="absolute inset-0 bg-emerald-950 group-hover:scale-105 transition duration-700">
                <div className="absolute inset-0 opacity-30" style={{ backgroundImage: 'repeating-linear-gradient(0deg, transparent, transparent 19px, #10b981 19px, #10b981 20px)' }}></div>
                <div className="absolute inset-0 opacity-20" style={{ backgroundImage: 'linear-gradient(90deg, transparent 49%, #34d399 49%, #34d399 51%, transparent 51%)', backgroundSize: '60px 100%' }}></div>
                <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[150%] h-48 bg-emerald-400/30 filter blur-[60px] group-hover:bg-emerald-400/50 transition duration-700 transform rotate-12"></div>
                <div className="absolute inset-0 bg-gradient-to-t from-emerald-950 via-emerald-950/80 to-transparent"></div>
              </div>
              <div className="relative h-full p-6 flex justify-between items-end">
                <div>
                  <div className="w-8 h-8 rounded bg-indigo-500 flex items-center justify-center mb-3">
                    <MessageSquare className="w-4 h-4 text-white" />
                  </div>
                  <h3 className="text-lg font-medium text-white">Implement software</h3>
                </div>
                <div className="w-6 h-6 rounded-full bg-white/10 flex items-center justify-center group-hover:bg-white/20 transition">
                  <svg className="w-3.5 h-3.5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 5l7 7-7 7"></path></svg>
                </div>
              </div>
            </div>

            {/* Card 4: Assist customers */}
            <div className="group relative overflow-hidden rounded-xl aspect-[4/3] cursor-pointer shadow-sm hover:shadow-md transition">
              {/* Vibrant Ambient Mesh Background */}
              {/* Vibrant Ambient Mesh Background */}
              <div className="absolute inset-0 bg-fuchsia-950 overflow-hidden group-hover:scale-105 transition duration-700">
                <div className="absolute top-0 right-0 w-[500px] h-[500px] bg-pink-500 rounded-full mix-blend-screen filter blur-[120px] opacity-60 transform translate-x-1/4 -translate-y-1/4 group-hover:opacity-80 transition duration-700"></div>
                <div className="absolute bottom-0 left-0 w-[500px] h-[500px] bg-orange-500 rounded-full mix-blend-screen filter blur-[120px] opacity-60 transform -translate-x-1/4 translate-y-1/4 group-hover:opacity-80 transition duration-700"></div>
                <div className="absolute inset-0 bg-[url('data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4IiBoZWlnaHQ9IjgiPjxwYXRoIGQ9Ik0wIDBMMCA4TDggOEw4IDBaIiBmaWxsPSJub25lIi8+PHBhdGggZD0iTTAgMEw0IDRMOCAwIiBzdHJva2U9InJnYmEoMjU1LDI1NSwyNTUsMC4wMykiIHN0cm9rZS13aWR0aD0iMSIvPjwvc3ZnPg==')] opacity-80"></div>
                <div className="absolute inset-0 bg-gradient-to-t from-fuchsia-950 via-fuchsia-950/50 to-transparent"></div>
              </div>
              <div className="relative h-full p-6 flex justify-between items-end">
                <div>
                  <div className="w-8 h-8 rounded bg-indigo-500 flex items-center justify-center mb-3">
                    <ShieldCheck className="w-4 h-4 text-white" />
                  </div>
                  <h3 className="text-lg font-medium text-white">Assist customers</h3>
                </div>
                <div className="w-6 h-6 rounded-full bg-white/10 flex items-center justify-center group-hover:bg-white/20 transition">
                  <svg className="w-3.5 h-3.5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 5l7 7-7 7"></path></svg>
                </div>
              </div>
            </div>

            {/* Card 5: Onboard new hires */}
            <div className="group relative overflow-hidden rounded-xl aspect-[4/3] cursor-pointer shadow-sm hover:shadow-md transition">
              {/* Nodes / Matrix Background */}
              {/* Nodes / Matrix Background */}
              <div className="absolute inset-0 bg-blue-950 overflow-hidden group-hover:scale-105 transition duration-700">
                <div className="absolute inset-0 opacity-30" style={{ backgroundImage: 'radial-gradient(circle at center, #60a5fa 1px, transparent 1px)', backgroundSize: '32px 32px' }}></div>
                <div className="absolute inset-0 opacity-20" style={{ backgroundImage: 'linear-gradient(to right, #60a5fa 1px, transparent 1px), linear-gradient(to bottom, #60a5fa 1px, transparent 1px)', backgroundSize: '96px 96px' }}></div>
                <div className="absolute top-0 right-0 w-64 h-64 border-[2px] border-blue-400/30 rounded-full transform translate-x-1/2 -translate-y-1/2"></div>
                <div className="absolute top-0 right-0 w-96 h-96 border-[2px] border-blue-400/20 rounded-full transform translate-x-1/2 -translate-y-1/2"></div>
                <div className="absolute bottom-[-10%] right-[10%] w-64 h-64 bg-teal-400 rounded-full filter blur-[80px] opacity-50 group-hover:opacity-70 transition duration-700"></div>
                <div className="absolute inset-0 bg-gradient-to-t from-blue-950 via-blue-950/60 to-transparent"></div>
              </div>
              <div className="relative h-full p-6 flex justify-between items-end">
                <div>
                  <div className="w-8 h-8 rounded bg-indigo-500 flex items-center justify-center mb-3">
                    <Brain className="w-4 h-4 text-white" />
                  </div>
                  <h3 className="text-lg font-medium text-white">Onboard new hires</h3>
                </div>
                <div className="w-6 h-6 rounded-full bg-white/10 flex items-center justify-center group-hover:bg-white/20 transition">
                  <svg className="w-3.5 h-3.5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 5l7 7-7 7"></path></svg>
                </div>
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
