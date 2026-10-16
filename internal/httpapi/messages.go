package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/20age1million/WaterlooStar-Backend/internal/apierror"
	"github.com/20age1million/WaterlooStar-Backend/internal/auth"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
	"github.com/20age1million/WaterlooStar-Backend/internal/httpapi/gen"
)

// Messages inside a conversation. The rules are in conversations.go; the one
// added here is when a message can be sent at all.

// maxMessageLength is in characters, as the database's CHECK counts them.
const maxMessageLength = 4000

func (s *Server) ListMessages(ctx context.Context, request gen.ListMessagesRequestObject) (gen.ListMessagesResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.ListMessages401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to see your messages.")),
		}, nil
	}
	id := uuid.UUID(request.Id)

	if _, err := s.queries.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{
		UserID: principal.UserID, ID: id,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.ListMessages404JSONResponse{NotFoundJSONResponse: conversationNotFound()}, nil
		}
		s.log.Error("load conversation for messages", slog.String("error", err.Error()))
		return nil, err
	}

	page, perPage := paginationFrom(request.Params.Page, request.Params.PerPage)
	total, err := s.queries.CountMessages(ctx, id)
	if err != nil {
		s.log.Error("count messages", slog.String("error", err.Error()))
		return nil, err
	}
	rows, err := s.queries.ListMessages(ctx, sqlcgen.ListMessagesParams{
		ConversationID: id,
		Limit:          int32(perPage),
		Offset:         int32((page - 1) * perPage),
	})
	if err != nil {
		s.log.Error("list messages", slog.String("error", err.Error()))
		return nil, err
	}

	data := make([]gen.Message, 0, len(rows))
	for _, m := range rows {
		data = append(data, toMessage(m, principal.UserID))
	}
	return gen.ListMessages200JSONResponse{Data: data, Meta: pageMeta(page, perPage, total)}, nil
}

func (s *Server) SendMessage(ctx context.Context, request gen.SendMessageRequestObject) (gen.SendMessageResponseObject, error) {
	principal, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return gen.SendMessage401JSONResponse{
			UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse(
				errorBody(apierror.CodeUnauthorized, "Log in to send a message.")),
		}, nil
	}
	if !principal.Verified {
		return gen.SendMessage403JSONResponse(errorBody(apierror.CodeForbidden,
			"Confirm your uwaterloo.ca address before sending messages.")), nil
	}
	id := uuid.UUID(request.Id)

	// Participants first, so a stranger learns nothing from a validation error.
	row, err := s.queries.GetConversationForParticipant(ctx, sqlcgen.GetConversationForParticipantParams{
		UserID: principal.UserID, ID: id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.SendMessage404JSONResponse{NotFoundJSONResponse: conversationNotFound()}, nil
		}
		s.log.Error("load conversation to send", slog.String("error", err.Error()))
		return nil, err
	}

	var body *string
	if request.Body != nil {
		body = &request.Body.Body
	}
	text, problem := messageBody(body)
	if problem != "" {
		return gen.SendMessage400JSONResponse{
			BadRequestJSONResponse: gen.BadRequestJSONResponse(
				errorBodyWithDetails(apierror.CodeValidation, problem, map[string]string{"body": problem})),
		}, nil
	}

	// Read-only for a reason the sender can be told. The order is the order a
	// person would want to hear them in.
	if reason := sendRefusal(conversationRowFromGet(row)); reason != "" {
		return gen.SendMessage409JSONResponse(errorBody(apierror.CodeConflict, reason)), nil
	}

	message, err := s.queries.CreateMessage(ctx, sqlcgen.CreateMessageParams{
		ConversationID: id, SenderID: principal.UserID, Body: text,
	})
	if err != nil {
		s.log.Error("send message", slog.String("error", err.Error()))
		return nil, err
	}
	return gen.SendMessage201JSONResponse(toMessage(message, principal.UserID)), nil
}

// sendRefusal says why nothing can be sent in this conversation now, or ""
// when it can. It is the rule can_send reports, so the two cannot disagree.
func sendRefusal(c conversationRow) string {
	switch {
	case c.Conversation.ClosedAt != nil:
		return "This conversation was closed."
	case c.CounterpartSuspended:
		return "This account is no longer available."
	case c.ListingRemoved:
		return "This listing is no longer available."
	}
	return ""
}

// messageBody trims a message and checks it, returning the text to store or a
// problem to show. The database's CHECK is the backstop, not the first line.
func messageBody(body *string) (string, string) {
	if body == nil {
		return "", "Write a message first."
	}
	text := strings.TrimSpace(*body)
	switch {
	case text == "":
		return "", "Write a message first."
	case utf8.RuneCountInString(text) > maxMessageLength:
		return "", "Keep a message under 4000 characters."
	}
	return text, ""
}
