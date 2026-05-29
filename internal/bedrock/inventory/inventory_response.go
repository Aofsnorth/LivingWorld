package inventory

import "github.com/sandertv/gophertunnel/minecraft/protocol"

func RejectItemStackRequest(requestID int32) protocol.ItemStackResponse {
	return protocol.ItemStackResponse{
		Status:    protocol.ItemStackResponseStatusError,
		RequestID: requestID,
	}
}
