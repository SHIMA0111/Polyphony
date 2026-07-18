package room

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainroomfork "github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
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
//  2. creates a roomfork.Job row in roomfork.StatusPending. If this step
//     fails after the room was already created, the new room would
//     otherwise be stranded — permanently IsArchived with no Job ever able
//     to clear it — so this best-effort-deletes the just-created room
//     (roomRepo.Delete) before returning the original forkJobRepo.Create
//     error; if the compensating delete itself fails, that failure is only
//     logged, never returned, so the caller always sees the real cause of
//     the failure rather than a secondary cleanup error;
//  3. launches the background copy (runForkJob) in a new goroutine bound to
//     a fresh context.Background(), not ctx — the HTTP request that
//     triggered this call must not block on, or cancel, the copy. See
//     runForkJob's doc comment for this background copy's best-effort,
//     non-durable nature.
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
	if err := domainroom.Authorize(member.Role, domainroom.ActionManageRoom); err != nil {
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
		// newRoom was already persisted (above), so without this
		// compensating delete it would be stranded: permanently
		// IsArchived == true with no Job ever tracking it, since the only
		// thing that clears IsArchived is runForkJob completing a Job that
		// now doesn't exist. Best-effort: if the delete itself fails, log
		// it and still return the original forkJobRepo.Create error — a
		// leaked archived room is a lesser, recoverable-by-hand problem
		// compared to masking the actual failure the caller needs to see.
		if delErr := u.roomRepo.Delete(ctx, newRoom.ID); delErr != nil {
			slog.Error("failed to delete orphaned room after fork job creation failure",
				"room_id", newRoom.ID, "create_error", err, "delete_error", delErr)
		}
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
//
// Each of the two GetMember lookups is inspected against all three possible
// outcomes, not just success-or-not: a nil error takes the member path
// immediately; an error that is not domain.ErrNotFound (e.g. a genuine
// database failure) is returned as-is rather than silently treated as "not
// a member" — otherwise a transient DB error on the source-room lookup
// would incorrectly resolve to domain.ErrForbidden even for userID's who
// are, in fact, members, and would do so without ever giving the caller a
// chance to retry a recoverable error. domain.ErrForbidden is only returned
// once both lookups have conclusively resolved to domain.ErrNotFound.
func (u *RoomUsecase) GetForkJobStatus(ctx context.Context, userID, jobID string) (*domainroomfork.Job, error) {
	job, err := u.forkJobRepo.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if _, err := u.roomRepo.GetMember(ctx, job.SourceRoomID, userID); err == nil {
		return job, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	if _, err := u.roomRepo.GetMember(ctx, job.NewRoomID, userID); err == nil {
		return job, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	return nil, domain.ErrForbidden
}

// runForkJob is the background worker launched (detached, via `go`) by
// ForkRoom. It copies every message from sourceRoomID into newRoomID in
// ascending-sequence batches of up to forkBatchSize, then flips newRoomID's
// IsArchived flag off.
//
// This copy is best-effort and not durable across a process restart: the
// Job/progress state lives only in room_fork_jobs (and the partially-copied
// messages already committed to newRoomID), with nothing resembling a work
// queue or lease that a fresh process could pick back up. If the process
// hosting this goroutine crashes or is redeployed mid-copy, the Job is left
// orphaned in roomfork.StatusPending or roomfork.StatusRunning forever — no
// other process will ever resume or re-fail it — and newRoomID stays
// IsArchived == true indefinitely, since nothing ever calls
// forkJobRepo.CompleteAndUnarchive for it. Recovering from that state today
// requires manual intervention (inspecting room_fork_jobs for
// stuck Pending/Running rows past a reasonable age and deciding whether to
// re-run the fork or unarchive the room by hand); a durable, crash-recoverable
// worker (e.g. a persisted job queue a separate process polls) is explicitly
// out of scope here and left to a future follow-up.
//
// Algorithm: it reads sourceRoomID's message count and highest sequence
// number in one atomic snapshot (msgRepo.CountAndMaxSequence) and marks the
// job roomfork.StatusRunning with that total. maxSeq is frozen for the
// entire job — every ListByRoomAfter call below passes this same value,
// never a freshly re-queried "current" max — which both excludes any
// message posted to sourceRoomID after the copy began (they were never part
// of the snapshot being forked) and guarantees the loop terminates even
// under sustained concurrent writes to the source room, since the read
// window [afterSeq, maxSeq] only ever shrinks. It then loops: read up to
// forkBatchSize messages with afterSeq < sequence <= maxSeq
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
// the loop, newRoomID's IsArchived is cleared and the job is marked
// roomfork.StatusCompleted in one call (forkJobRepo.CompleteAndUnarchive),
// rather than as two independent writes: the previous SetArchived-then-
// MarkCompleted sequence left a window where, if MarkCompleted failed after
// SetArchived had already succeeded, the room would be live (accepting
// posts) while its Job stayed stuck in StatusRunning forever, with no
// poller ever able to observe completion. StatusCompleted therefore implies
// the room accepts posts, and the room accepting posts implies
// StatusCompleted — the two facts can no longer disagree.
func (u *RoomUsecase) runForkJob(ctx context.Context, jobID, sourceRoomID, newRoomID string) {
	logger := slog.With("job_id", jobID, "source_room_id", sourceRoomID, "new_room_id", newRoomID)
	logger.Info("room fork job started")

	fail := func(err error) {
		logger.Error("room fork job failed", "error", err)
		if markErr := u.forkJobRepo.MarkFailed(ctx, jobID, err.Error()); markErr != nil {
			logger.Error("failed to mark room fork job failed", "error", markErr)
		}
	}

	total, maxSeq, err := u.msgRepo.CountAndMaxSequence(ctx, sourceRoomID)
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
		batch, err := u.msgRepo.ListByRoomAfter(ctx, sourceRoomID, afterSeq, maxSeq, forkBatchSize)
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

	if err := u.forkJobRepo.CompleteAndUnarchive(ctx, jobID, newRoomID); err != nil {
		fail(err)
		return
	}
	logger.Info("room fork job completed", "copied_messages", copied, "total_messages", total)
}
