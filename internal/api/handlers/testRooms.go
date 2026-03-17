package handlers

import (
	"github.com/labstack/echo/v4"
)

type Room struct {
	RoomID string
	Host   string
	Users  map[string]*User
	Epoch  int64
}
type User struct {
	Username string
	Mode     int
}

func (h *Handler) TestRooms(c echo.Context) error {

	rooms := []Room{
		{
			RoomID: "room_1",
			Host:   "fp_a1",
			Epoch:  1,
			Users: map[string]*User{
				"fp_a1": {Username: "alice", Mode: 0},
			},
		},
		{
			RoomID: "room_2",
			Host:   "fp_b1",
			Epoch:  2,
			Users: map[string]*User{
				"fp_b1": {Username: "bob", Mode: 1},
				"fp_a1": {Username: "alice", Mode: 0},
			},
		},
		{
			RoomID: "room_3",
			Host:   "fp_c1",
			Epoch:  1,
			Users: map[string]*User{
				"fp_c1": {Username: "charlie", Mode: 1},
				"fp_d1": {Username: "david", Mode: 0},
				"fp_e1": {Username: "emma", Mode: 0},
			},
		},
		{
			RoomID: "room_4",
			Host:   "fp_f1",
			Epoch:  4,
			Users: map[string]*User{
				"fp_f1": {Username: "frank", Mode: 1},
			},
		},
		{
			RoomID: "room_5",
			Host:   "fp_g1",
			Epoch:  3,
			Users: map[string]*User{
				"fp_g1": {Username: "george", Mode: 1},
				"fp_h1": {Username: "henry", Mode: 0},
				"fp_i1": {Username: "ivan", Mode: 0},
				"fp_j1": {Username: "julia", Mode: 1},
				"fp_k1": {Username: "kate", Mode: 0},
			},
		},
		{
			RoomID: "room_6",
			Host:   "fp_l1",
			Epoch:  5,
			Users: map[string]*User{
				"fp_l1": {Username: "leo", Mode: 1},
				"fp_m1": {Username: "mia", Mode: 0},
			},
		},
		{
			RoomID: "room_7",
			Host:   "fp_n1",
			Epoch:  2,
			Users: map[string]*User{
				"fp_n1": {Username: "nick", Mode: 1},
				"fp_o1": {Username: "olga", Mode: 0},
				"fp_p1": {Username: "paul", Mode: 0},
				"fp_q1": {Username: "quinn", Mode: 1},
			},
		},
		{
			RoomID: "room_8",
			Host:   "fp_r1",
			Epoch:  1,
			Users: map[string]*User{
				"fp_r1": {Username: "rose", Mode: 0},
			},
		},
		{
			RoomID: "room_9",
			Host:   "fp_s1",
			Epoch:  7,
			Users: map[string]*User{
				"fp_s1": {Username: "sam", Mode: 1},
				"fp_t1": {Username: "tom", Mode: 0},
				"fp_u1": {Username: "urs", Mode: 0},
				"fp_v1": {Username: "vera", Mode: 1},
				"fp_w1": {Username: "will", Mode: 0},
				"fp_x1": {Username: "xena", Mode: 0},
				"fp_y1": {Username: "yuri", Mode: 1},
				"fp_z1": {Username: "zack", Mode: 0},
			},
		},
		{
			RoomID: "room_10",
			Host:   "fp_ad1",
			Epoch:  2,
			Users: map[string]*User{
				"fp_ad1": {Username: "adam", Mode: 1},
				"fp_be1": {Username: "bella", Mode: 0},
				"fp_ca1": {Username: "carl", Mode: 0},
				"fp_do1": {Username: "dora", Mode: 1},
				"fp_er1": {Username: "eric", Mode: 0},
				"fp_fa1": {Username: "faye", Mode: 0},
				"fp_ga1": {Username: "gary", Mode: 1},
				"fp_ha1": {Username: "hana", Mode: 0},
				"fp_ia1": {Username: "ian", Mode: 0},
				"fp_ja1": {Username: "jane", Mode: 1},
			},
		},
	}

	return c.JSON(200, rooms)
}
