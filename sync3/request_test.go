package sync3

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func TestRoomSubscriptionUnion(t *testing.T) {
	alice := "@alice:localhost"
	bob := "@bob:localhost"
	testCases := []struct {
		name              string
		a                 RoomSubscription
		b                 *RoomSubscription
		me                string
		userInTimeline    func(userID string) bool
		wantQueryStateMap map[string][]string
		matches           [][2]string
		noMatches         [][2]string
	}{
		{
			name:              "single event",
			a:                 RoomSubscription{RequiredState: [][2]string{{"m.room.name", ""}}},
			wantQueryStateMap: map[string][]string{"m.room.name": {""}},
			matches:           [][2]string{{"m.room.name", ""}},
			noMatches:         [][2]string{{"m.room.name2", ""}, {"m.room.name2", "2"}, {"m.room.name", "2"}},
		},
		{
			name: "two disjoint events",
			a:    RoomSubscription{RequiredState: [][2]string{{"m.room.name", ""}, {"m.room.topic", ""}}},
			wantQueryStateMap: map[string][]string{
				"m.room.name":  {""},
				"m.room.topic": {""},
			},
			matches: [][2]string{{"m.room.name", ""}, {"m.room.topic", ""}},
			noMatches: [][2]string{
				{"m.room.name2", ""}, {"m.room.name2", "2"}, {"m.room.name", "2"},
				{"m.room.topic2", ""}, {"m.room.topic2", "2"}, {"m.room.topic", "2"},
			},
		},
		{
			name: "single type, multiple state keys",
			a:    RoomSubscription{RequiredState: [][2]string{{"m.room.name", ""}, {"m.room.name", "foo"}}},
			wantQueryStateMap: map[string][]string{
				"m.room.name": {"", "foo"},
			},
			matches: [][2]string{{"m.room.name", ""}, {"m.room.name", "foo"}},
			noMatches: [][2]string{
				{"m.room.name2", "foo"}, {"m.room.name2", ""}, {"m.room.name", "2"},
			},
		},
		{
			name: "single type, multiple state keys UNION",
			a:    RoomSubscription{RequiredState: [][2]string{{"m.room.name", ""}}},
			b:    &RoomSubscription{RequiredState: [][2]string{{"m.room.name", "foo"}}},
			wantQueryStateMap: map[string][]string{
				"m.room.name": {"", "foo"},
			},
			matches: [][2]string{{"m.room.name", ""}, {"m.room.name", "foo"}},
			noMatches: [][2]string{
				{"m.room.name2", "foo"}, {"m.room.name2", ""}, {"m.room.name", "2"},
			},
		},
		{
			name:              "all events *,*",
			a:                 RoomSubscription{RequiredState: [][2]string{{Wildcard, Wildcard}}},
			wantQueryStateMap: make(map[string][]string),
			matches:           [][2]string{{"m.room.name", ""}, {"m.room.name", "foo"}},
		},
		{
			name:              "all events *,* with other event -> filters",
			a:                 RoomSubscription{RequiredState: [][2]string{{Wildcard, Wildcard}, {"m.specific.name", ""}}},
			wantQueryStateMap: make(map[string][]string),
			matches:           [][2]string{{"m.specific.name", ""}, {"other", "foo"}, {"a", ""}},
			noMatches: [][2]string{
				{"m.specific.name", "foo"},
			},
		},
		{
			name:              "all events *,* with other event UNION",
			a:                 RoomSubscription{RequiredState: [][2]string{{"m.room.name", ""}}},
			b:                 &RoomSubscription{RequiredState: [][2]string{{Wildcard, Wildcard}}},
			wantQueryStateMap: make(map[string][]string),
			matches:           [][2]string{{"m.room.name", ""}, {"a", "b"}},
			noMatches:         [][2]string{{"m.room.name", "foo"}},
		},
		{
			name:              "all events *,* with other events UNION",
			a:                 RoomSubscription{RequiredState: [][2]string{{"m.room.name", ""}, {"m.room.topic", ""}}},
			b:                 &RoomSubscription{RequiredState: [][2]string{{Wildcard, Wildcard}, {"m.room.alias", ""}}},
			wantQueryStateMap: make(map[string][]string),
			matches:           [][2]string{{"m.room.name", ""}, {"a", "b"}, {"m.room.topic", ""}, {"m.room.alias", ""}},
			noMatches:         [][2]string{{"m.room.name", "foo"}, {"m.room.topic", "bar"}, {"m.room.alias", "baz"}},
		},
		{
			name: "wildcard state keys with explicit state keys",
			a:    RoomSubscription{RequiredState: [][2]string{{"m.room.name", Wildcard}, {"m.room.name", ""}}},
			wantQueryStateMap: map[string][]string{
				"m.room.name": nil,
			},
			matches:   [][2]string{{"m.room.name", ""}, {"m.room.name", "foo"}},
			noMatches: [][2]string{{"m.room.name2", ""}, {"foo", "bar"}},
		},
		{
			name:              "wildcard state keys with wildcard event types",
			a:                 RoomSubscription{RequiredState: [][2]string{{"m.room.name", Wildcard}, {Wildcard, "foo"}}},
			wantQueryStateMap: make(map[string][]string),
			matches: [][2]string{
				{"m.room.name", ""}, {"m.room.name", "foo"}, {"name", "foo"},
			},
			noMatches: [][2]string{
				{"m.room.name2", ""}, {"foo", "bar"},
			},
		},
		{
			name:              "wildcard state keys with wildcard event types UNION",
			a:                 RoomSubscription{RequiredState: [][2]string{{"m.room.name", Wildcard}}},
			b:                 &RoomSubscription{RequiredState: [][2]string{{Wildcard, "foo"}}},
			wantQueryStateMap: make(map[string][]string),
			matches: [][2]string{
				{"m.room.name", ""}, {"m.room.name", "foo"}, {"name", "foo"},
			},
			noMatches: [][2]string{
				{"m.room.name2", ""}, {"foo", "bar"},
			},
		},
		{
			name:              "wildcard event types with explicit state keys",
			a:                 RoomSubscription{RequiredState: [][2]string{{Wildcard, "foo"}, {Wildcard, "bar"}, {"m.room.name", ""}}},
			wantQueryStateMap: make(map[string][]string),
			matches:           [][2]string{{"m.room.name", ""}, {"m.room.name", "foo"}, {"name", "foo"}, {"name", "bar"}},
			noMatches:         [][2]string{{"name", "baz"}, {"name", ""}},
		},
		{
			name: "event types with $ME state keys",
			me:   alice,
			a:    RoomSubscription{RequiredState: [][2]string{{"m.room.member", StateKeyMe}}},
			wantQueryStateMap: map[string][]string{
				"m.room.member": {alice},
			},
			matches:   [][2]string{{"m.room.member", alice}},
			noMatches: [][2]string{{"name", "baz"}, {"name", ""}, {"name", StateKeyMe}, {"m.room.name", alice}},
		},
		{
			name:              "wildcard event types with $ME state keys",
			me:                alice,
			a:                 RoomSubscription{RequiredState: [][2]string{{Wildcard, StateKeyMe}}},
			wantQueryStateMap: make(map[string][]string),
			matches:           [][2]string{{"m.room.member", alice}, {"m.room.name", alice}},
			noMatches:         [][2]string{{"name", "baz"}, {"name", ""}, {"name", StateKeyMe}},
		},
		{
			// this is what we expect clients to use, so check it works
			name: "wildcard with $ME",
			me:   alice,
			a: RoomSubscription{RequiredState: [][2]string{
				{"m.room.member", StateKeyMe},
				{Wildcard, Wildcard},
				// Include does not implement lazy loading, so we expect this to do nothing
				{"m.room.member", StateKeyLazy},
			}},
			wantQueryStateMap: make(map[string][]string),
			matches:           [][2]string{{"m.room.member", alice}, {"a", "b"}},
			noMatches:         [][2]string{{"m.room.member", "@someone-else"}, {"m.room.member", ""}, {"m.room.member", bob}},
		},
	}
	for _, tc := range testCases {
		sub := tc.a
		if tc.b != nil {
			sub = tc.a.Combine(*tc.b)
		}
		rsm := sub.RequiredStateMap(tc.me)
		got := rsm.QueryStateMap()
		if !reflect.DeepEqual(got, tc.wantQueryStateMap) {
			t.Errorf("%s: got query state map %+v want %+v", tc.name, got, tc.wantQueryStateMap)
		}
		if tc.matches != nil {
			for _, match := range tc.matches {
				if !rsm.Include(match[0], match[1]) {
					t.Errorf("%s: want '%s' %s' to match but it didn't", tc.name, match[0], match[1])
				}
			}
			for _, noMatch := range tc.noMatches {
				if rsm.Include(noMatch[0], noMatch[1]) {
					t.Errorf("%s: want '%s' %s' to NOT match but it did", tc.name, noMatch[0], noMatch[1])
				}
			}
		}
	}
}

func TestRoomSubscriptionRequiredStateChanged(t *testing.T) {
	a := RoomSubscription{
		TimelineLimit: 5,
		RequiredState: [][2]string{
			{"a", "b"},
			{"c", ""},
		},
	}
	b := RoomSubscription{
		TimelineLimit: 5,
		RequiredState: [][2]string{
			{"a", "b"},
		},
	}
	c := RoomSubscription{
		TimelineLimit: 5,
		RequiredState: [][2]string{
			{"c", ""},
			{"a", "b"},
		},
	}
	assertBool(t, "same required_state", a.RequiredStateChanged(a), false)
	assertBool(t, "different length", a.RequiredStateChanged(b), true)
	// This is TRUE even though semantically it is false
	assertBool(t, "reordered required_state", a.RequiredStateChanged(c), true)
}

type testData struct {
	name string
	next Request
	want Request
}

func TestRequestApplyDeltas(t *testing.T) {
	boolTrue := true
	testCases := []struct {
		input *Request
		tests []struct {
			testData
			wantDelta func(input *Request, d testData) RequestDelta
		}
	}{
		{
			input: nil, // no previous input -> first request
			tests: []struct {
				testData
				wantDelta func(input *Request, d testData) RequestDelta
			}{
				{
					testData: testData{
						name: "initial: room sub only",
						next: Request{
							RoomSubscriptions: map[string]RoomSubscription{
								"!foo:bar": {
									TimelineLimit: 10,
								},
							},
						},
						want: Request{
							Lists: map[string]RequestList{},
							RoomSubscriptions: map[string]RoomSubscription{
								"!foo:bar": {
									TimelineLimit: 10,
								},
							},
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Subs:  []string{"!foo:bar"},
							Lists: map[string]RequestListDelta{},
						}
					},
				},
				{
					testData: testData{
						name: "initial: list only",
						next: Request{
							Lists: map[string]RequestList{
								"a": {
									Ranges: [][2]int64{{0, 20}},
									Sort:   []string{SortByHighlightCount},
								},
							},
						},
						want: Request{
							Lists: map[string]RequestList{
								"a": {
									Ranges: [][2]int64{{0, 20}},
									Sort:   []string{SortByHighlightCount},
								},
							},
							RoomSubscriptions: make(map[string]RoomSubscription),
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Lists: map[string]RequestListDelta{
								"a": {
									Prev: nil,
									Curr: listPtr(d.want.Lists["a"]),
								},
							},
						}
					},
				},
				{
					testData: testData{
						name: "initial: sets sort order to be by_recency if missing",
						next: Request{
							Lists: map[string]RequestList{
								"a": {
									Ranges: [][2]int64{{0, 20}},
								},
							},
						},
						want: Request{
							Lists: map[string]RequestList{
								"a": {
									Ranges: [][2]int64{{0, 20}},
									Sort:   []string{SortByRecency},
								},
							},
							RoomSubscriptions: make(map[string]RoomSubscription),
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Lists: map[string]RequestListDelta{
								"a": {
									Prev: nil,
									Curr: listPtr(d.want.Lists["a"]),
								},
							},
						}
					},
				},
				{
					testData: testData{
						name: "initial: multiple lists",
						next: Request{
							Lists: map[string]RequestList{
								"z": {
									Ranges: [][2]int64{{0, 20}},
									Sort:   []string{SortByHighlightCount},
								},
								"a": {
									Ranges: [][2]int64{{0, 10}},
									Filters: &RequestFilters{
										IsEncrypted: &boolTrue,
									},
									Sort: []string{SortByRecency},
								},
								"b": {
									Ranges: [][2]int64{{0, 5}},
									Sort:   []string{SortByRecency, SortByName},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 11,
										RequiredState: [][2]string{
											{"m.room.create", ""},
										},
									},
								},
							},
						},
						want: Request{
							Lists: map[string]RequestList{
								"z": {
									Ranges: [][2]int64{{0, 20}},
									Sort:   []string{SortByHighlightCount},
								},
								"a": {
									Ranges: [][2]int64{{0, 10}},
									Filters: &RequestFilters{
										IsEncrypted: &boolTrue,
									},
									Sort: []string{SortByRecency},
								},
								"b": {
									Ranges: [][2]int64{{0, 5}},
									Sort:   []string{SortByRecency, SortByName},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 11,
										RequiredState: [][2]string{
											{"m.room.create", ""},
										},
									},
								},
							},
							RoomSubscriptions: make(map[string]RoomSubscription),
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Lists: map[string]RequestListDelta{
								"z": {
									Prev: nil,
									Curr: listPtr(d.want.Lists["z"]),
								},
								"a": {
									Prev: nil,
									Curr: listPtr(d.want.Lists["a"]),
								},
								"b": {
									Prev: nil,
									Curr: listPtr(d.want.Lists["b"]),
								},
							},
						}
					},
				},
				{
					testData: testData{
						name: "initial: list and sub",
						next: Request{
							Lists: map[string]RequestList{
								"f": {
									Ranges: [][2]int64{{0, 20}},
									Sort:   []string{SortByHighlightCount},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{
								"!foo:bar": {
									TimelineLimit: 10,
								},
							},
						},
						want: Request{
							Lists: map[string]RequestList{
								"f": {
									Ranges: [][2]int64{{0, 20}},
									Sort:   []string{SortByHighlightCount},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{
								"!foo:bar": {
									TimelineLimit: 10,
								},
							},
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Subs: []string{"!foo:bar"},
							Lists: map[string]RequestListDelta{
								"f": {
									Prev: nil,
									Curr: listPtr(d.want.Lists["f"]),
								},
							},
						}
					},
				},
			},
		},
		{
			input: &Request{
				Lists: map[string]RequestList{
					"q": {
						Sort: []string{SortByName},
						RoomSubscription: RoomSubscription{
							TimelineLimit: 5,
						},
					},
				},
				RoomSubscriptions: map[string]RoomSubscription{
					"!foo:bar": {
						TimelineLimit: 10,
					},
				},
			},
			tests: []struct {
				testData
				wantDelta func(input *Request, d testData) RequestDelta
			}{
				{
					// check overwriting of sort and updating subs without adding new ones
					testData: testData{
						name: "overwriting of sort and updating subs without adding new ones",
						next: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByRecency},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{
								"!foo:bar": {
									TimelineLimit: 100,
								},
							},
						},
						want: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByRecency},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 5,
									},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{
								"!foo:bar": {
									TimelineLimit: 100,
								},
							},
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Subs:   nil,
							Unsubs: nil,
							Lists: map[string]RequestListDelta{
								"q": {
									Prev: listPtr(input.Lists["q"]),
									Curr: listPtr(d.want.Lists["q"]),
								},
							},
						}
					},
				},
				{
					// check adding a subs
					testData: testData{
						name: "Adding a sub",
						next: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByRecency},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 5,
									},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{
								"!bar:baz": {
									TimelineLimit: 42,
								},
							},
						},
						want: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByRecency},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 5,
									},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{
								"!bar:baz": {
									TimelineLimit: 42,
								},
								"!foo:bar": {
									TimelineLimit: 10,
								},
							},
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Subs:   []string{"!bar:baz"},
							Unsubs: nil,
							Lists: map[string]RequestListDelta{
								"q": {
									Prev: listPtr(input.Lists["q"]),
									Curr: listPtr(d.want.Lists["q"]),
								},
							},
						}
					},
				},
				{
					// check unsubscribing
					testData: testData{
						name: "Unsubscribing",
						next: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByName},
								},
							},
							UnsubscribeRooms: []string{"!foo:bar"},
						},
						want: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByName},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 5,
									},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{},
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Subs:   nil,
							Unsubs: []string{"!foo:bar"},
							Lists: map[string]RequestListDelta{
								"q": {
									Prev: listPtr(input.Lists["q"]),
									Curr: listPtr(d.want.Lists["q"]),
								},
							},
						}
					},
				},
				{
					// check subscribing and unsubscribing = no change
					testData: testData{
						name: "Subscribing/Unsubscribing in one request",
						next: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByRecency},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{
								"!bar:baz": {
									TimelineLimit: 42,
								},
							},
							UnsubscribeRooms: []string{"!bar:baz"},
						},
						want: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByRecency},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 5,
									},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{
								"!foo:bar": {
									TimelineLimit: 10,
								},
							},
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Subs:   nil,
							Unsubs: nil,
							Lists: map[string]RequestListDelta{
								"q": {
									Prev: listPtr(input.Lists["q"]),
									Curr: listPtr(d.want.Lists["q"]),
								},
							},
						}
					},
				},
				{
					testData: testData{
						name: "deleting a list",
						next: Request{
							Lists: map[string]RequestList{
								"q": {
									Deleted: true,
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{},
						},
						want: Request{
							Lists: map[string]RequestList{},
							RoomSubscriptions: map[string]RoomSubscription{
								"!foo:bar": {
									TimelineLimit: 10,
								},
							},
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Subs:   nil,
							Unsubs: nil,
							Lists: map[string]RequestListDelta{
								"q": {
									Prev: listPtr(input.Lists["q"]),
									Curr: nil,
								},
							},
						}
					},
				},
				{
					testData: testData{
						name: "adding a list",
						next: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByRecency},
								},
								"s": {
									Sort: []string{SortByHighlightCount},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 9000,
									},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{},
						},
						want: Request{
							Lists: map[string]RequestList{
								"q": {
									Sort: []string{SortByRecency},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 5,
									},
								},
								"s": {
									Sort: []string{SortByHighlightCount},
									RoomSubscription: RoomSubscription{
										TimelineLimit: 9000,
									},
								},
							},
							RoomSubscriptions: map[string]RoomSubscription{
								"!foo:bar": {
									TimelineLimit: 10,
								},
							},
						},
					},
					wantDelta: func(input *Request, d testData) RequestDelta {
						return RequestDelta{
							Subs:   nil,
							Unsubs: nil,
							Lists: map[string]RequestListDelta{
								"q": {
									Prev: listPtr(input.Lists["q"]),
									Curr: listPtr(d.want.Lists["q"]),
								},
								"s": {
									Prev: nil,
									Curr: listPtr(d.want.Lists["s"]),
								},
							},
						}
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		for _, test := range tc.tests {
			gotRequest, gotDelta := tc.input.ApplyDelta(&test.next)
			jsonEqual(t, test.name, gotRequest, test.want)
			wd := test.wantDelta(tc.input, test.testData)
			jsonEqual(t, test.name, gotDelta, wd)
		}
	}
}

func TestRequestListDiffs(t *testing.T) {
	boolTrue := true
	boolFalse := false
	testCases := []struct {
		name        string
		a           *RequestList
		b           RequestList
		sortChanged *bool
	}{
		{
			name: "initial: set sort",
			a:    nil,
			b: RequestList{
				Sort: []string{SortByHighlightCount},
			},
			sortChanged: &boolTrue,
		},
		{
			name: "same sort",
			a: &RequestList{
				Sort: []string{SortByHighlightCount},
			},
			b: RequestList{
				Sort: []string{SortByHighlightCount},
			},
			sortChanged: &boolFalse,
		},
		{
			name: "changed sort",
			a: &RequestList{
				Sort: []string{SortByHighlightCount},
			},
			b: RequestList{
				Sort: []string{SortByName},
			},
			sortChanged: &boolTrue,
		},
		{
			name: "changed sort additional",
			a: &RequestList{
				Sort: []string{SortByHighlightCount},
			},
			b: RequestList{
				Sort: []string{SortByName, SortByRecency},
			},
			sortChanged: &boolTrue,
		},
		{
			name: "changed sort removal",
			a: &RequestList{
				Sort: []string{SortByName, SortByRecency},
			},
			b: RequestList{
				Sort: []string{SortByName},
			},
			sortChanged: &boolTrue,
		},
	}
	for _, tc := range testCases {
		if tc.sortChanged != nil {
			got := tc.a.SortOrderChanged(&tc.b)
			if got != *tc.sortChanged {
				t.Errorf("SORT: %s : got %v want %v", tc.name, got, *tc.sortChanged)
			}
		}
	}
}

func TestRequestList_CalculateMoveIndexes(t *testing.T) {
	testCases := []struct {
		name        string
		rl          RequestList
		from        int
		to          int
		wantFromTos [][2]int
	}{
		{
			name: "move from inside range to inside range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 10}},
			},
			from:        5,
			to:          0,
			wantFromTos: [][2]int{{5, 0}},
		},
		{
			name: "move from outside range to inside range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 10}},
			},
			from:        15,
			to:          0,
			wantFromTos: [][2]int{{10, 0}},
		},
		{
			name: "move from inside range to outside range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 10}},
			},
			from:        5,
			to:          20,
			wantFromTos: [][2]int{{5, 10}},
		},
		{
			name: "move from outside range to outside range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 10}},
			},
			from: 50,
			to:   20,
		},
		{
			name: "move from outside range to outside range, 1 jump",
			rl: RequestList{
				Ranges: [][2]int64{{10, 20}},
			},
			from:        50,
			to:          2,
			wantFromTos: [][2]int{{20, 10}},
		},
		{
			name: "move from between two ranges to inside first range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 10}, {20, 30}},
			},
			from:        15,
			to:          2,
			wantFromTos: [][2]int{{10, 2}},
		},
		{
			name: "move from between two ranges to inside second range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 10}, {20, 30}},
			},
			from: 15,
			to:   25,
			// Moving from x to y:
			// [0...10]  x [20..y..30]
			// means the timeline is now:
			// [0...10] 11,12,13,14,DELETE,16,17,18,19 [20..INSERT..30]
			// which creates a gap in 15 causing an insert on 25, but we are not tracking 15,
			// so instead 20 gets deleted.
			wantFromTos: [][2]int{{20, 25}},
		},
		{
			name: "move from between two ranges to outside range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 10}, {20, 30}},
			},
			from:        15,
			to:          45,
			wantFromTos: [][2]int{{20, 30}},
		},
		// multiple range fun
		{
			name: "jump over 2 ranges towards zero",
			rl: RequestList{
				Ranges: [][2]int64{{10, 20}, {30, 40}},
			},
			from:        50,
			to:          5,
			wantFromTos: [][2]int{{20, 10}, {40, 30}},
		},
		{
			name: "jump from outside range edge to inside range edge",
			rl: RequestList{
				Ranges: [][2]int64{{10, 20}, {30, 40}},
			},
			from: 30,
			to:   10,
			// Moving from x to y:
			// [10y...20]   [30x....40]
			// means the timeline is now:
			// [30, 10, 11...19] 20, 21 ... 28, [29,31...40]
			// from a window perspective, this means we lost element 20@i=20 and gained element 30@i=10
			// AND
			// we lost element 30@i=30 and gained element 29@i=30.
			wantFromTos: [][2]int{{20, 10}, {30, 30}},
		},
		{
			name: "jump over 2 ranges towards infinity",
			rl: RequestList{
				Ranges: [][2]int64{{10, 20}, {30, 40}},
			},
			from:        5,
			to:          50,
			wantFromTos: [][2]int{{10, 20}, {30, 40}},
		},
		{
			name: "jump over 2 ranges towards zero into a 3rd range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 5}, {10, 20}, {30, 40}},
			},
			from:        50,
			to:          2,
			wantFromTos: [][2]int{{5, 2}, {20, 10}, {40, 30}},
		},
		{
			name: "jump over 2 ranges towards infinity into a 3rd range",
			rl: RequestList{
				Ranges: [][2]int64{{3, 5}, {10, 20}, {30, 40}},
			},
			from:        0,
			to:          35,
			wantFromTos: [][2]int{{3, 5}, {10, 20}, {30, 35}},
		},
		{
			name: "move from inside range to jump over 2 ranges towards zero into a 4th range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 5}, {10, 20}, {30, 40}, {50, 60}},
			},
			from:        55,
			to:          2,
			wantFromTos: [][2]int{{5, 2}, {20, 10}, {40, 30}, {55, 50}},
		},
		{
			name: "move from inside range to jump over 2 ranges towards infinity into a 4th range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 5}, {10, 20}, {30, 40}, {50, 60}},
			},
			from:        2,
			to:          55,
			wantFromTos: [][2]int{{2, 5}, {10, 20}, {30, 40}, {50, 55}},
		},
		{
			name: "move across ranges which are next to each other",
			rl: RequestList{
				Ranges: [][2]int64{{0, 10}, {11, 20}},
			},
			from:        25,
			to:          0,
			wantFromTos: [][2]int{{10, 0}, {20, 11}},
		},
		{ // regression test
			name: "move from outside range to inside range single element",
			rl: RequestList{
				Ranges: [][2]int64{{0, 0}},
			},
			from:        1,
			to:          0,
			wantFromTos: [][2]int{{0, 0}},
		},
	}
	for _, tc := range testCases {
		gots := tc.rl.CalculateMoveIndexes(tc.from, tc.to)
		sort.Slice(gots, func(i, j int) bool {
			return gots[i][0] < gots[j][0]
		})
		if !reflect.DeepEqual(gots, tc.wantFromTos) {
			t.Errorf("%s: from/tos: got %v want %v", tc.name, gots, tc.wantFromTos)
		}
	}
}

func TestRequestList_WriteDeleteOp(t *testing.T) {
	noIndex := -1
	testCases := []struct {
		name             string
		rl               RequestList
		deleteIndex      int
		wantDeletedIndex int
	}{
		{
			name: "basic delete",
			rl: RequestList{
				Ranges: [][2]int64{{0, 20}},
			},
			deleteIndex:      5,
			wantDeletedIndex: 5,
		},
		{
			name: "delete outside range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 20}},
			},
			deleteIndex:      30,
			wantDeletedIndex: noIndex,
		},
		{
			name: "delete edge of range",
			rl: RequestList{
				Ranges: [][2]int64{{0, 20}},
			},
			deleteIndex:      0,
			wantDeletedIndex: 0,
		},
		{
			name: "delete between range no-ops",
			rl: RequestList{
				Ranges: [][2]int64{{0, 20}, {30, 40}},
			},
			deleteIndex:      25,
			wantDeletedIndex: noIndex,
		},
	}
	for _, tc := range testCases {
		gotOp := tc.rl.WriteDeleteOp(tc.deleteIndex)
		if gotOp == nil {
			if tc.wantDeletedIndex == noIndex {
				continue
			}
			t.Errorf("WriteDeleteOp: %s got no ip, wanted %v", tc.name, tc.wantDeletedIndex)
			continue
		}
		if *gotOp.Index != tc.wantDeletedIndex {
			t.Errorf("WriteDeleteOp: %s got %v want %v", tc.name, *gotOp.Index, tc.wantDeletedIndex)
		}
	}
}

func jsonEqual(t *testing.T, name string, got, want interface{}) {
	aa, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("failed to marshal: %s", err)
	}
	bb, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("failed to marshal: %s", err)
	}
	if !bytes.Equal(aa, bb) {
		t.Errorf("%s\ngot  %s\nwant %s", name, string(aa), string(bb))
	}
}

func listPtr(l RequestList) *RequestList {
	return &l
}
