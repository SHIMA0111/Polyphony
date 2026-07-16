package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
	attachmentusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/attachment"
)

// AttachmentHandler handles HTTP requests for attachment endpoints:
// requesting a presigned upload URL, linking an uploaded object to a
// message, and listing a message's attachments with fresh presigned view
// URLs. It delegates business logic to AttachmentUsecase.
type AttachmentHandler struct {
	usecase *attachmentusecase.AttachmentUsecase
}

// NewAttachmentHandler creates a new AttachmentHandler with the given
// AttachmentUsecase.
func NewAttachmentHandler(usecase *attachmentusecase.AttachmentUsecase) *AttachmentHandler {
	return &AttachmentHandler{usecase: usecase}
}

// RequestUpload handles POST /rooms/:roomId/attachments/upload-url. It
// validates the request body, requests a presigned upload ticket from the
// usecase, and returns HTTP 201 with a PresignUploadResponse. It returns
// HTTP 400 for invalid input, an unsupported MIME type, or an oversized
// declared size; HTTP 403 if the user is not a room member; and HTTP 404 if
// the room is not found.
func (h *AttachmentHandler) RequestUpload(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	var req PresignUploadRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}
	if req.MimeType == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "mime_type is required"})
	}

	ticket, err := h.usecase.RequestUpload(c.Request().Context(), userID, roomID, req.MimeType, req.SizeBytes)
	if err != nil {
		return handleAttachmentError(c, err)
	}

	return c.JSON(http.StatusCreated, PresignUploadResponse{
		AttachmentID: ticket.AttachmentID,
		S3Key:        ticket.S3Key,
		UploadURL:    ticket.UploadURL,
		ExpiresAt:    ticket.ExpiresAt,
	})
}

// Attach handles POST /rooms/:roomId/messages/:messageId/attachments. It
// links a previously-uploaded attachment to the given message and returns
// HTTP 200 with an AttachmentResponse. It returns HTTP 400 for invalid
// input, HTTP 403 if the user is not a room member or not the message's
// sender, HTTP 404 if the room/message/attachment is not found, and HTTP 409
// if the attachment is already linked to a message.
func (h *AttachmentHandler) Attach(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")
	messageID := c.Param("messageId")

	var req AttachAttachmentRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid request body"})
	}
	if req.AttachmentID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "attachment_id is required"})
	}

	a, err := h.usecase.AttachToMessage(c.Request().Context(), userID, roomID, messageID, req.AttachmentID)
	if err != nil {
		return handleAttachmentError(c, err)
	}

	return c.JSON(http.StatusOK, toAttachmentResponse(a))
}

// List handles GET /rooms/:roomId/messages/:messageId/attachments. It
// returns HTTP 200 with an AttachmentListResponse containing every
// attachment linked to the message, each with a freshly-presigned view URL.
// It returns HTTP 403 if the user is not a room member and HTTP 404 if the
// room/message is not found.
func (h *AttachmentHandler) List(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")
	messageID := c.Param("messageId")

	attachments, err := h.usecase.ListAttachments(c.Request().Context(), userID, roomID, messageID)
	if err != nil {
		return handleAttachmentError(c, err)
	}

	responses := make([]AttachmentViewResponse, len(attachments))
	for i, a := range attachments {
		responses[i] = toAttachmentViewResponse(a)
	}

	return c.JSON(http.StatusOK, AttachmentListResponse{Attachments: responses})
}

func toAttachmentResponse(a *domainattachment.Attachment) AttachmentResponse {
	return AttachmentResponse{
		ID:        a.ID,
		MessageID: a.MessageID,
		S3Key:     a.S3Key,
		MimeType:  a.MimeType,
		SizeBytes: a.SizeBytes,
		CreatedAt: a.CreatedAt,
	}
}

func toAttachmentViewResponse(a attachmentusecase.AttachmentWithURL) AttachmentViewResponse {
	return AttachmentViewResponse{
		ID:        a.Attachment.ID,
		MessageID: a.Attachment.MessageID,
		S3Key:     a.Attachment.S3Key,
		MimeType:  a.Attachment.MimeType,
		SizeBytes: a.Attachment.SizeBytes,
		ViewURL:   a.ViewURL,
		CreatedAt: a.Attachment.CreatedAt,
	}
}

func handleAttachmentError(c echo.Context, err error) error {
	if errors.Is(err, domain.ErrUnsupportedMimeType) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "unsupported mime type"})
	}
	if errors.Is(err, domain.ErrAttachmentTooLarge) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Message: "attachment too large"})
	}
	if errors.Is(err, domain.ErrForbidden) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Message: "forbidden"})
	}
	if errors.Is(err, domain.ErrNotFound) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Message: "not found"})
	}
	if errors.Is(err, domain.ErrAttachmentAlreadyLinked) {
		return c.JSON(http.StatusConflict, ErrorResponse{Message: "attachment already linked"})
	}
	middleware.GetLogger(c).Error("unhandled attachment error", "error", err)
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
}
