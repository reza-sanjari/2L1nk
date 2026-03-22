package hub

import (
	"2L1nk/internal/logger"
	"2L1nk/internal/session"

	"go.uber.org/zap"
)

type Hub struct {
	logg              *logger.Logger
	s                 *session.Store
	Rooms             map[string]*Room
	Users             map[string]*User
	Events            chan HubEvent
	Broadcast         chan BroadcastRequest
	InboundMessages   chan WSMessageEnvelope
	RegisterRoom      chan CreateRoomRequest
	UnregisterRoom    chan string
	RegisterUser      chan *User
	UnregisterUser    chan *User
	JoinRoom          chan RoomMembersChangeRequest
	LeaveRoom         chan RoomMembersChangeRequest
	AddToRoom         chan AddToRoomRequest
	RestoreRoom       chan RestoreRoomRequest
	RemoveFromRoom    chan RemoveFromRoomRequest
	LoadRoomAndDeliver chan LoadRoomAndDeliverRequest
	SendErrorToUser   chan SendErrorRequest
}

type RoomMembersChangeRequest struct {
	RoomID string
	UserFP string
}

type CreateRoomRequest struct {
	Host         *session.User
	GroupName    string
	ResponseChan chan string
}

type Room struct {
	Name     string
	RoomID   string
	Host     *User            // live WS connection; nil when host is offline
	HostFP   string           // always set
	HostName string           // always set when known
	Users    map[string]*User // only active WS connections
	Epoch    int64
}

func New(s *session.Store, logg *logger.Logger) *Hub {
	return &Hub{
		logg:               logg,
		s:                  s,
		Rooms:              make(map[string]*Room),
		Users:              make(map[string]*User),
		Events:             make(chan HubEvent, 256),
		Broadcast:          make(chan BroadcastRequest),
		InboundMessages:    make(chan WSMessageEnvelope),
		RegisterRoom:       make(chan CreateRoomRequest),
		UnregisterRoom:     make(chan string),
		RegisterUser:       make(chan *User),
		UnregisterUser:     make(chan *User),
		JoinRoom:           make(chan RoomMembersChangeRequest),
		LeaveRoom:          make(chan RoomMembersChangeRequest),
		AddToRoom:          make(chan AddToRoomRequest),
		RestoreRoom:        make(chan RestoreRoomRequest),
		RemoveFromRoom:     make(chan RemoveFromRoomRequest),
		LoadRoomAndDeliver: make(chan LoadRoomAndDeliverRequest),
		SendErrorToUser:    make(chan SendErrorRequest),
	}
}

// emit sends a HubEvent non-blocking. Drops the event if the channel is full
// so the hub's main loop is never stalled by a slow consumer.
func (h *Hub) emit(event HubEvent) {
	select {
	case h.Events <- event:
	default:
		h.logg.Warn("hub event channel full, dropping event", zap.String("type", string(event.Type)))
	}
}

func (h *Hub) Run() {
	for {
		select {
		case req := <-h.RegisterRoom:
			h.handleRegisterRoom(req)

		case newUser := <-h.RegisterUser:
			h.handleRegisterUser(newUser)

		case user := <-h.UnregisterUser:
			h.handleUnregisterUser(user)

		case msg := <-h.InboundMessages:
			h.handleInboundMessage(msg)

		case req := <-h.JoinRoom:
			h.handleJoinRoom(req)

		case req := <-h.AddToRoom:
			h.handleAddToRoom(req)

		case req := <-h.RestoreRoom:
			h.handleRestoreRoom(req)

		case req := <-h.RemoveFromRoom:
			h.handleRemoveFromRoom(req)

		case req := <-h.LoadRoomAndDeliver:
			h.handleLoadRoomAndDeliver(req)

		case req := <-h.SendErrorToUser:
			h.handleSendErrorToUser(req)
		}
	}
}
