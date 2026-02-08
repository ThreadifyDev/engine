export default function AuditableLogSection() {
  return (
    <section className="max-w-7xl mx-auto px-6 py-16">
      <div className="text-center mb-12">
        <h2 className="text-5xl font-bold mb-6">Auditable Log</h2>
        <p className="text-xl text-gray-500 max-w-3xl mx-auto">
          Cryptographically verified audit trail that ensures every step in your workflow is tamper-proof and traceable.
        </p>
      </div>

      {/* Visual representation */}
      <div className="bg-gray-100 rounded-2xl h-96 flex items-center justify-center">
        <div className="text-gray-300 text-6xl font-bold">A</div>
      </div>

      {/* Small label */}
      <div className="mt-8 text-center">
        <span className="inline-block px-4 py-2 bg-gray-100 rounded-full text-sm text-gray-600">
          Works how you already do
        </span>
      </div>
    </section>
  );
}
