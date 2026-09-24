import { TabBar } from '~/components/TabBar';
import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { Check } from 'lucide-react';
import { api } from '~/lib/api';
import AgentToggleButton from '~/components/agent/AgentToggleButton';


export default function GettingStarted() {
  const navigate = useNavigate();
  const [apiKey, setApiKey] = useState<string>('');
  const [hasApiKey, setHasApiKey] = useState(false);
  const [creatingKey, setCreatingKey] = useState(false);
  const [copiedApiKey, setCopiedApiKey] = useState(false);
  const [copiedCode, setCopiedCode] = useState(false);
  const [copiedAISkill, setCopiedAISkill] = useState(false);
  const [selectedLanguage, setSelectedLanguage] = useState('javascript');
  const [codeSamples, setCodeSamples] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [checkingInstrumentation, setCheckingInstrumentation] = useState(false);

  useEffect(() => {
    // Check if user is authenticated
    const token = api.isAuthenticated();
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

    // If user has already completed first instrumentation, redirect to dashboard
    if (user.first_instrumentation_done) {
      navigate('/u/dashboard');
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

  const copyApiKey = () => {
    navigator.clipboard.writeText(apiKey);
    setCopiedApiKey(true);
    setTimeout(() => setCopiedApiKey(false), 2000);
  };

  const copyCode = () => {
    navigator.clipboard.writeText(codeWithKey);
    setCopiedCode(true);
    setTimeout(() => setCopiedCode(false), 2000);
  };

  const copyAISkill = async () => {
    try {
      const response = await fetch(`/AI-${selectedLanguage}.md`);
      const content = await response.text();
      await navigator.clipboard.writeText(content);
      setCopiedAISkill(true);
      setTimeout(() => setCopiedAISkill(false), 2000);
    } catch (err) {
      console.error('Failed to copy AI skill:', err);
    }
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
    <div className="min-h-screen bg-[#f8f8f6]">
      <div className="border-b border-stone-200 bg-white">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
          <span className="text-sm font-semibold tracking-tight text-stone-900">Threadify</span>
          <AgentToggleButton />
        </div>
      </div>

      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6 sm:py-12">
        <header className="mb-8">
          <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-700">Welcome / First connection</p>
          <h1 className="text-3xl font-semibold tracking-tight text-stone-950 sm:text-4xl">Connect your first workflow</h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-stone-500">Create an API key, instrument your application, and send your first thread.</p>
        </header>

        {/* Error Message */}
        {error && (
          <div className="mb-6 rounded-xl border border-red-200 bg-red-50 p-4 text-sm">
            <p className="text-red-600">{error}</p>
          </div>
        )}

        {/* API Key Section */}
        <div className="mb-8 rounded-2xl border border-stone-200 bg-white p-6 shadow-sm sm:p-8">
          <h3 className="mb-2 text-lg font-semibold tracking-tight text-stone-900">Your API Key</h3>
          {!hasApiKey ? (
            <>
              <p className="text-sm text-gray-600 mb-4">
                Create your first API key to start instrumenting your application with Threadify.
              </p>
              <button
                onClick={handleCreateAPIKey}
                disabled={creatingKey}
                className="rounded-lg bg-stone-900 px-5 py-2.5 text-sm font-medium text-white transition hover:bg-stone-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {creatingKey ? 'Creating...' : 'Create API Key'}
              </button>
            </>
          ) : (
            <>
              <p className="text-sm text-gray-600 mb-4">
                This is the <strong>only time</strong> you'll see this key. Copy it now and store it securely.
              </p>
              <div className="mb-6 flex flex-col gap-2 sm:flex-row">
                <code className="min-w-0 flex-1 break-all rounded-lg border border-stone-200 bg-stone-50 px-4 py-3 font-mono text-sm">
                  {apiKey}
                </code>
                <button
                  onClick={copyApiKey}
                  className="whitespace-nowrap rounded-lg bg-stone-900 px-5 py-3 text-sm font-medium text-white transition hover:bg-stone-700"
                >
                  {copiedApiKey ? (
                    <span className="flex items-center gap-1">
                      <Check className="w-4 h-4" /> Copied!
                    </span>
                  ) : 'Copy'}
                </button>
              </div>

              {/* AI Context Guide */}
              <div className="border-t border-stone-100 pt-6">
                <div className="flex items-start gap-3">
                  <svg className="w-6 h-6 text-gray-600 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                  </svg>
                  <div className="flex-1">
                    <h4 className="text-base font-bold text-gray-900 mb-2">Building with an AI-Powered IDE?</h4>
                    <p className="text-sm text-gray-700 mb-3">
                      Give your AI assistant context about Threadify's SDK to accelerate development.
                    </p>
                    <button
                      onClick={copyAISkill}
                      className="rounded-lg bg-stone-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-stone-700"
                    >
                      {copiedAISkill ? (
                        <span className="flex items-center gap-1">
                          <Check className="w-4 h-4" /> Copied!
                        </span>
                      ) : 'Copy AI Skill'}
                    </button>
                    <p className="text-xs text-gray-600 mt-3">
                      Share with Cursor, Windsurf, or any LLM-powered IDE for better code suggestions.
                    </p>
                  </div>
                </div>
              </div>
            </>
          )}
        </div>


        <TabBar label="SDK language" value={selectedLanguage} onChange={setSelectedLanguage} panelId="language-panel" className="mb-4"
          items={Object.keys(codeSamples).map(lang => ({value:lang,label:lang.charAt(0).toUpperCase()+lang.slice(1)}))} />

        {/* Code Sample */}
        <div id="language-panel" role="tabpanel" aria-label="SDK example" className="mb-8 overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm">
          <div className="overflow-x-auto bg-stone-950 p-6">
            <pre className="text-sm text-green-400 font-mono">
              <code>{loading ? 'Loading...' : codeWithKey}</code>
            </pre>
          </div>
          <div className="flex justify-end border-t border-stone-100 bg-white p-4">
            <button
              onClick={copyCode}
              className="rounded-lg border border-stone-200 px-4 py-2 text-sm font-medium text-stone-700 transition hover:bg-stone-50"
            >
              {copiedCode ? (
                <span className="flex items-center gap-1">
                  <Check className="w-4 h-4" /> Copied!
                </span>
              ) : 'Copy Code'}
            </button>
          </div>
        </div>

        {/* Check Instrumentation Button */}
        <div className="text-center">
          <button
            onClick={handleCheckInstrumentation}
            disabled={checkingInstrumentation || !hasApiKey}
            className="rounded-lg bg-stone-900 px-6 py-3 text-sm font-medium text-white transition hover:bg-stone-700 disabled:cursor-not-allowed disabled:opacity-50"
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
      </div>
    </div>
  );
}
