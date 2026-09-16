import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router';
import { api, type User } from '~/lib/api';
import { CheckCircle, XCircle } from 'lucide-react';
import AppLayout from '~/components/AppLayout';


export default function Dashboard() {
  const navigate = useNavigate();
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    // Check if user is authenticated
    if (!api.isAuthenticated()) {
      navigate('/login');
      return;
    }

    // Get stored user
    const storedUser = api.getStoredUser();
    if (!storedUser) {
      navigate('/login');
      return;
    }

    // Check if user has completed first instrumentation
    if (!storedUser.first_instrumentation_done) {
      // Redirect to getting-started (non-skippable)
      navigate('/u/getting-started');
      return;
    }

    setUser(storedUser);
  }, [navigate]);

  if (!user) {
    return (
      <div className="min-h-screen bg-white flex items-center justify-center">
        <div className="text-black">Loading...</div>
      </div>
    );
  }

  return (
    <AppLayout>
      <div className="min-w-0 p-4 sm:p-6 lg:p-8">
        {/* Welcome Section */}
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-black mb-2">
            Welcome back{user.full_name ? `, ${user.full_name}` : ''}!
          </h2>
          <p className="text-gray-600">
            {user.email} • {user.job_role || 'No role specified'}
          </p>
        </div>

        {/* Status Cards */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-12">
          <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm hover:shadow-md transition-shadow">
            <h3 className="text-sm font-medium text-gray-600 mb-2">Email Status</h3>
            <p className="text-2xl font-bold text-gray-900 flex items-center gap-2">
              {user.email_verified ? (
                <><CheckCircle className="w-6 h-6 text-green-600" /> Verified</>
              ) : (
                <><XCircle className="w-6 h-6 text-red-600" /> Not Verified</>
              )}
            </p>
          </div>

          <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm hover:shadow-md transition-shadow">
            <h3 className="text-sm font-medium text-gray-600 mb-2">Onboarding</h3>
            <p className="text-2xl font-bold text-gray-900 flex items-center gap-2">
              {user.onboarding_completed ? (
                <><CheckCircle className="w-6 h-6 text-green-600" /> Complete</>
              ) : (
                'Pending'
              )}
            </p>
          </div>

          <div className="border border-gray-200 rounded-lg p-6 bg-white shadow-sm hover:shadow-md transition-shadow">
            <h3 className="text-sm font-medium text-gray-600 mb-2">First Instrumentation</h3>
            <p className="text-2xl font-bold text-gray-900 flex items-center gap-2">
              {user.first_instrumentation_done ? (
                <><CheckCircle className="w-6 h-6 text-green-600" /> Done</>
              ) : (
                'Not Started'
              )}
            </p>
          </div>
        </div>

        {/* Getting Started */}
        <div className="border border-gray-200 rounded-lg p-8 bg-white shadow-sm">
          <h3 className="text-2xl font-bold text-gray-900 mb-4">Getting Started</h3>
          <div className="space-y-4">
            <div className="flex items-start">
              <div className="flex-shrink-0 w-8 h-8 bg-black text-white flex items-center justify-center font-bold mr-4">
                1
              </div>
              <div>
                <h4 className="font-bold text-black mb-1">Create a Contract</h4>
                <p className="text-gray-600 text-sm">
                  Define the rules for your business workflow
                </p>
              </div>
            </div>

            <div className="flex items-start">
              <div className="flex-shrink-0 w-8 h-8 bg-black text-white flex items-center justify-center font-bold mr-4">
                2
              </div>
              <div>
                <h4 className="font-bold text-black mb-1">Start a Thread</h4>
                <p className="text-gray-600 text-sm">
                  Begin tracking a business process instance
                </p>
              </div>
            </div>

            <div className="flex items-start">
              <div className="flex-shrink-0 w-8 h-8 bg-black text-white flex items-center justify-center font-bold mr-4">
                3
              </div>
              <div>
                <h4 className="font-bold text-black mb-1">Instrument Your Code</h4>
                <p className="text-gray-600 text-sm">
                  Add Threadify SDK calls to track steps and get real-time validation
                </p>
              </div>
            </div>
          </div>

          <button onClick={() => window.open("https://docs.threadify.dev", "_blank")} className="mt-6 bg-black text-white px-6 py-3 rounded-lg font-medium hover:bg-gray-800 transition-colors">
            View Documentation
          </button>
        </div>
      </div>
    </AppLayout>
  );
}
