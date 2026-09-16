import { json, type ActionFunctionArgs } from "@remix-run/node";
import { createRegistrySignup, signupAvailable, verifyRecaptcha } from "~/lib/signup.server";

type SignupInput = {
  accountName: string;
  products: string[];
  email: string;
  hosted: boolean;
  recaptchaToken: string;
  verificationCode: string;
};

// loader rejects reads while advertising the only supported method.
export async function loader() {
  return methodNotAllowed();
}

// action protects both email-code delivery and verified account creation at the public edge.
export async function action({ request }: ActionFunctionArgs) {
  // Reject unsupported methods before consuming public input.
  if (request.method !== "POST") {
    return methodNotAllowed();
  }
  // A missing server credential must fail closed for both signup steps.
  if (!signupAvailable()) {
    return noStore({ error: "Signup is temporarily unavailable." }, 503);
  }

  let input: SignupInput;
  try {
    input = normalizeSignupInput(await readBoundedJSON(request, 16 << 10));
  } catch {
    // Invalid or oversized JSON never reaches Registry or the email connector.
    return noStore({ error: "Please complete the form and try again." }, 400);
  }

  const validationError = validateSignupInput(input);
  // Invalid form fields must not trigger email delivery.
  if (validationError) {
    return noStore({ error: validationError }, 400);
  }
  // Require a fresh CAPTCHA for code sends, resends, and verification attempts.
  if (!await verifyRecaptcha(request, input.recaptchaToken)) {
    return noStore({ error: "Please complete the security check and try again." }, 400);
  }

  return submitRegistrySignup(registrySetup(input));
}

// methodNotAllowed returns the shared 405 response for loader and non-POST action calls.
function methodNotAllowed() {
  return json({ error: "Method not allowed" }, { status: 405, headers: { Allow: "POST" } });
}

// signupString preserves the public form's existing falsy-value normalization.
function signupString(value: unknown) {
  return String(value || "");
}

// normalizeSignupInput converts the bounded JSON document into canonical fields.
function normalizeSignupInput(input: Record<string, unknown>): SignupInput {
  return {
    accountName: signupString(input.account_name).trim(),
    // Omission selects Threadify signup; malformed explicit selections remain invalid.
    products: input.products === undefined ? ["threadify"] : Array.isArray(input.products) ? input.products : [],
    email: signupString(input.email).trim().toLowerCase(),
    hosted: input.deployment === "hosted" || input.hosted_engine !== undefined,
    recaptchaToken: signupString(input.recaptcha_token),
    verificationCode: signupString(input.verification_code).trim(),
  };
}

// validateSignupInput returns the same user-facing error for the first invalid field group.
function validateSignupInput(input: SignupInput) {
  // Bound identity fields and reject malformed recipients before sending email.
  if (!input.accountName || input.accountName.length > 100 || input.email.length > 128 || !/^\S+@\S+\.\S+$/.test(input.email)) {
    return "Enter a valid account name and email.";
  }
  // The public Threadify form cannot provision another product by editing its request.
  if (input.products.length !== 1 || input.products[0] !== "threadify") return "This signup is for Threadify only.";
  if (input.hosted) return "Threadify signup provides a self-hosted license.";
  // Allow provider-configured code lengths while bounding untrusted input.
  if (input.verificationCode.length > 64) return "Enter a valid email verification code.";
  return "";
}

// registrySetup builds the credential-free signup request accepted by Registry.
function registrySetup(input: SignupInput): Record<string, unknown> {
  const setup: Record<string, unknown> = {
    account_name: input.accountName,
    products: ["threadify"],
    email: input.email,
    verification_code: input.verificationCode,
    key_name: "Threadify homepage signup key",
    // Registry also enforces dev for the scoped signup route. Keeping this
    // explicit documents that public signup cannot self-assign paid plans.
    plan: "dev",
  };
  return setup;
}

// registrySignupError selects a safe Registry message without exposing server failures.
function registrySignupError(response: Awaited<ReturnType<typeof createRegistrySignup>>) {
  const registryMessage = typeof response.result?.error === "string" ? response.result.error : "";
  return response.status < 500 && registryMessage
    ? registryMessage
    : "We couldn’t create your account right now. Please try again.";
}

// registrySignupResult exposes only the new account and one-time Threadify license.
function registrySignupResult(response: Awaited<ReturnType<typeof createRegistrySignup>>) {
  return { account_id: response.result?.account_id, products: response.result?.products, api_key: response.result?.api_key, message: response.result?.message };
}

// submitRegistrySignup maps Registry and network outcomes to the public API contract.
async function submitRegistrySignup(setup: Record<string, unknown>) {
  try {
    const response = await createRegistrySignup(setup);
    // Preserve safe client errors such as invalid codes and provider throttling.
    if (!response.ok) {
      return noStore({ error: registrySignupError(response) }, response.status);
    }
    // Email delivery is a pending signup, never a created account or license.
    if (response.status === 202 && response.result?.verification_required === true) {
      return noStore({ verification_required: true, email: response.result.email }, 202);
    }
    if (typeof response.result?.api_key !== "string" || !response.result.api_key || !response.result.account_id) {
      return noStore({ error: "The Registry returned an incomplete signup response. Please try again." }, 502);
    }
    return noStore(registrySignupResult(response), 201);
  } catch (error) {
    // A transport failure must not claim either delivery or creation succeeded.
    console.error("Failed to create homepage signup:", error);
    return noStore({ error: "We couldn’t reach the signup service right now. Please try again shortly." }, 502);
  }
}

function noStore(body: Record<string, unknown>, status: number) {
  return json(body, { status, headers: { "Cache-Control": "no-store" } });
}

async function readBoundedJSON(request: Request, maximumBytes: number) {
  const reader = request.body?.getReader();
  if (!reader) throw new Error("missing body");
  const decoder = new TextDecoder();
  let text = "";
  let total = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > maximumBytes) {
      await reader.cancel();
      throw new Error("request body too large");
    }
    text += decoder.decode(value, { stream: true });
  }
  text += decoder.decode();
  return JSON.parse(text) as Record<string, unknown>;
}
