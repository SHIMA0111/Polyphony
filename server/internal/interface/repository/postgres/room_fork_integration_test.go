//go:build integration

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroomfork "github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
)

// TestRoomForkIntegration_MultiBatchEndToEnd is the step's required
// end-to-end, >1-batch test: it seeds a source room with 1500 messages
// (more than roomusecase's 1000-message batch size), including a human/AI
// pair positioned exactly at sequences 1000/1001 so the pair straddles the
// batch boundary, then drives the *real* asynchronous path —
// RoomUsecase.ForkRoom's actual `go u.runForkJob(...)` launch, not a direct
// synchronous call — polling GetForkJobStatus until the job completes.
//
// It asserts: the new room ends up with the same message count as the
// source, sequences form a contiguous 1..N range, message order/content/
// sender/type match the source in order, and the human/AI pair's
// in_response_to_message_id correctly points at the pair's *new* IDs in the
// destination room.
func TestRoomForkIntegration_MultiBatchEndToEnd(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	msgRepo := NewMessageRepository(pool)
	forkJobRepo := NewRoomForkRepository(pool)

	sourceRoom := seedUserAndRoom(ctx, t, userRepo, roomRepo, "fork-e2e-owner")

	const totalMessages = 1500
	const boundarySeq = 1000 // roomusecase.forkBatchSize; the human/AI pair straddles this batch boundary

	first, err := msgRepo.ReserveSequenceRange(ctx, sourceRoom.ID, totalMessages)
	if err != nil {
		t.Fatalf("ReserveSequenceRange failed: %v", err)
	}
	if first != 1 {
		t.Fatalf("expected first sequence 1 for a freshly created room, got %d", first)
	}

	now := time.Now()
	msgs := make([]*domainmessage.Message, totalMessages)
	var humanAtBoundaryID string
	for i := 0; i < totalMessages; i++ {
		seq := first + int64(i)
		if seq == boundarySeq+1 {
			msgs[i] = &domainmessage.Message{
				ID:                    uuid.New().String(),
				RoomID:                sourceRoom.ID,
				SenderID:              nil,
				Content:               "AI reply straddling the batch boundary",
				Type:                  domainmessage.MessageTypeAI,
				Status:                domainmessage.MessageStatusCompleted,
				Sequence:              seq,
				InResponseToMessageID: &humanAtBoundaryID,
				Visibility:            domainmessage.MessageVisibilityPublic,
				CreatedAt:             now,
				UpdatedAt:             now,
			}
			continue
		}

		id := uuid.New().String()
		if seq == boundarySeq {
			humanAtBoundaryID = id
		}
		msgs[i] = &domainmessage.Message{
			ID:         id,
			RoomID:     sourceRoom.ID,
			SenderID:   &sourceRoom.OwnerID,
			Content:    fmt.Sprintf("message %d", seq),
			Type:       domainmessage.MessageTypeHuman,
			Status:     domainmessage.MessageStatusCompleted,
			Sequence:   seq,
			Visibility: domainmessage.MessageVisibilityPublic,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}
	if err := msgRepo.CreateBatch(ctx, msgs); err != nil {
		t.Fatalf("seed CreateBatch failed: %v", err)
	}

	uc := roomusecase.NewRoomUsecase(roomRepo, msgRepo, forkJobRepo, nil)

	job, newRoom, err := uc.ForkRoom(ctx, sourceRoom.OwnerID, sourceRoom.ID)
	if err != nil {
		t.Fatalf("ForkRoom failed: %v", err)
	}
	if !newRoom.IsArchived {
		t.Fatal("expected the newly forked room to start archived")
	}

	deadline := time.Now().Add(10 * time.Second)
	var final *domainroomfork.Job
	for time.Now().Before(deadline) {
		final, err = uc.GetForkJobStatus(ctx, sourceRoom.OwnerID, job.ID)
		if err != nil {
			t.Fatalf("GetForkJobStatus failed: %v", err)
		}
		if final.Status == domainroomfork.StatusCompleted || final.Status == domainroomfork.StatusFailed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if final == nil || final.Status != domainroomfork.StatusCompleted {
		var status domainroomfork.Status
		var errMsg *string
		if final != nil {
			status, errMsg = final.Status, final.ErrorMessage
		}
		t.Fatalf("expected fork job to complete within the deadline, got status=%s error=%v", status, errMsg)
	}
	if final.TotalMessages != totalMessages || final.CopiedMessages != totalMessages {
		t.Fatalf("expected total/copied messages %d/%d, got %d/%d", totalMessages, totalMessages, final.TotalMessages, final.CopiedMessages)
	}

	newCount, _, err := msgRepo.CountAndMaxSequence(ctx, newRoom.ID)
	if err != nil {
		t.Fatalf("CountAndMaxSequence (new room) failed: %v", err)
	}
	if newCount != totalMessages {
		t.Fatalf("expected %d messages in the new room, got %d", totalMessages, newCount)
	}

	newMsgs, err := msgRepo.ListByRoomAfter(ctx, newRoom.ID, 0, int64(totalMessages+10), totalMessages+10)
	if err != nil {
		t.Fatalf("ListByRoomAfter (new room) failed: %v", err)
	}
	if len(newMsgs) != totalMessages {
		t.Fatalf("expected %d messages via ListByRoomAfter, got %d", totalMessages, len(newMsgs))
	}

	for i, m := range newMsgs {
		wantSeq := int64(i + 1)
		if m.Sequence != wantSeq {
			t.Fatalf("expected contiguous sequence %d at index %d, got %d", wantSeq, i, m.Sequence)
		}
		src := msgs[i]
		if m.Content != src.Content {
			t.Fatalf("index %d: expected content %q, got %q", i, src.Content, m.Content)
		}
		if m.Type != src.Type {
			t.Fatalf("index %d: expected type %s, got %s", i, src.Type, m.Type)
		}
		if (m.SenderID == nil) != (src.SenderID == nil) {
			t.Fatalf("index %d: sender_id nil-ness mismatch (source nil=%v, copy nil=%v)", i, src.SenderID == nil, m.SenderID == nil)
		}
		if m.SenderID != nil && src.SenderID != nil && *m.SenderID != *src.SenderID {
			t.Fatalf("index %d: expected sender_id %s, got %s", i, *src.SenderID, *m.SenderID)
		}
	}

	// The human/AI pair at the batch boundary: index boundarySeq-1 is the
	// human message (new sequence 1000), index boundarySeq is its AI reply
	// (new sequence 1001).
	newHuman := newMsgs[boundarySeq-1]
	newAI := newMsgs[boundarySeq]
	if newAI.Type != domainmessage.MessageTypeAI {
		t.Fatalf("expected the message at new sequence %d to be the AI reply, got type %s", boundarySeq+1, newAI.Type)
	}
	if newAI.InResponseToMessageID == nil {
		t.Fatal("expected the copied AI reply's in_response_to_message_id to be set")
	}
	if *newAI.InResponseToMessageID == humanAtBoundaryID {
		t.Fatal("expected in_response_to_message_id to be remapped to the new room's ID, still points at the source ID")
	}
	if *newAI.InResponseToMessageID != newHuman.ID {
		t.Fatalf("expected in_response_to_message_id %s (new boundary human), got %s", newHuman.ID, *newAI.InResponseToMessageID)
	}

	finalRoom, err := roomRepo.GetByID(ctx, newRoom.ID)
	if err != nil {
		t.Fatalf("GetByID (new room) failed: %v", err)
	}
	if finalRoom.IsArchived {
		t.Fatal("expected the new room's is_archived to be cleared after job completion")
	}
}
