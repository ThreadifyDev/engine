import { Link, useLoaderData } from "@remix-run/react";
import { json, type MetaFunction } from "@remix-run/node";
import ThreadifyLogo from "~/components/ThreadifyLogo";
import PricingValue, { type PricingData } from "~/components/PricingValue";
import { ArrowRight } from "lucide-react";
import { getConfig } from "~/config.server";
import Footer from "~/components/homepage/Footer";

export const meta: MetaFunction = () => {
  return [
    { title: "Pricing — Threadify" },
    { name: "description", content: "Simple, transparent pricing. Pay only for what you use. No subscriptions, no commitments." },
  ];
};  

const FIFTEEN_DOLLARS_IN_MILLICENTS = 1_500_000;

type CreditConfig = {
  ingress_cost_millicents: number;
  egress_cost_millicents: number;
  seat_cost_millicents: number;
  contract_cost_millicents: number;
  llm_token_cost_millicents: number;
};

type PricingAPIResponse = {
  credit: CreditConfig;
};

const toDollars = (millicents: number) => millicents / 100_000;

const calculateExample = (totalMillicents: number, costMillicents: number) =>
  costMillicents > 0 ? Math.floor(totalMillicents / costMillicents) : 0;

export const loader = async () => {
  const { apiUrl } = getConfig();  
  const response = await fetch(`${apiUrl}/api/pricing`);
  
  if (!response.ok) {
    throw new Error(`Failed to fetch pricing: ${response.statusText}`);
  }

  const data: PricingAPIResponse = await response.json();
  const credit = data.credit;

  const contractMillicents = credit.contract_cost_millicents;
  const seatMillicents = credit.seat_cost_millicents;
  const ingressMillicents = credit.ingress_cost_millicents;
  const egressMillicents = credit.egress_cost_millicents;
  const llmMillicents = credit.llm_token_cost_millicents;

  const costs = {
    contract: toDollars(contractMillicents),
    seat: toDollars(seatMillicents),
    ingress: toDollars(ingressMillicents),
    egress: toDollars(egressMillicents),
    llmPerToken: toDollars(llmMillicents),
    llmPerThousand: toDollars(llmMillicents * 1000),
  } satisfies PricingData["costs"];

  const example = {
    contracts: calculateExample(FIFTEEN_DOLLARS_IN_MILLICENTS, contractMillicents),
    seats: calculateExample(FIFTEEN_DOLLARS_IN_MILLICENTS, seatMillicents),
    ingressOps: calculateExample(FIFTEEN_DOLLARS_IN_MILLICENTS, ingressMillicents),
    egressMb: calculateExample(FIFTEEN_DOLLARS_IN_MILLICENTS, egressMillicents),
    llmTokens: calculateExample(FIFTEEN_DOLLARS_IN_MILLICENTS, llmMillicents),
  } satisfies PricingData["example"];

  const pricingData: PricingData = { costs, example };

  return json({ pricingData });
};

export default function Pricing() {
  const { pricingData } = useLoaderData<typeof loader>();
  return (
    <div className="bg-white text-gray-900 font-sans antialiased min-h-screen">
      {/* Navigation */}
      <nav className="fixed top-0 left-0 right-0 z-50 bg-white/80 backdrop-blur-xl border-b border-black/5">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <Link to="/" className="flex items-center">
            <ThreadifyLogo height={26} />
          </Link>
          <div className="hidden md:flex items-center gap-8">
            <Link to="/#how-it-works" className="text-sm text-gray-600 hover:text-black transition-colors">How it works</Link>
            <a href="https://docs.threadify.dev" className="text-sm text-gray-600 hover:text-black transition-colors">Documentation</a>
            <Link to="/pricing" className="text-sm text-black font-medium">Pricing</Link>
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
      <section className="pt-32 pb-16 px-6">
        <div className="max-w-7xl mx-auto text-center">
          <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-gradient-to-r from-gray-50 to-gray-100 border border-gray-200/80 mb-8">
            <span className="text-sm font-medium text-gray-700">Simple, transparent pricing</span>
          </div>
          
          <h1 className="text-5xl sm:text-6xl md:text-7xl font-semibold text-black leading-tight tracking-tight max-w-4xl mx-auto mb-6">
            Pay only for{" "}
            <span className="bg-gradient-to-r from-gray-900 via-gray-600 to-gray-400 bg-clip-text text-transparent">
              what you use
            </span>
          </h1>
          
          <p className="text-xl md:text-2xl text-gray-600 max-w-2xl mx-auto mb-12 leading-relaxed">
            No subscriptions. No tiers. Just add credits to your wallet and pay as you go.
          </p>
        </div>
      </section>

      {/* Pricing Component */}
      <section className="py-6 px-6">
        <PricingValue pricing={pricingData} />
      </section>

      {/* FAQ Section */}
      <section className="py-16 px-6 bg-gray-50">
        <div className="max-w-4xl mx-auto">
          <h2 className="text-3xl md:text-4xl font-bold text-center mb-12">
            Frequently asked questions
          </h2>
          
          <div className="space-y-6">
            <div className="bg-white rounded-2xl p-8 border border-gray-200">
              <h3 className="text-xl font-bold text-gray-900 mb-3">What happens if I run out of credits?</h3>
              <p className="text-gray-600">
                Your requests will pause until you top up your wallet. You can enable auto-recharge to automatically add credits when your balance drops below a threshold you set.
              </p>
            </div>

            <div className="bg-white rounded-2xl p-8 border border-gray-200">
              <h3 className="text-xl font-bold text-gray-900 mb-3">Can I set spending limits?</h3>
              <p className="text-gray-600">
                Yes! You can configure a maximum monthly charge for auto top-ups, giving you complete control over your spending.
              </p>
            </div>

            <div className="bg-white rounded-2xl p-8 border border-gray-200">
              <h3 className="text-xl font-bold text-gray-900 mb-3">Do credits expire?</h3>
              <p className="text-gray-600">
                No, your credits never expire. They're yours to use whenever you need them.
              </p>
            </div>

            <div className="bg-white rounded-2xl p-8 border border-gray-200">
              <h3 className="text-xl font-bold text-gray-900 mb-3">Can I get a refund?</h3>
              <p className="text-gray-600">
                Unused credits can be refunded within 30 days of purchase. Contact our support team for assistance.
              </p>
            </div>

            <div className="bg-white rounded-2xl p-8 border border-gray-200">
              <h3 className="text-xl font-bold text-gray-900 mb-3">What payment methods do you accept?</h3>
              <p className="text-gray-600">
                We accept all major credit cards via Stripe. Your payment information is secure and never stored on our servers.
              </p>
            </div>

            <div className="bg-white rounded-2xl p-8 border border-gray-200">
              <h3 className="text-xl font-bold text-gray-900 mb-3">Is there a free trial?</h3>
              <p className="text-gray-600">
                Yes! Sign up and get started with free credits to explore Threadify. No credit card required.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* CTA Section */}
      <section className="py-20 px-6">
        <div className="max-w-4xl mx-auto text-center">
          <h2 className="text-4xl md:text-5xl font-bold mb-6">
            <span className="bg-gradient-to-r from-gray-900 via-gray-700 to-gray-900 bg-clip-text text-transparent">
              Ready to get started?
            </span>
          </h2>
          <p className="text-xl text-gray-600 mb-10 max-w-2xl mx-auto">
            Start with free credits. No credit card required.
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

      {/* Footer */}
      <Footer />
    </div>
  );
}
