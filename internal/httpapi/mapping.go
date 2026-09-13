package httpapi

import (
	"net/http"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/waterloostar-api/internal/apierror"
	"github.com/20age1million/waterloostar-api/internal/db/sqlcgen"
	"github.com/20age1million/waterloostar-api/internal/httpapi/gen"
)

// toUser maps a database row to the contract's User.
//
// includeEmail is explicit rather than inferred so that exposing one user's
// address to another requires a deliberate `true` at the call site. The password
// hash has no field to map to at all, which is the safest kind of guarantee.
func toUser(row sqlcgen.User, includeEmail bool) gen.User {
	u := gen.User{
		Id:         openapi_types.UUID(row.ID),
		Username:   row.Username,
		Role:       gen.UserRole(row.Role),
		Verified:   row.Verified,
		Level:      int(row.Level),
		StarPoints: int(row.StarPoints),
		CreatedAt:  row.CreatedAt,
	}

	if includeEmail {
		email := openapi_types.Email(row.Email)
		u.Email = &email
	}

	if row.AvatarUrl != nil {
		u.AvatarUrl = nullable.NewNullableWithValue(*row.AvatarUrl)
	} else {
		u.AvatarUrl = nullable.NewNullNullable[string]()
	}

	return u
}

// errorBody builds the shared envelope for a handler-returned error response.
func errorBody(code apierror.Code, message string) gen.Error {
	return gen.Error{Code: gen.ErrorCode(code), Message: message}
}

// errorBodyWithDetails builds the envelope with field-level problems attached.
func errorBodyWithDetails(code apierror.Code, message string, details map[string]string) gen.Error {
	body := errorBody(code, message)
	body.Details = &details
	return body
}

// Status codes used by handlers that construct envelopes directly.
const (
	statusBadRequest   = http.StatusBadRequest
	statusUnauthorized = http.StatusUnauthorized
	statusConflict     = http.StatusConflict
)
