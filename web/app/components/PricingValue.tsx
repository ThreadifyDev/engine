import { Check } from "lucide-react";

export type PricingData = {
  costs: {
    contract: number;
    seat: number;
    ingress: number;
    egress: number;
    llmPerToken: number;
    llmPerThousand: number;
  };
  example: {
    contracts: number;
    seats: number;
    ingressOps: number;
    egressMb: number;
    llmTokens: number;
  };
};

type PricingValueProps = {
  pricing: PricingData;
};

const compactFormatter = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

const integerFormatter = new Intl.NumberFormat("en-US");

const formatMoney = (value: number) => {
  if (value >= 1) {
    return value.toLocaleString("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    });
  }

  if (value >= 0.01) {
    return `$${value.toFixed(2)}`;
  }

  if (value >= 0.001) {
    return `$${value.toFixed(3)}`;
  }

  if (value >= 0.0001) {
    return `$${value.toFixed(4)}`;
  }

  return `$${value.toFixed(5)}`;
};

export default function PricingValue({ pricing }: PricingValueProps) {
  const { costs, example } = pricing;

  return (
    <div className="max-w-4xl mx-auto">

      {/* Detailed Pricing Table */}
      <div className="bg-white border border-gray-200 rounded-2xl overflow-hidden mb-12">
        <div className="bg-gray-50 px-8 py-6 border-b border-gray-200">
          <h3 className="text-2xl font-bold text-gray-900">Detailed pricing</h3>
          <p className="text-gray-600 mt-1">Transparent, per-use costs</p>
        </div>
        
        <div className="divide-y divide-gray-200">
          <div className="px-8 py-5 flex justify-between items-center hover:bg-gray-50 transition-colors">
            <div>
              <div className="font-semibold text-gray-900">Contract Creation</div>
              <div className="text-sm text-gray-500">Per contract created</div>
            </div>
            <div className="text-2xl font-bold text-gray-900">{formatMoney(costs.contract)}</div>
          </div>

          <div className="px-8 py-5 flex justify-between items-center hover:bg-gray-50 transition-colors">
            <div>
              <div className="font-semibold text-gray-900">AI Token Usage</div>
              <div className="text-sm text-gray-500">Per 1,000 tokens for intelligent analysis</div>
            </div>
            <div className="text-2xl font-bold text-gray-900">{formatMoney(costs.llmPerThousand)}</div>
          </div>

          <div className="px-8 py-5 flex justify-between items-center hover:bg-gray-50 transition-colors">
            <div>
              <div className="font-semibold text-gray-900">Thread Reads (Egress)</div>
              <div className="text-sm text-gray-500">Per MB when reading thread data</div>
            </div>
            <div className="text-2xl font-bold text-gray-900">{formatMoney(costs.egress)}</div>
          </div>

          <div className="px-8 py-5 flex justify-between items-center hover:bg-gray-50 transition-colors">
            <div>
              <div className="font-semibold text-gray-900">Thread Writes (Ingress)</div>
              <div className="text-sm text-gray-500">Per unit when creating or writing to threads</div>
            </div>
            <div className="text-2xl font-bold text-gray-900">{formatMoney(costs.ingress)}</div>
          </div>

          <div className="px-8 py-5 flex justify-between items-center hover:bg-gray-50 transition-colors">
            <div>
              <div className="font-semibold text-gray-900">Team Seats</div>
              <div className="text-sm text-gray-500">Per seat per month</div>
            </div>
            <div className="text-2xl font-bold text-gray-900">{formatMoney(costs.seat)}</div>
          </div>
        </div>
      </div>

      {/* Pricing Example Card */}
      <div className="bg-gradient-to-br from-gray-50 to-white border-2 border-gray-200 rounded-3xl p-10 md:p-12 shadow-xl mb-12">
        <div className="text-center mb-8">
          <div className="inline-block px-4 py-2 bg-black text-white rounded-full text-sm font-semibold mb-4">
            Example
          </div>
          <h3 className="text-3xl md:text-4xl font-bold mb-3">
            What does <span className="text-purple-600">$15</span> get you?
          </h3>
          <p className="text-gray-600">Perfect for testing and small projects</p>
        </div>

        <div className="grid md:grid-cols-2 gap-6 mb-8">
          <div className="bg-white rounded-2xl p-6 border border-gray-200">
            <div className="flex items-start gap-3">
              <div className="mt-1">
                <Check className="w-5 h-5 text-emerald-600" />
              </div>
              <div>
                <div className="font-bold text-2xl text-gray-900 mb-1">{integerFormatter.format(example.contracts)}</div>
                <div className="text-gray-600">Contracts created</div>
                <div className="text-sm text-gray-500 mt-1">at {formatMoney(costs.contract)} each</div>
              </div>
            </div>
          </div>

          <div className="bg-white rounded-2xl p-6 border border-gray-200">
            <div className="flex items-start gap-3">
              <div className="mt-1">
                <Check className="w-5 h-5 text-emerald-600" />
              </div>
              <div>
                <div className="font-bold text-2xl text-gray-900 mb-1">{compactFormatter.format(example.llmTokens)}</div>
                <div className="text-gray-600">AI tokens</div>
                <div className="text-sm text-gray-500 mt-1">{integerFormatter.format(example.llmTokens)} tokens at {formatMoney(costs.llmPerThousand)} per 1K</div>
              </div>
            </div>
          </div>

          <div className="bg-white rounded-2xl p-6 border border-gray-200">
            <div className="flex items-start gap-3">
              <div className="mt-1">
                <Check className="w-5 h-5 text-emerald-600" />
              </div>
              <div>
                <div className="font-bold text-2xl text-gray-900 mb-1">{compactFormatter.format(example.ingressOps)}</div>
                <div className="text-gray-600">Thread creates & writes</div>
                <div className="text-sm text-gray-500 mt-1">ingress ops at {formatMoney(costs.ingress)} each</div>
              </div>
            </div>
          </div>

          <div className="bg-white rounded-2xl p-6 border border-gray-200">
            <div className="flex items-start gap-3">
              <div className="mt-1">
                <Check className="w-5 h-5 text-emerald-600" />
              </div>
              <div>
                <div className="font-bold text-2xl text-gray-900 mb-1">{compactFormatter.format(example.egressMb)}</div>
                <div className="text-gray-600">Thread reads</div>
                <div className="text-sm text-gray-500 mt-1">{integerFormatter.format(example.egressMb)} MB at {formatMoney(costs.egress)} per MB</div>
              </div>
            </div>
          </div>

          <div className="bg-white rounded-2xl p-6 border border-gray-200">
            <div className="flex items-start gap-3">
              <div className="mt-1">
                <Check className="w-5 h-5 text-emerald-600" />
              </div>
              <div>
                <div className="font-bold text-2xl text-gray-900 mb-1">{compactFormatter.format(example.seats)}</div>
                <div className="text-gray-600">Team seats</div>
                <div className="text-sm text-gray-500 mt-1">at {formatMoney(costs.seat)} per seat</div>
              </div>
            </div>
          </div>
        </div>

        <div className="bg-emerald-50 border border-emerald-200 rounded-xl p-6 text-center">
          <p className="text-emerald-900 font-medium">
            With $15, you could create <strong className="text-emerald-700">{integerFormatter.format(example.contracts)} contracts</strong>,{" "}
            <strong className="text-emerald-700 italic">or</strong> process{" "}
            <strong className="text-emerald-700">{compactFormatter.format(example.ingressOps)} thread writes</strong>,{" "}
            <strong className="text-emerald-700 italic">or</strong> serve{" "}
            <strong className="text-emerald-700">{integerFormatter.format(example.seats)} seats</strong>,{" "}
            <strong className="text-emerald-700 italic">or</strong> consume{" "}
            <strong className="text-emerald-700">{integerFormatter.format(example.llmTokens)} AI tokens</strong>,{" "}
            <strong className="text-emerald-700 italic">or</strong> any combination that totals $15.
          </p>
        </div>
      </div>

      {/* Features */}
      <div className="grid md:grid-cols-3 gap-6 mb-12">
        <div className="text-center p-6">
          <div className="w-12 h-12 bg-emerald-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <Check className="w-6 h-6 text-emerald-600" />
          </div>
          <h4 className="font-bold text-gray-900 mb-2">No Subscriptions</h4>
          <p className="text-gray-600 text-sm">Add any amount, anytime. No monthly fees or commitments.</p>
        </div>

        <div className="text-center p-6">
          <div className="w-12 h-12 bg-emerald-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <Check className="w-6 h-6 text-emerald-600" />
          </div>
          <h4 className="font-bold text-gray-900 mb-2">Credits Never Expire</h4>
          <p className="text-gray-600 text-sm">Your credits are yours forever. Use them at your own pace.</p>
        </div>

        <div className="text-center p-6">
          <div className="w-12 h-12 bg-emerald-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <Check className="w-6 h-6 text-emerald-600" />
          </div>
          <h4 className="font-bold text-gray-900 mb-2">Auto Top-Up Available</h4>
          <p className="text-gray-600 text-sm">Set thresholds and spending caps for automatic recharges.</p>
        </div>
      </div>
    </div>
  );
}
