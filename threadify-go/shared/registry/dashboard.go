package registry

import "context"

type dashboardSessionKey struct{}

// WithDashboardSession marks a request made by a signed-in dashboard user whose
// session, CSRF token and account the Engine has already verified. Such requests
// skip the per-second request limit so the dashboard stays usable while
// integrations are throttled; their bytes still count toward bandwidth.
//
// Only server code can set this marker. Clients cannot supply it through
// headers or cookies.
func WithDashboardSession(ctx context.Context) context.Context {
	return context.WithValue(ctx, dashboardSessionKey{}, true)
}

func isDashboardSession(ctx context.Context) bool {
	marked, _ := ctx.Value(dashboardSessionKey{}).(bool)
	return marked
}
