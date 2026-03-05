import { useEffect, useState } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { api } from '~/lib/api';
import Nav from '~/components/homepage/Nav';
import LiveThreadDemo from '~/components/homepage/LiveThreadDemo';
import FeatureSection from '~/components/homepage/FeatureSection';
import Footer from '~/components/homepage/Footer';
import { Zap, Link2, Lock, Radio, MessageSquare, Clock, Database, ShieldCheck, Webhook, Brain, Route, ChevronDown } from 'lucide-react';

export const meta: MetaFunction = () => {
  return [
    { title: "Threadify - Real-Time Execution Graph Infrastructure" },
    { name: "description", content: "Understand how your systems execute customer requests" },
  ];
};

export default function Index() {
  const navigate = useNavigate();
  const [openFaqIndex, setOpenFaqIndex] = useState<number | null>(null);

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
      <section className="relative bg-white text-black py-12 lg:py-16 overflow-hidden">
        {/* Background Pattern */}
        <div className="absolute inset-0 opacity-[0.03]">
          <svg className="w-full h-full" xmlns="http://www.w3.org/2000/svg">
            <defs>
              <pattern id="hexagons" x="0" y="0" width="100" height="87" patternUnits="userSpaceOnUse">
                <path d="M50 0L93.3 25L93.3 62L50 87L6.7 62L6.7 25Z" fill="none" stroke="currentColor" strokeWidth="1"/>
              </pattern>
            </defs>
            <rect width="100%" height="100%" fill="url(#hexagons)" />
          </svg>
        </div>
        
        {/* Thread/Node Icons */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="absolute top-20 left-10 w-16 h-16 opacity-5">
            <svg viewBox="0 0 24 24" fill="currentColor">
              <circle cx="12" cy="12" r="8" />
              <circle cx="12" cy="12" r="3" fill="white" />
            </svg>
          </div>
          <div className="absolute top-40 right-20 w-12 h-12 opacity-5">
            <svg viewBox="0 0 24 24" fill="currentColor">
              <circle cx="12" cy="12" r="8" />
              <circle cx="12" cy="12" r="3" fill="white" />
            </svg>
          </div>
          <div className="absolute bottom-32 left-1/4 w-10 h-10 opacity-5">
            <svg viewBox="0 0 24 24" fill="currentColor">
              <circle cx="12" cy="12" r="8" />
              <circle cx="12" cy="12" r="3" fill="white" />
            </svg>
          </div>
          <div className="absolute top-1/2 right-1/3 w-14 h-14 opacity-5">
            <svg viewBox="0 0 24 24" fill="currentColor">
              <circle cx="12" cy="12" r="8" />
              <circle cx="12" cy="12" r="3" fill="white" />
            </svg>
          </div>
          {/* Connection lines */}
          <svg className="absolute inset-0 w-full h-full opacity-5" xmlns="http://www.w3.org/2000/svg">
            <line x1="10%" y1="20%" x2="80%" y2="40%" stroke="currentColor" strokeWidth="1" strokeDasharray="4 4" />
            <line x1="25%" y1="60%" x2="70%" y2="50%" stroke="currentColor" strokeWidth="1" strokeDasharray="4 4" />
          </svg>
        </div>

        <div className="max-w-7xl mx-auto px-6 relative z-10">
          <div className="grid lg:grid-cols-5 gap-8 items-center">
            <div className="lg:col-span-2">
              <h1 className="text-4xl lg:text-5xl font-bold mb-6 leading-tight">
                Threadify is the layer
                <br />
                your stack is missing.
              </h1>

              <p className="text-base lg:text-lg text-gray-600 mb-6 leading-relaxed">
                Real-time execution graph infrastructure that tracks what your business process actually did — across every service, every team, every boundary.
              </p>

              <div className="flex gap-4">
                <button onClick={() => navigate('/signup')} className="px-6 py-3 bg-black text-white font-semibold rounded-lg hover:bg-gray-800 transition">
                  Get Started
                </button>
                <a href="https://docs.threadify.dev" target="_blank" rel="noopener noreferrer" className="px-6 py-3 border border-gray-300 text-black font-semibold rounded-lg hover:border-gray-500 transition inline-block">
                  View Docs
                </a>
              </div>
            </div>

            {/* Video Placeholder - Now takes 3 columns */}
            <div className="lg:col-span-3">
              <div className="w-full">
                <div className="relative aspect-video bg-gradient-to-br from-indigo-100 via-purple-50 to-pink-100 rounded-2xl shadow-2xl overflow-hidden border border-gray-200">
                  <div className="absolute inset-0 flex items-center justify-center">
                    <div className="text-center">
                      <div className="w-28 h-28 mx-auto mb-6 rounded-full bg-black/10 backdrop-blur-sm flex items-center justify-center hover:bg-black/20 transition cursor-pointer group">
                        <svg className="w-12 h-12 text-gray-700 group-hover:scale-110 transition" fill="currentColor" viewBox="0 0 20 20">
                          <path d="M6.3 2.841A1.5 1.5 0 004 4.11V15.89a1.5 1.5 0 002.3 1.269l9.344-5.89a1.5 1.5 0 000-2.538L6.3 2.84z" />
                        </svg>
                      </div>
                      <p className="text-gray-700 font-semibold text-xl">Watch Demo</p>
                      <p className="text-sm text-gray-500 mt-2">See Threadify in action</p>
                    </div>
                  </div>
                  {/* Decorative elements */}
                  <div className="absolute top-8 right-8 w-40 h-40 bg-purple-300/30 rounded-full blur-3xl"></div>
                  <div className="absolute bottom-8 left-8 w-48 h-48 bg-blue-300/30 rounded-full blur-3xl"></div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* See it for yourself tagline */}
      <section className="bg-gray-50 py-12 px-6">
        <div className="max-w-7xl mx-auto text-center">
          <h2 className="text-2xl lg:text-3xl font-bold text-black">
            See it for yourself
          </h2>
        </div>
      </section>

      {/* Chapter 1 - The Blind Spot */}
      <section className="relative bg-white py-24 px-6 overflow-hidden">
        {/* Thread/Web Pattern Background */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          {/* Web nodes */}
          <div className="absolute top-10 left-20 w-3 h-3 bg-gray-900 rounded-full opacity-[0.04]"></div>
          <div className="absolute top-32 left-40 w-2 h-2 bg-gray-900 rounded-full opacity-[0.04]"></div>
          <div className="absolute top-20 right-32 w-3 h-3 bg-gray-900 rounded-full opacity-[0.04]"></div>
          <div className="absolute top-48 right-20 w-2 h-2 bg-gray-900 rounded-full opacity-[0.04]"></div>
          <div className="absolute bottom-40 left-32 w-3 h-3 bg-gray-900 rounded-full opacity-[0.04]"></div>
          <div className="absolute bottom-20 right-40 w-2 h-2 bg-gray-900 rounded-full opacity-[0.04]"></div>
          <div className="absolute top-1/2 left-1/4 w-2 h-2 bg-gray-900 rounded-full opacity-[0.04]"></div>
          <div className="absolute top-1/3 right-1/3 w-3 h-3 bg-gray-900 rounded-full opacity-[0.04]"></div>
          
          {/* Web connecting lines */}
          <svg className="absolute inset-0 w-full h-full opacity-[0.03]" xmlns="http://www.w3.org/2000/svg">
            <line x1="10%" y1="8%" x2="20%" y2="25%" stroke="currentColor" strokeWidth="1" />
            <line x1="20%" y1="25%" x2="25%" y2="50%" stroke="currentColor" strokeWidth="1" />
            <line x1="10%" y1="8%" x2="80%" y2="15%" stroke="currentColor" strokeWidth="1" />
            <line x1="80%" y1="15%" x2="85%" y2="35%" stroke="currentColor" strokeWidth="1" />
            <line x1="25%" y1="50%" x2="50%" y2="50%" stroke="currentColor" strokeWidth="1" />
            <line x1="50%" y1="50%" x2="70%" y2="40%" stroke="currentColor" strokeWidth="1" />
            <line x1="70%" y1="40%" x2="85%" y2="35%" stroke="currentColor" strokeWidth="1" />
            <line x1="25%" y1="50%" x2="15%" y2="75%" stroke="currentColor" strokeWidth="1" />
            <line x1="15%" y1="75%" x2="80%" y2="85%" stroke="currentColor" strokeWidth="1" />
            <line x1="80%" y1="85%" x2="85%" y2="35%" stroke="currentColor" strokeWidth="1" strokeDasharray="3 3" />
          </svg>
        </div>
        
        <div className="max-w-7xl mx-auto relative z-10">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            {/* Left: Terminal visualization */}
            <div className="flex justify-center">
              <div className="bg-white rounded-lg border border-gray-200 shadow-lg overflow-hidden font-mono text-sm w-full max-w-lg">
                <div className="bg-gray-100 px-4 py-2 flex gap-2 items-center">
                  <div className="w-3 h-3 rounded-full bg-red-500"></div>
                  <div className="w-3 h-3 rounded-full bg-yellow-500"></div>
                  <div className="w-3 h-3 rounded-full bg-green-500"></div>
                </div>
                <div className="p-6 space-y-2">
                  <div className="flex items-start gap-2">
                    <span className="text-green-600 flex-shrink-0">✓</span>
                    <span className="text-gray-700 flex-1">authorize_payment($24/.50)</span>
                    <span className="text-gray-500 text-xs flex-shrink-0">payment-svc · 142ms</span>
                  </div>
                  <div className="flex items-start gap-2">
                    <span className="text-green-600 flex-shrink-0">✓</span>
                    <span className="text-gray-700 flex-1">fraud_check()</span>
                    <span className="text-gray-500 text-xs flex-shrink-0">risk-svc · 89ms</span>
                  </div>
                  <div className="flex items-start gap-2">
                    <span className="text-gray-400 flex-shrink-0">−</span>
                    <span className="text-gray-400 flex-1">fulfill_order()</span>
                    <span className="text-gray-400 text-xs flex-shrink-0">never recorded</span>
                  </div>
                  <div className="flex items-start gap-2">
                    <span className="text-gray-400 flex-shrink-0">−</span>
                    <span className="text-gray-400 flex-1">notify_customer()</span>
                    <span className="text-gray-400 text-xs flex-shrink-0">never recorded</span>
                  </div>
                  <div className="mt-4 pt-4 border-t border-gray-200">
                    <div className="flex items-start gap-2">
                      <span className="text-yellow-600 flex-shrink-0">⚠</span>
                      <span className="text-yellow-600 flex-1 text-xs">process went silent after step 2 · thread:order-8821</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {/* Right: Story content */}
            <div>
              <div className="text-sm font-semibold tracking-wider mb-6 text-gray-600">
                CHAPTER 01 · THE BLIND SPOT
              </div>
              
              <h2 className="text-5xl font-bold mb-6 leading-tight text-black">
                It returned 200.
                <br />
                The order never arrived.
              </h2>

              <div className="space-y-4 text-gray-600 leading-relaxed">
                <p>
                  The payment API responded. No exceptions anywhere in the stack. Grafana is green. But fulfillment never ran — and nothing caught it because your tools were watching your infrastructure, not your business process.
                </p>
                <p>
                  Threadify instruments the layer between your services. The business steps your logs were never designed to track.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Chapter 2 - Instrument & Discover */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            {/* Left: Execution graph visualization */}
            <div className="flex justify-center">
              <div className="bg-gradient-to-br from-blue-50 to-indigo-50 rounded-lg border border-blue-200 shadow-lg p-8 w-full max-w-md">
                <div className="space-y-4">
                  <div className="flex items-center">
                    <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                      create_account
                      <div className="text-xs text-gray-600 mt-1">auth-svc</div>
                    </div>
                  </div>
                  <div className="ml-8 flex items-center">
                    <div className="w-px h-6 bg-gray-300"></div>
                  </div>
                  <div className="ml-8 flex items-center">
                    <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                      verify_email
                      <div className="text-xs text-gray-600 mt-1">comms-svc</div>
                    </div>
                  </div>
                  <div className="ml-8 flex items-center">
                    <div className="w-px h-6 bg-gray-300"></div>
                  </div>
                  <div className="ml-8 flex items-center">
                    <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                      provision_workspace
                      <div className="text-xs text-gray-600 mt-1">infra-svc</div>
                    </div>
                  </div>
                  <div className="ml-8 flex items-center">
                    <div className="w-px h-6 bg-gray-300"></div>
                  </div>
                  <div className="ml-8 flex items-center">
                    <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                      assign_plan
                      <div className="text-xs text-gray-600 mt-1">billing-svc</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {/* Right: Story content */}
            <div>
              <div className="text-sm font-semibold tracking-wider mb-6 text-gray-600">
                CHAPTER 02 · INSTRUMENT & DISCOVER
              </div>
              
              <h2 className="text-5xl font-bold mb-6 leading-tight text-black">
                Instrument once.
                <br />
                The graph emerges.
              </h2>

              <div className="space-y-4 text-gray-600 leading-relaxed">
                <p>
                  No schema to define. No process documentation required. Wrap your business steps with the Threadify SDK and a live execution graph builds itself — across every service, in real time.
                </p>
                <p>
                  Most teams discover their real process looks nothing like the Confluence doc. Now you know what it actually is.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Chapter 3 - Validate */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            {/* Left: Story content */}
            <div>
              <div className="text-sm font-semibold tracking-wider mb-6 text-gray-600">
                CHAPTER 03 · VALIDATE
              </div>
              
              <h2 className="text-5xl font-bold mb-6 leading-tight text-black">
                Now you can see it.
                <br />
                Now you can validate it.
              </h2>

              <div className="space-y-4 text-gray-600 leading-relaxed">
                <p>
                  Write a contract — define the correct sequence — and Threadify validates every execution against it in real time. The moment a step runs out of order, gets skipped, or violates a rule, you know instantly.
                </p>
                <p>
                  Not from a batch job. Not from a customer ticket. The instant it happens.
                </p>
              </div>
            </div>

            {/* Right: Contract validation visualization */}
            <div className="flex justify-center">
              <div className="bg-white rounded-lg border border-gray-200 shadow-lg p-8 w-full max-w-md">
                <div className="space-y-6">
                  <div className="border border-dashed border-gray-300 rounded p-4">
                    <div className="text-xs text-gray-500 font-mono mb-2">contract.yml</div>
                    <div className="space-y-1 text-sm font-mono text-gray-700">
                      <div>steps:</div>
                      <div className="ml-4">- kyc_check</div>
                      <div className="ml-4">- approve_account</div>
                      <div className="ml-4">- activate_card</div>
                      <div className="ml-4">- notify_customer</div>
                    </div>
                  </div>
                  
                  <div className="space-y-3">
                    <div className="flex items-start gap-3">
                      <div className="px-3 py-1.5 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono flex-1">
                        kyc_check → approve_account
                      </div>
                      <span className="text-green-600 text-xs flex-shrink-0 mt-1.5">✓ sequence valid</span>
                    </div>
                    
                    <div className="flex items-start gap-3">
                      <div className="px-3 py-1.5 bg-red-100 border border-red-300 rounded text-red-700 text-sm font-mono flex-1">
                        activate_card
                      </div>
                      <span className="text-red-600 text-xs flex-shrink-0 mt-1.5">✗ kyc_check skipped</span>
                    </div>
                    
                    <div className="mt-4 p-3 bg-red-50 border border-red-200 rounded">
                      <div className="text-red-700 text-sm font-mono">
                        violation detected
                        <div className="text-xs text-gray-500 mt-1">thread:onboard-4421</div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* AI Thought Section */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-4xl mx-auto text-center">
          <div className="text-sm font-semibold tracking-wider mb-6 text-gray-600">
            A THOUGHT
          </div>
          
          <h2 className="text-5xl font-bold mb-6 leading-tight text-black">
            Built it with AI?
            <br />
            Now make sure it actually works.
          </h2>

          <p className="text-xl text-gray-600 leading-relaxed max-w-2xl mx-auto mb-8">
            Vibe coded apps run. They just don't always do what you think. Instrument with Threadify and see exactly what your AI-generated system is doing in production — before your users find out.
          </p>

          <button onClick={() => navigate('/signup')} className="px-6 py-3 bg-black text-white font-semibold rounded-lg hover:bg-gray-800 transition">
            Validate Your AI App
          </button>
        </div>
      </section>

      {/* Chapter 4 - Cross-Boundary */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            {/* Left: Cross-org visualization */}
            <div className="flex justify-center">
              <div className="space-y-4 w-full max-w-md">
                <div className="border border-dashed border-gray-300 rounded-lg p-6 bg-white shadow-lg">
                  <div className="text-xs text-gray-600 font-mono mb-4">your org</div>
                  <div className="space-y-3">
                    <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                      place_order
                      <div className="text-xs text-gray-600 mt-1">orders-svc</div>
                    </div>
                    <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                      payment_captured
                      <div className="text-xs text-gray-600 mt-1">payment-svc</div>
                    </div>
                    <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                      notify_customer
                      <div className="text-xs text-gray-600 mt-1">comms-svc</div>
                    </div>
                  </div>
                </div>

                <div className="border border-dashed border-blue-300 rounded-lg p-6 bg-white shadow-lg">
                  <div className="text-xs text-blue-600 font-mono mb-4">logistics partner</div>
                  <div className="space-y-3">
                    <div className="px-4 py-2 bg-blue-100 border border-blue-300 rounded text-blue-700 text-sm font-mono">
                      fulfill_order
                      <div className="text-xs text-gray-600 mt-1">warehouse-svc</div>
                    </div>
                    <div className="px-4 py-2 bg-blue-100 border border-blue-300 rounded text-blue-700 text-sm font-mono">
                      dispatch_courier
                      <div className="text-xs text-gray-600 mt-1">logistics-svc</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {/* Right: Story content */}
            <div>
              <div className="text-sm font-semibold tracking-wider mb-6 text-gray-600">
                CHAPTER 04 · CROSS-BOUNDARY
              </div>
              
              <h2 className="text-5xl font-bold mb-6 leading-tight text-black">
                Your process doesn't stop
                <br />
                at your API boundary.
              </h2>

              <div className="space-y-4 text-gray-600 leading-relaxed">
                <p>
                  Invite a partner into the thread. Their services instrument their side. You get one shared execution graph across organizational boundaries — their steps and yours, in the same coherent timeline.
                </p>
                <p>
                  No more "send me your logs." No more cross-company finger pointing. One thread. Full picture.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Chapter 5 - React */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            {/* Left: Story content */}
            <div>
              <div className="text-sm font-semibold tracking-wider mb-6 text-gray-600">
                CHAPTER 05 · REACT
              </div>
              
              <h2 className="text-5xl font-bold mb-6 leading-tight text-black">
                Don't just watch it.
                <br />
                Take action on it.
              </h2>

              <div className="space-y-4 text-gray-600 leading-relaxed">
                <p>
                  Wire up actions to process state. When a trial expires and payment fails, suspend the account and send a proactive message — automatically, the moment the process says so.
                </p>
                <p>
                  Your system stops being reactive. It becomes context-aware.
                </p>
              </div>
            </div>

            {/* Right: Reaction flow visualization */}
            <div className="flex justify-center">
              <div className="bg-gradient-to-br from-blue-50 to-indigo-50 rounded-lg border border-blue-200 shadow-lg p-8 w-full max-w-md">
                <div className="space-y-4">
                  <div className="px-4 py-2 bg-yellow-100 border border-yellow-300 rounded text-yellow-700 text-sm font-mono">
                    suspend_account()
                    <div className="text-xs text-gray-600 mt-1">access blocked</div>
                  </div>

                  <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                    payment_failed
                    <div className="text-xs text-gray-600 mt-1">billing-svc · stalled</div>
                  </div>

                  <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                    notify_customer()
                    <div className="text-xs text-gray-600 mt-1">proactive message sent</div>
                  </div>

                  <div className="px-4 py-2 bg-cyan-100 border border-cyan-300 rounded text-cyan-700 text-sm font-mono">
                    alert_oncall()
                    <div className="text-xs text-gray-600 mt-1">pagerduty triggered</div>
                  </div>

                  <div className="px-4 py-2 bg-green-100 border border-green-300 rounded text-green-700 text-sm font-mono">
                    your_system
                    <div className="text-xs text-gray-600 mt-1">context-aware</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Chapter 6 - LLM Context */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-16 items-center">
            {/* Left: LLM visualization */}
            <div className="flex justify-center">
              <div className="bg-white rounded-lg border border-gray-200 shadow-lg p-8 w-full max-w-md">
                <div className="space-y-6">
                  <div className="border border-dashed border-gray-300 rounded p-4">
                    <div className="text-xs text-gray-500 font-mono mb-2">execution graph</div>
                    <div className="space-y-2">
                      <div className="text-sm font-mono text-gray-700 flex items-center gap-2">
                        <span className="text-green-600">✓</span> place_order
                      </div>
                      <div className="text-sm font-mono text-gray-700 flex items-center gap-2">
                        <span className="text-green-600">✓</span> fraud_check
                      </div>
                      <div className="text-sm font-mono text-gray-400 flex items-center gap-2">
                        <span className="text-gray-400">−</span> fulfill_order
                      </div>
                    </div>
                  </div>

                  <div className="px-4 py-3 bg-blue-50 border border-blue-200 rounded">
                    <div className="text-xs text-blue-600 font-mono mb-2">llm agent</div>
                    <div className="text-sm text-gray-700">
                      "why did this order stall?"
                      <div className="text-xs text-gray-500 mt-2">"fulfill_order never recorded after fraud_check passed"</div>
                    </div>
                  </div>

                  <div className="flex gap-2">
                    <div className="px-3 py-1.5 bg-green-100 border border-green-300 rounded text-green-700 text-xs font-mono">
                      MCP
                    </div>
                    <div className="px-3 py-1.5 bg-green-600 rounded text-white text-xs font-mono">
                      fix bug
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {/* Right: Story content */}
            <div>
              <div className="text-sm font-semibold tracking-wider mb-6 text-gray-600">
                CHAPTER 06 · LLM CONTEXT
              </div>
              
              <h2 className="text-5xl font-bold mb-6 leading-tight text-black">
                Give your AI agents
                <br />
                full execution context.
              </h2>

              <div className="space-y-4 text-gray-600 leading-relaxed">
                <p>
                  Threadify's MCP server exposes your execution graph to any LLM agent. Instead of reasoning from raw logs, your agent sees exactly what the business process did — every step, every outcome, across every service involved.
                </p>
                <p>
                  Ask it what happened. Ask it why it failed. Ask it what should happen next. It knows.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Final CTA Section */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-4xl mx-auto text-center">
          <div className="text-sm font-semibold tracking-wider mb-6 text-gray-600">
            READY TO INSTRUMENT
          </div>
          
          <h2 className="text-5xl font-bold mb-6 leading-tight text-black">
            See your first execution graph
            <br />
            in under 5 minutes.
          </h2>

          <p className="text-xl text-gray-600 mb-12">
            No schema · No contracts required · Just instrument and go
          </p>

          <div className="flex gap-4 justify-center">
            <a href="https://docs.threadify.dev" target="_blank" rel="noopener noreferrer" className="px-6 py-3 bg-black text-white font-semibold rounded-lg hover:bg-gray-800 transition inline-block">
              Read the Docs
            </a>
            <button onClick={() => navigate('/signup')} className="px-6 py-3 border border-gray-300 text-black font-semibold rounded-lg hover:border-gray-500 transition">
              Get Started
            </button>
          </div>
        </div>
      </section>

      {/* Use Cases Section */}
      <section className="bg-white py-24 px-6">
        <div className="max-w-7xl mx-auto">
          {/* Section Header/Tagline */}
          <div className="text-center mb-16">
            <h2 className="text-4xl lg:text-5xl font-bold text-black mb-4 leading-tight">
              Built for intelligent systems.
            </h2>
            <p className="text-lg text-gray-600 max-w-2xl mx-auto">
              Turn execution into intelligence to answer questions instantly, prevent failures, and build smarter systems.
            </p>
          </div>

          {/* Cards Grid */}
          <div className="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-5 gap-4">

            {/* Card 1: Capture customer journeys */}
            <div className="bg-gradient-to-br from-blue-50 to-indigo-50 rounded-lg p-5 border border-blue-100">
              <div className="w-10 h-10 rounded-lg bg-blue-500 flex items-center justify-center mb-3">
                <Route className="w-5 h-5 text-white" />
              </div>
              <h3 className="text-sm font-semibold text-gray-900 mb-1">Capture Customer Journeys</h3>
              <p className="text-xs text-gray-600 leading-relaxed">See exactly how your system delivers on every customer request, step by step across all services.</p>
            </div>

            {/* Card 2: Catch business logic violations */}
            <div className="bg-gradient-to-br from-cyan-50 to-blue-50 rounded-lg p-5 border border-cyan-100">
              <div className="w-10 h-10 rounded-lg bg-cyan-500 flex items-center justify-center mb-3">
                <ShieldCheck className="w-5 h-5 text-white" />
              </div>
              <h3 className="text-sm font-semibold text-gray-900 mb-1">Catch Violations Instantly</h3>
              <p className="text-xs text-gray-600 leading-relaxed">Validate your execution workflow in real-time and catch business logic violations the moment they happen.</p>
            </div>

            {/* Card 3: Build context-aware systems */}
            <div className="bg-gradient-to-br from-purple-50 to-pink-50 rounded-lg p-5 border border-purple-100">
              <div className="w-10 h-10 rounded-lg bg-purple-500 flex items-center justify-center mb-3">
                <Brain className="w-5 h-5 text-white" />
              </div>
              <h3 className="text-sm font-semibold text-gray-900 mb-1">Build Context-Aware Systems</h3>
              <p className="text-xs text-gray-600 leading-relaxed">React intelligently to violations and state changes with full awareness of what your process is doing.</p>
            </div>

            {/* Card 4: Resolve support tickets */}
            <div className="bg-gradient-to-br from-indigo-50 to-blue-50 rounded-lg p-5 border border-indigo-100">
              <div className="w-10 h-10 rounded-lg bg-indigo-500 flex items-center justify-center mb-3">
                <MessageSquare className="w-5 h-5 text-white" />
              </div>
              <h3 className="text-sm font-semibold text-gray-900 mb-1">Resolve Tickets Faster</h3>
              <p className="text-xs text-gray-600 leading-relaxed">Empower support teams to diagnose issues and answer questions without waiting for engineering.</p>
            </div>

            {/* Card 5: Analyze execution patterns */}
            <div className="bg-gradient-to-br from-emerald-50 to-teal-50 rounded-lg p-5 border border-emerald-100">
              <div className="w-10 h-10 rounded-lg bg-emerald-500 flex items-center justify-center mb-3">
                <Database className="w-5 h-5 text-white" />
              </div>
              <h3 className="text-sm font-semibold text-gray-900 mb-1">Empower Your LLMs</h3>
              <p className="text-xs text-gray-600 leading-relaxed">Give your AI agents complete execution context so they can make better decisions for you.</p>
            </div>
          </div>
        </div>
      </section>

      {/* Footer */}
      <Footer />
    </div>
  );
}
