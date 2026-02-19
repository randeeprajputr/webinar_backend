package zego

import (
	"encoding/json"
	"fmt"

	"github.com/ZEGOCLOUD/zego_server_assistant/token/go/src/token04"
)

// RtcRoomPayload is the payload for room-based token (live streaming). See ZEGOCLOUD token04 docs.
type RtcRoomPayload struct {
	RoomID       string         `json:"RoomId"`
	Privilege    map[int]int    `json:"Privilege"`
	StreamIDList []string       `json:"StreamIdList,omitempty"`
}

// GenerateRoomToken generates a ZEGOCLOUD token04 token for the given user and webinar (room).
// role: "speaker" or "admin" => can publish; "audience" => can only pull stream.
// appID and serverSecret from ZEGOCLOUD console; serverSecret must be 32 characters.
func GenerateRoomToken(appID uint32, serverSecret, roomID, userID, role string, effectiveTimeSec int64) (string, error) {
	if appID == 0 || serverSecret == "" {
		return "", fmt.Errorf("zego: app_id and server_secret required")
	}
	if len(serverSecret) != 32 {
		return "", fmt.Errorf("zego: server_secret must be 32 characters")
	}
	privilege := map[int]int{
		token04.PrivilegeKeyLogin: token04.PrivilegeEnable,
		token04.PrivilegeKeyPublish: token04.PrivilegeDisable,
	}
	if role == "speaker" || role == "admin" {
		privilege[token04.PrivilegeKeyPublish] = token04.PrivilegeEnable
	}
	payload := RtcRoomPayload{
		RoomID:    roomID,
		Privilege: privilege,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("zego: marshal payload: %w", err)
	}
	return token04.GenerateToken04(appID, userID, serverSecret, effectiveTimeSec, string(payloadJSON))
}
