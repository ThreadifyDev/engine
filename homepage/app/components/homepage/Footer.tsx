import ThreadifyLogo from "~/components/ThreadifyLogo";

export default function Footer() {
  return (
    <footer className="py-12 px-6 border-t border-gray-200 bg-white">
      <div className="max-w-7xl mx-auto">
        <div className="flex flex-col md:flex-row justify-between items-center gap-6 mb-6">
          <div className="flex items-center">
            <ThreadifyLogo height={26} />
          </div>
          <div className="flex items-center gap-6">
            <a href="https://docs.threadify.dev" className="text-sm text-gray-600 hover:text-gray-900 transition">Docs</a>
            <a href="/pricing" className="text-sm text-gray-600 hover:text-black transition-colors">Pricing</a>
            <a href="https://blog.threadify.dev" className="text-sm text-gray-600 hover:text-gray-900 transition">Our Blog</a>
            <a href="https://docs.threadify.dev/core-concepts/mcp-integration" className="text-sm text-gray-600 hover:text-gray-900 transition">MCP</a>
            <a href="https://threadify.dev/AI.md" className="text-sm text-gray-600 hover:text-gray-900 transition">AI.md</a>
          </div>
        </div>
        <p className="text-sm text-gray-500 text-center">© {new Date().getFullYear()} Threadify. Execution intelligence for services and AI agents.</p>
      </div>
    </footer>
  );
}
