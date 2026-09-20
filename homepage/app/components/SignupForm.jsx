import { useEffect, useState } from "react";
import { ArrowRight, Check, CheckCircle2, Copy, KeyRound } from "lucide-react";

const recaptchaAction = "signup";
let recaptchaScriptPromise;

function loadRecaptcha(siteKey) {
  if (!siteKey || window.grecaptcha) return Promise.resolve();
  if (recaptchaScriptPromise) return recaptchaScriptPromise;
  recaptchaScriptPromise = new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = `https://www.google.com/recaptcha/api.js?render=${encodeURIComponent(siteKey)}&trustedtypes=true`;
    script.async = true;
    script.defer = true;
    script.dataset.fusedRecaptcha = "true";
    script.addEventListener("load", resolve, { once: true });
    script.addEventListener("error", () => reject(new Error("Security verification could not load.")), { once: true });
    document.head.appendChild(script);
  }).catch((error) => {
    recaptchaScriptPromise = undefined;
    throw error;
  });
  return recaptchaScriptPromise;
}

async function createRecaptchaToken(siteKey) {
  if (!siteKey) return "";
  await loadRecaptcha(siteKey);
  if (!window.grecaptcha) throw new Error("Security verification is unavailable.");
  return new Promise((resolve, reject) => {
    window.grecaptcha.ready(() => {
      window.grecaptcha.execute(siteKey, { action: recaptchaAction }).then(resolve, reject);
    });
  });
}

// SignupForm collects signup details and proves email ownership before showing credentials.
export default function SignupForm({ recaptchaSiteKey = "", available = true }) {
  const [error, setError] = useState("");
  const toast = { error: setError };
  const [formState, setFormState] = useState("idle");
  const [accountName, setAccountName] = useState("");
  const [result, setResult] = useState(null);
  const [copied, setCopied] = useState(false);
  const [verificationEmail, setVerificationEmail] = useState("");
  const [verificationCode, setVerificationCode] = useState("");

  useEffect(() => {
    if (recaptchaSiteKey) loadRecaptcha(recaptchaSiteKey).catch(() => undefined);
  }, [recaptchaSiteKey]);

  // Reset mailbox proof whenever the user edits the recipient address.
  function resetEmailVerification() {
    setVerificationEmail("");
    setVerificationCode("");
  }

  // handleSubmit sends or verifies a code before accepting Registry's account creation result.
  async function handleSubmit(event) {
    event.preventDefault();
    // Suppress duplicate in-flight sends and account creation requests.
    if (!available || formState === "submitting") return;
    setFormState("submitting");
    setError("");
    const formData = new FormData(event.currentTarget);
    const resending = event.nativeEvent.submitter?.value === "resend";
    try {
      // v3 tokens expire quickly, so generate one for the exact signup action
      // immediately before sending the server-verified request.
      const recaptchaToken = await createRecaptchaToken(recaptchaSiteKey);
      const response = await fetch("/api/signup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          account_name: formData.get("account_name"),
          email: formData.get("email"),
          // Resends explicitly request fresh proof instead of retrying a rejected code.
          verification_code: resending ? "" : verificationCode,
          recaptcha_token: recaptchaToken,
        }),
      });
      const next = await response.json().catch(() => null);
      // Keep the form and code entry available after provider or validation errors.
      if (!response.ok) throw new Error(next?.error || "We couldn’t create your account. Please try again.");
      // A sent email leaves signup pending and must not reveal a success screen or start polling.
      if (next?.verification_required) {
        setVerificationEmail(next.email);
        setVerificationCode("");
        setFormState("idle");
        return;
      }
      setResult(next);
      setFormState("success");
    } catch (error) {
      // Errors preserve entered details so the user can correct or resend the code.
      console.error("Failed to create Threadify account:", error);
      setFormState("idle");
      toast.error(error instanceof Error ? error.message : "Failed to create account. Please try again.");
    }
  }

  async function copyKey() {
    try {
      await navigator.clipboard.writeText(result.api_key);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      toast.error("Copy failed. Select the key and copy it manually.");
    }
  }

  return (
    <section className="access-request" id="create-account">
      <div className="access-request-shell">
        <div className="access-request-copy">
          <p className="access-request-kicker">Start with Threadify</p>
          <h1>Give your workflows a shared referee.</h1>
          <p>Follow work across your services and AI agents, check the rules, and keep the evidence behind each result. Start with one workflow on your own infrastructure.</p>
          <div className="access-request-points">
            <div className="access-request-point"><span><Check size={13}/></span>Your license key is shown once</div>
            <div className="access-request-point"><span><Check size={13}/></span>Your data stays in your infrastructure</div>
            <div className="access-request-point"><span><Check size={13}/></span>Start with one workflow and its rules</div>
          </div>
        </div>
        <div className="access-form-card">
          {!available && <p role="status" className="mb-5 text-sm text-gray-600">Signup is temporarily unavailable. Please try again later.</p>}
          {error && <p role="alert" className="mb-5 text-sm text-red-700">{error}</p>}
          {formState === "success" ? (
            <div className="signup-success" role="status">
              <span className="signup-success-icon"><CheckCircle2 size={27}/></span>
              <h3>Your account is ready.</h3>
              <p>Save this key now. It cannot be shown again.</p>
              {result?.api_key ? <div className="signup-key-card">
                <label>One-time license key</label>
                <div className="signup-key-value"><code>{result.api_key}</code><button type="button" onClick={copyKey} aria-label="Copy license key"><Copy size={16}/></button></div>
                <p>{copied ? "Copied to clipboard." : "Store this in your password manager before leaving this page."}</p>
              </div> : null}
              <div className="signup-progress"><div className="signup-progress-head"><strong>Ready to self-host</strong></div><p>Use your license to start the Threadify Engine. Open its dashboard, connect a service, and choose the first workflow you want to follow.</p><a href="https://docs.threadify.dev" target="_blank" rel="noreferrer">Install and configure Threadify <ArrowRight size={14}/></a></div>
            </div>
          ) : (
            <>
              <div className="access-form-heading"><span>Developer signup</span><h3>Create your account.</h3><p>Verify your email first. Accounts start on Dev. Threadify runs on your own infrastructure.</p></div>
              <form className="access-form" onSubmit={handleSubmit}>
                <div className="access-form-row">
                  <div className="access-field"><label htmlFor="signup-account">Account or team name</label><input id="signup-account" name="account_name" required maxLength={100} autoComplete="organization" placeholder="Acme" value={accountName} onChange={(event) => setAccountName(event.target.value)} /></div>
                  <div className="access-field"><label htmlFor="signup-email">Work email</label><input id="signup-email" name="email" type="email" required maxLength={128} autoComplete="email" placeholder="you@company.com" readOnly={formState === "submitting"} onChange={resetEmailVerification} /></div>
                </div>
                <p className="text-sm text-gray-600">Your Dev license is for a self-hosted Threadify Engine.</p>
                {/* Code entry appears only after delivery; email edits reset this step. */}
                {verificationEmail && <div className="access-field">
                  <p className="signup-verification-note" role="status">We sent a verification code to <strong>{verificationEmail}</strong>. Enter it below to create your account.</p>
                  <label htmlFor="signup-code">Email verification code</label>
                  {/* Keep the code as text to preserve leading zeros and provider-configured characters. */}
                  <input id="signup-code" name="verification_code" required maxLength={64} autoComplete="one-time-code" value={verificationCode} onChange={(event) => { /* Retain only the current challenge's input in memory. */ setVerificationCode(event.target.value); }} />
                </div>}
                {/* The final action stays explicit: only verified signup can provision resources. */}
                <button className="access-submit" type="submit" disabled={!available || formState === "submitting"}>{formState === "submitting" ? <><span className="access-spinner"/>Processing…</> : <>{verificationEmail ? "Verify email & create account" : "Send verification code"} <ArrowRight size={16}/></>}</button>
                {/* Keep resending after the primary submit so Enter in the code field verifies it. */}
                {verificationEmail && <button className="signup-resend" type="submit" name="intent" value="resend" formNoValidate disabled={!available || formState === "submitting"}>Send a new code</button>}
                <p className="access-privacy"><KeyRound size={10}/> Save your license key when it appears; it is shown only once. Paid plans are enabled separately.</p>
              </form>
            </>
          )}
        </div>
      </div>
    </section>
  );
}
