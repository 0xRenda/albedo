package checker

import "strings"

type CheckResult struct {
	Status string
	Reason string
}

func ClassifyResponse(gatewayResponse string) CheckResult {
	raw := strings.ToLower(gatewayResponse)
	switch {
	case containsAny(raw, "incorrect_cvc", "invalid_cvc"):
		return CheckResult{Status: "LIVE", Reason: "INCORRECT_CVC"}
	case containsAny(raw, "insufficient_funds", "not_enough_money"):
		return CheckResult{Status: "LIVE", Reason: "INSUFFICIENT_FUNDS"}
	case containsAny(raw, "3d_secure", "otp", "authentication_required"):
		return CheckResult{Status: "OTP", Reason: "OTP_REQUIRED"}
	case containsAny(raw, "charged", "captured", "order_placed", "success"):
		return CheckResult{Status: "CHARGED", Reason: "CHARGED"}
	case containsAny(raw, "declined", "rejected"):
		return CheckResult{Status: "DEAD", Reason: "CARD_DECLINED"}
	case containsAny(raw, "expired"):
		return CheckResult{Status: "DEAD", Reason: "EXPIRED_CARD"}
	case containsAny(raw, "invalid", "not_recognized"):
		return CheckResult{Status: "DEAD", Reason: "INVALID_CARD"}
	default:
		return CheckResult{Status: "DEAD", Reason: "UNKNOWN_ERROR"}
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
