package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// Conversations: a listing's owner and a student asking about it.
//
// Four rules shape this file and messages.go:
//
//   - Only the two participants can see a conversation. Everyone else gets 404,
//     never 403, and the participant check is in the SQL.
//   - There is no accept step. Either side can write from the first message on,
//     and either can close the conversation, after which neither can send.
//   - A conversation whose listing was taken down, or whose other participant
//     is suspended, stays readable but read-only. Nothing is hidden from the
//     student who may be the one reporting it, and nothing is rewritten, so
//     reinstating restores it.
//   - Opening a conversation reveals nothing: no email, no phone, no address.
//     The exact address reaches the enquirer only through ShareAddress, the
//     owner's explicit act, and only in that one conversation.

// ------------------------------------------------------------------- start

func (s *Server) StartConversation(ctx context.Context, request gen.StartConversationRequestObject) (gen.StartConversationResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.StartConversation401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to message an owner.")),
		}, nil
	}
	if !principal.Verified {
		return gen.StartConversation403JSONResponse(errorBody(apierror.CodeForbidden,
			"Confirm your uwaterloo.ca address before messaging an owner.")), nil
	}

	// Only a listing the public can see: a draft, paused or taken-down listing,
	// or one whose owner is suspended, answers as though it were never there.
	listing, err := s.queries.GetPublishedListing(ctx, uuid.UUID(request.Id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.StartConversation404JSONResponse{
				NotFoundJSONResponse: gen.NotFoundJSONResponse(
					errorBody(apierror.CodeNotFound, "No listing with that id.")),
			}, nil
		}
		s.log.Error("load listing for conversation", slog.String("error", err.Error()))
		return nil, err
	}
	if listing.Listing.OwnerID == principal.UserID {
		return gen.StartConversation400JSONResponse(errorBody(apierror.CodeValidation,
			"That is your own listing. Students who are interested will message you.")), nil
	}

	var body *string
	if request.Body != nil {
		body = &request.Body.Body
	}
	text, problem := messageBody(body)
	if problem != "" {
		return gen.StartConversation400JSONResponse(
			errorBodyWithDetails(apierror.CodeValidation, problem, map[string]string{"body": problem})), nil
	}

	// Said in words, and naming the conversation, rather than as a constraint
	// violation. Open or closed: closing is what stops a second one.
	if existing, err := s.queries.GetConversationByListingAndEnquirer(ctx, sqlcgen.GetConversationByListingAndEnquirerParams{
		ListingID: listing.Listing.ID, EnquirerID: principal.UserID,
	}); err == nil {
		return gen.StartConversation409JSONResponse(existingConversation(existing)), nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		s.log.Error("check existing conversation", slog.String("error", err.Error()))
		return nil, err
	}

	// The conversation and its first message commit together: a conversation
	// with no message would show the owner an empty thread from a stranger.
	var conversation sqlcgen.Conversation
	err = s.tx(ctx, func(q sqlcgen.Querier) error {
		var err error
		conversation, err = q.CreateConversation(ctx, sqlcgen.CreateConversationParams{
			ListingID:  listing.Listing.ID,
			OwnerID:    listing.Listing.OwnerID,
			EnquirerID: principal.UserID,
			OpenedBy:   principal.UserID,
		})
		if err != nil {
			return err
		}
		_, err = q.CreateMessage(ctx, sqlcgen.CreateMessageParams{
			ConversationID: conversation.ID, SenderID: principal.UserID, Body: text,
		})
		return err
	})
	if err != nil {
		// Two starts at once can both pass the check above; the unique
		// constraint is the real guarantee, and the answer is the same 409.
		if isUniqueViolation(err) {
			if existing, getErr := s.queries.GetConversationByListingAndEnquirer(ctx, sqlcgen.GetConversationByListingAndEnquirerParams{
				ListingID: listing.Listing.ID, EnquirerID: principal.UserID,
			}); getErr == nil {
				return gen.StartConversation409JSONResponse(existingConversation(existing)), nil
			}
		}
		s.log.Error("start conversation", slog.String("error", err.Error()))
		return nil, err
	}

	s.log.Info("conversation started",
		slog.String("conversation_id", conversation.ID.String()),
		slog.String("listing_id", listing.Listing.ID.String()))

	// The owner hears that someone wrote, not what they wrote. The opening
	// message is covered by this email, not by a reply email.
	s.notify(ctx, notification{
		kind: notifyConversationOpened, conversationID: conversation.ID, senderID: principal.UserID,
	})

	view, err := s.conversationFor(ctx, conversation.ID, principal)
	if err != nil {
		s.log.Error("read started conversation", slog.String("error", err.Error()))
		return nil, err
	}
	return gen.StartConversation201JSONResponse(view), nil
}

// existingConversation is the 409 for a second start, with the first one's id
// so a client can open it.
func existingConversation(c sqlcgen.Conversation) gen.Error {
	message := "You already have a conversation about this listing."
	if c.ClosedAt != nil {
		message = "Your conversation about this listing was closed, so another cannot be started."
	}
	return errorBodyWithDetails(apierror.CodeConflict, message,
		map[string]string{"conversation_id": c.ID.String()})
}

// --------------------------------------------------------------------- read

func (s *Server) ListMyConversations(ctx context.Context, request gen.ListMyConversationsRequestObject) (gen.ListMyConversationsResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.ListMyConversations401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to see your messages.")),
		}, nil
	}

	p := request.Params
	page, perPage := paginationFrom(p.Page, p.PerPage)

	// The generated binding takes any string for an enum parameter, so the
	// values are checked here.
	filters := sqlcgen.CountConversationsForUserParams{UserID: principal.UserID}
	if p.Status != nil {
		status := string(*p.Status)
		if status != string(gen.ListMyConversationsParamsStatusOpen) && status != string(gen.ListMyConversationsParamsStatusClosed) {
			return gen.ListMyConversations400JSONResponse{
				BadRequestJSONResponse: gen.BadRequestJSONResponse(
					errorBody(apierror.CodeBadRequest, "status must be open or closed.")),
			}, nil
		}
		filters.Status = &status
	}
	if p.Side != nil {
		side := string(*p.Side)
		if side != string(gen.Owner) && side != string(gen.Enquirer) {
			return gen.ListMyConversations400JSONResponse{
				BadRequestJSONResponse: gen.BadRequestJSONResponse(
					errorBody(apierror.CodeBadRequest, "side must be owner or enquirer.")),
			}, nil
		}
		filters.Side = &side
	}

	total, err := s.queries.CountConversationsForUser(ctx, filters)
	if err != nil {
		s.log.Error("count conversations", slog.String("error", err.Error()))
		return nil, err
	}
	rows, err := s.queries.ListConversationsForUser(ctx, sqlcgen.ListConversationsForUserParams{
		UserID: principal.UserID,
		Status: filters.Status,
		Side:   filters.Side,
		Limit:  int32(perPage),
		Offset: int32((page - 1) * perPage),
	})
	if err != nil {
		s.log.Error("list conversations", slog.String("error", err.Error()))
		return nil, err
	}

	data := make([]gen.Conversation, 0, len(rows))
	for _, r := range rows {
		data = append(data, toConversation(conversationRowFromList(r), principal))
	}
	return gen.ListMyConversations200JSONResponse{Data: data, Meta: pageMeta(page, perPage, total)}, nil
}

func (s *Server) GetUnreadCounts(ctx context.Context, _ gen.GetUnreadCountsRequestObject) (gen.GetUnreadCountsResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.GetUnreadCounts401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to see your messages.")),
		}, nil
	}

	counts, err := s.queries.UnreadCountsForUser(ctx, principal.UserID)
	if err != nil {
		s.log.Error("unread counts", slog.String("error", err.Error()))
		return nil, err
	}
	return gen.GetUnreadCounts200JSONResponse{
		Conversations: int(counts.Conversations),
		Messages:      int(counts.Messages),
	}, nil
}

func (s *Server) GetConversation(ctx context.Context, request gen.GetConversationRequestObject) (gen.GetConversationResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.GetConversation401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to see your messages.")),
		}, nil
	}

	view, err := s.conversationFor(ctx, uuid.UUID(request.Id), principal)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.GetConversation404JSONResponse{NotFoundJSONResponse: conversationNotFound()}, nil
		}
		s.log.Error("load conversation", slog.String("error", err.Error()))
		return nil, err
	}
	return gen.GetConversation200JSONResponse(view), nil
}

// -------------------------------------------------------------- read marker

func (s *Server) MarkConversationRead(ctx context.Context, request gen.MarkConversationReadRequestObject) (gen.MarkConversationReadResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.MarkConversationRead401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to see your messages.")),
		}, nil
	}

	// The participant check is in the UPDATE: no row means not yours.
	affected, err := s.queries.MarkConversationRead(ctx, sqlcgen.MarkConversationReadParams{
		UserID: principal.UserID, ID: uuid.UUID(request.Id),
	})
	if err != nil {
		s.log.Error("mark conversation read", slog.String("error", err.Error()))
		return nil, err
	}
	if affected == 0 {
		return gen.MarkConversationRead404JSONResponse{NotFoundJSONResponse: conversationNotFound()}, nil
	}
	return gen.MarkConversationRead204Response{}, nil
}

// -------------------------------------------------------------------- close

func (s *Server) CloseConversation(ctx context.Context, request gen.CloseConversationRequestObject) (gen.CloseConversationResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.CloseConversation401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to close a conversation.")),
		}, nil
	}
	id := uuid.UUID(request.Id)

	// Read first, so a stranger gets 404 rather than learning anything from
	// the update.
	if _, err := s.queries.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{
		UserID: principal.UserID, ID: id,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.CloseConversation404JSONResponse{NotFoundJSONResponse: conversationNotFound()}, nil
		}
		s.log.Error("load conversation to close", slog.String("error", err.Error()))
		return nil, err
	}

	// No row means it was already closed. That is success: the caller wanted
	// it closed, and it is, by whoever closed it first.
	if _, err := s.queries.CloseConversation(ctx, sqlcgen.CloseConversationParams{
		UserID: principal.UserID, ID: id,
	}); err == nil {
		s.log.Info("conversation closed", slog.String("conversation_id", id.String()))
	} else if !errors.Is(err, pgx.ErrNoRows) {
		s.log.Error("close conversation", slog.String("error", err.Error()))
		return nil, err
	}

	view, err := s.conversationFor(ctx, id, principal)
	if err != nil {
		s.log.Error("read closed conversation", slog.String("error", err.Error()))
		return nil, err
	}
	return gen.CloseConversation200JSONResponse(view), nil
}

// ------------------------------------------------------------ share address

// ShareAddress is the owner sharing the listing's exact address with the
// enquirer of this one conversation. It is the only thing that sets
// address_shared_at, and so the only way the address reaches another student.
// It cannot be undone, and sharing again answers 200 with nothing changed.
func (s *Server) ShareAddress(ctx context.Context, request gen.ShareAddressRequestObject) (gen.ShareAddressResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.ShareAddress401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to share an address.")),
		}, nil
	}
	id := uuid.UUID(request.Id)

	// Participants first, so a stranger gets 404 and learns nothing else.
	row, err := s.queries.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{
		UserID: principal.UserID, ID: id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.ShareAddress404JSONResponse{NotFoundJSONResponse: conversationNotFound()}, nil
		}
		s.log.Error("load conversation to share", slog.String("error", err.Error()))
		return nil, err
	}
	conversation := conversationRowFromGet(row)

	if conversation.Conversation.OwnerID != principal.UserID {
		return gen.ShareAddress403JSONResponse(errorBody(apierror.CodeForbidden,
			"Only the owner can share the address of their place.")), nil
	}
	if !principal.Verified {
		return gen.ShareAddress403JSONResponse(errorBody(apierror.CodeForbidden,
			"Confirm your uwaterloo.ca address before sharing your address.")), nil
	}

	// Already shared: the caller wanted it shared, and it is. Nothing changes,
	// and the first share's time stands.
	if conversation.Conversation.AddressSharedAt != nil {
		return gen.ShareAddress200JSONResponse(toConversation(conversation, principal)), nil
	}

	if reason := shareRefusal(conversation); reason != "" {
		return gen.ShareAddress409JSONResponse(errorBody(apierror.CodeConflict, reason)), nil
	}

	// The listing's address is read here only to refuse an empty one; it is
	// never put in a response from this read.
	listing, err := s.queries.GetListingForOwner(ctx, conversation.Conversation.ListingID)
	if err != nil {
		s.log.Error("load listing to share", slog.String("error", err.Error()))
		return nil, err
	}
	if strings.TrimSpace(listing.AddressLine) == "" {
		return gen.ShareAddress409JSONResponse(errorBody(apierror.CodeConflict, noAddressToShare)), nil
	}

	// The query repeats every condition, so a change between the read above
	// and this write cannot share what should not be shared.
	affected, err := s.queries.ShareConversationAddress(ctx, sqlcgen.ShareConversationAddressParams{
		ID: id, OwnerID: principal.UserID,
	})
	if err != nil {
		s.log.Error("share address", slog.String("error", err.Error()))
		return nil, err
	}

	after, err := s.queries.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{
		UserID: principal.UserID, ID: id,
	})
	if err != nil {
		s.log.Error("read shared conversation", slog.String("error", err.Error()))
		return nil, err
	}
	shared := conversationRowFromGet(after)
	if shared.Conversation.AddressSharedAt == nil {
		// Something changed between the read and the write: say what, if the
		// re-read shows it.
		reason := shareRefusal(shared)
		if reason == "" {
			reason = "The address cannot be shared in this conversation now."
		}
		return gen.ShareAddress409JSONResponse(errorBody(apierror.CodeConflict, reason)), nil
	}
	if affected > 0 {
		s.log.Info("address shared", slog.String("conversation_id", id.String()))
		// Only the first share emails: the enquirer hears that the address
		// is in the conversation, never the address itself.
		s.notify(ctx, notification{kind: notifyAddressShared, conversationID: id, senderID: principal.UserID})
	}
	return gen.ShareAddress200JSONResponse(toConversation(shared, principal)), nil
}

// noAddressToShare is the refusal when the listing has no exact address.
const noAddressToShare = "Add the exact address to your listing before sharing it."

// shareRefusal says why the owner cannot share the address in this
// conversation now, or "" when they can. It is the rule can_share_address
// reports, so the two cannot disagree. Whether it was already shared is not a
// refusal: sharing again succeeds with nothing changed.
func shareRefusal(c conversationRow) string {
	switch {
	case c.Conversation.ClosedAt != nil:
		return "This conversation was closed."
	case c.CounterpartSuspended:
		return "This account is no longer available."
	case c.ListingRemoved || !c.ListingAvailable:
		return "This listing is not available, so its address cannot be shared."
	}
	return ""
}

// --------------------------------------------------------------- shared bits

// conversationFor reads one conversation as principal sees it. pgx.ErrNoRows
// means not found or not theirs, which callers answer alike.
func (s *Server) conversationFor(ctx context.Context, id uuid.UUID, principal auth.Principal) (gen.Conversation, error) {
	row, err := s.queries.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{
		UserID: principal.UserID, ID: id,
	})
	if err != nil {
		return gen.Conversation{}, err
	}
	return toConversation(conversationRowFromGet(row), principal), nil
}

func conversationNotFound() gen.NotFoundJSONResponse {
	return gen.NotFoundJSONResponse(errorBody(apierror.CodeNotFound, "No conversation of yours with that id."))
}
