package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
)

// requireAdmin is the gate every /admin handler passes through first. It
// returns the principal, or refuses and reports that it did.
//
// Anyone who is not an admin — an anonymous caller included — gets exactly the
// response the router gives a path that does not exist. A 401 would tell an
// anonymous prober that the path is real and wants a login; a 403 would tell a
// student it is real and wants a role.
//
// "Exactly" is literal. The generated response types encode with a trailing
// newline and a bare application/json, where the router's NoRoute writes
// through gin with a charset — so a refusal built from the generated 404 type
// could still be told apart from an unknown path by its headers. Instead the
// refusal is written here with the same apierror.NotFound the router uses, and
// the handler returns unrouted{}, whose Visit writes nothing more.
//
// The role is read from the access token, not the database. A demoted admin
// keeps it for at most the token's fifteen-minute life, which the Admin and
// Moderation spec accepts; ChangeRole revokes their sessions so it goes no
// further than that.
func requireAdmin(ctx context.Context) (auth.Principal, bool) {
	principal, ok := auth.PrincipalFrom(ctx)
	if ok && principal.IsAdmin() {
		return principal, true
	}

	// The strict handlers are handed the *gin.Context as their context (see
	// ContextWithFallback in router.go), so this always succeeds in the router.
	if gc, isGin := ctx.(*gin.Context); isGin {
		apierror.NotFound(gc, "No endpoint at "+gc.Request.URL.Path)
	}
	return auth.Principal{}, false
}

// unrouted is the response an /admin handler returns after requireAdmin has
// already written the refusal. It satisfies each admin operation's response
// interface and writes nothing.
type unrouted struct{}

func (unrouted) VisitGetAdminOverviewResponse(http.ResponseWriter) error    { return nil }
func (unrouted) VisitListAdminUsersResponse(http.ResponseWriter) error      { return nil }
func (unrouted) VisitGetAdminUserResponse(http.ResponseWriter) error        { return nil }
func (unrouted) VisitListAdminActionsResponse(http.ResponseWriter) error    { return nil }
func (unrouted) VisitSuspendAdminUserResponse(http.ResponseWriter) error    { return nil }
func (unrouted) VisitReinstateAdminUserResponse(http.ResponseWriter) error  { return nil }
func (unrouted) VisitVerifyAdminUserResponse(http.ResponseWriter) error     { return nil }
func (unrouted) VisitSetAdminUserRoleResponse(http.ResponseWriter) error    { return nil }
func (unrouted) VisitListAdminListingsResponse(http.ResponseWriter) error   { return nil }
func (unrouted) VisitRemoveAdminListingResponse(http.ResponseWriter) error  { return nil }
func (unrouted) VisitRestoreAdminListingResponse(http.ResponseWriter) error { return nil }
func (unrouted) VisitListAdminRequestsResponse(http.ResponseWriter) error   { return nil }
func (unrouted) VisitRemoveAdminRequestResponse(http.ResponseWriter) error  { return nil }
func (unrouted) VisitRestoreAdminRequestResponse(http.ResponseWriter) error { return nil }
