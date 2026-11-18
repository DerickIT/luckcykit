package model

import "strings"

func PendingMediaUsageStates() []string   { return []string{"pending", "reserved"} }
func PendingMediaBillingStates() []string { return []string{"", "reserved", "precharged"} }

func IsPendingMediaUsage(state string) bool {
	return mediaStateIn(state, PendingMediaUsageStates())
}

func (task Task) MediaSettlementPending() bool {
	return (task.LuckyUsageState != nil && IsPendingMediaUsage(*task.LuckyUsageState)) ||
		(task.LuckyBillingState != nil && mediaStateIn(*task.LuckyBillingState, PendingMediaBillingStates()))
}

func mediaStateIn(state string, values []string) bool {
	state = strings.ToLower(strings.TrimSpace(state))
	for _, value := range values {
		if value == state {
			return true
		}
	}
	return false
}
