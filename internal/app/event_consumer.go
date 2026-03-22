package app

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/logger"
	"2L1nk/internal/service"

	"go.uber.org/zap"
)

func startEventConsumer(
	events <-chan hub.HubEvent,
	roomSvc *service.RoomService,
	msgSvc *service.MessageService,
	logg *logger.Logger,
) {
	go func() {
		for event := range events {
			switch event.Type {
			case hub.HubEventRoomCreated:
				payload := event.Payload.(hub.RoomCreatedPayload)
				if err := roomSvc.CreateRoom(payload); err != nil {
					logg.Error("event consumer: failed to persist room", zap.Error(err))
				}
			case hub.HubEventMemberJoined:
				payload := event.Payload.(hub.MemberJoinedPayload)
				if err := roomSvc.AddMember(payload); err != nil {
					logg.Error("event consumer: failed to persist member", zap.Error(err))
				}
			case hub.HubEventMessageCreated:
				payload := event.Payload.(hub.MessageCreatedPayload)
				if err := msgSvc.ProcessMessage(payload); err != nil {
					logg.Error("event consumer: failed to persist message", zap.Error(err))
				}
			}
		}
	}()
}
