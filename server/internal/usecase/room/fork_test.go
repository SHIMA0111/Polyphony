package room

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
// CompleteAndUnarchive is runForkJob's success-path terminal call (see
// fork.go); MarkFailed remains its failure-path terminal call. closeDone
// guards the close(g.done) with a sync.Once because CompleteAndUnarchive can
// itself fail (e.g. the room-side SetArchived write), in which case
// runForkJob falls through to also call MarkFailed — without the Once,
// that second terminal call would close an already-closed channel and
// panic.
type gatedForkJobRepo struct {
	*mocks.ForkJobRepo
	done     chan struct{}
	doneOnce sync.Once
}

func (g *gatedForkJobRepo) closeDone() {
	g.doneOnce.Do(func() { close(g.done) })
}

func (g *gatedForkJobRepo) CompleteAndUnarchive(ctx context.Context, jobID, newRoomID string) error {
	err := g.ForkJobRepo.CompleteAndUnarchive(ctx, jobID, newRoomID)
	g.closeDone()
	return err
}

func (g *gatedForkJobRepo) MarkFailed(ctx context.Context, id string, errMsg string) error {
	err := g.ForkJobRepo.MarkFailed(ctx, id, errMsg)
	g.closeDone()
	return err
}

func TestForkRoomForbiddenForMember(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{})
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

// TestForkRoomRejectsArchivedSource proves ForkRoom returns
// domain.ErrArchivedRoom, without creating either a new room or a Job, when
// sourceRoomID is itself still archived — e.g. an in-progress fork
// destination whose runForkJob has not yet called CompleteAndUnarchive.
func TestForkRoomRejectsArchivedSource(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	forkJobRepo := &mocks.ForkJobRepo{}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo)
	ctx := context.Background()

	created, err := uc.CreateRoom(ctx, "owner", "Source Room", "desc")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if err := roomRepo.SetArchived(ctx, created.Room.ID, true); err != nil {
		t.Fatalf("SetArchived failed: %v", err)
	}

	job, newRoom, err := uc.ForkRoom(ctx, "owner", created.Room.ID)
	if err != domain.ErrArchivedRoom {
		t.Fatalf("expected ErrArchivedRoom for an archived source room, got %v", err)
	}
	if job != nil || newRoom != nil {
		t.Fatalf("expected nil job/newRoom on rejection, got job=%v newRoom=%v", job, newRoom)
	}
	if len(roomRepo.Rooms) != 1 {
		t.Fatalf("expected no new room to have been created, got %d rooms", len(roomRepo.Rooms))
	}
}

// failingCreateForkJobRepo wraps *mocks.ForkJobRepo, always failing Create
// with createErr while leaving every other method (including GetByID,
// needed to assert the room-fork.Job was in fact never persisted) delegated
// to the embedded mock.
type failingCreateForkJobRepo struct {
	*mocks.ForkJobRepo
	createErr error
}

func (f *failingCreateForkJobRepo) Create(_ context.Context, _ *domainroomfork.Job) error {
	return f.createErr
}

// failingDeleteRoomRepo wraps *mocks.RoomRepo, always failing Delete with
// deleteErr while leaving every other method delegated to the embedded
// mock.
type failingDeleteRoomRepo struct {
	*mocks.RoomRepo
	deleteErr error
}

func (f *failingDeleteRoomRepo) Delete(_ context.Context, _ string) error {
	return f.deleteErr
}

// TestForkRoomCompensatesOrphanedRoomOnJobCreateFailure proves that when
// forkJobRepo.Create fails after roomRepo.Create already succeeded,
// ForkRoom best-effort deletes the just-created room (rather than leaving
// it permanently archived and jobless) and returns forkJobRepo.Create's
// original error.
func TestForkRoomCompensatesOrphanedRoomOnJobCreateFailure(t *testing.T) {
	roomRepo := &mocks.RoomRepo{}
	createErr := errors.New("job create boom")
	forkJobRepo := &failingCreateForkJobRepo{ForkJobRepo: &mocks.ForkJobRepo{}, createErr: createErr}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo)
	ctx := context.Background()

	src, err := uc.CreateRoom(ctx, "owner", "Source Room", "desc")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _, err = uc.ForkRoom(ctx, "owner", src.Room.ID)
	if !errors.Is(err, createErr) {
		t.Fatalf("expected the original forkJobRepo.Create error, got %v", err)
	}

	// The compensating delete must have removed the orphaned room: the only
	// room left in the store is the source room itself.
	if _, err := roomRepo.GetByID(ctx, src.Room.ID); err != nil {
		t.Fatalf("expected the source room to still exist, got error: %v", err)
	}
	if len(roomRepo.Rooms) != 1 {
		t.Fatalf("expected only the source room to remain after compensation, got %d rooms", len(roomRepo.Rooms))
	}
}

// TestForkRoomReturnsOriginalErrorWhenCompensatingDeleteAlsoFails proves
// that if the compensating roomRepo.Delete (triggered by a
// forkJobRepo.Create failure) itself fails, ForkRoom still returns the
// original forkJobRepo.Create error, not the delete error — a leaked
// archived room is a lesser problem than masking the real failure cause.
func TestForkRoomReturnsOriginalErrorWhenCompensatingDeleteAlsoFails(t *testing.T) {
	createErr := errors.New("job create boom")
	deleteErr := errors.New("delete boom")
	roomRepo := &failingDeleteRoomRepo{RoomRepo: &mocks.RoomRepo{}, deleteErr: deleteErr}
	forkJobRepo := &failingCreateForkJobRepo{ForkJobRepo: &mocks.ForkJobRepo{}, createErr: createErr}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo)
	ctx := context.Background()

	src, err := uc.CreateRoom(ctx, "owner", "Source Room", "desc")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _, err = uc.ForkRoom(ctx, "owner", src.Room.ID)
	if !errors.Is(err, createErr) {
		t.Fatalf("expected the original forkJobRepo.Create error even though the compensating delete also failed, got %v", err)
	}
}

func TestForkRoomSuccess(t *testing.T) {
	gate := make(chan struct{})
	done := make(chan struct{})
	msgRepo := &gatedMessageRepo{MessageRepo: &mocks.MessageRepo{}, proceed: gate}
	roomRepo := &mocks.RoomRepo{}
	forkJobRepo := &gatedForkJobRepo{ForkJobRepo: &mocks.ForkJobRepo{Rooms: roomRepo}, done: done}

	uc := NewRoomUsecase(roomRepo, msgRepo, forkJobRepo)
	ctx := context.Background()

	src, err := uc.CreateRoom(ctx, "owner", "Source Room", "desc")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	aiProvider, aiModel := "anthropic", "claude-opus-4"
	src.Room.AIProvider = &aiProvider
	src.Room.AIModel = &aiModel

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
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo)
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
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo)
	ctx := context.Background()

	_, err := uc.GetForkJobStatus(ctx, "someone", "nonexistent-job")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// erroringGetMemberRoomRepo wraps *mocks.RoomRepo, returning getMemberErr
// (a non-domain.ErrNotFound error, e.g. simulating a database failure)
// whenever GetMember is called for erroringRoomID, and delegating to the
// embedded mock for every other room.
type erroringGetMemberRoomRepo struct {
	*mocks.RoomRepo
	erroringRoomID string
	getMemberErr   error
}

func (r *erroringGetMemberRoomRepo) GetMember(ctx context.Context, roomID, userID string) (*domainroom.RoomMember, error) {
	if roomID == r.erroringRoomID {
		return nil, r.getMemberErr
	}
	return r.RoomRepo.GetMember(ctx, roomID, userID)
}

// TestGetForkJobStatusPropagatesNonNotFoundSourceRoomError proves that a
// database failure on the source-room membership lookup is returned as-is
// rather than folded into domain.ErrForbidden, even when the caller would
// have failed the new-room lookup too (so the previous, err == nil-only
// check would have produced ErrForbidden instead of surfacing the real
// failure).
func TestGetForkJobStatusPropagatesNonNotFoundSourceRoomError(t *testing.T) {
	dbErr := errors.New("database is down")
	roomRepo := &erroringGetMemberRoomRepo{
		RoomRepo:       &mocks.RoomRepo{},
		erroringRoomID: "source-room",
		getMemberErr:   dbErr,
	}
	forkJobRepo := &mocks.ForkJobRepo{}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo)
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
	if !errors.Is(err, dbErr) {
		t.Fatalf("expected the source-room GetMember's underlying error to propagate, got %v", err)
	}
}

// TestGetForkJobStatusPropagatesNonNotFoundNewRoomError proves the same
// propagation rule for the *second* (new-room) GetMember lookup: a
// non-ErrNotFound error there is also returned as-is, not folded into
// domain.ErrForbidden, even though the source-room lookup already
// conclusively resolved to ErrNotFound (not a member).
func TestGetForkJobStatusPropagatesNonNotFoundNewRoomError(t *testing.T) {
	dbErr := errors.New("database is down")
	roomRepo := &erroringGetMemberRoomRepo{
		RoomRepo:       &mocks.RoomRepo{},
		erroringRoomID: "new-room",
		getMemberErr:   dbErr,
	}
	forkJobRepo := &mocks.ForkJobRepo{}
	uc := NewRoomUsecase(roomRepo, &mocks.MessageRepo{}, forkJobRepo)
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
	if !errors.Is(err, dbErr) {
		t.Fatalf("expected the new-room GetMember's underlying error to propagate, got %v", err)
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
// batch 2). It asserts the job reaches StatusCompleted, CompleteAndUnarchive
// cleared the new room's IsArchived, all 5 messages were copied with
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

	uc := NewRoomUsecase(roomRepo, msgRepo, forkJobRepo)
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

// delayedVisibilityMessageRepo wraps *mocks.MessageRepo, modeling the gap
// between ReserveSequenceRange committing a sequence number and its row
// later becoming visible (see runForkJob's "Reserved-but-uncommitted tail"
// doc section): delayed is withheld from the underlying store's Messages map
// until ListByRoomAfter has been called revealAfterCalls times, simulating
// an in-flight AI generation whose row lands only partway through the tail
// grace period rather than before the main copy loop's first pass.
type delayedVisibilityMessageRepo struct {
	*mocks.MessageRepo
	delayed          *domainmessage.Message
	revealAfterCalls int
	calls            int
}

func (d *delayedVisibilityMessageRepo) ListByRoomAfter(ctx context.Context, roomID string, afterSequence, maxSequence int64, limit int) ([]*domainmessage.Message, error) {
	d.calls++
	if d.calls > d.revealAfterCalls {
		if _, ok := d.Messages[d.delayed.ID]; !ok {
			d.Messages[d.delayed.ID] = d.delayed
		}
	}
	return d.MessageRepo.ListByRoomAfter(ctx, roomID, afterSequence, maxSequence, limit)
}

// TestRunForkJobTailGraceRecoversLeapfroggedReservation is the regression
// test for the "Reserved-but-uncommitted tail" fix in runForkJob: sequence 6
// commits (and is visible) before sequence 5 -- modeling
// ReserveSequenceRange having reserved 5 for an AI response whose completion
// is still in flight when 6 (a later, unrelated message) finishes and
// commits first -- so CountAndMaxSequence's snapshot reports maxSeq=6 while
// only 5 of those 6 rows actually exist yet. The main copy loop's first pass
// necessarily leapfrogs straight from sequence 4 to 6 and terminates with
// copied=5, short of maxSeq=6. Message 5 becomes visible (revealAfterCalls)
// exactly when the main loop's second (terminating) ListByRoomAfter call
// happens, i.e. before the tail grace period's first re-scan — proving the
// grace period recovers it even though it was never on the main loop's
// (already-advanced-past-5) cursor.
func TestRunForkJobTailGraceRecoversLeapfroggedReservation(t *testing.T) {
	const sourceRoomID = "source-room"
	const newRoomID = "new-room"

	now := time.Now()
	msg := func(n int64) *domainmessage.Message {
		return &domainmessage.Message{
			ID: fmt.Sprintf("src-%d", n), RoomID: sourceRoomID,
			Content: fmt.Sprintf("msg-%d", n), Type: domainmessage.MessageTypeHuman,
			Status: domainmessage.MessageStatusCompleted, Sequence: n,
			CreatedAt: now, UpdatedAt: now,
		}
	}
	delayedMsg := msg(5)

	msgRepo := &delayedVisibilityMessageRepo{
		MessageRepo: &mocks.MessageRepo{
			Messages: map[string]*domainmessage.Message{
				"src-1": msg(1), "src-2": msg(2), "src-3": msg(3), "src-4": msg(4),
				// src-5 deliberately absent: reserved but not yet committed
				// at CountAndMaxSequence's snapshot time.
				"src-6": msg(6),
			},
		},
		delayed:          delayedMsg,
		revealAfterCalls: 1,
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

	uc := NewRoomUsecase(roomRepo, msgRepo, forkJobRepo)
	uc.forkTailGraceDelay = time.Millisecond // keep the test fast; see the field's doc comment

	uc.runForkJob(context.Background(), job.ID, sourceRoomID, newRoomID)

	got, err := forkJobRepo.GetByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != domainroomfork.StatusCompleted {
		t.Fatalf("expected job to complete, got status %s (error: %v)", got.Status, got.ErrorMessage)
	}
	if got.CopiedMessages != 6 {
		t.Fatalf("expected copied_messages 6 (the leapfrogged sequence 5 recovered by the tail grace period), got %d", got.CopiedMessages)
	}

	var copiedMsgs []*domainmessage.Message
	for _, m := range msgRepo.Messages {
		if m.RoomID == newRoomID {
			copiedMsgs = append(copiedMsgs, m)
		}
	}
	if len(copiedMsgs) != 6 {
		t.Fatalf("expected 6 copied messages in the new room, got %d", len(copiedMsgs))
	}
	seqSeen := make(map[int64]*domainmessage.Message, len(copiedMsgs))
	for _, m := range copiedMsgs {
		seqSeen[m.Sequence] = m
	}
	for seq := int64(1); seq <= 6; seq++ {
		if _, ok := seqSeen[seq]; !ok {
			t.Fatalf("expected a copied message with new-room sequence %d, none found", seq)
		}
	}
	if seqSeen[6].Content != "msg-5" {
		t.Fatalf("expected the last-copied message (recovered by the tail grace period) to carry source msg-5's content, got %q", seqSeen[6].Content)
	}
}

// TestRunForkJobTailGraceExhaustsAndCompletesWithResidualGap proves the
// documented residual-window behavior: if a reserved-but-uncommitted
// sequence never becomes visible before the tail grace period's retries are
// exhausted (e.g. the process that reserved it crashed before persisting
// anything for it at all), runForkJob still completes the job -- it does not
// hang or fail -- with that message permanently missing from the fork.
func TestRunForkJobTailGraceExhaustsAndCompletesWithResidualGap(t *testing.T) {
	const sourceRoomID = "source-room"
	const newRoomID = "new-room"

	now := time.Now()
	msg := func(n int64) *domainmessage.Message {
		return &domainmessage.Message{
			ID: fmt.Sprintf("src-%d", n), RoomID: sourceRoomID,
			Content: fmt.Sprintf("msg-%d", n), Type: domainmessage.MessageTypeHuman,
			Status: domainmessage.MessageStatusCompleted, Sequence: n,
			CreatedAt: now, UpdatedAt: now,
		}
	}

	// src-5 is never seeded and never becomes visible (revealAfterCalls set
	// far beyond however many calls this job will ever make), modeling a
	// permanent gap.
	msgRepo := &delayedVisibilityMessageRepo{
		MessageRepo: &mocks.MessageRepo{
			Messages: map[string]*domainmessage.Message{
				"src-1": msg(1), "src-2": msg(2), "src-3": msg(3), "src-4": msg(4),
				"src-6": msg(6),
			},
		},
		delayed:          msg(5),
		revealAfterCalls: 1000,
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

	uc := NewRoomUsecase(roomRepo, msgRepo, forkJobRepo)
	uc.forkTailGraceDelay = time.Millisecond
	uc.forkTailGraceRetries = 2

	uc.runForkJob(context.Background(), job.ID, sourceRoomID, newRoomID)

	got, err := forkJobRepo.GetByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != domainroomfork.StatusCompleted {
		t.Fatalf("expected job to complete despite the permanent gap, got status %s (error: %v)", got.Status, got.ErrorMessage)
	}
	if got.CopiedMessages != 5 {
		t.Fatalf("expected copied_messages 5 (sequence 5 permanently missing), got %d", got.CopiedMessages)
	}

	newRoomAfter, err := roomRepo.GetByID(context.Background(), newRoomID)
	if err != nil {
		t.Fatalf("GetByID(newRoom) failed: %v", err)
	}
	if newRoomAfter.IsArchived {
		t.Fatal("expected new room's is_archived to be cleared even though the fork completed with a residual gap")
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

	uc := NewRoomUsecase(roomRepo, msgRepo, forkJobRepo)
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
