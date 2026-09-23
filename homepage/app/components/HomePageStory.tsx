import { Link } from "@remix-run/react";
import ThreadifyLogo from "~/components/ThreadifyLogo";
import Footer from "~/components/homepage/Footer";
import CodeBlock from "~/components/CodeBlock";
import profileExample from "~/data/entity-profile-workload.json";
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
            <Link to="https://blog.threadify.dev" onClick={() => setIsMobileMenuOpen(false)} className="text-base font-medium text-gray-700 py-2">Our Blog</Link>
            <Link to="/pricing" onClick={() => setIsMobileMenuOpen(false)} className="text-base font-medium text-gray-700 py-2">Pricing</Link>
            <hr className="border-gray-100 my-2" />
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
              <span className="text-sm font-medium text-gray-700">Execution intelligence · Early access</span>
            </div>
            
            <h1 className="text-5xl sm:text-6xl md:text-7xl lg:text-[5.5rem] font-semibold text-black leading-[1.08] tracking-tight max-w-5xl mx-auto mb-8">
              Your workflows.
              <br />
              <span className="bg-gradient-to-r from-gray-900 via-gray-600 to-gray-400 bg-clip-text text-transparent">A shared referee.</span>
              <br />
              <span className="text-gray-500">Every step explained.</span>
            </h1>
            
            <p className="text-xl md:text-2xl text-gray-500 max-w-3xl mx-auto mb-12 leading-relaxed font-light">
              Keep your workflows where they are. Threadify follows work across your services and AI agents, checks your rules, and helps you decide what happens next.
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
                <div className="text-sm text-gray-400 font-mono">ref_4821 · Customer Refund</div>
              </div>
              
              <div className="space-y-4 font-mono text-sm">
                <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-100 shadow-sm">
                  <div className="w-6 h-6 rounded-full bg-emerald-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Check className="w-3.5 h-3.5 text-emerald-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 sm:gap-4">
                      <span className="font-medium text-gray-900 break-all">refund_requested</span>
                      <span className="text-gray-400 text-xs sm:text-right">support agent</span>
                    </div>
                    <div className="text-gray-500 text-xs mt-1">Customer request recorded</div>
                  </div>
                </div>
                
                <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-100 shadow-sm">
                  <div className="w-6 h-6 rounded-full bg-emerald-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Check className="w-3.5 h-3.5 text-emerald-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 sm:gap-4">
                      <span className="font-medium text-gray-900 break-all">payment_verified</span>
                      <span className="text-xs px-2 py-0.5 bg-blue-50 text-blue-600 rounded-full w-fit sm:text-right">payment service</span>
                    </div>
                    <div className="text-gray-500 text-xs mt-1">Payment reference matches the order</div>
                  </div>
                </div>
                
                <div className="flex items-start gap-4 p-4 bg-white rounded-xl border border-gray-100 shadow-sm">
                  <div className="w-6 h-6 rounded-full bg-emerald-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Check className="w-3.5 h-3.5 text-emerald-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 sm:gap-4">
                      <span className="font-medium text-gray-900 break-all">refund_prepared</span>
                      <span className="text-gray-400 text-xs sm:text-right">your backend</span>
                    </div>
                    <div className="text-gray-500 text-xs mt-1">Refund details recorded</div>
                  </div>
                </div>

                <div className="flex items-start gap-4 p-4 bg-gray-50 rounded-xl border border-dashed border-gray-200">
                  <div className="w-6 h-6 rounded-full bg-gray-200 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <Clock className="w-3.5 h-3.5 text-gray-400" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 sm:gap-4">
                      <span className="font-medium text-gray-400 break-all">refund_issued</span>
                      <span className="text-gray-300 text-xs sm:text-right">awaiting approval</span>
                    </div>
                  </div>
                </div>
                
                <div className="flex items-start gap-4 p-4 bg-amber-50/50 rounded-xl border border-amber-200/50">
                  <div className="w-6 h-6 rounded-full bg-amber-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                    <AlertTriangle className="w-3.5 h-3.5 text-amber-600" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 sm:gap-4">
                      <span className="font-medium text-amber-700 break-all">Approval required</span>
                      <span className="text-amber-600 text-xs font-medium sm:text-right">Rule not satisfied</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Tagline */}
          <div className="mt-6 text-center mb-20">
            <p className="text-sm text-gray-400 italic">
              One request. Several systems.{" "}
              <span className="text-gray-600">A shared record of what happened.</span>
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
            Keep your workflows. Add a referee.
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
                Follow the work
              </div>
              <h2 className="text-4xl md:text-5xl font-semibold text-gray-900 tracking-tight mb-6 leading-[1.1]">
                See the work across every handoff
              </h2>
              <div className="space-y-5 text-lg text-gray-600 leading-relaxed">
                <p>
                  A customer asks for a refund. An agent picks it up, your backend checks the order, and a payment service handles the money.
                </p>
                <p>
                  Threadify brings the steps you record into one shared timeline, called a thread. See who did what, what happened next, and where work is waiting.
                </p>
                <p className="text-black font-medium">
                  Your services and agents keep doing the work. Threadify follows it across them.
                </p>
              </div>
            </div>
            <div>
              <CodeBlock
                title="Start tracking"
                headerColor="gray"
                code={`// Follow a refund across services
const thread = await connection.thread("Refund-4821", { label: "Refund-4821" });
await thread.step("refund_requested")
  .addContext(data).success();`}
              />
              <div className="mt-6">
                <a href="https://docs.threadify.dev/core-concepts/tracking-workflows" className="inline-flex items-center gap-2 text-black font-medium hover:gap-3 transition-all">
                  Learn how to follow a workflow
                  <ArrowRight className="w-4 h-4" />
                </a>
              </div>
            </div>
          </div>

          {/* OpenTelemetry Banner */}
          <div className="mt-16 bg-gradient-to-r from-gray-50 to-gray-100 border border-gray-200 rounded-2xl p-8 flex flex-col md:flex-row items-center justify-between gap-6 shadow-sm">
            <div className="space-y-2">
              <div className="flex flex-col items-start sm:flex-row sm:items-center gap-2 sm:gap-3">
                <span className="bg-black text-white text-[10px] sm:text-xs font-bold px-2 py-1 rounded uppercase tracking-wider whitespace-nowrap w-fit">Use your existing traces</span>
                <h4 className="text-lg sm:text-xl font-semibold text-black leading-tight">Native OpenTelemetry Support</h4>
              </div>
              <p className="text-gray-600 text-lg">
                Already using OpenTelemetry? Send your traces to Threadify to bring the recorded steps of your workflows together.
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
                  <span className="text-sm text-gray-600">Approval recorded before refund</span>
                  <Check className="w-4 h-4 text-emerald-500" />
                </div>
                <div className="flex items-center justify-between p-3 bg-white rounded-lg border border-gray-100">
                  <span className="text-sm text-gray-600">Payment matches the original order</span>
                  <Check className="w-4 h-4 text-emerald-500" />
                </div>
                <div className="flex items-center justify-between p-3 bg-white rounded-lg border border-gray-100">
                  <span className="text-sm text-gray-600">Payment reference has the right format</span>
                  <Check className="w-4 h-4 text-emerald-500" />
                </div>
                <div className="flex items-center justify-between p-3 bg-red-50 rounded-lg border border-red-200">
                  <span className="text-sm font-medium text-red-700">Retry needs a fresh approval</span>
                  <AlertTriangle className="w-4 h-4 text-red-500" />
                </div>
              </div>
            </div>
            <div className="order-1 lg:order-2">
              <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-gray-100 text-sm font-medium text-gray-600 mb-6">
                <span className="w-5 h-5 rounded-full bg-black text-white text-xs flex items-center justify-center">2</span>
                Check the rules
              </div>
              <h2 className="text-4xl md:text-5xl font-semibold text-gray-900 tracking-tight mb-6 leading-[1.1]">
                Know if the work followed your rules
              </h2>
              <div className="space-y-5 text-lg text-gray-600 leading-relaxed">
                <p>
                  Put the important rules in a contract: a refund needs approval, an amount must be positive, or a payment reference must match the original order.
                </p>
                <p>
                  Threadify checks the recorded steps and their details against those rules. See what passed, what failed, and the evidence behind each result.
                </p>
                <p className="text-black font-medium">
                  The same rules follow the work across services, partners, and AI agents.
                </p>
                <div className="mt-6 p-4 bg-gray-50 rounded-lg border border-gray-100">
                  <p className="text-gray-600 text-sm">
                    <span className="font-semibold text-gray-900">Start with one important rule.</span> Use readable contracts to describe approvals, valid details, and the order of steps. Add more checks as you learn.
                  </p>
                </div>
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
                Decide what happens next
              </div>
              <h2 className="text-4xl md:text-5xl font-semibold text-gray-900 tracking-tight mb-6 leading-[1.1]">
                Choose where work needs a check
              </h2>
              <div className="space-y-5 text-lg text-gray-600 leading-relaxed mb-8">
                <p>
                  Start by watching for rule violations and notifying your team. For sensitive actions, have your application ask Threadify whether the next step is allowed before it proceeds.
                </p>
                <p className="text-black font-medium">
                  You choose the response. Your application carries it out.
                </p>
              </div>
              
              {/* Taglines */}
              <div className="flex flex-wrap gap-6">
                <div className="flex items-center gap-3">
                  <Users className="w-5 h-5 text-gray-400" />
                  <span className="text-sm font-medium text-gray-700">Notify the right team</span>
                </div>
                <div className="flex items-center gap-3">
                  <Bot className="w-5 h-5 text-gray-400" />
                  <span className="text-sm font-medium text-gray-700">Give agents the history</span>
                </div>
                <div className="flex items-center gap-3">
                  <Zap className="w-5 h-5 text-gray-400" />
                  <span className="text-sm font-medium text-gray-700">Require checks before actions</span>
                </div>
              </div>
            </div>
            
            {/* Code Example */}
            <CodeBlock
              title="Check before issuing a refund"
              headerColor="gray"
              code={`// Your application waits for permission.
await thread.waitFor("refund_issued");

// It then performs the action.
await issueRefund();

// Record the outcome and await validation.
await thread.step("refund_issued")
  .success("Refund issued", { waitFor: true });`}
            />
          </div>
        </div>
      </section>

      {/* Entity Profile Section */}
      <section id="execution-history" className="py-32 px-6 bg-gray-50">
        <div className="max-w-4xl mx-auto text-center">
          <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-gray-100 text-sm font-medium text-gray-600 mb-6">
            <span className="w-5 h-5 rounded-full bg-black text-white text-xs flex items-center justify-center">4</span>
            Learn from execution
          </div>
          <h2 className="text-4xl md:text-5xl font-semibold text-gray-900 tracking-tight mb-6 leading-[1.1]">
            Give the next decision some history.
          </h2>
          <p className="text-xl text-gray-600 leading-relaxed mb-6">
            Has this agent missed an approval before? Where does this partner’s work keep getting stuck? An <span className="font-semibold text-gray-900">Entity Profile</span> connects their past runs so your team can see which rules held, which failed, and what needs attention.
          </p>
          <p className="text-xl text-black font-medium">
            Use that history to review mistakes and improve the next run.
          </p>


          <div className="mt-12 w-full rounded-2xl overflow-hidden border border-gray-200 shadow-2xl bg-white transition-transform hover:shadow-3xl">
            <div className="p-6 sm:p-8 text-left" aria-label="Example entity profile for a support agent">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-6 border-b border-gray-100">
                <div className="flex items-center gap-3">
                  <div className="w-11 h-11 rounded-xl bg-gray-100 flex items-center justify-center shrink-0">
                    <Bot className="w-5 h-5 text-gray-700" aria-hidden="true" />
                  </div>
                  <div>
                    <p className="text-xs text-gray-500 mb-1">Entity Profile · Support agent</p>
                    <h3 className="text-lg font-semibold text-gray-900">Refund assistant</h3>
                  </div>
                </div>
                <span className="text-xs font-medium text-gray-500 bg-gray-50 border border-gray-200 rounded-full px-3 py-1 w-fit">Generated from a synthetic workload</span>
              </div>
              <div className="flex flex-wrap gap-x-6 gap-y-2 py-5 text-sm text-gray-500">
                <span><strong className="text-gray-900">{profileExample.counts.runs.toLocaleString("en-GB")}</strong> recorded runs</span>
                <span><strong className="text-gray-900">{profileExample.counts.steps.toLocaleString("en-GB")}</strong> recorded steps</span>
                <span><strong className="text-amber-700">{profileExample.counts.violations}</strong> runs flagged for missing approval</span>
              </div>
              <iframe
                src="https://player.mux.com/qX87EoG4o8zpQULQJ3OPYJPGU2K2GATmFyCLzaLPPus?metadata-video-title=Entity+Profile+Walkthrough&video-title=Entity+Profile+Walkthrough"
                title="Entity Profile walkthrough: metric configuration, delivery health and execution history"
                loading="lazy"
                allow="accelerometer; gyroscope; encrypted-media; picture-in-picture; fullscreen"
                allowFullScreen
                className="block w-full rounded-xl border border-gray-200 bg-gray-50"
                style={{ aspectRatio: "53 / 62" }}
              />
              <p className="mt-6 pt-5 border-t border-gray-100 text-sm text-gray-600 leading-relaxed">
                <span className="font-medium text-gray-900">Completion isn’t the whole story.</span> Every run eventually completed, but {profileExample.counts.violations} had already reported a refund without approval. The history keeps those mistakes visible, even after recovery.
              </p>
              <a href="/examples/entity-profile-workload.json" className="inline-flex items-center gap-2 mt-4 text-sm font-medium text-gray-900 underline underline-offset-4 hover:text-gray-600">
                View the test evidence <ArrowRight className="w-4 h-4" aria-hidden="true" />
              </a>
            </div>
          </div>
        </div>
      </section>

      {/* Use Cases Section */}
      <section id="product" className="py-24 px-6 bg-white">
        <div className="max-w-7xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-4xl md:text-5xl font-bold mb-4">
              <span className="bg-gradient-to-r from-gray-900 to-gray-600 bg-clip-text text-transparent">
                One execution history. Useful across your team.
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
                  Follow a request across services and partners. See where it stopped, which rule failed, and what happened before it.
                </p>
                <div className="bg-gray-50 rounded-xl p-6 border border-gray-200">
                  <div className="flex items-center gap-3 mb-3">
                    <TrendingUp className="w-5 h-5 text-gray-400" />
                    <span className="font-semibold text-gray-900">Evidence in one place</span>
                  </div>
                  <p className="text-sm text-gray-600">Explain an outcome without piecing together separate logs.</p>
                </div>
              </div>
            </div>

            {/* Product & Engineering */}
            <div className="relative">
              <div className="absolute -left-4 top-0 w-1 h-full bg-gradient-to-b from-gray-800 to-transparent rounded-full"></div>
              <div className="pl-8">
                <h3 className="text-2xl font-bold mb-4 text-gray-900">Product & Engineering</h3>
                <p className="text-gray-600 leading-relaxed mb-6">
                  Understand where workflows break down, which rules fail repeatedly, and how outcomes change over time.
                </p>
                <div className="bg-gray-50 rounded-xl p-6 border border-gray-200">
                  <div className="flex items-center gap-3 mb-3">
                    <TrendingUp className="w-5 h-5 text-gray-400" />
                    <span className="font-semibold text-gray-900">Improve how work runs</span>
                  </div>
                  <p className="text-sm text-gray-600">Find recurring problems across customers, services, and partners.</p>
                </div>
              </div>
            </div>

            {/* Systems & Agents */}
            <div className="relative">
              <div className="absolute -left-4 top-0 w-1 h-full bg-gradient-to-b from-gray-700 to-transparent rounded-full"></div>
              <div className="pl-8">
                <h3 className="text-2xl font-bold mb-4 text-gray-900">Systems & Agents</h3>
                <p className="text-gray-600 leading-relaxed mb-6">
                  Give services and agents the recorded history behind a decision. Have them check whether the next step is allowed before continuing.
                </p>
                <div className="bg-gray-50 rounded-xl p-6 border border-gray-200">
                  <div className="flex items-center gap-3 mb-3">
                    <Zap className="w-5 h-5 text-gray-400" />
                    <span className="font-semibold text-gray-900">Shared rules</span>
                  </div>
                  <p className="text-sm text-gray-600">Apply consistent checks across the systems doing the work.</p>
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
              Shared work needs shared evidence.
              <br />
              <span className="text-gray-500">Across every handoff.</span>
            </h2>
            <p className="text-lg text-gray-600 max-w-2xl mx-auto">
              A reviewer records an approval. A service carries out the next step. Invite participants into the same thread so their actions and rule checks remain part of one record.
            </p>
          </div>

          {/* Code Examples */}
          <div className="grid md:grid-cols-2 gap-8 max-w-5xl mx-auto">
            {/* Your API - Invite */}
            <CodeBlock
              title="Your API"
              headerColor="gray"
              code={`// Invite a reviewer to the thread
const invitation = await thread
  .inviteParty({
    role: "reviewer",
    expiresIn: "48h"
  });

// Share the invitation with the reviewer
console.log(invitation.token);`}
              />

            {/* Partner API - Join */}
            <CodeBlock
              title="Reviewer service"
              headerColor="purple"
              code={`// Join thread with token
const thread = await connection
  .join(invitationToken);

// Record the approval
await thread.step('approval')
  .addContext({ payment_reference: 'PAY-12345678' })
  .success();`}
            />
          </div>

          <div className="text-center space-y-4 pt-8">
            <p className="text-lg font-semibold text-gray-900">
              Keep the handoff, the decision, and the outcome connected.
            </p>
            <p className="text-black font-bold text-2xl">Their steps and yours. One shared thread.</p>
          </div>
        </div>
      </section>

      {/* Final CTA */}
      <section className="py-32 px-6 bg-gradient-to-b from-gray-50 to-white">
        <div className="max-w-4xl mx-auto text-center">
          <h2 className="text-5xl md:text-6xl font-bold mb-6">
            <span className="bg-gradient-to-r from-gray-900 via-gray-700 to-gray-900 bg-clip-text text-transparent">
              A shared referee for your workflows.
            </span>
          </h2>
          <p className="text-xl text-gray-600 mb-10 max-w-2xl mx-auto">
            Follow what happens. Check the rules. Decide when work can continue.
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
