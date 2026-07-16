package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// newRBACContext builds an Echo context with the given roomId route param
// and authenticated user_id already set, mirroring what JWTAuth would have
// populated upstream of RequireRole in the real middleware chain.
func newRBACContext(userID, roomID string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/rooms/"+roomID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues(roomID)
	c.Set(userIDKey, userID)
	return c, rec
}

func TestRequireRoleReaderDeniedInvokeAI(t *testing.T) {
	repo := &mocks.RoomRepo{}
	repo.SeedMember("room-1", "user-1", string(domainroom.RoleReader))

	mw := RequireRole(repo, domainroom.ActionInvokeAI)
	c, rec := newRBACContext("user-1", "room-1")

	handler := mw(func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRequireRoleGuestDeniedInvokeAIButAllowedSendMessage(t *testing.T) {
	repo := &mocks.RoomRepo{}
	repo.SeedMember("room-1", "user-1", string(domainroom.RoleGuest))

	// Denied: invoke AI.
	mwAI := RequireRole(repo, domainroom.ActionInvokeAI)
	c, rec := newRBACContext("user-1", "room-1")
	if err := mwAI(func(c echo.Context) error { return c.String(http.StatusOK, "ok") })(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for guest invoking AI, got %d", rec.Code)
	}

	// Allowed: send message.
	mwSend := RequireRole(repo, domainroom.ActionSendMessage)
	c2, rec2 := newRBACContext("user-1", "room-1")
	if err := mwSend(func(c echo.Context) error { return c.String(http.StatusOK, "ok") })(c2); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for guest sending a message, got %d", rec2.Code)
	}
}

func TestRequireRoleAdminAllowedManageMembersDeniedDeleteRoom(t *testing.T) {
	repo := &mocks.RoomRepo{}
	repo.SeedMember("room-1", "user-1", string(domainroom.RoleAdmin))

	mwManage := RequireRole(repo, domainroom.ActionManageMembers)
	c, rec := newRBACContext("user-1", "room-1")
	if err := mwManage(func(c echo.Context) error { return c.String(http.StatusOK, "ok") })(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin managing members, got %d", rec.Code)
	}

	mwDelete := RequireRole(repo, domainroom.ActionDeleteRoom)
	c2, rec2 := newRBACContext("user-1", "room-1")
	if err := mwDelete(func(c echo.Context) error { return c.String(http.StatusOK, "ok") })(c2); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin deleting room, got %d", rec2.Code)
	}
}

func TestRequireRoleMasterAllowedEverything(t *testing.T) {
	repo := &mocks.RoomRepo{}
	repo.SeedMember("room-1", "user-1", string(domainroom.RoleMaster))

	actions := []domainroom.Action{
		domainroom.ActionSendMessage,
		domainroom.ActionInvokeAI,
		domainroom.ActionManageMembers,
		domainroom.ActionManageRoom,
		domainroom.ActionDeleteRoom,
	}
	for _, action := range actions {
		mw := RequireRole(repo, action)
		c, rec := newRBACContext("user-1", "room-1")
		if err := mw(func(c echo.Context) error { return c.String(http.StatusOK, "ok") })(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for master on action %s, got %d", action, rec.Code)
		}
	}
}

func TestRequireRoleNonMemberReturns403NotFound(t *testing.T) {
	repo := &mocks.RoomRepo{}
	// No membership seeded for user-1 in room-1 at all.

	mw := RequireRole(repo, domainroom.ActionSendMessage)
	c, rec := newRBACContext("user-1", "room-1")
	if err := mw(func(c echo.Context) error { return c.String(http.StatusOK, "ok") })(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-member (never 404, to avoid leaking room existence), got %d", rec.Code)
	}
}
