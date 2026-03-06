import { useState, useEffect } from 'react';
import type { MetaFunction } from "@remix-run/node";
import { useNavigate } from '@remix-run/react';
import { Check } from 'lucide-react';
import { api } from '~/lib/api';

export const meta: MetaFunction = () => {
  return [
    { title: "Getting Started - Threadify" },
    { name: "description", content: "Get started with Threadify" },
  ];
};

export default function GettingStarted() {
  const navigate = useNavigate();
  const [apiKey, setApiKey] = useState<string>('');
  const [hasApiKey, setHasApiKey] = useState(false);
  const [creatingKey, setCreatingKey] = useState(false);
  const [copied, setCopied] = useState(false);
  const [selectedLanguage, setSelectedLanguage] = useState('javascript');
  const [codeSamples, setCodeSamples] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [checkingInstrumentation, setCheckingInstrumentation] = useState(false);

  useEffect(() => {
    // Check if user is authenticated
    const token = api.getStoredToken();
    if (!token) {
      navigate('/login');
      return;
    }

    // Check if user has completed onboarding
    const user = api.getStoredUser();
    if (!user?.onboarding_completed) {
      navigate('/u/onboarding');
      return;
    }

    // Fetch code samples
    fetchCodeSamples();
  }, [navigate]);

  const fetchCodeSamples = async () => {
    try {
      setLoading(true);
      const response = await api.getCodeSamples('basic_instrumentation');
      setCodeSamples(response.samples);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load code samples');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateAPIKey = async () => {
    setCreatingKey(true);
    setError('');

    try {
      const response = await api.createAPIKey({
        name: 'Getting Started API Key',
      });
      setApiKey(response.key);
      setHasApiKey(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create API key');
    } finally {
      setCreatingKey(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleCheckInstrumentation = async () => {
    setCheckingInstrumentation(true);
    setError('');

    try {
      // Mark user as having completed first instrumentation
      const response = await api.markInstrumentationDone();
      
      // Update local user state
      api.setUser(response.user);
      
      // Navigate to dashboard
      navigate('/u/dashboard');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to mark instrumentation as complete');
    } finally {
      setCheckingInstrumentation(false);
    }
  };

  const currentCode = codeSamples[selectedLanguage] || '';
  const codeWithKey = hasApiKey 
    ? currentCode.replace(/YOUR_API_KEY/g, apiKey)
    : currentCode; // Show placeholder if no key created yet

  return (
    <div className="min-h-screen bg-white">
      {/* Header */}
      <div className="border-b-2 border-black">
        <div className="max-w-6xl mx-auto px-6 py-4">
          <h1 className="text-2xl font-bold" style={{ fontFamily: 'Block, sans-serif' }}>
            Threadify
          </h1>
        </div>
      </div>

      <div className="max-w-4xl mx-auto px-6 py-12">
        {/* Title */}
        <div className="mb-8">
          <h2 className="text-4xl font-bold mb-4" style={{ fontFamily: 'Block, sans-serif' }}>
            We've created an API Key so you can get started with Threadify
          </h2>
          <p className="text-lg text-gray-600">
            Complete your first instrumentation to unlock the full Threadify dashboard.
          </p>
        </div>

        {/* Error Message */}
        {error && (
          <div className="mb-6 p-4 border-2 border-red-600 bg-red-50">
            <p className="text-red-600">{error}</p>
          </div>
        )}

        {/* API Key Section */}
        <div className="mb-8 p-6 border-2 border-black bg-gray-50">
          <h3 className="text-xl font-bold mb-2">Your API Key</h3>
          {!hasApiKey ? (
            <>
              <p className="text-sm text-gray-600 mb-4">
                Create your first API key to start instrumenting your application with Threadify.
              </p>
              <button
                onClick={handleCreateAPIKey}
                disabled={creatingKey}
                className="px-6 py-3 bg-black rounded-lg text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {creatingKey ? 'Creating...' : 'Create API Key'}
              </button>
            </>
          ) : (
            <>
              <p className="text-sm text-gray-600 mb-4">
                This is the <strong>only time</strong> you'll see this key. Copy it now and store it securely.
              </p>
              <div className="flex gap-2">
                <code className="flex-1 bg-white border-2 border-black px-4 py-3 text-sm break-all font-mono">
                  {apiKey}
                </code>
                <button
                  onClick={() => copyToClipboard(apiKey)}
                  className="px-6 py-3 bg-black text-white hover:bg-gray-800 transition-colors whitespace-nowrap font-medium"
                >
                  {copied ? (
                    <span className="flex items-center gap-1">
                      <Check className="w-4 h-4" /> Copied!
                    </span>
                  ) : 'Copy'}
                </button>
              </div>
            </>
          )}
        </div>


        {/* Language Tabs */}
        <div className="mb-4">
          <div className="flex gap-2 border-b-2 border-black">
            {Object.keys(codeSamples).map((lang) => (
              <button
                key={lang}
                onClick={() => setSelectedLanguage(lang)}
                className={`px-6 py-3 font-medium transition-colors ${
                  selectedLanguage === lang
                    ? 'bg-black text-white'
                    : 'bg-white hover:bg-gray-100'
                }`}
              >
                {lang.charAt(0).toUpperCase() + lang.slice(1)}
              </button>
            ))}
          </div>
        </div>

        {/* Code Sample */}
        <div className="mb-8 border-2 border-black">
          <div className="bg-gray-900 p-6 overflow-x-auto">
            <pre className="text-sm text-green-400 font-mono">
              <code>{loading ? 'Loading...' : codeWithKey}</code>
            </pre>
          </div>
          <div className="p-4 bg-gray-50 border-t-2 border-black flex justify-end">
            <button
              onClick={() => copyToClipboard(codeWithKey)}
              className="px-4 py-2 border-2 rounded-lg border-black hover:bg-black hover:text-white transition-colors font-medium"
            >
              Copy Code
            </button>
          </div>
        </div>

        {/* Check Instrumentation Button */}
        <div className="text-center">
          <button
            onClick={handleCheckInstrumentation}
            disabled={checkingInstrumentation || !hasApiKey}
            className="px-8 py-4 bg-black text-white rounded-lg text-lg font-bold hover:bg-gray-800 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {checkingInstrumentation ? 'Checking...' : 'I\'ve Completed My First Instrumentation'}
          </button>
          <p className="mt-4 text-sm text-gray-500">
            {!hasApiKey 
              ? 'Create an API key above to get started.'
              : 'Run the code above to create your first thread, then click this button to continue.'
            }
          </p>
        </div>

               {/* LLM IDE Helper */}
        <div className="mb-8 p-6 border-2 border-gray-300 bg-gray-50">
          <div className="flex items-start gap-3 mb-3">
            <svg className="w-6 h-6 text-gray-600 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
            <div>
              <h3 className="text-lg font-bold text-gray-900 mb-2">Building with an AI-Powered IDE?</h3>
              <p className="text-sm text-gray-700 mb-3">
                Give your AI assistant context about Threadify's SDK to accelerate development.
              </p>
              <a
                href="/AI.md"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-block px-4 py-2 bg-gray-800 rounded-lg text-white hover:bg-gray-900 transition-colors font-medium text-sm"
              >
                View AI Context Guide →
              </a>
              <p className="text-xs text-gray-600 mt-3">
                Copy the guide and share it with Cursor, Windsurf, or any LLM-powered IDE for better code suggestions.
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
