package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRequestLoggerWritesRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := RequestLogger(logger)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Response().Header().Set(echo.HeaderXRequestID, "req-123")

	var capturedLogger *slog.Logger
	handler := mw(func(c echo.Context) error {
		capturedLogger = GetLogger(c)
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if capturedLogger == nil {
		t.Fatal("expected a logger to be set on the Echo context")
	}
	if !strings.Contains(buf.String(), `"request_id":"req-123"`) {
		t.Fatalf("expected log output to contain request_id=req-123, got: %s", buf.String())
	}
}

func TestRequestLoggerLogsErrorLevelOn5xx(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := RequestLogger(logger)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Response().Header().Set(echo.HeaderXRequestID, "req-500")

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusInternalServerError, "boom")
	})

	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !strings.Contains(buf.String(), `"level":"ERROR"`) {
		t.Fatalf("expected an ERROR-level log line for a 5xx response, got: %s", buf.String())
	}
}

// TestRequestLoggerLogsErrorLevelOnUncommittedHTTPError is a regression test:
// when a handler returns an *echo.HTTPError without writing anything to the
// response (as happens when RequestLogger runs before Echo's central error
// handler commits the response), the logged status/level must still reflect
// the error's code rather than the zero-value default status.
func TestRequestLoggerLogsErrorLevelOnUncommittedHTTPError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := RequestLogger(logger)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Response().Header().Set(echo.HeaderXRequestID, "req-boom")

	handler := mw(func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "boom")
	})

	if err := handler(c); err == nil {
		t.Fatal("expected the handler's error to be returned unchanged so Echo's error handler runs")
	}
	if !strings.Contains(buf.String(), `"level":"ERROR"`) {
		t.Fatalf("expected an ERROR-level log line for an uncommitted 500 error, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"status":500`) {
		t.Fatalf("expected status=500 in the log line, got: %s", buf.String())
	}
}

func TestGetLoggerFallsBackToDefault(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	logger := GetLogger(c)
	if logger == nil {
		t.Fatal("expected GetLogger to fall back to a non-nil default logger")
	}
}
