package api

import "slices"

func canTransitionOrder(from, to string) bool {
	allowed := map[string][]string{
		"WAITING_FOR_PAYMENT": {"FINDING", "CANCELLED"},
		"SCHEDULING":          {"FINDING", "ASSIGNED", "CANCELLED"},
		"FINDING":             {"ASSIGNED", "CANCELLED"},
		"ASSIGNED":            {"IN PROCESS", "CANCELLED"},
		"IN PROCESS":          {"COMPLETED"},
	}
	return slices.Contains(allowed[from], to)
}

func canTransitionPoint(from, to string) bool {
	if from == to {
		return true
	}
	allowed := map[string][]string{
		"CREATED":    {"ARRIVING", "FAILED"},
		"ARRIVING":   {"NEARED", "NEARED_50M", "FAILED"},
		"NEARED":     {"NEARED_50M", "ARRIVED", "FAILED"},
		"NEARED_50M": {"ARRIVED", "FAILED"},
		"ARRIVED":    {"COMPLETED", "FAILED"},
	}
	return slices.Contains(allowed[from], to)
}
