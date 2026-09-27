package orgknowledge

import (
	"net/http"

	"cortisol-server/internal/identity"
)

func principalFrom(r *http.Request) (identity.Principal, bool) {
	return identity.PrincipalFromContext(r.Context())
}
