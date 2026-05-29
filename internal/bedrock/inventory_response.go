package bedrock

import "github.com/sandertv/gophertunnel/minecraft/protocol"

func rejectItemStackRequest(requestID int32) protocol.ItemStackResponse {
	return protocol.ItemStackResponse{
		Status:    protocol.ItemStackResponseStatusError,
		RequestID: requestID,
	}
}
