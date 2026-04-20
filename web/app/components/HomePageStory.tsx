import { Link } from "@remix-run/react";
import { useState, useEffect } from "react";
import { Activity, Eye, Zap, Network, ArrowRight, CheckCircle2, TrendingUp, Shield, Check, Clock, AlertTriangle, Users, Bot, GitMerge } from "lucide-react";

export default function HomePageStory() {
  return (
    <div className="bg-white text-gray-900 font-sans antialiased selection:bg-black/10">
      {/* Navigation */}
      <nav className="fixed top-0 left-0 right-0 z-50 bg-white/80 backdrop-blur-xl border-b border-black/5">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <Link to="/" className="flex items-center">
            <svg className="h-7" viewBox="0 0 120 32" fill="none" xmlns="http://www.w3.org/2000/svg">
              {/* Text: Thread */}
              <text x="0" y="22" fontFamily="system-ui, -apple-system, sans-serif" fontSize="18" fontWeight="700" fill="currentColor">T</text>
              <text x="10" y="22" fontFamily="system-ui, -apple-system, sans-serif" fontSize="18" fontWeight="600" fill="currentColor">hread</text>
              
              {/* Sewing Needle as 'i' */}
              <ellipse cx="58" cy="8" rx="1.5" ry="1.2" fill="currentColor"/>
              <path d="M58 12L58 22" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"/>
              
              {/* Text: f */}
              <text x="64" y="22" fontFamily="system-ui, -apple-system, sans-serif" fontSize="18" fontWeight="600" fill="currentColor">f</text>
              
              {/* Y with cursive tail */}
              <path d="M76 10L80 16L80 20Q80 24 84 26Q88 28 90 26" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" fill="none"/>
              <path d="M88 10L80 16" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" fill="none"/>
            </svg>
          </Link>
          <div className="hidden md:flex items-center gap-8">
            <a href="#how-it-works" className="text-sm text-gray-600 hover:text-black transition-colors">How it works</a>
            <a href="https://docs.threadify.dev" className="text-sm text-gray-600 hover:text-black transition-colors">Documentation</a>
            <a href="#pricing" className="text-sm text-gray-600 hover:text-black transition-colors">Pricing</a>
          </div>
          <div className="flex items-center gap-3">
            <Link to="/login" className="text-sm font-medium text-gray-700 hover:text-black transition-colors px-4 py-2">Sign in</Link>
            <Link to="/signup" className="text-sm font-medium px-4 py-2 bg-black text-white rounded-full hover:bg-gray-800 transition-colors">
              Get started free
            </Link>
          </div>
        </div>
      </nav>

      {/* Hero Section */}
      <section className="pt-32 pb-0 px-6 overflow-hidden">
        <div className="max-w-7xl mx-auto">
          <div className="text-center pt-16 pb-20">
            <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-gradient-to-r from-gray-50 to-gray-100 border border-gray-200/80 mb-8">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
              <span className="text-sm font-medium text-gray-700">Now in early access</span>
            </div>
            
            <h1 className="text-5xl sm:text-6xl md:text-7xl lg:text-[5.5rem] font-semibold text-black leading-[1.08] tracking-tight max-w-5xl mx-auto mb-8">
              See exactly how your business{" "}
              <span className="bg-gradient-to-r from-gray-900 via-gray-600 to-gray-400 bg-clip-text text-transparent">delivers.</span>
              <br />
              <span className="text-gray-400">Every request. In real time.</span>
            </h1>
            
            <p className="text-xl md:text-2xl text-gray-500 max-w-3xl mx-auto mb-12 leading-relaxed font-light">
              Turn every customer request into delivery intelligence your teams and systems can act on.
            </p>
            
            <div className="flex flex-col sm:flex-row items-center justify-center gap-4 mb-20">
              <Link to="/signup" className="group flex items-center gap-2 px-8 py-4 bg-black text-white rounded-full font-medium text-lg hover:bg-gray-800 transition-all hover:gap-3">
                Start for free
                <ArrowRight className="w-5 h-5 transition-transform group-hover:translate-x-0.5" />
              </Link>
              <a href="#how-it-works" className="flex items-center gap-2 px-8 py-4 text-gray-700 font-medium text-lg hover:text-black transition-colors">
                See how it works
              </a>
            </div>
          </div>

          {/* Live Thread Visualization */}
          <div className="relative max-w-4xl mx-auto">
            <div className="absolute inset-0 bg-gradient-to-b from-transparent via-transparent to-white z-10 pointer-events-none h-full" />
            <div className="bg-gradient-to-b from-gray-50 to-white rounded-t-3xl border border-gray-200 border-b-0 p-8 md:p-12">
              <div className="flex items-center justify-between mb-8">
                <div className="flex items-center gap-3">
                  <div className="w-3 h-3 rounded-full bg-emerald-500 animate-pulse" />
                  <span className="text-sm font-medium text-gray-600">Live Thread</span>
                </div>
                <div className="text-sm text-gray-400 font-mono">ref_4821 · Account Application</div>
              </div>
              
              <div className="space-y-4 font-mono text-sm">
                <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-100 shadow-sm">
                  <div className="w-6 h-6 rounded-full bg-emerald-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Check className="w-3.5 h-3.5 text-emerald-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between gap-4">
                      <span className="font-medium text-gray-900">identity_verified</span>
                      <span className="text-gray-400 text-xs">your platform</span>
                    </div>
                    <div className="text-gray-500 text-xs mt-1">Completed in 1.2s</div>
                  </div>
                </div>
                
                <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-100 shadow-sm">
                  <div className="w-6 h-6 rounded-full bg-emerald-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Check className="w-3.5 h-3.5 text-emerald-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between gap-4">
                      <span className="font-medium text-gray-900">credit_check_passed</span>
                      <span className="text-xs px-2 py-0.5 bg-blue-50 text-blue-600 rounded-full">partner: credit-bureau</span>
                    </div>
                    <div className="text-gray-500 text-xs mt-1">Completed in 3.4s</div>
                  </div>
                </div>
                
                <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-100 shadow-sm">
                  <div className="w-6 h-6 rounded-full bg-emerald-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Check className="w-3.5 h-3.5 text-emerald-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between gap-4">
                      <span className="font-medium text-gray-900">documents_requested</span>
                      <span className="text-gray-400 text-xs">compliance-team</span>
                    </div>
                    <div className="text-gray-500 text-xs mt-1">Completed in 0.8s</div>
                  </div>
                </div>

                <div className="flex items-start gap-4 p-4 bg-gray-50 rounded-xl border border-dashed border-gray-200">
                  <div className="w-6 h-6 rounded-full bg-gray-200 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Clock className="w-3.5 h-3.5 text-gray-400" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between gap-4">
                      <span className="font-medium text-gray-400">account_activated</span>
                      <span className="text-gray-300 text-xs">waiting</span>
                    </div>
                  </div>
                </div>
                
                <div className="flex items-start gap-4 p-4 bg-amber-50/50 rounded-xl border border-amber-200/50">
                  <div className="w-6 h-6 rounded-full bg-amber-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <AlertTriangle className="w-3.5 h-3.5 text-amber-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between gap-4">
                      <span className="font-medium text-amber-700">SLA breach detected</span>
                      <span className="text-amber-600 text-xs font-medium">8 hours waiting</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Tagline Section */}
      <section className="py-32 px-6 bg-white">
        <div className="max-w-4xl mx-auto text-center">
          <p className="text-2xl md:text-3xl lg:text-4xl text-gray-400 leading-relaxed font-light">
            This is what Threadify sees.{" "}
            <span className="text-black font-normal">For the first time, so can you.</span>
          </p>
        </div>
      </section>

      {/* Value Prop Banner */}
      <section className="py-20 px-6 bg-black text-white">
        <div className="max-w-5xl mx-auto text-center">
          <h2 className="text-3xl md:text-4xl lg:text-5xl font-semibold leading-tight tracking-tight mb-6">
            The process crosses every boundary.
            <br />
            <span className="bg-gradient-to-r from-white via-gray-300 to-gray-500 bg-clip-text text-transparent">The intelligence doesn't.</span>
          </h2>
          <p className="text-xl text-gray-400 italic">Until now.</p>
        </div>
      </section>

      {/* How It Works - Capture */}
      <section id="how-it-works" className="py-32 px-6 border-b border-gray-100">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            <div>
              <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-gray-100 text-sm font-medium text-gray-600 mb-6">
                <span className="w-5 h-5 rounded-full bg-black text-white text-xs flex items-center justify-center">1</span>
                Capture
              </div>
              <h2 className="text-4xl md:text-5xl font-semibold text-black tracking-tight mb-6 leading-[1.1]">
                Every request sets a delivery process in motion
              </h2>
              <div className="space-y-5 text-lg text-gray-600 leading-relaxed">
                <p>
                  Crossing services, teams, partners, and boundaries no single system can see end to end.
                </p>
                <p>
                  One line per business action. A live execution graph builds itself across every service involved.
                </p>
                <p className="text-black font-medium">
                  Most teams discover their real process looks nothing like the Confluence doc. Now you know what it actually is.
                </p>
              </div>
            </div>
            <div>
              <div className="bg-gray-50 rounded-2xl p-8 border border-gray-200">
                <div className="font-mono text-sm">
                  <div className="text-gray-400 mb-4">// Start tracking</div>
                  <div className="space-y-2 mb-6">
                    <div><span className="text-gray-500">const</span> thread = <span className="text-gray-500">await</span> threadify.<span className="text-black">start</span>();</div>
                    <div>thread.<span className="text-black">step</span>(<span className="text-gray-600">"payment_captured"</span>)</div>
                  </div>
                  <div className="text-gray-400 mb-2">// Add context</div>
                  <div className="space-y-2">
                    <div><span className="text-gray-500">const</span> step = thread.<span className="text-black">step</span>(<span className="text-gray-600">"fraud_check"</span>)</div>
                    <div>step.<span className="text-black">addContext</span>(data).<span className="text-black">success</span>()</div>
                  </div>
                </div>
              </div>
              <div className="mt-6">
                <a href="https://docs.threadify.dev/core-concepts/tracking-workflows" className="inline-flex items-center gap-2 text-black font-medium hover:gap-3 transition-all">
                  Learn how to track workflows
                  <ArrowRight className="w-4 h-4" />
                </a>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* How It Works - Validate */}
      <section className="py-32 px-6 border-b border-gray-100">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            <div className="order-2 lg:order-1 bg-gray-50 rounded-2xl p-8 border border-gray-200">
              <div className="space-y-4">
                <div className="flex items-center justify-between p-3 bg-white rounded-lg border border-gray-100">
                  <span className="text-sm text-gray-600">Sequence check</span>
                  <Check className="w-4 h-4 text-emerald-500" />
                </div>
                <div className="flex items-center justify-between p-3 bg-white rounded-lg border border-gray-100">
                  <span className="text-sm text-gray-600">Required steps present</span>
                  <Check className="w-4 h-4 text-emerald-500" />
                </div>
                <div className="flex items-center justify-between p-3 bg-white rounded-lg border border-gray-100">
                  <span className="text-sm text-gray-600">SLA compliance</span>
                  <Check className="w-4 h-4 text-emerald-500" />
                </div>
                <div className="flex items-center justify-between p-3 bg-red-50 rounded-lg border border-red-200">
                  <span className="text-sm font-medium text-red-700">payment_capture timeout</span>
                  <AlertTriangle className="w-4 h-4 text-red-500" />
                </div>
              </div>
            </div>
            <div className="order-1 lg:order-2">
              <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-gray-100 text-sm font-medium text-gray-600 mb-6">
                <span className="w-5 h-5 rounded-full bg-black text-white text-xs flex items-center justify-center">2</span>
                Validate
              </div>
              <h2 className="text-4xl md:text-5xl font-semibold text-black tracking-tight mb-6 leading-[1.1]">
                Know whether it delivered correctly
              </h2>
              <div className="space-y-5 text-lg text-gray-600 leading-relaxed">
                <p>
                  Define what correct looks like. Threadify validates every execution against it in real time.
                </p>
                <p>
                  A step skipped. A sequence broken. A partner silent. You know instantly.
                </p>
                <p className="text-black font-medium">
                  Not from a batch job. Not from a customer complaint. The instant it happens.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* How It Works - React */}
      <section className="py-32 px-6 border-b border-gray-100">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            <div>
              <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-gray-100 text-sm font-medium text-gray-600 mb-6">
                <span className="w-5 h-5 rounded-full bg-black text-white text-xs flex items-center justify-center">3</span>
                React
              </div>
              <h2 className="text-4xl md:text-5xl font-semibold text-black tracking-tight mb-6 leading-[1.1]">
                Build systems that respond intelligently
              </h2>
              <div className="space-y-5 text-lg text-gray-600 leading-relaxed">
                <p>
                  When a step gets skipped, a workflow stops before it goes further. When a payment stalls, an account is suspended automatically.
                </p>
                <p className="text-black font-medium">
                  Your system stops being reactive. It becomes intelligent.
                </p>
              </div>
            </div>
            <div className="bg-gray-50 rounded-2xl p-8 border border-gray-200">
              <div className="space-y-4">
                <div className="flex items-center gap-3 p-4 bg-white rounded-lg border border-gray-100">
                  <Users className="w-5 h-5 text-gray-400" />
                  <span className="text-sm font-medium text-gray-900">Proactive customer messaging</span>
                </div>
                <div className="flex items-center gap-3 p-4 bg-white rounded-lg border border-gray-100">
                  <Bot className="w-5 h-5 text-gray-400" />
                  <span className="text-sm font-medium text-gray-900">AI agents with full context</span>
                </div>
                <div className="flex items-center gap-3 p-4 bg-white rounded-lg border border-gray-100">
                  <Zap className="w-5 h-5 text-gray-400" />
                  <span className="text-sm font-medium text-gray-900">Circuit breakers that fire early</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Cross-Boundary Section */}
      <section className="py-32 px-6 bg-gray-50">
        <div className="max-w-5xl mx-auto text-center">
          <h2 className="text-4xl md:text-5xl font-semibold text-black tracking-tight mb-6 leading-[1.1]">
            Your process doesn't stop at your boundary.
            <br />
            <span className="text-gray-400">Your intelligence shouldn't either.</span>
          </h2>
          <p className="text-lg text-gray-600 max-w-2xl mx-auto mb-12">
            Invite a partner into the thread. They instrument their side. One shared execution graph — their steps and yours, in one timeline.
          </p>
          <div className="flex items-center justify-center gap-8">
            <div className="px-6 py-4 bg-white rounded-lg border border-gray-200 font-medium">Your API</div>
            <GitMerge className="w-6 h-6 text-gray-400" />
            <div className="px-6 py-4 bg-white rounded-lg border border-gray-200 font-medium">Partner API</div>
          </div>
        </div>
      </section>

      {/* Use Cases Section */}
      <section id="product" className="py-24 px-6 bg-white">
        <div className="max-w-7xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-4xl md:text-5xl font-bold mb-4">
              <span className="bg-gradient-to-r from-gray-900 to-gray-600 bg-clip-text text-transparent">
                Know how you're delivering. For every customer.
              </span>
            </h2>
          </div>

          <div className="grid md:grid-cols-3 gap-12">
            {/* Operations & Support */}
            <div className="relative">
              <div className="absolute -left-4 top-0 w-1 h-full bg-gradient-to-b from-gray-900 to-transparent rounded-full"></div>
              <div className="pl-8">
                <h3 className="text-2xl font-bold mb-4 text-gray-900">Operations & Support</h3>
                <p className="text-gray-600 leading-relaxed mb-6">
                  Stop waiting for complaints to find failures. See exactly where every request stands — across every service and partner — right now.
                </p>
                <div className="bg-gray-50 rounded-xl p-6 border border-gray-200">
                  <div className="flex items-center gap-3 mb-3">
                    <TrendingUp className="w-5 h-5 text-gray-400" />
                    <span className="font-semibold text-gray-900">Instant answers</span>
                  </div>
                  <p className="text-sm text-gray-600">From 30-minute escalations to 10-second answers</p>
                </div>
              </div>
            </div>

            {/* Engineering & Product */}
            <div className="relative">
              <div className="absolute -left-4 top-0 w-1 h-full bg-gradient-to-b from-gray-800 to-transparent rounded-full"></div>
              <div className="pl-8">
                <h3 className="text-2xl font-bold mb-4 text-gray-900">Engineering & Product</h3>
                <p className="text-gray-600 leading-relaxed mb-6">
                  See what your system actually does, not what the docs say. Debug faster. Onboard faster.
                </p>
                <div className="bg-gray-50 rounded-xl p-6 border border-gray-200">
                  <div className="flex items-center gap-3 mb-3">
                    <Network className="w-5 h-5 text-gray-400" />
                    <span className="font-semibold text-gray-900">Living documentation</span>
                  </div>
                  <p className="text-sm text-gray-600">Living documentation from real execution</p>
                </div>
              </div>
            </div>

            {/* Systems & Agents */}
            <div className="relative">
              <div className="absolute -left-4 top-0 w-1 h-full bg-gradient-to-b from-gray-700 to-transparent rounded-full"></div>
              <div className="pl-8">
                <h3 className="text-2xl font-bold mb-4 text-gray-900">Systems & Agents</h3>
                <p className="text-gray-600 leading-relaxed mb-6">
                  Give your automations and agents complete delivery context — every step, every outcome. So they act on it, not guess.
                </p>
                <div className="bg-gray-50 rounded-xl p-6 border border-gray-200">
                  <div className="flex items-center gap-3 mb-3">
                    <Zap className="w-5 h-5 text-gray-400" />
                    <span className="font-semibold text-gray-900">Full context</span>
                  </div>
                  <p className="text-sm text-gray-600">Full execution context. No log diving.</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Continuous Delivery Record Section */}
      <section className="py-32 px-6 bg-gray-50">
        <div className="max-w-4xl mx-auto text-center">
          <h2 className="text-4xl md:text-5xl font-semibold text-black tracking-tight mb-6 leading-[1.1]">
            Every thread is one moment. Every customer has many.
          </h2>
          <p className="text-xl text-gray-600 leading-relaxed mb-6">
            Threadify connects every request into a continuous delivery record per customer. See who you're serving well — and who you're quietly failing.
          </p>
          <p className="text-xl text-black font-medium">
            That's not monitoring. That's delivery intelligence.
          </p>
        </div>
      </section>

      {/* Final CTA */}
      <section className="py-32 px-6 bg-gradient-to-b from-gray-50 to-white">
        <div className="max-w-4xl mx-auto text-center">
          <h2 className="text-5xl md:text-6xl font-bold mb-6">
            <span className="bg-gradient-to-r from-gray-900 via-gray-700 to-gray-900 bg-clip-text text-transparent">
              Start seeing how you deliver
            </span>
          </h2>
          <p className="text-xl text-gray-600 mb-10 max-w-2xl mx-auto">
            Join teams turning service delivery into intelligence. No credit card required.
          </p>
          <div className="flex flex-wrap justify-center gap-4">
            <Link to="/signup" className="group px-10 py-5 bg-gray-900 text-white rounded-xl font-bold hover:bg-gray-800 transition-all shadow-xl hover:shadow-2xl text-lg flex items-center gap-2">
              Start free trial
              <ArrowRight className="w-5 h-5 group-hover:translate-x-1 transition-transform" />
            </Link>
            <a href="https://docs.threadify.dev" className="px-10 py-5 bg-white text-gray-900 rounded-xl font-bold border-2 border-gray-200 hover:border-gray-300 transition-all text-lg">
              Read documentation
            </a>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="py-12 px-6 border-t border-gray-200 bg-white">
        <div className="max-w-7xl mx-auto">
          <div className="flex flex-col md:flex-row justify-between items-center gap-6 mb-6">
            <div className="flex items-center">
              <svg className="h-8" viewBox="0 0 120 32" fill="none" xmlns="http://www.w3.org/2000/svg">
                {/* Text: Thread */}
                <text x="0" y="22" fontFamily="system-ui, -apple-system, sans-serif" fontSize="18" fontWeight="700" fill="currentColor">T</text>
                <text x="10" y="22" fontFamily="system-ui, -apple-system, sans-serif" fontSize="18" fontWeight="600" fill="currentColor">hread</text>
                
                {/* Sewing Needle as 'i' */}
                <ellipse cx="58" cy="8" rx="1.5" ry="1.2" fill="currentColor"/>
                <path d="M58 12L58 22" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"/>
                
                {/* Text: f */}
                <text x="64" y="22" fontFamily="system-ui, -apple-system, sans-serif" fontSize="18" fontWeight="600" fill="currentColor">f</text>
                
                {/* Y with cursive tail */}
                <path d="M76 10L80 16L80 20Q80 24 84 26Q88 28 90 26" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" fill="none"/>
                <path d="M88 10L80 16" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" fill="none"/>
              </svg>
            </div>
            <div className="flex items-center gap-6">
              <a href="https://docs.threadify.dev/core-concepts/mcp-integration" className="text-sm text-gray-600 hover:text-gray-900 transition">MCP Integration</a>
              <a href="https://threadify.dev/AI.md" className="text-sm text-gray-600 hover:text-gray-900 transition">AI Assistant Guide</a>
              <a href="https://docs.threadify.dev" className="text-sm text-gray-600 hover:text-gray-900 transition">Documentation</a>
            </div>
          </div>
          <p className="text-sm text-gray-500 text-center">© {new Date().getFullYear()} Threadify. Service delivery intelligence platform.</p>
        </div>
      </footer>
    </div>
  );
}
