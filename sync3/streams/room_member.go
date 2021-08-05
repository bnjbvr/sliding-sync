package streams

import (
	"encoding/json"

	"github.com/matrix-org/sync-v3/state"
	"github.com/matrix-org/sync-v3/sync3"
)

const (
	defaultRoomMemberLimit = 50
	maxRoomMemberLimit     = 1000
)

type RoomMemberSortOrder string

var (
	sortRoomMemberByPL   RoomMemberSortOrder   = "by_pl"
	sortRoomMemberByName RoomMemberSortOrder   = "by_name"
	roomMemberSortOrders []RoomMemberSortOrder = []RoomMemberSortOrder{
		sortRoomMemberByPL,
		sortRoomMemberByName,
	}
)

type FilterRoomMember struct {
	Limit  int64               `json:"limit"`
	RoomID string              `json:"room_id"`
	SortBy RoomMemberSortOrder `json:"sort_by"`
	P      *P                  `json:"p,omitempty"`
}

type RoomMemberResponse struct {
	Limit  int64             `json:"limit"`
	Events []json.RawMessage `json:"events"`
}

// RoomMember represents a stream of room members.
type RoomMember struct {
	storage *state.Storage
}

func NewRoomMember(s *state.Storage) *RoomMember {
	return &RoomMember{s}
}

func (s *RoomMember) Position(tok *sync3.Token) int64 {
	return tok.RoomMemberPosition()
}

func (s *RoomMember) SetPosition(tok *sync3.Token, pos int64) {
	tok.SetRoomMemberPosition(pos)
}

func (s *RoomMember) SessionConfirmed(session *sync3.Session, confirmedPos int64, allSessions bool) {
}

// Extract a chunk of room members from this stream. This stream can operate in 2 modes: paginated and streaming.
//  * If `Request.RoomMember.P` is non-empty, operate in pagination mode and see what page of results to return for `fromExcl`.
//  * If `Request.RoomMember.P` is empty, operate in streaming mode and return the delta between `fromExcl` and `toIncl` (as-is normal)
//
// More specifically, streaming mode is active if and only if `fromExcl` is non-zero (not first sync) and `p` is empty. This will
// then return a delta between `fromExcl` and `toIncl`. Otherwise, it operates in paginated mode. This means the first request from a
// new client is always a paginated request, leaving it up to the client to either pull all members then stream or keep tracking the first
// page of result via the use of
func (s *RoomMember) DataInRange(session *sync3.Session, fromExcl, toIncl int64, request *Request, resp *Response) (int64, error) {
	if request.RoomMember == nil {
		return 0, ErrNotRequested
	}
	if request.RoomMember.P == nil {
		return s.streamingDataInRange(session, fromExcl, toIncl, request, resp)
	}

	// validate P
	var sortOrder RoomMemberSortOrder
	for _, knownSortOrder := range roomMemberSortOrders {
		if request.RoomMember.P.Sort == string(knownSortOrder) {
			sortOrder = RoomMemberSortOrder(request.RoomMember.P.Sort)
		}
	}

	// flesh out the response - if we have been given a position then use it, else default to the latest position (for first syncs)
	paginationPos := fromExcl
	if paginationPos == 0 {
		paginationPos = toIncl
	}
	s.paginatedDataAtPoint(session, paginationPos, sortOrder, request, resp)

	// pagination never advances the token
	return fromExcl, nil
}

func (s *RoomMember) paginatedDataAtPoint(session *sync3.Session, pos int64, sortOrder RoomMemberSortOrder, request *Request, resp *Response) {
	// Load the room members in sorted order at point pos
	// return the right subslice based on P, honouring the limit
}

func (s *RoomMember) streamingDataInRange(session *sync3.Session, fromExcl, toIncl int64, request *Request, resp *Response) (int64, error) {
	// Load the room member delta (honouring the limit) for the room
	return fromExcl, nil
}

/*
Dev notes:

Initially clients will call this stream with a room ID and know nothing about the room. They need to
specify how they want paginated results to be sorted (by PL, by name, etc). They will also need to set a sensible limit depending on their
needs (LB clients may have a limit as low as 5, ele-web may be 50). For small rooms, this may return the entire room member list and no P section.
All is well. For big rooms, a P block is returned and results are sorted by the `sort` value given.
Omission of a sort is valid, and implies "chronological" or "arrival" time, starting at the oldest.

The client then has to decide between incrementally filling in the room member list or leaving
it alone. Ele-Web may do the former but LB clients will do the latter. To fill in, the next sync request
must include the P block with the `P.next` value in it and they MUST NOT advance the since token. They
repeat this operation until all events are received. For LB clients, they do nothing special here as they
already have all the data they are comfortable receiving.

Clients then want to get deltas on the data they already have (full or partial). For full clients, they
just advance their since token and by default they will receive new member events in arrival order: that
is to say the omission of a `sort` implies `sort: arrival`. For LB clients, they advance their token with
a sort order to control whether or not new member events should be returned to them. For example, `sort: by_pl`
with a limit of 5 on the first page of results in a room with 10 admins and 100 regular users would
NOT NOTIFY the client if a regular user joined or left in this API because it doesn't change the first page
of results (think the right-hand-side member list on element-web).

Behind the scenes, the server is tracking a few things. Each event in any room increments the event position,
and this is used to anchor paginated responses (this is what from/to positions are in this file). At any given
event position, the member list is /generally/ treated as immutable. New event positions MAY alter prior state (think merging two forks in the DAG).
If this happens, any existing paginated requests are invalidated and clients will need to start paginating again. <-- TODO

TODO: how much state do we need to remember to do deltas correctly? Specifically for first-page-only thin clients
where in practice we only have arrival deltas and need to then apply them over-the-top of an existing snapshot? Or
grab 2 complete room snapshots and then re-calculate the sort order?
*/