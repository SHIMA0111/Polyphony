package message

import "errors"

// ErrSequenceConflict is returned by MessageRepository.Create when the
// insert violates the messages_room_sequence_unique UNIQUE(room_id,
// sequence) constraint -- another row already occupies (msg.RoomID,
// msg.Sequence). In normal operation this should not happen: every caller
// obtains msg.Sequence from MessageRepository.ReserveSequenceRange, which
// atomically hands out a range no other caller can also receive. This
// sentinel exists for the cases where that invariant is nonetheless
// violated -- e.g. a caller that bypasses ReserveSequenceRange (a room-fork
// copy, or a defensive retry that accidentally reuses a sequence) -- so
// implementations can surface a stable domain error instead of leaking a
// raw *pgconn.PgError to callers that only depend on this package's
// interface, not on the postgres implementation package.
//
// It carries no external dependencies (only the standard library), so it
// can be used from any layer without creating an upward dependency from the
// domain layer.
var ErrSequenceConflict = errors.New("message: sequence conflict for room")
