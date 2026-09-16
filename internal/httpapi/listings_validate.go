package httpapi

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/oapi-codegen/nullable"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// Validation and the small conversions the write handlers need.
//
// The database has check constraints for all of this, and they are the real
// guarantee. These exist so a person gets "the end date must fall after the
// start" against the right field, rather than a constraint violation.

func validateListing(p sqlcgen.CreateListingParams) map[string]string {
	return validateListingRow(sqlcgen.Listing{
		Title: p.Title, PriceCents: p.PriceCents, DepositCents: p.DepositCents,
		StartDate: p.StartDate, EndDate: p.EndDate, LeaseMonths: p.LeaseMonths,
		UnitType: p.UnitType, BedroomsTotal: p.BedroomsTotal, BedroomOf: p.BedroomOf,
		Bathrooms: p.Bathrooms, BathType: p.BathType, AddressLine: p.AddressLine,
		Conditions: p.Conditions, Status: p.Status,
	})
}

// validateListingRow checks a listing as a whole. Edits are checked in this
// form deliberately: moving only the end date can still invert the term, and a
// patch examined in isolation would not notice.
func validateListingRow(l sqlcgen.Listing) map[string]string {
	problems := map[string]string{}

	if len(strings.TrimSpace(l.Title)) < 8 {
		problems["title"] = "Give the place a title of at least 8 characters."
	}
	if l.PriceCents <= 0 {
		problems["price_cents"] = "Rent must be more than zero."
	}
	if l.DepositCents != nil && *l.DepositCents < 0 {
		problems["deposit_cents"] = "A deposit cannot be negative."
	}
	if !l.EndDate.After(l.StartDate) {
		problems["end_date"] = "The end of the term must fall after its start."
	}
	if l.LeaseMonths < 1 || l.LeaseMonths > 24 {
		problems["lease_months"] = "A lease runs between 1 and 24 months."
	}
	if l.BedroomsTotal < 0 || l.BedroomsTotal > 20 {
		problems["bedrooms_total"] = "That is not a plausible number of bedrooms."
	}
	if l.BedroomOf != nil {
		if *l.BedroomOf < 1 {
			problems["bedroom_of"] = "Number the room from 1."
		} else if *l.BedroomOf > l.BedroomsTotal {
			problems["bedroom_of"] = fmt.Sprintf(
				"You cannot offer room %d of %d.", *l.BedroomOf, l.BedroomsTotal)
		}
	}
	if l.Bathrooms < 0 {
		problems["bathrooms"] = "A bathroom count cannot be negative."
	}
	if strings.TrimSpace(l.AddressLine) == "" {
		problems["address_line"] = "Say which street the place is on."
	}
	if len(l.Conditions) > maxConditions {
		problems["conditions"] = fmt.Sprintf("Keep it to %d conditions or fewer.", maxConditions)
	}
	// A studio has no separate bedroom, so numbering one is incoherent.
	if l.UnitType == "studio" && l.BedroomOf != nil {
		problems["bedroom_of"] = "A studio has no separate bedroom to number."
	}
	if l.Status != "" && l.Status != "draft" && l.Status != "published" &&
		l.Status != "paused" && l.Status != "archived" {
		problems["status"] = "Unknown status."
	}

	return problems
}

// applyUpdate mirrors the SQL's COALESCE, so validation sees what the row will
// actually become.
func applyUpdate(l *sqlcgen.Listing, in gen.ListingUpdate) {
	if in.Title != nil {
		l.Title = *in.Title
	}
	if in.Body != nil {
		l.Body = *in.Body
	}
	if in.Conditions != nil {
		l.Conditions = *in.Conditions
	}
	if in.PriceCents != nil {
		l.PriceCents = int32(*in.PriceCents)
	}
	if v, ok := nullableValue(in.DepositCents); ok {
		l.DepositCents = int32Ptr(v)
	}
	if in.StartDate != nil {
		l.StartDate = in.StartDate.Time
	}
	if in.EndDate != nil {
		l.EndDate = in.EndDate.Time
	}
	if in.LeaseMonths != nil {
		l.LeaseMonths = int32(*in.LeaseMonths)
	}
	if in.UnitType != nil {
		l.UnitType = string(*in.UnitType)
	}
	if in.BedroomsTotal != nil {
		l.BedroomsTotal = int32(*in.BedroomsTotal)
	}
	if v, ok := nullableValue(in.BedroomOf); ok {
		l.BedroomOf = int32Ptr(v)
	}
	if in.Bathrooms != nil {
		l.Bathrooms = float64(*in.Bathrooms)
	}
	if in.BathType != nil {
		l.BathType = string(*in.BathType)
	}
	if in.AddressLine != nil {
		l.AddressLine = *in.AddressLine
	}
}

// updateParams maps a partial edit onto the query's parameters. A nil stays
// nil, and the SQL's COALESCE leaves that column as it was.
func updateParams(id uuid.UUID, in gen.ListingUpdate) sqlcgen.UpdateListingParams {
	p := sqlcgen.UpdateListingParams{ID: id}

	p.Title = trimmedPtr(in.Title)
	p.Body = trimmedPtr(in.Body)
	if in.Conditions != nil {
		p.Conditions = trimAll(*in.Conditions)
	}
	p.PriceCents = int32From(in.PriceCents)
	if v, ok := nullableValue(in.DepositCents); ok {
		p.DepositCents = int32Ptr(v)
	}
	if in.StartDate != nil {
		d := in.StartDate.Time
		p.StartDate = &d
	}
	if in.EndDate != nil {
		d := in.EndDate.Time
		p.EndDate = &d
	}
	p.LeaseMonths = int32From(in.LeaseMonths)
	p.TermTag = trimmedPtr(in.TermTag)
	if in.UnitType != nil {
		s := string(*in.UnitType)
		p.UnitType = &s
	}
	p.BedroomsTotal = int32From(in.BedroomsTotal)
	if v, ok := nullableValue(in.BedroomOf); ok {
		p.BedroomOf = int32Ptr(v)
	}
	if in.Bathrooms != nil {
		f := float64(*in.Bathrooms)
		p.Bathrooms = &f
	}
	if in.BathType != nil {
		s := string(*in.BathType)
		p.BathType = &s
	}
	p.Furnished = in.Furnished
	if in.Utilities != nil {
		p.Utilities = utilityStrings(in.Utilities)
	}
	p.Parking = in.Parking
	p.Pets = in.Pets
	p.Laundry = in.Laundry
	p.AddressLine = trimmedPtr(in.AddressLine)
	p.Neighbourhood = trimmedPtr(in.Neighbourhood)
	if v, ok := nullableValue(in.DistanceM); ok {
		p.DistanceM = int32Ptr(v)
	}

	return p
}

func trimmedPtr(v *string) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	return &t
}

func int32From(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

// ------------------------------------------------------- small conversions

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefBool(v *bool) bool {
	return v != nil && *v
}

func derefStrings(v *[]string) []string {
	if v == nil {
		return []string{}
	}
	return *v
}

func trimAll(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func utilityStrings[T ~string](v *[]T) []string {
	if v == nil {
		return []string{}
	}
	out := make([]string, 0, len(*v))
	for _, u := range *v {
		out = append(out, string(u))
	}
	return out
}

// nullableToPtrInt flattens the generated "absent or null or value" into a
// plain pointer, which is all the database needs.
func nullableToPtrInt(n nullable.Nullable[int]) *int {
	if !n.IsSpecified() || n.IsNull() {
		return nil
	}
	v, err := n.Get()
	if err != nil {
		return nil
	}
	return &v
}

// nullableValue reports whether the caller said anything about the field at all.
func nullableValue(n nullable.Nullable[int]) (*int, bool) {
	if !n.IsSpecified() {
		return nil, false
	}
	if n.IsNull() {
		return nil, true
	}
	v, err := n.Get()
	if err != nil {
		return nil, false
	}
	return &v, true
}
