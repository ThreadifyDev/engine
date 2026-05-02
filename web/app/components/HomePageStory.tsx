import { Link } from "@remix-run/react";
import ThreadifyLogo from "~/components/ThreadifyLogo";
import Footer from "~/components/homepage/Footer";
import CodeBlock from "~/components/CodeBlock";
import { useState, useEffect } from "react";
import { Activity, Eye, Zap, Network, ArrowRight, CheckCircle2, TrendingUp, Shield, Check, Clock, AlertTriangle, Users, Bot, GitMerge, ChevronDown, Menu, X } from "lucide-react";

export default function HomePageStory() {
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  return (
    <div className="bg-white text-gray-900 font-sans antialiased selection:bg-black/10">
      {/* Navigation */}
      <nav className="fixed top-0 left-0 right-0 z-50 bg-white/80 backdrop-blur-xl border-b border-black/5">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <Link to="/" className="flex items-center z-50">
            <ThreadifyLogo height={26} />
          </Link>
          
          {/* Desktop Nav */}
          <div className="hidden md:flex items-center gap-8">
            <a href="#how-it-works" className="text-sm text-gray-600 hover:text-black transition-colors">How it works</a>
            <a href="https://docs.threadify.dev" className="text-sm text-gray-600 hover:text-black transition-colors">Documentation</a>
            <Link to="/pricing" className="text-sm text-gray-600 hover:text-black transition-colors">Pricing</Link>
          </div>
          <div className="hidden md:flex items-center gap-3">
            <Link to="/login" className="text-sm font-medium text-gray-700 hover:text-black transition-colors px-4 py-2">Sign in</Link>
            <Link to="/signup" className="text-sm font-medium px-4 py-2 bg-black text-white rounded-full hover:bg-gray-800 transition-colors">
              Get started free
            </Link>
          </div>

          {/* Mobile Menu Toggle */}
          <button 
            className="md:hidden z-50 p-2 -mr-2 text-gray-600 hover:text-black"
            onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
          >
            {isMobileMenuOpen ? <X className="w-6 h-6" /> : <Menu className="w-6 h-6" />}
          </button>
        </div>

        {/* Mobile Menu Dropdown */}
        {isMobileMenuOpen && (
          <div className="md:hidden absolute top-16 left-0 right-0 bg-white border-b border-gray-100 shadow-xl py-4 px-6 flex flex-col gap-4">
            <a href="#how-it-works" onClick={() => setIsMobileMenuOpen(false)} className="text-base font-medium text-gray-700 py-2">How it works</a>
            <a href="https://docs.threadify.dev" onClick={() => setIsMobileMenuOpen(false)} className="text-base font-medium text-gray-700 py-2">Documentation</a>
            <Link to="/pricing" onClick={() => setIsMobileMenuOpen(false)} className="text-base font-medium text-gray-700 py-2">Pricing</Link>
            <hr className="border-gray-100 my-2" />
            <Link to="/login" onClick={() => setIsMobileMenuOpen(false)} className="text-base font-medium text-gray-700 py-2">Sign in</Link>
            <Link to="/signup" onClick={() => setIsMobileMenuOpen(false)} className="text-base font-medium text-center py-3 bg-black text-white rounded-xl mt-2 hover:bg-gray-800 transition-colors">
              Get started free
            </Link>
          </div>
        )}
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
              Your delivery process.
              <br />
              <span className="bg-gradient-to-r from-gray-900 via-gray-600 to-gray-400 bg-clip-text text-transparent">Fully visible.</span>
              <br />
              <span className="text-gray-500">Fully understood.</span>
            </h1>
            
            <p className="text-xl md:text-2xl text-gray-500 max-w-3xl mx-auto mb-12 leading-relaxed font-light">
              Track every customer request from start to finish — across every system, team, and partner involved
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
            <div className="bg-gradient-to-b from-gray-50 to-white rounded-t-3xl border border-gray-200 border-b-0 p-5 sm:p-8 md:p-12">
              <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 mb-8">
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
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 sm:gap-4">
                      <span className="font-medium text-gray-900 break-all">identity_verified</span>
                      <span className="text-gray-400 text-xs sm:text-right">your platform</span>
                    </div>
                    <div className="text-gray-500 text-xs mt-1">Completed in 1.2s</div>
                  </div>
                </div>
                
                <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-100 shadow-sm">
                  <div className="w-6 h-6 rounded-full bg-emerald-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Check className="w-3.5 h-3.5 text-emerald-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 sm:gap-4">
                      <span className="font-medium text-gray-900 break-all">credit_check_passed</span>
                      <span className="text-xs px-2 py-0.5 bg-blue-50 text-blue-600 rounded-full w-fit sm:text-right">partner: credit-bureau</span>
                    </div>
                    <div className="text-gray-500 text-xs mt-1">Completed in 3.4s</div>
                  </div>
                </div>
                
                <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-100 shadow-sm">
                  <div className="w-6 h-6 rounded-full bg-emerald-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Check className="w-3.5 h-3.5 text-emerald-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 sm:gap-4">
                      <span className="font-medium text-gray-900 break-all">documents_requested</span>
                      <span className="text-gray-400 text-xs sm:text-right">compliance-team</span>
                    </div>
                    <div className="text-gray-500 text-xs mt-1">Completed in 0.8s</div>
                  </div>
                </div>

                <div className="flex items-start gap-4 p-4 bg-gray-50 rounded-xl border border-dashed border-gray-200">
                  <div className="w-6 h-6 rounded-full bg-gray-200 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Clock className="w-3.5 h-3.5 text-gray-400" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 sm:gap-4">
                      <span className="font-medium text-gray-400 break-all">account_activated</span>
                      <span className="text-gray-300 text-xs sm:text-right">waiting</span>
                    </div>
                  </div>
                </div>
                
                <div className="flex items-start gap-4 p-4 bg-amber-50/50 rounded-xl border border-amber-200/50">
                  <div className="w-6 h-6 rounded-full bg-amber-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <AlertTriangle className="w-3.5 h-3.5 text-amber-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 sm:gap-4">
                      <span className="font-medium text-amber-700 break-all">SLA breach detected</span>
                      <span className="text-amber-600 text-xs font-medium sm:text-right">8 hours waiting</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Tagline */}
          <div className="mt-6 text-center mb-20">
            <p className="text-sm text-gray-400 italic">
              This is what Threadify sees.{" "}
              <span className="text-gray-600">For the first time, so can you.</span>
            </p>
          </div>

          {/* Divider */}
          <div className="max-w-4xl mx-auto">
            <hr className="border-gray-200" />
          </div>
        </div>
      </section>

      {/* Value Prop Banner - Header for Capture/Validate/React sections */}
      <section className="pt-12 pb-12 px-6 bg-white">
        <div className="max-w-5xl mx-auto text-center">
          <h2 className="text-3xl md:text-4xl lg:text-5xl font-semibold leading-tight tracking-tight text-black mb-4">
            From capture to intelligence.
            {/* <span className="bg-gradient-to-r from-gray-900 via-gray-600 to-gray-400 bg-clip-text text-transparent">
              Across every boundary.
            </span> */}
          </h2>
        </div>
      </section>

      {/* How It Works - Capture */}
      <section id="how-it-works" className="pt-4 pb-16 px-6 border-b border-gray-100">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            <div>
              <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-gray-100 text-sm font-medium text-gray-600 mb-6">
                <span className="w-5 h-5 rounded-full bg-black text-white text-xs flex items-center justify-center">1</span>
                Capture
              </div>
              <h2 className="text-4xl md:text-5xl font-semibold text-gray-900 tracking-tight mb-6 leading-[1.1]">
                Every request sets a delivery process in motion
              </h2>
              <div className="space-y-5 text-lg text-gray-600 leading-relaxed">
                <p>
                  Crossing services, teams, partners, and boundaries no single system can see end to end.
                </p>
                <p>
                  One line per business action. A live execution graph (a Thread) builds itself across every service involved.
                </p>
                <p className="text-black font-medium">
                  Stop guessing at the customer experience. Now you have real-time intelligence into exactly how you deliver for every customer.
                </p>
              </div>
            </div>
            <div>
              <CodeBlock
                title="Start tracking"
                headerColor="gray"
                code={`// Start tracking
const thread = await threadify.start();
thread.step("payment_captured")

// Add context
const step = thread.step("fraud_check")
step.addContext(data).success()`}
              />
              <div className="mt-6">
                <a href="https://docs.threadify.dev/core-concepts/tracking-workflows" className="inline-flex items-center gap-2 text-black font-medium hover:gap-3 transition-all">
                  Learn how to track service delivery
                  <ArrowRight className="w-4 h-4" />
                </a>
              </div>
            </div>
          </div>

          {/* OpenTelemetry Banner */}
          <div className="mt-16 bg-gradient-to-r from-gray-50 to-gray-100 border border-gray-200 rounded-2xl p-8 flex flex-col md:flex-row items-center justify-between gap-6 shadow-sm">
            <div className="space-y-2">
              <div className="flex flex-col items-start sm:flex-row sm:items-center gap-2 sm:gap-3">
                <span className="bg-black text-white text-[10px] sm:text-xs font-bold px-2 py-1 rounded uppercase tracking-wider whitespace-nowrap w-fit">Zero Code Changes</span>
                <h4 className="text-lg sm:text-xl font-semibold text-black leading-tight">Native OpenTelemetry Support</h4>
              </div>
              <p className="text-gray-600 text-lg max-w-2xl">
                Already instrumented with OTel? Drop in the Threadify Exporter to automatically convert your existing technical traces into business-level service delivery intelligence.
              </p>
            </div>
            <a 
              href="https://docs.threadify.dev/opentelemetry" 
              target="_blank"
              rel="noopener noreferrer"
              className="px-6 py-3 bg-white text-black border border-gray-200 rounded-lg font-medium hover:bg-gray-50 transition-colors whitespace-nowrap"
            >
              Read OTel Docs
            </a>
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
              <h2 className="text-4xl md:text-5xl font-semibold text-gray-900 tracking-tight mb-6 leading-[1.1]">
                Know whether it delivered correctly
              </h2>
              <div className="space-y-5 text-lg text-gray-600 leading-relaxed">
                <p>
                  Define what correct looks like - Contracts. Threadify validates every execution against it in real time.
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
              <h2 className="text-4xl md:text-5xl font-semibold text-gray-900 tracking-tight mb-6 leading-[1.1]">
                Build systems that respond intelligently
              </h2>
              <div className="space-y-5 text-lg text-gray-600 leading-relaxed mb-8">
                <p>
                  When a step gets skipped, the process stops before it goes further. When a payment stalls, an account is suspended automatically.
                </p>
                <p className="text-black font-medium">
                  Your system stops being reactive. It becomes proactive and intelligent.
                </p>
              </div>
              
              {/* Taglines */}
              <div className="flex flex-wrap gap-6">
                <div className="flex items-center gap-3">
                  <Users className="w-5 h-5 text-gray-400" />
                  <span className="text-sm font-medium text-gray-700">Proactive customer messaging</span>
                </div>
                <div className="flex items-center gap-3">
                  <Bot className="w-5 h-5 text-gray-400" />
                  <span className="text-sm font-medium text-gray-700">AI agents with full context</span>
                </div>
                <div className="flex items-center gap-3">
                  <Zap className="w-5 h-5 text-gray-400" />
                  <span className="text-sm font-medium text-gray-700">Circuit breakers that fire early</span>
                </div>
              </div>
            </div>
            
            {/* Code Example */}
            <CodeBlock
              title="Event Listeners"
              headerColor="gray"
              code={`// React to step completion
connection.subscribe('step.success', 'order_placed', (notification) => {
  console.log('Order placed:', notification.context);
  notification.ack();
});

// React to rule violations
connection.subscribe('rule.violated', 'payment_processed', (notification) => {
  console.log('Violation:', notification.severity);
  notification.ack();
});`}
            />
          </div>
        </div>
      </section>

      {/* Entity Profile Section */}
      <section className="py-32 px-6 bg-gray-50">
        <div className="max-w-4xl mx-auto text-center">
          <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-gray-100 text-sm font-medium text-gray-600 mb-6">
            <span className="w-5 h-5 rounded-full bg-black text-white text-xs flex items-center justify-center">4</span>
            Intelligence
          </div>
          <h2 className="text-4xl md:text-5xl font-semibold text-gray-900 tracking-tight mb-6 leading-[1.1]">
            Every thread is one moment. Every customer has many.
          </h2>
          <p className="text-xl text-gray-600 leading-relaxed mb-6">
            One thread tells you if a request succeeded. A hundred threads tell you if a customer is thriving. Threadify aggregates execution across every entity — customer, partner, feature — into an <span className="font-semibold text-gray-900">Entity Profile</span> so you can see the patterns that matter.
          </p>
          <p className="text-xl text-black font-medium">
            Intelligence from execution, not guesswork.
          </p>
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

            {/* Product & Analytics */}
            <div className="relative">
              <div className="absolute -left-4 top-0 w-1 h-full bg-gradient-to-b from-gray-800 to-transparent rounded-full"></div>
              <div className="pl-8">
                <h3 className="text-2xl font-bold mb-4 text-gray-900">Product & Analytics</h3>
                <p className="text-gray-600 leading-relaxed mb-6">
                  Build intelligence with entity profiles. Customer health. Feature performance. Partner reliability. All from real execution, not surveys.
                </p>
                <div className="bg-gray-50 rounded-xl p-6 border border-gray-200">
                  <div className="flex items-center gap-3 mb-3">
                    <TrendingUp className="w-5 h-5 text-gray-400" />
                    <span className="font-semibold text-gray-900">Intelligence from patterns</span>
                  </div>
                  <p className="text-sm text-gray-600">Who's thriving. Who's struggling. What's working.</p>
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

      {/* Cross-Boundary Section */}
      <section className="py-32 px-6 bg-gray-50">
        <div className="max-w-6xl mx-auto space-y-12">
          <div className="text-center space-y-8">
            <h2 className="text-4xl md:text-5xl font-semibold text-black tracking-tight leading-[1.1]">
              Your process doesn't stop at your boundary.
              <br />
              <span className="text-gray-500">Your intelligence shouldn't either.</span>
            </h2>
            <p className="text-lg text-gray-600 max-w-2xl mx-auto">
              Invite a partner into the thread. They instrument their side. One shared execution graph — their steps and yours, in one timeline.
            </p>
          </div>

          {/* Code Examples */}
          <div className="grid md:grid-cols-2 gap-8 max-w-5xl mx-auto">
            {/* Your API - Invite */}
            <CodeBlock
              title="Your API"
              headerColor="gray"
              code={`// Invite partner to thread
const invitation = await thread
  .inviteParty({
    role: "logistics",
    expiresIn: "48h"
  });

// Share token with partner
console.log(invitation.token);`}
            />

            {/* Partner API - Join */}
            <CodeBlock
              title="Partner API"
              headerColor="purple"
              code={`// Join thread with token
const thread = await connection
  .join(invitationToken);

// Record their steps
await thread.step('package_shipped')
  .addContext({ tracking: '1Z999' })
  .success();`}
            />
          </div>

          <div className="text-center space-y-4 pt-8">
            <p className="text-lg font-semibold text-gray-900">
              Service delivery doesn't stop at your boundary. Your visibility shouldn't either.
            </p>
            <p className="text-black font-bold text-2xl">One thread. Their steps and yours. Full picture.</p>
          </div>
        </div>
      </section>

      {/* Final CTA */}
      <section className="py-32 px-6 bg-gradient-to-b from-gray-50 to-white">
        <div className="max-w-4xl mx-auto text-center">
          <h2 className="text-5xl md:text-6xl font-bold mb-6">
            <span className="bg-gradient-to-r from-gray-900 via-gray-700 to-gray-900 bg-clip-text text-transparent">
              Service Delivery Intelligence.
            </span>
          </h2>
          <p className="text-xl text-gray-600 mb-10 max-w-2xl mx-auto">
            Built for teams where how you deliver is as important as what you deliver.
            No credit card required.
          </p>
          <div className="flex flex-wrap justify-center gap-4">
            <Link to="/signup" className="group px-10 py-5 bg-gray-900 text-white rounded-xl font-bold hover:bg-gray-800 transition-all shadow-xl hover:shadow-2xl text-lg flex items-center gap-2">
              Get Started Free
              <ArrowRight className="w-5 h-5 group-hover:translate-x-1 transition-transform" />
            </Link>
            <a href="https://docs.threadify.dev" className="px-10 py-5 bg-white text-gray-900 rounded-xl font-bold border-2 border-gray-200 hover:border-gray-300 transition-all text-lg">
              Read documentation
            </a>
          </div>
        </div>
      </section>

      <Footer />
    </div>
  );
}
