package httpapi

import (
	"net/http"
	"time"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
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

// toListing maps a listing row, its owner and its photos to the contract shape.
//
// Nothing is formatted here: no date range string, no "1 of 4 bed", no price
// label. Those are the frontend's business, and inventing them server-side is
// what the prototype's type layer did wrong.
func toListing(row sqlcgen.Listing, owner gen.ListingOwner, photos []sqlcgen.ListingPhoto) gen.Listing {
	out := gen.Listing{
		Id:            openapi_types.UUID(row.ID),
		Title:         row.Title,
		Body:          row.Body,
		Conditions:    row.Conditions,
		PriceCents:    int(row.PriceCents),
		DepositCents:  nullableInt(row.DepositCents),
		StartDate:     openapi_types.Date{Time: row.StartDate},
		EndDate:       openapi_types.Date{Time: row.EndDate},
		LeaseMonths:   int(row.LeaseMonths),
		TermTag:       row.TermTag,
		UnitType:      gen.ListingUnitType(row.UnitType),
		BedroomsTotal: int(row.BedroomsTotal),
		BedroomOf:     nullableInt(row.BedroomOf),
		Bathrooms:     float32(row.Bathrooms),
		BathType:      gen.ListingBathType(row.BathType),
		Furnished:     row.Furnished,
		Utilities:     toUtilities(row.Utilities),
		Parking:       row.Parking,
		Pets:          row.Pets,
		Laundry:       row.Laundry,
		AddressLine:   row.AddressLine,
		Neighbourhood: row.Neighbourhood,

		Lat:              nullableFloat(row.Lat),
		Lng:              nullableFloat(row.Lng),
		DistanceM:        nullableInt(row.DistanceM),
		CommuteMinutes:   nullableInt(row.CommuteMinutes),
		CommuteMode:      gen.ListingCommuteMode(row.CommuteMode),
		MinutesToTransit: nullableInt(row.MinutesToTransit),
		MinutesToGrocery: nullableInt(row.MinutesToGrocery),

		Status:    gen.ListingStatus(row.Status),
		Views:     int(row.Views),
		Replies:   int(row.Replies),
		Saves:     int(row.Saves),
		CreatedAt: row.CreatedAt,

		Owner: owner,
		// An empty array, never null: the client should not have to guard a
		// field that is always a list.
		Photos: make([]gen.ListingPhoto, 0, len(photos)),
	}

	for _, photo := range photos {
		out.Photos = append(out.Photos, gen.ListingPhoto{
			Id:       openapi_types.UUID(photo.ID),
			Url:      photo.Url,
			Position: int(photo.Position),
		})
	}

	return out
}

// toRequest maps a "Looking for Housing" row and its poster to the contract
// shape. As with toListing, nothing is formatted here.
//
// offers is passed in rather than read off the row: it is counted by the read
// queries, because an offer stops counting when its listing is taken down and a
// stored number would drift from what the student can see.
func toRequest(row sqlcgen.HousingRequest, poster gen.ListingOwner, offers int64) gen.HousingRequest {
	out := gen.HousingRequest{
		Id:                 openapi_types.UUID(row.ID),
		Title:              row.Title,
		Body:               row.Body,
		BudgetCents:        int(row.BudgetCents),
		StartDate:          openapi_types.Date{Time: row.StartDate},
		EndDate:            openapi_types.Date{Time: row.EndDate},
		LeaseMonths:        int(row.LeaseMonths),
		TermTag:            row.TermTag,
		Occupants:          int(row.Occupants),
		Pets:               row.Pets,
		FurnishedPreferred: row.FurnishedPreferred,
		ParkingNeeded:      row.ParkingNeeded,
		LaundryNeeded:      row.LaundryNeeded,
		MaxDistanceM:       nullableInt(row.MaxDistanceM),
		Neighbourhood:      row.Neighbourhood,

		Status:    gen.HousingRequestStatus(row.Status),
		Views:     int(row.Views),
		Offers:    int(offers),
		CreatedAt: row.CreatedAt,

		Poster: poster,
	}

	if row.PublishedAt != nil {
		out.PublishedAt = nullable.NewNullableWithValue(*row.PublishedAt)
	} else {
		out.PublishedAt = nullable.NewNullNullable[time.Time]()
	}

	return out
}

func toUtilities(values []string) []gen.ListingUtilities {
	out := make([]gen.ListingUtilities, 0, len(values))
	for _, v := range values {
		out = append(out, gen.ListingUtilities(v))
	}
	return out
}

// The generated types use nullable.Nullable to tell "absent" from "null", so
// each optional column needs an explicit conversion rather than a bare pointer.

func nullableInt(value *int32) nullable.Nullable[int] {
	if value == nil {
		return nullable.NewNullNullable[int]()
	}
	return nullable.NewNullableWithValue(int(*value))
}

func nullableFloat(value *float64) nullable.Nullable[float32] {
	if value == nil {
		return nullable.NewNullNullable[float32]()
	}
	return nullable.NewNullableWithValue(float32(*value))
}

func nullableString(value *string) nullable.Nullable[string] {
	if value == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*value)
}
