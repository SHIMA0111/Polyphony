package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	domainattachment "github.com/SHIMA0111/multi-user-ai/server/internal/domain/attachment"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	attachmentusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/attachment"
)

// newAttachmentFixture builds a minimal attachment fixture for directly
// seeding a mocks.AttachmentRepo via Create, bypassing RequestUpload.
// messageID is nil (unlinked) if empty.
func newAttachmentFixture(id, messageID string) *domainattachment.Attachment {
	a := &domainattachment.Attachment{
		ID:        id,
		S3Key:     "attachments/room-1/" + id,
		MimeType:  "image/png",
		SizeBytes: 1024,
		CreatedAt: time.Now(),
	}
	if messageID != "" {
		a.MessageID = &messageID
	}
	return a
}

func setupAttachmentTest(isMember bool) (*echo.Echo, *AttachmentHandler, *mocks.AttachmentRepo, *mocks.MessageRepo) {
	attachmentRepo := &mocks.AttachmentRepo{}
	roomRepo := &mocks.RoomRepo{}
	msgRepo := &mocks.MessageRepo{}
	if isMember {
		roomRepo.SeedMember("room-1", "user-1", "member")
	}
	uc := attachmentusecase.NewAttachmentUsecase(attachmentRepo, roomRepo, msgRepo, &mocks.ObjectStorage{})
	return echo.New(), NewAttachmentHandler(uc), attachmentRepo, msgRepo
}

func TestRequestUploadHandler201(t *testing.T) {
	e, h, _, _ := setupAttachmentTest(true)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/attachments/upload-url",
		strings.NewReader(`{"mime_type":"image/png","size_bytes":1024}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.RequestUpload(c); err != nil {
		t.Fatalf("RequestUpload error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"upload_url"`) {
		t.Fatalf("expected upload_url in response, got %s", rec.Body.String())
	}
}

func TestRequestUploadHandler400UnsupportedMimeType(t *testing.T) {
	e, h, _, _ := setupAttachmentTest(true)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/attachments/upload-url",
		strings.NewReader(`{"mime_type":"application/pdf","size_bytes":100}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.RequestUpload(c); err != nil {
		t.Fatalf("RequestUpload error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRequestUploadHandler400TooLarge(t *testing.T) {
	e, h, _, _ := setupAttachmentTest(true)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/attachments/upload-url",
		strings.NewReader(`{"mime_type":"image/png","size_bytes":99999999999}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.RequestUpload(c); err != nil {
		t.Fatalf("RequestUpload error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRequestUploadHandler403(t *testing.T) {
	e, h, _, _ := setupAttachmentTest(false)

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/attachments/upload-url",
		strings.NewReader(`{"mime_type":"image/png","size_bytes":1024}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "user-1")

	if err := h.RequestUpload(c); err != nil {
		t.Fatalf("RequestUpload error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestAttachHandler200(t *testing.T) {
	e, h, attachmentRepo, msgRepo := setupAttachmentTest(true)
	ctx := context.Background()

	senderID := "user-1"
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", SenderID: &senderID},
	}
	if err := attachmentRepo.Create(ctx, newAttachmentFixture("att-1", "")); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/msg-1/attachments",
		strings.NewReader(`{"attachment_id":"att-1"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "messageId")
	c.SetParamValues("room-1", "msg-1")
	c.Set("user_id", "user-1")

	if err := h.Attach(c); err != nil {
		t.Fatalf("Attach error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"message_id":"msg-1"`) {
		t.Fatalf("expected message_id msg-1 in response, got %s", rec.Body.String())
	}
}

func TestAttachHandler409AlreadyLinked(t *testing.T) {
	e, h, attachmentRepo, msgRepo := setupAttachmentTest(true)
	ctx := context.Background()

	senderID := "user-1"
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", SenderID: &senderID},
	}
	if err := attachmentRepo.Create(ctx, newAttachmentFixture("att-1", "msg-1")); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/messages/msg-1/attachments",
		strings.NewReader(`{"attachment_id":"att-1"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "messageId")
	c.SetParamValues("room-1", "msg-1")
	c.Set("user_id", "user-1")

	if err := h.Attach(c); err != nil {
		t.Fatalf("Attach error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListAttachmentsHandler200(t *testing.T) {
	e, h, attachmentRepo, msgRepo := setupAttachmentTest(true)
	ctx := context.Background()

	senderID := "user-1"
	msgRepo.Messages = map[string]*domainmessage.Message{
		"msg-1": {ID: "msg-1", RoomID: "room-1", SenderID: &senderID},
	}
	if err := attachmentRepo.Create(ctx, newAttachmentFixture("att-1", "msg-1")); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/rooms/room-1/messages/msg-1/attachments", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "messageId")
	c.SetParamValues("room-1", "msg-1")
	c.Set("user_id", "user-1")

	if err := h.List(c); err != nil {
		t.Fatalf("List error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"view_url"`) {
		t.Fatalf("expected view_url in response, got %s", rec.Body.String())
	}
}
