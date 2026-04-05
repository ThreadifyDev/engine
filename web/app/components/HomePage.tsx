import { useEffect, useRef, useState } from "react";
import { Link } from "@remix-run/react";
import { ArrowDown, ArrowRight, CheckCircle2, XCircle, Clock, AlertTriangle } from "lucide-react";

export default function HomePage() {
  const [threadVisible, setThreadVisible] = useState(false);
  const [stepIndex, setStepIndex] = useState(0);

  useEffect(() => {
    const timer = setTimeout(() => setThreadVisible(true), 1000);
    return () => clearTimeout(timer);
  }, []);

  useEffect(() => {
    if (threadVisible && stepIndex < 7) {
      const timer = setTimeout(() => setStepIndex(stepIndex + 1), 600);
      return () => clearTimeout(timer);
    }
  }, [threadVisible, stepIndex]);

  const threadSteps = [
    { icon: CheckCircle2, text: "identity_verified", service: "your platform", status: "success" },
    { icon: CheckCircle2, text: "credit_check_passed", service: "partner: credit-bureau", status: "success" },
    { icon: CheckCircle2, text: "documents_requested", service: "compliance-team", status: "success" },
    { icon: XCircle, text: "account_activated", service: "never ran", status: "failed" },
    { icon: XCircle, text: "credit_facility_assigned", service: "never ran", status: "failed" },
    { icon: XCircle, text: "welcome_notification", service: "never ran", status: "failed" },
    { icon: AlertTriangle, text: "still waiting", service: "8 hours later", status: "warning" },
  ];

  return (
    <div className="min-h-screen bg-gradient-to-b from-slate-50 to-white">
      {/* SECTION 1 — The Hook */}
      <section className="min-h-screen flex flex-col items-center justify-center px-6 py-20">
        <div className="max-w-4xl mx-auto text-center space-y-12">
          <h1 className="text-5xl md:text-7xl font-bold text-slate-900 leading-tight">
            Every customer request tells a story.
            <br />
            <span className="text-slate-600">Do you know yours?</span>
          </h1>

          {threadVisible && (
            <div className="bg-white rounded-2xl shadow-2xl p-8 md:p-12 border border-slate-200 animate-fade-in">
              <div className="text-left space-y-6">
                <div className="text-sm text-slate-500 font-mono">
                  Customer: ref_4821 · Account application
                </div>

                <div className="space-y-3">
                  {threadSteps.map((step, index) => {
                    const Icon = step.icon;
                    const isVisible = index < stepIndex;
                    
                    return (
                      <div
                        key={index}
                        className={`flex items-start gap-3 transition-all duration-500 ${
                          isVisible ? "opacity-100 translate-y-0" : "opacity-0 translate-y-4"
                        }`}
                      >
                        <Icon
                          className={`w-5 h-5 mt-0.5 flex-shrink-0 ${
                            step.status === "success"
                              ? "text-green-500"
                              : step.status === "failed"
                              ? "text-red-500"
                              : "text-amber-500"
                          }`}
                        />
                        <div className="flex-1">
                          <span className="font-mono text-slate-900">{step.text}</span>
                          <span className="text-slate-400 text-sm ml-2">· {step.service}</span>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>
          )}

          {stepIndex >= 7 && (
            <div className="space-y-4 animate-fade-in">
              <p className="text-slate-600 text-lg">This is what Threadify sees.</p>
              <p className="text-slate-900 text-xl font-semibold">
                For the first time, so can you.
              </p>
            </div>
          )}
        </div>

        <div className="absolute bottom-12 animate-bounce">
          <ArrowDown className="w-8 h-8 text-slate-400" />
        </div>
      </section>

      {/* SECTION 2 — The Hero Line */}
      <section className="py-32 px-6 bg-slate-900 text-white">
        <div className="max-w-5xl mx-auto text-center space-y-8">
          <h2 className="text-4xl md:text-6xl font-bold leading-tight">
            The process crosses every boundary.
            <br />
            The intelligence doesn't.
            <br />
            <span className="text-blue-400">Until now.</span>
          </h2>
          <p className="text-xl md:text-2xl text-slate-300 max-w-4xl mx-auto leading-relaxed">
            Threadify is a service delivery intelligence system — capturing and validating how your
            business delivers on every request, across every system, team, and boundary, and turning
            that into intelligence your teams, systems, and agents can act on.
          </p>
          <div className="pt-8">
            <a
              href="#capture"
              className="inline-flex items-center gap-2 text-blue-400 hover:text-blue-300 transition-colors text-lg"
            >
              See how it works <ArrowDown className="w-5 h-5" />
            </a>
          </div>
        </div>
      </section>

      {/* SECTION 3 — Capture */}
      <section id="capture" className="py-32 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="bg-white rounded-2xl shadow-xl border border-slate-200 overflow-hidden">
            <div className="grid md:grid-cols-2 gap-12 p-12">
              <div className="bg-slate-100 rounded-xl flex items-center justify-center min-h-[400px]">
                <div className="text-slate-400 text-center">
                  <div className="w-24 h-24 mx-auto mb-4 bg-slate-200 rounded-lg flex items-center justify-center">
                    <svg className="w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                  </div>
                  <p className="text-sm">Video: Capture in action</p>
                </div>
              </div>
              <div className="flex flex-col justify-center space-y-6">
                <h3 className="text-4xl font-bold text-slate-900">Capture</h3>
                <div className="space-y-4 text-lg text-slate-700 leading-relaxed">
                  <p>
                    Every customer request sets a delivery process in motion — crossing services,
                    teams, partners, and boundaries that no single system can see end to end.
                  </p>
                  <p>
                    Threadify instruments that process as it happens. Wrap your business steps with
                    our SDK — one line per action, AI assisted — and a live execution graph builds
                    itself across every service and boundary involved.
                  </p>
                  <p className="font-semibold text-slate-900">
                    Most teams discover their real delivery process looks nothing like the Confluence
                    doc.
                  </p>
                  <p className="text-blue-600 font-semibold">Now you know what it actually is.</p>
                </div>
              </div>
            </div>
          </div>
          <div className="flex justify-center py-8">
            <div className="w-0.5 h-16 bg-gradient-to-b from-slate-300 to-transparent"></div>
          </div>
        </div>
      </section>

      {/* SECTION 4 — Validate */}
      <section className="py-32 px-6 bg-slate-50">
        <div className="max-w-7xl mx-auto">
          <div className="bg-white rounded-2xl shadow-xl border border-slate-200 overflow-hidden">
            <div className="grid md:grid-cols-2 gap-12 p-12">
              <div className="bg-slate-100 rounded-xl flex items-center justify-center min-h-[400px]">
                <div className="text-slate-400 text-center">
                  <div className="w-24 h-24 mx-auto mb-4 bg-slate-200 rounded-lg flex items-center justify-center">
                    <svg className="w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                  </div>
                  <p className="text-sm">Video: Validation in action</p>
                </div>
              </div>
              <div className="flex flex-col justify-center space-y-6">
                <h3 className="text-4xl font-bold text-slate-900">Validate</h3>
                <div className="space-y-4 text-lg text-slate-700 leading-relaxed">
                  <p>
                    Knowing what your delivery process does is one thing. Knowing whether it did it
                    correctly is another.
                  </p>
                  <p>
                    Define what correct looks like with a contract. Threadify validates every
                    execution against it in real time — the moment a step is skipped, a sequence
                    breaks, a timeout breaches, or a partner never responds.
                  </p>
                  <p className="font-semibold text-slate-900">
                    Not from a batch job. Not from a customer complaint.
                  </p>
                  <p className="text-blue-600 font-semibold">The instant it happens.</p>
                </div>
              </div>
            </div>
          </div>
          <div className="flex justify-center py-8">
            <ArrowDown className="w-8 h-8 text-slate-400" />
          </div>
        </div>
      </section>

      {/* SECTION 5 — Intelligence */}
      <section className="py-32 px-6">
        <div className="max-w-7xl mx-auto space-y-16">
          <div className="text-center">
            <h3 className="text-4xl md:text-5xl font-bold text-slate-900 mb-4">
              Execution context that powers more than visibility
            </h3>
          </div>

          <div className="grid md:grid-cols-2 gap-12">
            {/* For your Teams */}
            <div className="bg-gradient-to-br from-blue-50 to-white rounded-2xl p-10 border border-blue-100 shadow-lg">
              <h4 className="text-3xl font-bold text-slate-900 mb-6">For your Teams</h4>
              <div className="space-y-4 text-slate-700 leading-relaxed">
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
                <p className="text-blue-600 font-semibold text-lg pt-4">
                  Your teams know how you're delivering for every customer.
                </p>
              </div>
            </div>

            {/* For your Systems and Agents */}
            <div className="bg-gradient-to-br from-purple-50 to-white rounded-2xl p-10 border border-purple-100 shadow-lg">
              <h4 className="text-3xl font-bold text-slate-900 mb-6">For your Systems and Agents</h4>
              <div className="space-y-4 text-slate-700 leading-relaxed">
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
                <p className="text-purple-600 font-semibold text-lg pt-4">
                  Your agents know how to deliver for every customer.
                  <br />
                  Your systems fix delivery processes the moment they go wrong.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* SECTION 6 — Building Smart Systems */}
      <section className="py-32 px-6 bg-slate-900 text-white">
        <div className="max-w-5xl mx-auto space-y-8">
          <h3 className="text-4xl md:text-5xl font-bold mb-6">
            Build systems that know what they're doing
          </h3>
          <div className="space-y-4 text-xl text-slate-300 leading-relaxed">
            <p>
              Most systems react to what they receive. Threadify gives your systems awareness of what
              they're doing — the full execution context of every delivery process in flight.
            </p>
            <p>
              Proactive customer messaging when a process stalls. Circuit breakers that fire before
              failures cascade. Workflow coordination across teams and partners. AI agents that act
              with full delivery context rather than guessing from partial information.
            </p>
            <p className="text-blue-400 font-semibold text-2xl pt-4">
              Your system stops being reactive. It becomes intelligent.
            </p>
          </div>
        </div>
      </section>

      {/* SECTION 7 — Cross Boundary */}
      <section className="py-32 px-6 bg-gradient-to-br from-blue-50 to-purple-50">
        <div className="max-w-5xl mx-auto text-center space-y-8">
          <h3 className="text-4xl md:text-5xl font-bold text-slate-900 leading-tight">
            Your process doesn't stop at your boundary.
            <br />
            <span className="text-blue-600">Your intelligence shouldn't either.</span>
          </h3>
          <div className="space-y-4 text-xl text-slate-700 leading-relaxed max-w-3xl mx-auto">
            <p>
              Invite a partner into the thread. Their services instrument their side. You get one
              shared execution graph — their steps and yours, in the same coherent timeline.
            </p>
            <p className="font-semibold text-slate-900">
              No more "send me your logs." No more cross-company finger pointing.
            </p>
            <p className="text-blue-600 font-bold text-2xl">One thread. Full picture.</p>
          </div>
        </div>
      </section>

      {/* CLOSING */}
      <section className="py-32 px-6 bg-white">
        <div className="max-w-4xl mx-auto text-center space-y-12">
          <h2 className="text-5xl md:text-6xl font-bold text-slate-900 leading-tight">
            Now you know how you're delivering
            <br />
            for every customer.
          </h2>
          <div className="space-y-4">
            <p className="text-3xl font-bold text-slate-900">Threadify.</p>
            <p className="text-xl text-slate-600">
              Service delivery intelligence for how your business delivers.
            </p>
          </div>

          {/* CTA */}
          <div className="flex flex-col sm:flex-row gap-4 justify-center pt-8">
            <Link
              to="/u/getting-started"
              className="inline-flex items-center justify-center gap-2 px-8 py-4 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors text-lg font-semibold shadow-lg hover:shadow-xl"
            >
              Get started <ArrowRight className="w-5 h-5" />
            </Link>
            <a
              href="https://docs.threadify.com"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center justify-center gap-2 px-8 py-4 bg-white text-slate-900 border-2 border-slate-300 rounded-lg hover:border-slate-400 transition-colors text-lg font-semibold"
            >
              Read the docs <ArrowRight className="w-5 h-5" />
            </a>
          </div>
        </div>
      </section>

      {/* Footer Navigation */}
      <footer className="py-12 px-6 bg-slate-900 text-white border-t border-slate-800">
        <div className="max-w-7xl mx-auto">
          <div className="flex flex-col md:flex-row justify-between items-center gap-6">
            <div className="flex gap-8 text-slate-400">
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
            <div className="text-slate-500 text-sm">
              © {new Date().getFullYear()} Threadify. All rights reserved.
            </div>
          </div>
        </div>
      </footer>
    </div>
  );
}
