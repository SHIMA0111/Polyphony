package room

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainmessage "github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainroomfork "github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// --- ForkRoom ---

// gatedMessageRepo wraps *mocks.MessageRepo, blocking CountAndMaxSequence
// (runForkJob's very first call) until proceed is closed. This lets
// TestForkRoomSuccess assert on ForkRoom's synchronously-returned Job/Room
// before the detached background goroutine it launches can mutate either
// one, without an actual (non-deterministic) data race between the test
// goroutine and the worker.
type gatedMessageRepo struct {
	*mocks.MessageRepo
	proceed chan struct{}
}

func (g *gatedMessageRepo) CountAndMaxSequence(ctx context.Context, roomID string) (int64, int64, error) {
	<-g.proceed
	return g.MessageRepo.CountAndMaxSequence(ctx, roomID)
}

// gatedForkJobRepo wraps *mocks.ForkJobRepo, closing done once the job
// reaches a terminal state, so a test can deterministically wait for the
// background worker to finish before making further assertions.
type gatedForkJobRepo struct {
	*mocks.ForkJobRepo
	done chan struct{}
}

func (g *gatedForkJobRepo) CompleteAndUnarchive(ctx context.Context, id, newRoomID string) error {
	err := g.ForkJobRepo.CompleteAndUnarchive(ctx, id, newRoomID)
	close(g.done)
	return err
}

func (g *gatedForkJobRepo) MarkFailed(ctx context.Context, id string, errMsg string) error {
	err := g.ForkJobRepo.MarkFailed(ctx, id, errMsg)
	close(g.done)
	return err
}

func TestForkRoomForbiddenForMember(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
	ctx := context.Background()

	created, err := uc.CreateRoom(ctx, "owner", "Source Room", "desc")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if err := roomRepo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: created.Room.ID, UserID: "member-user", Role: domainroom.RoleMember,
	}); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	_, _, err = uc.ForkRoom(ctx, "member-user", created.Room.ID)
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for a member caller, got %v", err)
	}
}

func TestForkRoomSuccess(t *testing.T) {
	gate := make(chan struct{})
	done := make(chan struct{})
	msgRepo := &gatedMessageRepo{MessageRepo: &mocks.MessageRepo{}, proceed: gate}
	roomRepo := &mocks.RoomRepo{}
	forkJobRepo := &gatedForkJobRepo{ForkJobRepo: &mocks.ForkJobRepo{Rooms: roomRepo}, done: done}

	uc := NewRoomUsecase(roomRepo, msgRepo, forkJobRepo, nil)
	ctx := context.Background()

	src, err := uc.CreateRoom(ctx, "owner", "Source Room", "desc")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	// Set via UpdateAISettings rather than mutating src.Room directly:
	// mocks.RoomRepo now stores (and returns) clones of every Room, so
	// src.Room is no longer the same object as the fake's internal state —
	// only a real repo write is observed by ForkRoom's own GetByID read
	// below.
	aiProvider, aiModel := "anthropic", "claude-opus-4"
	if err := roomRepo.UpdateAISettings(ctx, src.Room.ID, true, &aiProvider, true, &aiModel); err != nil {
		t.Fatalf("UpdateAISettings failed: %v", err)
	}

	job, newRoom, err := uc.ForkRoom(ctx, "owner", src.Room.ID)
	if err != nil {
		t.Fatalf("ForkRoom failed: %v", err)
	}

	// Assert on the synchronously-returned values before letting the
	// background worker (blocked on gate) proceed.
	if job.Status != domainroomfork.StatusPending {
		t.Fatalf("expected job status pending, got %s", job.Status)
	}
	if job.SourceRoomID != src.Room.ID || job.NewRoomID != newRoom.ID {
		t.Fatalf("job room IDs mismatch: source=%s new=%s (job: source=%s new=%s)",
			src.Room.ID, newRoom.ID, job.SourceRoomID, job.NewRoomID)
	}
	if !newRoom.IsArchived {
		t.Fatal("expected new room to be archived")
	}
	if newRoom.ForkedFromRoomID == nil || *newRoom.ForkedFromRoomID != src.Room.ID {
		t.Fatalf("expected forked_from_room_id %s, got %v", src.Room.ID, newRoom.ForkedFromRoomID)
	}
	if newRoom.Name != "Source Room (Fork)" {
		t.Fatalf("expected name 'Source Room (Fork)', got %s", newRoom.Name)
	}
	if newRoom.AIProvider == nil || *newRoom.AIProvider != aiProvider {
		t.Fatalf("expected ai_provider copied from source, got %v", newRoom.AIProvider)
	}
	if newRoom.AIModel == nil || *newRoom.AIModel != aiModel {
		t.Fatalf("expected ai_model copied from source, got %v", newRoom.AIModel)
	}
	if newRoom.OwnerID != "owner" {
		t.Fatalf("expected owner 'owner', got %s", newRoom.OwnerID)
	}

	// Let the background worker run to completion and wait for it, so the
	// test doesn't leak a goroutine racing past the end of the test.
	close(gate)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for background fork job to complete")
	}

	completed, err := uc.GetForkJobStatus(ctx, "owner", job.ID)
	if err != nil {
		t.Fatalf("GetForkJobStatus failed: %v", err)
	}
	if completed.Status != domainroomfork.StatusCompleted {
		t.Fatalf("expected job to complete, got status %s", completed.Status)
	}
}

// --- GetForkJobStatus ---

func TestGetForkJobStatusMembershipRules(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	forkJobRepo := &mocks.ForkJobRepo{}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo, nil)
	ctx := context.Background()

	roomRepo.SeedMember("source-room", "source-member", "member")
	roomRepo.SeedMember("new-room", "new-room-member", "master")

	now := time.Now()
	job := &domainroomfork.Job{
		ID:           "job-1",
		SourceRoomID: "source-room",
		NewRoomID:    "new-room",
		Status:       domainroomfork.StatusRunning,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := forkJobRepo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if _, err := uc.GetForkJobStatus(ctx, "source-member", "job-1"); err != nil {
		t.Fatalf("expected source-room member to view job status, got error: %v", err)
	}
	if _, err := uc.GetForkJobStatus(ctx, "new-room-member", "job-1"); err != nil {
		t.Fatalf("expected new-room member to view job status, got error: %v", err)
	}
	if _, err := uc.GetForkJobStatus(ctx, "outsider", "job-1"); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for an outsider, got %v", err)
	}
}

func TestGetForkJobStatusNotFound(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	forkJobRepo := &mocks.ForkJobRepo{}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo, nil)
	ctx := context.Background()

	_, err := uc.GetForkJobStatus(ctx, "someone", "nonexistent-job")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// errGetMemberRoomRepo wraps *mocks.RoomRepo, making GetMember return a
// fixed non-domain.ErrNotFound error for one specific roomID (simulating a
// genuine repository failure, e.g. a DB outage), while every other roomID
// falls through to the embedded fake's normal behavior.
type errGetMemberRoomRepo struct {
	*mocks.RoomRepo
	errRoomID string
	err       error
}

func (r *errGetMemberRoomRepo) GetMember(ctx context.Context, roomID, userID string) (*domainroom.RoomMember, error) {
	if roomID == r.errRoomID {
		return nil, r.err
	}
	return r.RoomRepo.GetMember(ctx, roomID, userID)
}

// TestGetForkJobStatusPropagatesNonNotFoundError proves that
// GetForkJobStatus never folds a genuine (non-domain.ErrNotFound)
// GetMember failure into domain.ErrForbidden: a source-room GetMember call
// that fails with an arbitrary repository error must propagate that exact
// error to the caller, rather than being treated the same as "not a
// member" and falling through to the new-room check.
func TestGetForkJobStatusPropagatesNonNotFoundError(t *testing.T) {
	boom := errors.New("boom: db unavailable")
	roomRepo := &errGetMemberRoomRepo{RoomRepo: &mocks.RoomRepo{}, errRoomID: "source-room", err: boom}
	forkJobRepo := &mocks.ForkJobRepo{}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo, nil)
	ctx := context.Background()

	now := time.Now()
	job := &domainroomfork.Job{
		ID: "job-1", SourceRoomID: "source-room", NewRoomID: "new-room",
		Status: domainroomfork.StatusRunning, CreatedAt: now, UpdatedAt: now,
	}
	if err := forkJobRepo.Create(ctx, job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err := uc.GetForkJobStatus(ctx, "someone", "job-1")
	if !errors.Is(err, boom) {
		t.Fatalf("expected the stubbed source-room GetMember error to propagate, got %v", err)
	}
}

// --- runForkJob (synchronous, direct call — no goroutine) ---

// batchedMessageRepo wraps *mocks.MessageRepo, overriding only
// ListByRoomAfter to return a fixed sequence of pre-built batches
// regardless of afterSequence/maxSequence/limit, so a multi-batch copy can
// be exercised deterministically without needing forkBatchSize-many (1000)
// messages. CountAndMaxSequence/ReserveSequenceRange/CreateBatch are
// inherited unmodified from the embedded mock, so the batches are actually
// persisted into Messages via the real (fake) CreateBatch/
// ReserveSequenceRange logic.
type batchedMessageRepo struct {
	*mocks.MessageRepo
	batches [][]*domainmessage.Message
	calls   int
}

func (b *batchedMessageRepo) ListByRoomAfter(_ context.Context, _ string, _ int64, _ int64, _ int) ([]*domainmessage.Message, error) {
	if b.calls >= len(b.batches) {
		return nil, nil
	}
	batch := b.batches[b.calls]
	b.calls++
	return batch, nil
}

// TestRunForkJobMultiBatchBoundaryRemap drives runForkJob synchronously
// (directly, not via ForkRoom's `go` launch) through two batches — sizes 3
// and 2 — where a human/AI pair's two messages straddle the batch boundary
// (the human message is the last of batch 1, its AI reply is the first of
// batch 2). It asserts the job reaches StatusCompleted, SetArchived(...,
// false) was called on the new room, all 5 messages were copied with
// contiguous 1..5 sequences, and the AI reply's InResponseToMessageID was
// remapped to its human counterpart's *new* ID in the destination room
// (never the old, source-room ID).
func TestRunForkJobMultiBatchBoundaryRemap(t *testing.T) {
	const sourceRoomID = "source-room"
	const newRoomID = "new-room"

	now := time.Now()
	humanID := func(n int) string { return "src-human-" + string(rune('0'+n)) }

	batch1 := []*domainmessage.Message{
		{ID: humanID(1), RoomID: sourceRoomID, Content: "msg1", Type: domainmessage.MessageTypeHuman, Status: domainmessage.MessageStatusCompleted, Sequence: 1, CreatedAt: now, UpdatedAt: now},
		{ID: humanID(2), RoomID: sourceRoomID, Content: "msg2", Type: domainmessage.MessageTypeHuman, Status: domainmessage.MessageStatusCompleted, Sequence: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "src-human-boundary", RoomID: sourceRoomID, Content: "boundary question", Type: domainmessage.MessageTypeHuman, Status: domainmessage.MessageStatusCompleted, Sequence: 3, CreatedAt: now, UpdatedAt: now},
	}
	boundaryHumanID := "src-human-boundary"
	batch2 := []*domainmessage.Message{
		{ID: "src-ai-boundary", RoomID: sourceRoomID, Content: "boundary answer", Type: domainmessage.MessageTypeAI, Status: domainmessage.MessageStatusCompleted, Sequence: 4, InResponseToMessageID: &boundaryHumanID, CreatedAt: now, UpdatedAt: now},
		{ID: humanID(5), RoomID: sourceRoomID, Content: "msg5", Type: domainmessage.MessageTypeHuman, Status: domainmessage.MessageStatusCompleted, Sequence: 5, CreatedAt: now, UpdatedAt: now},
	}

	msgRepo := &batchedMessageRepo{
		MessageRepo: &mocks.MessageRepo{
			Messages: map[string]*domainmessage.Message{
				batch1[0].ID: batch1[0], batch1[1].ID: batch1[1], batch1[2].ID: batch1[2],
				batch2[0].ID: batch2[0], batch2[1].ID: batch2[1],
			},
		},
		batches: [][]*domainmessage.Message{batch1, batch2},
	}

	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedRoom(newRoomID, nil)

	forkJobRepo := &mocks.ForkJobRepo{Rooms: roomRepo}
	job := &domainroomfork.Job{
		ID: "job-1", SourceRoomID: sourceRoomID, NewRoomID: newRoomID,
		Status: domainroomfork.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := forkJobRepo.Create(context.Background(), job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	uc := NewRoomUsecase(roomRepo, msgRepo, forkJobRepo, nil)
	uc.runForkJob(context.Background(), job.ID, sourceRoomID, newRoomID)

	got, err := forkJobRepo.GetByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != domainroomfork.StatusCompleted {
		t.Fatalf("expected job to complete, got status %s (error: %v)", got.Status, got.ErrorMessage)
	}
	if got.TotalMessages != 5 {
		t.Fatalf("expected total_messages 5, got %d", got.TotalMessages)
	}
	if got.CopiedMessages != 5 {
		t.Fatalf("expected copied_messages 5, got %d", got.CopiedMessages)
	}

	newRoomAfter, err := roomRepo.GetByID(context.Background(), newRoomID)
	if err != nil {
		t.Fatalf("GetByID(newRoom) failed: %v", err)
	}
	if newRoomAfter.IsArchived {
		t.Fatal("expected new room's is_archived to be cleared after completion")
	}

	// Collect the copied messages (RoomID == newRoomID) and verify
	// contiguous 1..5 sequences plus the cross-batch remap.
	var copiedMsgs []*domainmessage.Message
	for _, m := range msgRepo.Messages {
		if m.RoomID == newRoomID {
			copiedMsgs = append(copiedMsgs, m)
		}
	}
	if len(copiedMsgs) != 5 {
		t.Fatalf("expected 5 copied messages in the new room, got %d", len(copiedMsgs))
	}

	seqSeen := make(map[int64]*domainmessage.Message, len(copiedMsgs))
	for _, m := range copiedMsgs {
		seqSeen[m.Sequence] = m
	}
	for seq := int64(1); seq <= 5; seq++ {
		if _, ok := seqSeen[seq]; !ok {
			t.Fatalf("expected a copied message with sequence %d, none found", seq)
		}
	}

	newBoundaryHuman := seqSeen[3]
	newBoundaryAI := seqSeen[4]
	if newBoundaryAI.Type != domainmessage.MessageTypeAI {
		t.Fatalf("expected sequence 4 to be the AI reply, got type %s", newBoundaryAI.Type)
	}
	if newBoundaryAI.InResponseToMessageID == nil {
		t.Fatal("expected the copied AI reply's in_response_to_message_id to be remapped, got nil")
	}
	if *newBoundaryAI.InResponseToMessageID == boundaryHumanID {
		t.Fatal("expected in_response_to_message_id to be remapped to the new room's message ID, still points at the source ID")
	}
	if *newBoundaryAI.InResponseToMessageID != newBoundaryHuman.ID {
		t.Fatalf("expected in_response_to_message_id %s (new boundary human), got %s", newBoundaryHuman.ID, *newBoundaryAI.InResponseToMessageID)
	}
}

// TestRunForkJobDefensiveNilRemap asserts that if InResponseToMessageID
// points at an ID runForkJob has not (yet) seen — which should not happen
// given SendAIMessage's sequence invariant, but is guarded against
// defensively — the copied field is left nil rather than failing the job.
func TestRunForkJobDefensiveNilRemap(t *testing.T) {
	const sourceRoomID = "source-room"
	const newRoomID = "new-room"

	now := time.Now()
	unseenHumanID := "never-copied"
	batch := []*domainmessage.Message{
		{ID: "src-ai-orphan", RoomID: sourceRoomID, Content: "orphan answer", Type: domainmessage.MessageTypeAI, Status: domainmessage.MessageStatusCompleted, Sequence: 1, InResponseToMessageID: &unseenHumanID, CreatedAt: now, UpdatedAt: now},
	}

	msgRepo := &batchedMessageRepo{
		MessageRepo: &mocks.MessageRepo{
			Messages: map[string]*domainmessage.Message{batch[0].ID: batch[0]},
		},
		batches: [][]*domainmessage.Message{batch},
	}

	roomRepo := &mocks.RoomRepo{}
	roomRepo.SeedRoom(newRoomID, nil)

	forkJobRepo := &mocks.ForkJobRepo{Rooms: roomRepo}
	job := &domainroomfork.Job{
		ID: "job-1", SourceRoomID: sourceRoomID, NewRoomID: newRoomID,
		Status: domainroomfork.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := forkJobRepo.Create(context.Background(), job); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	uc := NewRoomUsecase(roomRepo, msgRepo, forkJobRepo, nil)
	uc.runForkJob(context.Background(), job.ID, sourceRoomID, newRoomID)

	got, err := forkJobRepo.GetByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != domainroomfork.StatusCompleted {
		t.Fatalf("expected job to complete despite the unresolvable remap, got status %s", got.Status)
	}

	for _, m := range msgRepo.Messages {
		if m.RoomID == newRoomID {
			if m.InResponseToMessageID != nil {
				t.Fatalf("expected in_response_to_message_id to be left nil, got %v", *m.InResponseToMessageID)
			}
			return
		}
	}
	t.Fatal("expected exactly one copied message in the new room")
}
