import { randomUUID } from "node:crypto";

const registryURL = () =>
  (process.env.BACKEND_URL || "https://registry.usefused.com").replace(/\/$/, "");

function signupToken() {
  return process.env.FUSED_SIGNUP_TOKEN || "";
}

export function signupAvailable() {
  return Boolean(signupToken());
}

const recaptchaAction = "signup";

type RecaptchaResult = { success?: boolean; score?: number; action?: string };

function recaptchaMinimumScore() {
  const configured = Number(process.env.RECAPTCHA_MIN_SCORE || "0.5");
  return Number.isFinite(configured) && configured >= 0 && configured <= 1
    ? configured
    : 0.5;
}

// recaptchaSecret returns the server-only credential used for verification.
function recaptchaSecret() {
  return process.env.RECAPTCHA_SECRET_KEY || "";
}

// recaptchaRemoteIP selects the first trusted forwarding hint for Google.
function recaptchaRemoteIP(request: Request) {
  return request.headers.get("CF-Connecting-IP") ||
    request.headers.get("X-Forwarded-For")?.split(",")[0]?.trim();
}

// recaptchaAccepted enforces the complete response, action, and score contract.
function recaptchaAccepted(response: Response, result: RecaptchaResult) {
  return response.ok && result.success === true &&
    result.action === recaptchaAction &&
    typeof result.score === "number" && result.score >= recaptchaMinimumScore();
}

// verifyRecaptcha verifies a signup token and fails closed on network errors.
export async function verifyRecaptcha(request: Request, token: string) {
  const secret = recaptchaSecret();
  if (!secret) {
    // local development remains usable without a Google credential, while
    // production never accepts an unverifiable public signup.
    return process.env.NODE_ENV !== "production";
  }
  if (!token) return false;

  const body = new URLSearchParams({ secret, response: token });
  const remoteIP = recaptchaRemoteIP(request);
  if (remoteIP) body.set("remoteip", remoteIP);

  try {
    const response = await fetch("https://www.google.com/recaptcha/api/siteverify", {
      method: "POST",
      body,
      signal: AbortSignal.timeout(10_000),
    });
    const result = await response.json() as RecaptchaResult;
    return recaptchaAccepted(response, result);
  } catch (error) {
    console.error("Failed to verify signup reCAPTCHA:", error);
    return false;
  }
}

export async function createRegistrySignup(input: Record<string, unknown>) {
  return registryRequest("/internal/signup", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": `homepage-${randomUUID()}`,
    },
    body: JSON.stringify(input),
  });
}

async function registryRequest(path: string, init: RequestInit) {
  const token = signupToken();
  if (!token) throw new Error("signup_unavailable");
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(`${registryURL()}${path}`, {
    ...init,
    headers,
    signal: AbortSignal.timeout(20_000),
  });
  const result = await response.json().catch(() => null);
  return { ok: response.ok, status: response.status, result };
}
