package room

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainroomfork "github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
)

// forkBatchSize is the maximum number of messages runForkJob copies from
// the source room to the destination room in a single
// ListByRoomAfter/ReserveSequenceRange/CreateBatch round trip, per
// phases.md Phase 20's "1000 messages/batch" requirement.
const forkBatchSize = 1000

// ForkRoom creates a new room that is a structural copy of sourceRoomID's
// entire message history (phases.md Phase 20), without blocking on however
// long that copy takes.
//
// The caller must hold domainroom.ActionManageRoom in sourceRoomID (the
// same admin/master-only rule UpdateRoom/UpdateSettings already enforce for
// room-settings changes) — it returns domain.ErrForbidden if the caller
// lacks that role or is not a member of sourceRoomID, and domain.ErrNotFound
// if sourceRoomID does not exist.
//
// On success, it synchronously:
//  1. creates a new room owned by userID, named "<source name> (Fork)",
//     with Description/AIProvider/AIModel copied verbatim from the source
//     room, ForkedFromRoomID pointing at sourceRoomID, and IsArchived true
//     (via roomRepo.Create, which also adds userID as the new room's sole
//     RoleMaster member and initializes its sequence counter — reused
//     unchanged, not duplicated here);
//  2. creates a roomfork.Job row in roomfork.StatusPending;
//  3. launches the background copy (runForkJob) in a new goroutine bound to
//     a fresh context.Background(), not ctx — the HTTP request that
//     triggered this call must not block on, or cancel, the copy.
//
// It returns the created Job and new Room immediately; the new room stays
// IsArchived == true (rejecting new posts with domain.ErrArchivedRoom via
// usecase/message.MessageUsecase.SendMessage/SendAIMessage) until
// runForkJob reaches roomfork.StatusCompleted.
func (u *RoomUsecase) ForkRoom(ctx context.Context, userID, sourceRoomID string) (*domainroomfork.Job, *domainroom.Room, error) {
	member, err := u.getMember(ctx, sourceRoomID, userID)
	if err != nil {
		return nil, nil, err
	}
	if err := middleware.Authorize(member.Role, domainroom.ActionManageRoom); err != nil {
		return nil, nil, err
	}

	src, err := u.roomRepo.GetByID(ctx, sourceRoomID)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	newRoom := &domainroom.Room{
		ID:               uuid.New().String(),
		Name:             src.Name + " (Fork)",
		Description:      src.Description,
		OwnerID:          userID,
		AIProvider:       src.AIProvider,
		AIModel:          src.AIModel,
		ForkedFromRoomID: &sourceRoomID,
		IsArchived:       true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := u.roomRepo.Create(ctx, newRoom); err != nil {
		return nil, nil, err
	}

	job := &domainroomfork.Job{
		ID:           uuid.New().String(),
		SourceRoomID: sourceRoomID,
		NewRoomID:    newRoom.ID,
		Status:       domainroomfork.StatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := u.forkJobRepo.Create(ctx, job); err != nil {
		return nil, nil, err
	}

	// Detached background context: the copy must survive the triggering
	// HTTP request's context being canceled once the request completes.
	go u.runForkJob(context.Background(), job.ID, sourceRoomID, newRoom.ID)

	return job, newRoom, nil
}

// GetForkJobStatus retrieves a room-fork Job's current progress/status.
// The caller must be a member of either job.SourceRoomID or job.NewRoomID
// (source membership is checked first, falling back to new-room
// membership) — it returns domain.ErrForbidden if userID belongs to
// neither, and domain.ErrNotFound if jobID does not exist.
func (u *RoomUsecase) GetForkJobStatus(ctx context.Context, userID, jobID string) (*domainroomfork.Job, error) {
	job, err := u.forkJobRepo.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if _, err := u.roomRepo.GetMember(ctx, job.SourceRoomID, userID); err == nil {
		return job, nil
	}
	if _, err := u.roomRepo.GetMember(ctx, job.NewRoomID, userID); err == nil {
		return job, nil
	}
	return nil, domain.ErrForbidden
}

// runForkJob is the background worker launched (detached, via `go`) by
// ForkRoom. It copies every message from sourceRoomID into newRoomID in
// ascending-sequence batches of up to forkBatchSize, then flips newRoomID's
// IsArchived flag off.
//
// Algorithm: it counts sourceRoomID's messages (msgRepo.CountByRoom) and
// marks the job roomfork.StatusRunning with that total. It then loops:
// read up to forkBatchSize messages with sequence > afterSeq
// (msgRepo.ListByRoomAfter, ascending order) — an empty result ends the
// loop. For a non-empty batch, it reserves a contiguous sequence range in
// newRoomID sized to the batch (msgRepo.ReserveSequenceRange) and builds one
// copied *domainmessage.Message per source message: a new ID, RoomID
// rewritten to newRoomID, Sequence = reserved-range-start + index, every
// other field (SenderID/Content/Type/Status/CreatedAt/UpdatedAt) copied
// verbatim. InResponseToMessageID is remapped through idMap, an
// in-memory map[oldID]newID built incrementally across the *whole* job
// (never reset per batch): because ReserveSequenceRange(ctx, roomID, 2)
// always allocates a human message's sequence strictly before its AI
// response's (see usecase/message.SendAIMessage), and ListByRoomAfter reads
// strictly ascending by sequence, idMap is guaranteed to already hold a
// human message's new ID by the time its AI reply is visited — even when
// the human message was copied in an earlier batch. If
// InResponseToMessageID is set but (contrary to that invariant) not yet in
// idMap, the copied field is left nil rather than failing the whole job.
// The batch is persisted via msgRepo.CreateBatch (all-or-nothing), afterSeq
// advances to the batch's last source sequence, and the job's progress is
// updated (forkJobRepo.UpdateProgress) with the running copied count.
//
// On any error at any step, the job is marked roomfork.StatusFailed with
// the error's message and the function returns without touching
// newRoomID's IsArchived — it stays archived. On successful completion of
// the loop, newRoomID's IsArchived is cleared (roomRepo.SetArchived) before
// the job is marked roomfork.StatusCompleted, so a poller that observes
// StatusCompleted can immediately rely on the room accepting new posts.
func (u *RoomUsecase) runForkJob(ctx context.Context, jobID, sourceRoomID, newRoomID string) {
	logger := slog.With("job_id", jobID, "source_room_id", sourceRoomID, "new_room_id", newRoomID)
	logger.Info("room fork job started")

	fail := func(err error) {
		logger.Error("room fork job failed", "error", err)
		if markErr := u.forkJobRepo.MarkFailed(ctx, jobID, err.Error()); markErr != nil {
			logger.Error("failed to mark room fork job failed", "error", markErr)
		}
	}

	total, err := u.msgRepo.CountByRoom(ctx, sourceRoomID)
	if err != nil {
		fail(err)
		return
	}
	if err := u.forkJobRepo.MarkRunning(ctx, jobID, total); err != nil {
		fail(err)
		return
	}

	idMap := make(map[string]string, total)
	var copied int64
	var afterSeq int64

	for {
		batch, err := u.msgRepo.ListByRoomAfter(ctx, sourceRoomID, afterSeq, forkBatchSize)
		if err != nil {
			fail(err)
			return
		}
		if len(batch) == 0 {
			break
		}

		firstSeq, err := u.msgRepo.ReserveSequenceRange(ctx, newRoomID, int64(len(batch)))
		if err != nil {
			fail(err)
			return
		}

		newMsgs := make([]*domainmessage.Message, len(batch))
		for i, src := range batch {
			newID := uuid.New().String()
			idMap[src.ID] = newID

			var inResponseTo *string
			if src.InResponseToMessageID != nil {
				if mapped, ok := idMap[*src.InResponseToMessageID]; ok {
					inResponseTo = &mapped
				}
				// else: defensively leave nil rather than failing the job
				// (see doc comment — should not happen given the sequence
				// invariant).
			}

			newMsgs[i] = &domainmessage.Message{
				ID:                    newID,
				RoomID:                newRoomID,
				SenderID:              src.SenderID,
				Content:               src.Content,
				Type:                  src.Type,
				Status:                src.Status,
				Sequence:              firstSeq + int64(i),
				InResponseToMessageID: inResponseTo,
				IsDeleted:             src.IsDeleted,
				ExcludeFromAI:         src.ExcludeFromAI,
				Visibility:            src.Visibility,
				CreatedAt:             src.CreatedAt,
				UpdatedAt:             src.UpdatedAt,
			}
		}

		if err := u.msgRepo.CreateBatch(ctx, newMsgs); err != nil {
			fail(err)
			return
		}

		afterSeq = batch[len(batch)-1].Sequence
		copied += int64(len(batch))
		if err := u.forkJobRepo.UpdateProgress(ctx, jobID, copied); err != nil {
			fail(err)
			return
		}
		logger.Info("room fork batch copied", "batch_size", len(batch), "copied_messages", copied, "total_messages", total)
	}

	if err := u.roomRepo.SetArchived(ctx, newRoomID, false); err != nil {
		fail(err)
		return
	}
	if err := u.forkJobRepo.MarkCompleted(ctx, jobID); err != nil {
		logger.Error("failed to mark room fork job completed", "error", err)
		return
	}
	logger.Info("room fork job completed", "copied_messages", copied, "total_messages", total)
}
