import { useEffect, useRef, useState } from "react";
import { Link } from "@remix-run/react";
import { ArrowDown, Play } from "lucide-react";

export default function HomePageStory() {
  const [scrollProgress, setScrollProgress] = useState(0);
  const [threadStep, setThreadStep] = useState(0);
  const [showThread, setShowThread] = useState(false);
  const hookSectionRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleScroll = () => {
      const scrolled = window.scrollY;
      const maxScroll = document.documentElement.scrollHeight - window.innerHeight;
      setScrollProgress(scrolled / maxScroll);
    };

    window.addEventListener("scroll", handleScroll);
    return () => window.removeEventListener("scroll", handleScroll);
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => setShowThread(true), 1500);
    return () => clearTimeout(timer);
  }, []);

  useEffect(() => {
    if (showThread && threadStep < 7) {
      const timer = setTimeout(() => setThreadStep(threadStep + 1), 800);
      return () => clearTimeout(timer);
    }
  }, [showThread, threadStep]);

  const threadSteps = [
    { symbol: "✓", text: "identity_verified", detail: "your platform", status: "success" },
    { symbol: "✓", text: "credit_check_passed", detail: "partner: credit-bureau", status: "success" },
    { symbol: "✓", text: "documents_requested", detail: "compliance-team", status: "success" },
    { symbol: "−", text: "account_activated", detail: "never ran", status: "missing" },
    { symbol: "−", text: "credit_facility_assigned", detail: "never ran", status: "missing" },
    { symbol: "−", text: "welcome_notification", detail: "never ran", status: "missing" },
    { symbol: "⚠", text: "still waiting", detail: "8 hours later", status: "warning" },
  ];

  return (
    <div className="story-container">
      {/* Progress indicator */}
      <div className="fixed top-0 left-0 w-full h-1 bg-slate-900/5 z-50">
        <div 
          className="h-full bg-blue-500 transition-all duration-300"
          style={{ width: `${scrollProgress * 100}%` }}
        />
      </div>

      {/* SECTION 1 — The Hook */}
      <section 
        ref={hookSectionRef}
        className="min-h-screen flex items-center justify-center relative overflow-hidden bg-black pt-16"
      >
        {/* Subtle grid background */}
        <div className="absolute inset-0 opacity-[0.03]">
          <div className="absolute inset-0" style={{
            backgroundImage: `
              linear-gradient(rgba(255,255,255,0.1) 1px, transparent 1px),
              linear-gradient(90deg, rgba(255,255,255,0.1) 1px, transparent 1px)
            `,
            backgroundSize: '50px 50px'
          }} />
        </div>

        <div className="max-w-5xl mx-auto px-6 relative z-10">
          <div className="text-center space-y-16">
            <h1 className="text-5xl md:text-7xl font-light text-white leading-tight tracking-tight fade-in-slow">
              Every customer request tells a story.
              <br />
              <span className="text-slate-400">Do you know yours?</span>
            </h1>

            {showThread && (
              <div className="thread-reveal max-w-2xl mx-auto">
                <div className="bg-slate-900/80 backdrop-blur-xl rounded-2xl border border-slate-700/50 p-8 md:p-12 shadow-2xl">
                  <div className="text-left space-y-8">
                    <div className="text-sm font-mono text-slate-500 tracking-wide">
                      Customer: ref_4821 · Account application
                    </div>

                    <div className="space-y-4">
                      {threadSteps.map((step, index) => (
                        <div
                          key={index}
                          className={`flex items-start gap-4 transition-all duration-700 ${
                            index < threadStep 
                              ? "opacity-100 translate-x-0" 
                              : "opacity-0 -translate-x-8"
                          }`}
                          style={{ transitionDelay: `${index * 100}ms` }}
                        >
                          <span
                            className={`text-xl font-mono flex-shrink-0 ${
                              step.status === "success"
                                ? "text-emerald-400"
                                : step.status === "missing"
                                ? "text-slate-600"
                                : "text-amber-400"
                            }`}
                          >
                            {step.symbol}
                          </span>
                          <div className="flex-1 min-w-0">
                            <span className={`font-mono text-base ${
                              step.status === "missing" ? "text-slate-600" : "text-slate-200"
                            }`}>
                              {step.text}
                            </span>
                            <span className="text-slate-500 text-sm ml-3">
                              · {step.detail}
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>

                {threadStep >= 7 && (
                  <div className="mt-12 space-y-3 text-center fade-in-delayed">
                    <p className="text-slate-400 text-lg">This is what Threadify sees.</p>
                    <p className="text-white text-2xl font-light">
                      For the first time, so can you.
                    </p>
                  </div>
                )}
              </div>
            )}
          </div>

          <div className="absolute bottom-12 left-1/2 -translate-x-1/2 animate-float">
            <ArrowDown className="w-6 h-6 text-slate-600" />
          </div>
        </div>
      </section>

      {/* SECTION 2 — The Hero Line */}
      <section className="min-h-screen flex items-center justify-center bg-white relative">
        <div className="absolute inset-0 bg-gradient-to-b from-black via-slate-900 to-white opacity-5" />
        
        <div className="max-w-6xl mx-auto px-6 text-center space-y-12 relative z-10">
          <div className="space-y-6">
            <h2 className="text-5xl md:text-7xl font-bold text-slate-900 leading-tight">
              The process crosses every boundary.
              <br />
              <span className="text-slate-400">The intelligence doesn't.</span>
              <br />
              <span className="text-blue-600">Until now.</span>
            </h2>
          </div>

          <div className="max-w-4xl mx-auto">
            <p className="text-xl md:text-2xl text-slate-600 leading-relaxed font-light">
              Threadify is a service delivery intelligence system — capturing and validating how your
              business delivers on every request, across every system, team, and boundary, and turning
              that into intelligence your teams, systems, and agents can act on.
            </p>
          </div>

          <div className="pt-8">
            <a
              href="#capture"
              className="inline-flex items-center gap-2 text-slate-400 hover:text-slate-900 transition-colors text-lg group"
            >
              See how it works 
              <ArrowDown className="w-5 h-5 group-hover:translate-y-1 transition-transform" />
            </a>
          </div>
        </div>
      </section>

      {/* SECTION 3 — Capture */}
      <section id="capture" className="py-32 px-6 bg-slate-50">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            {/* Video placeholder with unique design */}
            <div className="order-2 lg:order-1">
              <div className="relative aspect-[4/3] bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900 rounded-3xl overflow-hidden border border-slate-700 shadow-2xl group cursor-pointer">
                <div className="absolute inset-0 bg-gradient-to-br from-blue-500/10 to-purple-500/10" />
                
                {/* Animated grid */}
                <div className="absolute inset-0 opacity-20">
                  <div className="absolute inset-0" style={{
                    backgroundImage: `
                      linear-gradient(rgba(59,130,246,0.3) 1px, transparent 1px),
                      linear-gradient(90deg, rgba(59,130,246,0.3) 1px, transparent 1px)
                    `,
                    backgroundSize: '40px 40px'
                  }} />
                </div>

                {/* Play button */}
                <div className="absolute inset-0 flex items-center justify-center">
                  <div className="w-20 h-20 rounded-full bg-white/10 backdrop-blur-sm border border-white/20 flex items-center justify-center group-hover:scale-110 group-hover:bg-white/20 transition-all duration-300">
                    <Play className="w-8 h-8 text-white ml-1" fill="white" />
                  </div>
                </div>

                {/* Floating elements */}
                <div className="absolute top-8 left-8 px-4 py-2 bg-emerald-500/20 backdrop-blur-sm border border-emerald-500/30 rounded-lg text-emerald-300 text-sm font-mono">
                  step_recorded
                </div>
                <div className="absolute bottom-8 right-8 px-4 py-2 bg-blue-500/20 backdrop-blur-sm border border-blue-500/30 rounded-lg text-blue-300 text-sm font-mono">
                  live_execution
                </div>
              </div>
            </div>

            <div className="order-1 lg:order-2 space-y-8">
              <div>
                <h3 className="text-5xl font-bold text-slate-900 mb-4">Capture</h3>
                <div className="w-16 h-1 bg-blue-500" />
              </div>
              
              <div className="space-y-6 text-lg text-slate-700 leading-relaxed">
                <p>
                  Every customer request sets a delivery process in motion — crossing services,
                  teams, partners, and boundaries that no single system can see end to end.
                </p>
                <p>
                  Threadify instruments that process as it happens. Wrap your business steps with
                  our SDK — one line per action, AI assisted — and a live execution graph builds
                  itself across every service and boundary involved.
                </p>
                <p className="text-slate-900 font-medium">
                  Most teams discover their real delivery process looks nothing like the Confluence
                  doc.
                </p>
                <p className="text-blue-600 font-semibold text-xl">
                  Now you know what it actually is.
                </p>
              </div>
            </div>
          </div>

          {/* Connecting line */}
          <div className="flex justify-center py-16">
            <div className="w-px h-24 bg-gradient-to-b from-slate-300 via-slate-400 to-transparent" />
          </div>
        </div>
      </section>

      {/* SECTION 4 — Validate */}
      <section className="py-32 px-6 bg-white">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            <div className="space-y-8">
              <div>
                <h3 className="text-5xl font-bold text-slate-900 mb-4">Validate</h3>
                <div className="w-16 h-1 bg-purple-500" />
              </div>
              
              <div className="space-y-6 text-lg text-slate-700 leading-relaxed">
                <p>
                  Knowing what your delivery process does is one thing. Knowing whether it did it
                  correctly is another.
                </p>
                <p>
                  Define what correct looks like with a contract. Threadify validates every
                  execution against it in real time — the moment a step is skipped, a sequence
                  breaks, a timeout breaches, or a partner never responds.
                </p>
                <p className="text-slate-900 font-medium">
                  Not from a batch job. Not from a customer complaint.
                </p>
                <p className="text-purple-600 font-semibold text-xl">
                  The instant it happens.
                </p>
              </div>
            </div>

            {/* Validation visualization */}
            <div className="relative aspect-[4/3] bg-gradient-to-br from-purple-900 via-purple-800 to-slate-900 rounded-3xl overflow-hidden border border-purple-700 shadow-2xl group cursor-pointer">
              <div className="absolute inset-0 bg-gradient-to-br from-purple-500/10 to-pink-500/10" />
              
              {/* Contract visualization */}
              <div className="absolute inset-0 p-8 flex flex-col justify-center space-y-4">
                <div className="bg-white/5 backdrop-blur-sm border border-white/10 rounded-xl p-6 space-y-3">
                  <div className="flex items-center gap-3">
                    <div className="w-2 h-2 rounded-full bg-emerald-400" />
                    <span className="text-white/90 font-mono text-sm">kyc_check → approve_account</span>
                    <span className="text-emerald-400 text-xs ml-auto">✓ valid</span>
                  </div>
                  <div className="flex items-center gap-3">
                    <div className="w-2 h-2 rounded-full bg-red-400 animate-pulse" />
                    <span className="text-white/90 font-mono text-sm">activate_card</span>
                    <span className="text-red-400 text-xs ml-auto">✗ violation</span>
                  </div>
                </div>

                <div className="bg-red-500/10 backdrop-blur-sm border border-red-500/30 rounded-xl p-4">
                  <div className="text-red-300 font-mono text-sm">
                    Contract violation detected
                    <div className="text-red-400/60 text-xs mt-1">kyc_check was skipped</div>
                  </div>
                </div>
              </div>

              {/* Play button */}
              <div className="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity bg-black/20">
                <div className="w-20 h-20 rounded-full bg-white/10 backdrop-blur-sm border border-white/20 flex items-center justify-center">
                  <Play className="w-8 h-8 text-white ml-1" fill="white" />
                </div>
              </div>
            </div>
          </div>

          {/* Connecting arrow */}
          <div className="flex justify-center py-16">
            <div className="flex flex-col items-center">
              <div className="w-px h-16 bg-gradient-to-b from-slate-300 to-slate-400" />
              <ArrowDown className="w-6 h-6 text-slate-400 mt-2" />
            </div>
          </div>
        </div>
      </section>

      {/* SECTION 5 — Intelligence */}
      <section className="py-32 px-6 bg-slate-900 text-white">
        <div className="max-w-7xl mx-auto space-y-16">
          <div className="text-center max-w-4xl mx-auto">
            <h3 className="text-4xl md:text-5xl font-bold mb-6">
              Execution context that powers
              <br />
              <span className="text-slate-400">more than visibility</span>
            </h3>
          </div>

          <div className="grid lg:grid-cols-2 gap-8">
            {/* For your Teams */}
            <div className="group relative">
              <div className="absolute inset-0 bg-gradient-to-br from-blue-500/20 to-transparent rounded-3xl blur-xl group-hover:blur-2xl transition-all" />
              <div className="relative bg-slate-800/50 backdrop-blur-sm border border-slate-700 rounded-3xl p-10 hover:border-blue-500/50 transition-all">
                <div className="space-y-6">
                  <div>
                    <h4 className="text-3xl font-bold mb-2">For your Teams</h4>
                    <div className="w-12 h-1 bg-blue-500" />
                  </div>
                  
                  <div className="space-y-4 text-slate-300 leading-relaxed">
                    <p>
                      When a customer's request goes silent, your support team shouldn't have to wait for
                      an engineer to tell them what happened. When a process is stalling across your
                      delivery, your ops team shouldn't find out from a customer complaint. When your
                      product isn't delivering the way you designed it, your product team shouldn't be
                      guessing.
                    </p>
                    <p>
                      Ask Threadify what happened to any customer request — in plain English. Get the
                      full execution story instantly, across every service and team involved. No
                      engineering ticket. No log diving. No waiting.
                    </p>
                    <p className="text-blue-400 font-semibold text-lg pt-4">
                      Your teams know how you're delivering for every customer.
                    </p>
                  </div>
                </div>
              </div>
            </div>

            {/* For your Systems and Agents */}
            <div className="group relative">
              <div className="absolute inset-0 bg-gradient-to-br from-purple-500/20 to-transparent rounded-3xl blur-xl group-hover:blur-2xl transition-all" />
              <div className="relative bg-slate-800/50 backdrop-blur-sm border border-slate-700 rounded-3xl p-10 hover:border-purple-500/50 transition-all">
                <div className="space-y-6">
                  <div>
                    <h4 className="text-3xl font-bold mb-2">For your Systems and Agents</h4>
                    <div className="w-12 h-1 bg-purple-500" />
                  </div>
                  
                  <div className="space-y-4 text-slate-300 leading-relaxed">
                    <p>
                      Your AI agent knows exactly what has already happened before deciding what to do
                      next — no guessing, no acting on incomplete context.
                    </p>
                    <p>
                      And when something goes wrong mid-process, your systems don't wait to be told. They
                      listen. A required step gets skipped — a workflow breaks before it goes further. A
                      payment stalls — an account is suspended automatically. A process goes silent — your
                      customer gets a message before they even notice.
                    </p>
                    <p className="text-purple-400 font-semibold text-lg pt-4">
                      Your agents know how to deliver for every customer.
                      <br />
                      Your systems fix delivery processes the moment they go wrong.
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* SECTION 6 — Building Smart Systems */}
      <section className="py-32 px-6 bg-gradient-to-b from-slate-900 to-black text-white">
        <div className="max-w-5xl mx-auto text-center space-y-12">
          <div className="space-y-6">
            <h3 className="text-5xl md:text-6xl font-bold leading-tight">
              Build systems that know
              <br />
              <span className="text-transparent bg-clip-text bg-gradient-to-r from-blue-400 to-purple-400">
                what they're doing
              </span>
            </h3>
          </div>

          <div className="space-y-6 text-xl text-slate-300 leading-relaxed max-w-3xl mx-auto">
            <p>
              Most systems react to what they receive. Threadify gives your systems awareness of what
              they're doing — the full execution context of every delivery process in flight.
            </p>
            <p>
              Proactive customer messaging when a process stalls. Circuit breakers that fire before
              failures cascade. Workflow coordination across teams and partners. AI agents that act
              with full delivery context rather than guessing from partial information.
            </p>
            <p className="text-blue-400 font-semibold text-2xl pt-6">
              Your system stops being reactive. It becomes intelligent.
            </p>
          </div>
        </div>
      </section>

      {/* SECTION 7 — Cross Boundary */}
      <section className="py-32 px-6 bg-white">
        <div className="max-w-5xl mx-auto text-center space-y-12">
          <div className="space-y-6">
            <h3 className="text-5xl md:text-6xl font-bold text-slate-900 leading-tight">
              Your process doesn't stop at your boundary.
              <br />
              <span className="text-blue-600">Your intelligence shouldn't either.</span>
            </h3>
          </div>

          <div className="space-y-6 text-xl text-slate-700 leading-relaxed max-w-3xl mx-auto">
            <p>
              Invite a partner into the thread. Their services instrument their side. You get one
              shared execution graph — their steps and yours, in the same coherent timeline.
            </p>
            <p className="text-slate-900 font-semibold">
              No more "send me your logs." No more cross-company finger pointing.
            </p>
            <p className="text-blue-600 font-bold text-2xl">One thread. Full picture.</p>
          </div>

          {/* Visual representation */}
          <div className="pt-12">
            <div className="inline-flex items-center gap-4 px-8 py-4 bg-slate-50 border-2 border-slate-200 rounded-2xl">
              <div className="flex items-center gap-2">
                <div className="w-3 h-3 rounded-full bg-blue-500" />
                <span className="font-mono text-sm text-slate-600">your-org</span>
              </div>
              <div className="text-slate-400">→</div>
              <div className="flex items-center gap-2">
                <div className="w-3 h-3 rounded-full bg-purple-500" />
                <span className="font-mono text-sm text-slate-600">partner-org</span>
              </div>
              <div className="text-slate-400">→</div>
              <div className="flex items-center gap-2">
                <div className="w-3 h-3 rounded-full bg-emerald-500" />
                <span className="font-mono text-sm text-slate-600">shared-thread</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* CLOSING */}
      <section className="py-32 px-6 bg-black text-white">
        <div className="max-w-4xl mx-auto text-center space-y-16">
          <div className="space-y-8">
            <h2 className="text-5xl md:text-7xl font-light leading-tight">
              Now you know how you're delivering
              <br />
              <span className="text-slate-400">for every customer.</span>
            </h2>
          </div>

          <div className="space-y-4">
            <p className="text-4xl font-bold">Threadify.</p>
            <p className="text-xl text-slate-400">
              Service delivery intelligence for how your business delivers.
            </p>
          </div>

          {/* CTA */}
          <div className="flex flex-col sm:flex-row gap-4 justify-center pt-8">
            <Link
              to="/u/getting-started"
              className="group inline-flex items-center justify-center gap-2 px-8 py-4 bg-white text-black rounded-xl hover:bg-slate-100 transition-all text-lg font-semibold shadow-xl hover:shadow-2xl hover:scale-105"
            >
              Get started
              <span className="group-hover:translate-x-1 transition-transform">→</span>
            </Link>
            <a
              href="https://docs.threadify.com"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center justify-center gap-2 px-8 py-4 bg-transparent text-white border-2 border-slate-700 rounded-xl hover:border-slate-500 transition-all text-lg font-semibold"
            >
              Read the docs
            </a>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="py-12 px-6 bg-black border-t border-slate-800">
        <div className="max-w-7xl mx-auto">
          <div className="flex flex-col md:flex-row justify-between items-center gap-6">
            <div className="flex gap-8 text-slate-500">
              <Link to="/" className="hover:text-white transition-colors font-semibold">
                Threadify
              </Link>
              <a
                href="https://docs.threadify.com"
                target="_blank"
                rel="noopener noreferrer"
                className="hover:text-white transition-colors"
              >
                Docs
              </a>
              <a
                href="https://docs.threadify.com/mcp-server"
                target="_blank"
                rel="noopener noreferrer"
                className="hover:text-white transition-colors"
              >
                MCP Server
              </a>
              <a
                href="https://docs.threadify.com/ai-assistant"
                target="_blank"
                rel="noopener noreferrer"
                className="hover:text-white transition-colors"
              >
                AI Assistant Guide
              </a>
            </div>
            <div className="text-slate-600 text-sm">
              © {new Date().getFullYear()} Threadify. All rights reserved.
            </div>
          </div>
        </div>
      </footer>
    </div>
  );
}
