export default function FooterSection() {
  return (
    <footer className="bg-white border-t border-gray-200 mt-24">
      <div className="max-w-7xl mx-auto px-6 py-8">
        <div className="flex items-center justify-between text-sm text-gray-600">
          <span className="font-semibold text-gray-900">Threadify</span>
          <span>© {new Date().getFullYear()} Threadify. All rights reserved.</span>
        </div>
      </div>
    </footer>
  );
}
