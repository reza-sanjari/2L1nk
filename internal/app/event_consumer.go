package app

import (
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/logger"
	"2L1nk/internal/service"
	"time"

	"go.uber.org/zap"
)

func startEventConsumer(
	mainHub *hub.Hub,
	roomSvc *service.RoomService,
	msgSvc *service.MessageService,
	logg *logger.Logger,
) {
	go func() {
		for event := range mainHub.Events {
			switch event.Type {
			case hub.HubEventRoomCreated:
				payload := event.Payload.(hub.RoomCreatedPayload)
				if err := roomSvc.CreateRoom(payload); err != nil {
					logg.Error("event consumer: failed to persist room", zap.Error(err))
				}

			case hub.HubEventMessageCreated:
				payload := event.Payload.(hub.MessageCreatedPayload)
				if err := msgSvc.ProcessMessage(payload); err != nil {
					logg.Error("event consumer: failed to persist message", zap.Error(err))
				}

			case hub.HubEventRoomOffline:
				// Room is offline — load it from DB and re-deliver the pending message.
				payload := event.Payload.(hub.RoomOfflinePayload)
				room, err := roomSvc.GetRoomByID(payload.RoomID)
				if err != nil {
					logg.Error("event consumer: failed to get room by ID", zap.Error(err))
					mainHub.SendErrorToUser <- hub.SendErrorRequest{UserFP: payload.SenderFP, Message: "internal error"}
					continue
				}
				if room == nil {
					mainHub.SendErrorToUser <- hub.SendErrorRequest{UserFP: payload.SenderFP, Message: "room not found"}
					continue
				}
				memberKeys, err := roomSvc.GetMembersWithPublicKeys(payload.RoomID)
				if err != nil {
					logg.Error("event consumer: failed to get room members", zap.Error(err))
					mainHub.SendErrorToUser <- hub.SendErrorRequest{UserFP: payload.SenderFP, Message: "internal error"}
					continue
				}
				// Verify sender is a member of the room
				isMember := false
				hubMembers := make([]hub.MemberKeyInfo, len(memberKeys))
				for i, m := range memberKeys {
					hubMembers[i] = hub.MemberKeyInfo{FP: m.Fingerprint, X25519PublicKey: m.X25519PublicKey}
					if m.Fingerprint == payload.SenderFP {
						isMember = true
					}
				}
				if !isMember {
					mainHub.SendErrorToUser <- hub.SendErrorRequest{UserFP: payload.SenderFP, Message: "not a member of this room"}
					continue
				}
				mainHub.LoadRoomAndDeliver <- hub.LoadRoomAndDeliverRequest{
					RoomID:   room.ID,
					RoomName: room.Name,
					HostFP:   room.HostFP,
					Epoch:    room.CurrentEpoch,
					Members:  hubMembers,
					Message:  payload.Message,
				}

			case hub.HubEventKeyRotationTriggered:
				// Only emitted for hub-internal rotations (key creator disconnect reassignment).
				// REST-triggered rotations update DB directly and do not emit this event.
				payload := event.Payload.(hub.KeyRotationTriggeredPayload)
				if err := roomSvc.UpdateEpochAndKeyCreator(payload.RoomID, payload.Epoch, payload.KeyCreatorFP); err != nil {
					logg.Error("event consumer: failed to update epoch and key creator", zap.Error(err))
				}

			case hub.HubEventEpochKeysSubmitted:
				payload := event.Payload.(hub.EpochKeysSubmittedPayload)
				now := time.Now().Unix()
				slots := make([]infradb.KeySlotRecord, 0, len(payload.Keys))
				for _, k := range payload.Keys {
					slots = append(slots, infradb.KeySlotRecord{
						RoomID:       payload.RoomID,
						Epoch:        payload.Epoch,
						RecipientFP:  k.RecipientFP,
						EncryptedKey: k.EncryptedKey,
						CreatedAt:    now,
					})
				}
				if err := roomSvc.StoreKeySlots(slots); err != nil {
					logg.Error("event consumer: failed to store epoch key slots", zap.Error(err))
				}
			}
		}
	}()
}
