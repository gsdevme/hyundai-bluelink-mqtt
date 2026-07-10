package mock

import "net/http"

// addRoutes is the single place mapping the Bluelink EU URL surface to handlers.
// Keeping the route table in one file makes the mock API easy to find and audit.
func addRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET /auth/api/v2/user/oauth2/authorize", s.handleAuthorize)
	mux.HandleFunc("GET /auth/api/v1/accounts/certs", s.handleCerts)
	mux.HandleFunc("POST /auth/account/signin", s.handleSignin)
	mux.HandleFunc("POST /auth/api/v2/user/oauth2/token", s.handleToken)
	mux.HandleFunc("POST /api/v1/spa/notifications/register", s.handleRegister)
	mux.HandleFunc("GET /api/v1/spa/vehicles", s.handleVehicles)
	mux.HandleFunc("GET /api/v1/spa/vehicles/{id}/ccs2/carstatus/latest", s.handleCached)
	mux.HandleFunc("GET /api/v1/spa/vehicles/{id}/ccs2/carstatus", s.handleForce)
	mux.HandleFunc("GET /api/v1/spa/vehicles/{id}/location/park", s.handleLocation)
}
