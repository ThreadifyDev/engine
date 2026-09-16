import { json, type MetaFunction } from "@remix-run/node";
import { Link, useLoaderData } from "@remix-run/react";
import ThreadifyLogo from "~/components/ThreadifyLogo";
import Footer from "~/components/homepage/Footer";
import SignupForm from "~/components/SignupForm";
import "~/styles/signup.css";
import { signupAvailable } from "~/lib/signup.server";

export const meta: MetaFunction = () => [{ title: "Get your Threadify license" }];
export const headers = () => ({ "Cache-Control": "no-store" });
export const loader = () => json({
  siteKey: process.env.RECAPTCHA_SITE_KEY || "",
  available: signupAvailable() && (process.env.NODE_ENV !== "production" || Boolean(process.env.RECAPTCHA_SECRET_KEY && process.env.RECAPTCHA_SITE_KEY)),
}, { headers: { "Cache-Control": "no-store" } });

export default function Signup() {
  const { siteKey, available } = useLoaderData<typeof loader>();
  return (
    <div className="min-h-screen flex flex-col">
      <header className="max-w-6xl w-full mx-auto px-6 py-5"><Link to="/" aria-label="Threadify home"><ThreadifyLogo /></Link></header>
      <main className="flex-1">
        <SignupForm recaptchaSiteKey={siteKey} available={available} />
      </main>
      <Footer />
    </div>
  );
}
