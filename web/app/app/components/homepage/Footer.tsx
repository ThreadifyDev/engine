export default function Footer() {
  const currentYear = new Date().getFullYear();

  return (
    <footer className="bg-gray-50 border-t border-gray-200 py-8 px-6">
      <div className="max-w-7xl mx-auto">
        <div className="flex flex-col items-start justify-center gap-2">
          <h3 className="text-xl font-semibold text-black">Threadify</h3>
          <p className="text-sm text-gray-600">Enabling intelligent systems and teams.</p>
        </div>
        <div className="flex justify-center mt-4">
          <p className="text-sm text-gray-500">
            © {currentYear} Threadify. All rights reserved.
          </p>
        </div>
      </div>
    </footer>
  );
}
