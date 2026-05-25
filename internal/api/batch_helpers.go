package api

import "net/http"

func requireBatchPayload(w http.ResponseWriter, r *http.Request, confirmPhrase string, maxUsers int, tooManyMessage string) (map[string]any, []int64, bool) {
	payload := decodeMap(r)
	if confirmPhrase != "" && stringValue(payload, "confirm") != confirmPhrase {
		fail(w, http.StatusBadRequest, "missing confirm "+confirmPhrase)
		return nil, nil, false
	}
	uids := int64Slice(payload["uids"])
	if len(uids) == 0 {
		fail(w, http.StatusBadRequest, "uids required")
		return nil, nil, false
	}
	if maxUsers > 0 && len(uids) > maxUsers {
		fail(w, http.StatusBadRequest, tooManyMessage)
		return nil, nil, false
	}
	return payload, uids, true
}

func uniqueInt64s(values []int64) []int64 {
	seen := map[int64]bool{}
	unique := make([]int64, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}
