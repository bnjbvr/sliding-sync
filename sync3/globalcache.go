package sync3

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/matrix-org/sync-v3/state"
	"github.com/tidwall/gjson"
)

type GlobalCacheListener interface {
	OnNewEvent(event *EventData)
}

type GlobalCache struct {
	// inserts are done by v2 poll loops, selects are done by v3 request threads
	// there are lots of overlapping keys as many users (threads) can be joined to the same room (key)
	// hence you must lock this with `mu` before r/w
	globalRoomInfo   map[string]*SortableRoom
	globalRoomInfoMu *sync.RWMutex

	listeners   map[int]GlobalCacheListener
	listenersMu *sync.Mutex
	id          int
}

func NewGlobalCache() *GlobalCache {
	return &GlobalCache{
		globalRoomInfo:   make(map[string]*SortableRoom),
		globalRoomInfoMu: &sync.RWMutex{},
		listeners:        make(map[int]GlobalCacheListener),
		listenersMu:      &sync.Mutex{},
	}
}

func (c *GlobalCache) Subsribe(gcl GlobalCacheListener) (id int) {
	c.listenersMu.Lock()
	defer c.listenersMu.Unlock()
	id = c.id
	c.id += 1
	c.listeners[id] = gcl
	return
}

func (c *GlobalCache) Unsubscribe(id int) {
	c.listenersMu.Lock()
	defer c.listenersMu.Unlock()
	delete(c.listeners, id)
}

func (c *GlobalCache) LoadRoom(roomID string) *SortableRoom {
	c.globalRoomInfoMu.RLock()
	defer c.globalRoomInfoMu.RUnlock()
	sr := c.globalRoomInfo[roomID]
	if sr == nil {
		return nil
	}
	srCopy := *sr
	return &srCopy
}

func (c *GlobalCache) AssignRoom(r SortableRoom) {
	c.globalRoomInfoMu.Lock()
	defer c.globalRoomInfoMu.Unlock()
	c.globalRoomInfo[r.RoomID] = &r
}

// =================================================
// Listener functions called by v2 pollers are below
// =================================================

func (c *GlobalCache) OnNewEvents(
	roomID string, events []json.RawMessage, latestPos int64,
) {
	for _, event := range events {
		c.onNewEvent(roomID, event, latestPos)
	}
}

func (c *GlobalCache) onNewEvent(
	roomID string, event json.RawMessage, latestPos int64,
) {
	// parse the event to pull out fields we care about
	var stateKey *string
	ev := gjson.ParseBytes(event)
	if sk := ev.Get("state_key"); sk.Exists() {
		stateKey = &sk.Str
	}
	eventType := ev.Get("type").Str

	// update global state
	c.globalRoomInfoMu.Lock()
	globalRoom := c.globalRoomInfo[roomID]
	if globalRoom == nil {
		globalRoom = &SortableRoom{
			RoomID: roomID,
		}
	}
	if eventType == "m.room.name" && stateKey != nil && *stateKey == "" {
		globalRoom.Name = ev.Get("content.name").Str
	} else if eventType == "m.room.canonical_alias" && stateKey != nil && *stateKey == "" && globalRoom.Name == "" {
		globalRoom.Name = ev.Get("content.alias").Str
	}
	eventTimestamp := ev.Get("origin_server_ts").Int()
	globalRoom.LastMessageTimestamp = eventTimestamp
	globalRoom.LastEventJSON = event
	c.globalRoomInfo[globalRoom.RoomID] = globalRoom
	c.globalRoomInfoMu.Unlock()

	ed := &EventData{
		event:     event,
		roomID:    roomID,
		eventType: eventType,
		stateKey:  stateKey,
		content:   ev.Get("content"),
		latestPos: latestPos,
		timestamp: eventTimestamp,
	}

	// invoke listeners
	for _, l := range c.listeners {
		l.OnNewEvent(ed)
	}
}

// PopulateGlobalCache reads the database and sets data into the cache.
// Must be called prior to starting any v2 pollers else this operation can race. Consider:
//   - V2 poll loop started early
//   - Join event arrives, NID=50
//   - PopulateGlobalCache loads the latest NID=50, processes this join event in the process
//   - OnNewEvents is called with the join event
//   - join event is processed twice.
func PopulateGlobalCache(store *state.Storage, cache *GlobalCache) error {
	// TODO: load last N events as a sliding window?
	latestEvents, err := store.SelectLatestEventInAllRooms()
	if err != nil {
		return fmt.Errorf("failed to load latest event for all rooms: %s", err)
	}
	// every room will be present here
	for _, ev := range latestEvents {
		room := &SortableRoom{
			RoomID: ev.RoomID,
		}
		room.LastEventJSON = ev.JSON
		room.LastMessageTimestamp = gjson.ParseBytes(ev.JSON).Get("origin_server_ts").Int()
		cache.AssignRoom(*room)
	}
	// load state events we care about for sync v3
	roomIDToStateEvents, err := store.CurrentStateEventsInAllRooms([]string{
		"m.room.name", "m.room.canonical_alias",
	})
	if err != nil {
		return fmt.Errorf("failed to load state events for all rooms: %s", err)
	}
	for roomID, stateEvents := range roomIDToStateEvents {
		room := cache.LoadRoom(roomID)
		if room == nil {
			return fmt.Errorf("room %s has no latest event but does have state; this should be impossible", roomID)
		}
		for _, ev := range stateEvents {
			if ev.Type == "m.room.name" && ev.StateKey == "" {
				room.Name = gjson.ParseBytes(ev.JSON).Get("content.name").Str
			} else if ev.Type == "m.room.canonical_alias" && ev.StateKey == "" && room.Name == "" {
				room.Name = gjson.ParseBytes(ev.JSON).Get("content.alias").Str
			}
		}
		cache.AssignRoom(*room)
		fmt.Printf("Room: %s - %s - %s \n", room.RoomID, room.Name, time.Unix(room.LastMessageTimestamp/1000, 0))
	}
	return nil
}
