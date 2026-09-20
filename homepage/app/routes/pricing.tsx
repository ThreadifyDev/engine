import type { MetaFunction } from "@remix-run/node";
import { Link } from "@remix-run/react";
import ThreadifyLogo from "~/components/ThreadifyLogo";
import Footer from "~/components/homepage/Footer";

export const meta: MetaFunction = () => [
  { title: "Licensing — Threadify" },
  { name: "description", content: "Run a shared workflow referee on your own infrastructure. Get a Threadify license to follow execution, check rules, and keep the evidence." },
];

export default function Pricing() {
  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b border-gray-100">
        <nav className="max-w-6xl mx-auto px-6 py-5 flex items-center justify-between" aria-label="Main navigation">
          <Link to="/" aria-label="Threadify home"><ThreadifyLogo /></Link>
          <a href="https://docs.threadify.dev" className="text-sm text-gray-600 hover:text-black">Documentation</a>
        </nav>
      </header>
      <main className="max-w-3xl mx-auto px-6 py-24 flex-1">
        <p className="text-sm font-medium text-gray-500 uppercase tracking-widest">Threadify licensing</p>
        <h1 className="text-4xl sm:text-5xl font-bold tracking-tight mt-5">Your workflows. Your rules. Your infrastructure.</h1>
        <p className="text-lg text-gray-600 mt-6 leading-relaxed">Start with a Dev license. Bring the work your services and agents already do into one record, then choose the rules you want Threadify to check.</p>
        <ol className="mt-10 space-y-4 list-decimal pl-5 text-gray-700">
          <li>Create your account and verify your email.</li>
          <li>Run Threadify on your own infrastructure with your license.</li>
          <li>Connect a service or agent, follow one workflow, and add its rules.</li>
        </ol>
        <div className="flex flex-wrap items-center gap-5 mt-10">
          <Link to="/signup" className="px-6 py-3 rounded-full bg-black text-white font-medium hover:bg-gray-800">Get a license</Link>
          <a href="https://docs.threadify.dev" className="font-medium underline underline-offset-4">Installation guide</a>
        </div>
        <p className="mt-8 text-sm text-gray-500">Your dashboard runs with your Threadify Engine. Review recorded work and rule checks there; your applications decide how to respond.</p>
      </main>
      <Footer />
    </div>
  );
}
